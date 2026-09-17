package audio

import (
	"math"
	"math/rand"
	"os"
	"testing"
)

// fixturePath is the sample the smoke test and `make demo` replay in place of
// a microphone.
const fixturePath = "../../testdata/sample.wav"

// TestGenerateFixture rewrites testdata/sample.wav. It only runs on request:
//
//	EARS_WRITE_FIXTURE=1 go test ./internal/audio -run GenerateFixture
//
// The fixture is synthetic on purpose. The mock transcriber invents the words
// anyway, so nothing here needs to be real speech; what it does need is a real
// speech *shape*, which is to say loud syllables, quiet gaps between them, and
// proper silence between phrases. The sample this replaced had none of the
// last of those. Its quietest frame sat at 0.0056, well above any plausible
// threshold, so the segmenter never saw a pause in it from one end to the
// other, and a bug that made the detector go deaf partway through a sentence
// sailed through CI for want of a fixture that could show it.
func TestGenerateFixture(t *testing.T) {
	if os.Getenv("EARS_WRITE_FIXTURE") == "" {
		t.Skip("set EARS_WRITE_FIXTURE=1 to rewrite " + fixturePath)
	}
	if err := os.WriteFile(fixturePath, EncodeWAV(buildFixture(), SampleRate), 0o644); err != nil {
		t.Fatal(err)
	}
	pcm, _, err := DecodeWAV(mustRead(t, fixturePath))
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultChunkerConfig()
	cfg.PartialIntervalMs = 0
	r := replay(cfg, pcm)
	t.Logf("wrote %s: %s, %s", fixturePath, fmtMs(DurationMs(len(pcm))), r.line(""))
	if r.utterances < 4 {
		t.Errorf("fixture only segments into %d utterances; the smoke test needs lines to keep arriving", r.utterances)
	}
}

// buildFixture assembles alternating phrases and pauses.
func buildFixture() []float32 {
	rng := rand.New(rand.NewSource(20260917)) // fixed, so the file is reproducible
	var out []float32
	phase := 0.0

	// Phrase lengths chosen so no two land on the same boundary when the file
	// is looped, which is how the smoke test plays it.
	for _, phraseMs := range []int{2600, 3400, 2200, 4100, 2800} {
		out = append(out, phrase(phraseMs, &phase, rng)...)
		out = append(out, quiet(1200, rng)...)
	}
	return out
}

// phrase is a run of speech-shaped syllables at a healthy level.
func phrase(ms int, phase *float64, rng *rand.Rand) []float32 {
	// syllables holds the measured amplitude envelope of real speech; ×10
	// puts the loud frames around 0.05, which is a comfortable signal from a
	// microphone that somebody has set the gain on.
	out := make([]float32, 0, ms*SampleRate/1000)
	for i := 0; i < ms/FrameMs; i++ {
		level := syllables[i%len(syllables)] * 10
		f := tone(level, phase)
		for j := range f {
			f[j] += float32(rng.NormFloat64() * 0.0002) // room tone
		}
		out = append(out, f...)
	}
	return out
}

// quiet is the gap between phrases: the room, and nothing else.
func quiet(ms int, rng *rand.Rand) []float32 {
	out := make([]float32, ms*SampleRate/1000)
	for i := range out {
		out[i] = float32(rng.NormFloat64() * 0.0002)
	}
	return out
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestFixtureHasPauses guards the property the smoke test depends on, so that
// a future regenerated or replaced sample cannot quietly go back to being one
// unbroken wall of sound.
func TestFixtureHasPauses(t *testing.T) {
	pcm, rate, err := DecodeWAV(mustRead(t, fixturePath))
	if err != nil {
		t.Fatal(err)
	}
	pcm = Resample(pcm, rate, SampleRate)

	var quietest float64 = math.MaxFloat64
	for i := 0; i+FrameSize <= len(pcm); i += FrameSize {
		if l := rms(pcm[i : i+FrameSize]); l < quietest {
			quietest = l
		}
	}
	if quietest > DefaultVADConfig().MinRMS {
		t.Errorf("the fixture never falls below %.5f, so it contains no silence for the segmenter to find", quietest)
	}

	cfg := DefaultChunkerConfig()
	cfg.PartialIntervalMs = 0
	if r := replay(cfg, pcm); r.utterances < 4 {
		t.Errorf("fixture segments into only %d utterances", r.utterances)
	}
}
