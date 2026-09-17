package audio

import (
	"math"
	"testing"
)

// tone builds one frame of a sine at the given RMS amplitude.
func tone(rmsLevel float64, phase *float64) []float32 {
	f := make([]float32, FrameSize)
	amp := rmsLevel * math.Sqrt2 // a sine's peak is √2 times its RMS
	for i := range f {
		f[i] = float32(amp * math.Sin(*phase))
		*phase += 2 * math.Pi * 180 / SampleRate
	}
	return f
}

// syllables is one cycle of a speech-like amplitude envelope, about a quarter
// of a second long. The levels are the measured quantiles of real frame RMS
// taken over twenty seconds of continuous talking into a lectern microphone,
// which matters because speech is far quieter far more of the time than an
// invented test signal tends to be: a quarter of the frames during a sentence
// sit essentially at the noise floor, and those are the ones that mislead an
// energy detector.
var syllables = []float64{
	0.0002, 0.0002, 0.0005, 0.0012, 0.0027, 0.0044,
	0.0052, 0.0044, 0.0027, 0.0012, 0.0005, 0.0002,
}

// settle runs enough silence through a detector for it to learn the room.
func settle(v *VAD, room float64, phase *float64) {
	for i := 0; i < 100; i++ {
		v.Push(tone(room, phase))
	}
}

func TestSpeakingDoesNotRaiseTheNoiseFloor(t *testing.T) {
	// The bug this guards against: the detector adapted its noise floor on
	// every frame it did not classify as speech, and the quiet frames within a
	// sentence are exactly those. Talking therefore dragged the estimate up
	// toward the speaker's own level, which raised the threshold, which turned
	// more of the sentence into "noise" and raised it further. In a real room
	// a speaker who did not pause could talk themselves into silence, losing
	// the rest of the sentence and sometimes the next one too.
	//
	// The invariant is simple enough to state: speaking into the microphone
	// must not change what the detector believes the empty room sounds like.
	const room = 0.0002

	v := NewVAD(DefaultVADConfig())
	phase := 0.0
	settle(v, room, &phase)
	settled := v.NoiseFloor()

	voiced := 0
	const frames = 30 * 1000 / FrameMs // thirty seconds without a proper pause
	for i := 0; i < frames; i++ {
		if speaking, _ := v.Push(tone(syllables[i%len(syllables)], &phase)); speaking {
			voiced++
		}
	}

	if got := v.NoiseFloor(); got > settled*1.25 {
		t.Errorf("thirty seconds of speech raised the noise floor from %.5f to %.5f", settled, got)
	}
	if start, _ := v.Thresholds(); start > 0.0009 {
		t.Errorf("start threshold climbed to %.5f whilst somebody was talking", start)
	}
	// The envelope spends a third of its time below anything that could
	// reasonably count as speech, so this is a floor rather than a target.
	if min := frames * 6 / 10; voiced < min {
		t.Errorf("only %d of %d frames were heard as speech, wanted at least %d", voiced, frames, min)
	}
}

func TestVADFollowsARoomThatGetsQuieter(t *testing.T) {
	// Adaptation upwards is held back deliberately, so check the other
	// direction still works: a room that settles must not leave the detector
	// holding a stale, too-high threshold.
	v := NewVAD(DefaultVADConfig())
	phase := 0.0
	for i := 0; i < 200; i++ {
		v.Push(tone(0.002, &phase))
	}
	noisy := v.NoiseFloor()

	for i := 0; i < 500; i++ {
		v.Push(tone(0.0001, &phase))
	}
	if quiet := v.NoiseFloor(); quiet >= noisy/2 {
		t.Errorf("noise floor stayed at %.5f after the room quietened, from %.5f", quiet, noisy)
	}
}
