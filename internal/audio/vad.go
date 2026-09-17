package audio

import "math"

// VADConfig tunes voice activity detection.
type VADConfig struct {
	// StartFactor is how far above the estimated noise floor a frame must be
	// before it counts as speech. Conference rooms have air conditioning,
	// laptop fans and a permanent low rumble, so a simple fixed threshold
	// does not survive contact with reality.
	StartFactor float64
	// StopFactor is the lower threshold used once speech has started, giving
	// hysteresis so quiet syllables do not chop a sentence in half.
	StopFactor float64
	// MinRMS is an absolute floor; below this nothing is ever speech. It has
	// to sit well under the quietest frame of real speech, and a line input
	// running at a conservative level puts that lower than you would think:
	// measured on a TASCAM US-16x08 with a lectern microphone, ordinary speech
	// arrives at a frame RMS of 0.004 to 0.008, so the 0.004 this used to be
	// sat directly on top of the speaker and left the adaptive part of the
	// detector with nothing to do.
	MinRMS float64
	// AdaptRate is how quickly the noise floor follows the room downwards, per
	// frame: the audience settled, the air conditioning stopped.
	AdaptRate float64
	// AdaptRiseRate is the same for a room getting noisier, and is
	// deliberately much slower. Adaptation upwards is the dangerous direction,
	// because every rise makes speech harder to detect, which produces more
	// frames that look like noise, which raises the floor again.
	AdaptRiseRate float64
	// HangoverMs suspends adaptation entirely for this long after the last
	// frame of speech. The gaps between words are not silence: they are part
	// of a sentence, they sit well above the real noise floor, and letting
	// them into the estimate is what made a long sentence gradually deafen the
	// detector to the person speaking it.
	HangoverMs int
}

// DefaultVADConfig is tuned for a lapel or lectern microphone in a room with
// an audience in it.
func DefaultVADConfig() VADConfig {
	return VADConfig{
		StartFactor:   3.0,
		StopFactor:    1.8,
		MinRMS:        0.0008,
		AdaptRate:     0.02,
		AdaptRiseRate: 0.002,
		HangoverMs:    500,
	}
}

// VAD is a cheap energy-based voice activity detector with an adaptive noise
// floor. It is not trying to be Silero; it only needs to decide where an
// utterance ends so the model gets a sensible chunk.
type VAD struct {
	cfg        VADConfig
	noiseFloor float64
	speaking   bool
	primed     bool
	// quietMs is how long it has been since the last frame of speech, used to
	// hold adaptation off until the speaker has really stopped.
	quietMs int
}

// NewVAD returns a detector. A zero config gets the defaults.
func NewVAD(cfg VADConfig) *VAD {
	def := DefaultVADConfig()
	if cfg.StartFactor <= 0 {
		cfg.StartFactor = def.StartFactor
	}
	if cfg.StopFactor <= 0 {
		cfg.StopFactor = def.StopFactor
	}
	if cfg.MinRMS <= 0 {
		cfg.MinRMS = def.MinRMS
	}
	if cfg.AdaptRate <= 0 {
		cfg.AdaptRate = def.AdaptRate
	}
	if cfg.AdaptRiseRate <= 0 {
		cfg.AdaptRiseRate = def.AdaptRiseRate
	}
	if cfg.HangoverMs < 0 {
		cfg.HangoverMs = def.HangoverMs
	}
	return &VAD{cfg: cfg}
}

// NoiseFloor exposes the current estimate, which is handy in logs when
// somebody asks why the transcript has gone quiet.
func (v *VAD) NoiseFloor() float64 { return v.noiseFloor }

// Thresholds returns the levels a frame must currently reach to start speech
// and to sustain it. Both move with the noise floor, so logging them alongside
// the frame level is the only way to tell a quiet speaker from a detector that
// has talked itself into ignoring the room.
func (v *VAD) Thresholds() (start, stop float64) {
	return math.Max(v.noiseFloor*v.cfg.StartFactor, v.cfg.MinRMS),
		math.Max(v.noiseFloor*v.cfg.StopFactor, v.cfg.MinRMS*0.6)
}

// Push classifies one frame and returns whether it contains speech along with
// the frame's RMS level (0..1).
func (v *VAD) Push(frame []float32) (speech bool, level float64) {
	level = rms(frame)
	if !v.primed {
		v.noiseFloor = level
		v.primed = true
	}

	start, stop := v.Thresholds()

	if v.speaking {
		v.speaking = level >= stop
	} else {
		v.speaking = level >= start
	}

	// Only adapt the noise floor once the room has genuinely been quiet for a
	// while, and then far more readily downwards than upwards.
	//
	// Adapting on any frame that merely failed the speech test is not enough,
	// and was the bug this hangover exists to fix: the pauses between words
	// fail that test whilst sitting well above the real noise floor, so a long
	// sentence dragged the estimate up toward the speaker's own level, which
	// raised the threshold, which turned more of the sentence into "noise". A
	// speaker who did not pause could talk themselves into silence inside
	// about five seconds.
	if v.speaking {
		v.quietMs = 0
	} else {
		v.quietMs += int(DurationMs(len(frame)))
	}
	if !v.speaking && v.quietMs >= v.cfg.HangoverMs {
		rate := v.cfg.AdaptRate
		if level > v.noiseFloor {
			rate = v.cfg.AdaptRiseRate
		}
		v.noiseFloor += (level - v.noiseFloor) * rate
	}
	return v.speaking, level
}

func rms(frame []float32) float64 {
	if len(frame) == 0 {
		return 0
	}
	var sum float64
	for _, s := range frame {
		sum += float64(s) * float64(s)
	}
	return math.Sqrt(sum / float64(len(frame)))
}

// Peak returns the largest absolute sample in a frame.
func Peak(frame []float32) float64 {
	var p float64
	for _, s := range frame {
		p = math.Max(p, math.Abs(float64(s)))
	}
	return p
}
