package audio

import (
	"math"
	"testing"
)

// synth builds frames of speech-like tone or near-silence.
func synth(t *testing.T, ms int, amp float64) [][]float32 {
	t.Helper()
	frames := make([][]float32, 0, ms/FrameMs)
	phase := 0.0
	for i := 0; i < ms/FrameMs; i++ {
		f := make([]float32, FrameSize)
		for j := range f {
			if amp == 0 {
				f[j] = float32(0.0005 * math.Sin(phase*7))
			} else {
				f[j] = float32(amp * math.Sin(phase))
			}
			phase += 2 * math.Pi * 180 / SampleRate
		}
		frames = append(frames, f)
	}
	return frames
}

func push(t *testing.T, c *Chunker, frames [][]float32) []Request {
	t.Helper()
	var got []Request
	for _, f := range frames {
		reqs, _ := c.Push(f)
		got = append(got, reqs...)
	}
	return got
}

func TestChunkerCommitsOnSilence(t *testing.T) {
	cfg := DefaultChunkerConfig()
	cfg.PartialIntervalMs = 0 // finals only, to keep the assertions readable
	c := NewChunker(cfg)

	push(t, c, synth(t, 600, 0)) // let the noise floor settle
	var reqs []Request
	reqs = append(reqs, push(t, c, synth(t, 1500, 0.3))...)
	if len(reqs) != 0 {
		t.Fatalf("committed mid-utterance: %d requests", len(reqs))
	}
	reqs = append(reqs, push(t, c, synth(t, 1200, 0))...)

	if len(reqs) != 1 {
		t.Fatalf("expected exactly one final, got %d", len(reqs))
	}
	if reqs[0].Kind != KindFinal {
		t.Fatalf("expected a final, got kind %v", reqs[0].Kind)
	}
	gotMs := DurationMs(len(reqs[0].PCM))
	// 1500 ms of speech, plus pre-roll and the silence hangover.
	if gotMs < 1500 || gotMs > 1500+int64(cfg.PreRollMs)+int64(cfg.SilenceMs)+100 {
		t.Fatalf("unexpected utterance length: %d ms", gotMs)
	}
}

func TestChunkerEmitsPartials(t *testing.T) {
	cfg := DefaultChunkerConfig()
	cfg.PartialIntervalMs = 500
	c := NewChunker(cfg)

	push(t, c, synth(t, 600, 0))
	reqs := push(t, c, synth(t, 2600, 0.3))

	partials := 0
	for _, r := range reqs {
		if r.Kind == KindPartial {
			partials++
		}
	}
	if partials < 3 {
		t.Fatalf("expected several partials during 2.6 s of speech, got %d", partials)
	}
}

func TestChunkerForcesCommitOnLongSpeech(t *testing.T) {
	cfg := DefaultChunkerConfig()
	cfg.PartialIntervalMs = 0
	cfg.MaxUtteranceMs = 2000
	c := NewChunker(cfg)

	push(t, c, synth(t, 600, 0))
	reqs := push(t, c, synth(t, 7000, 0.3)) // a speaker who never pauses

	finals := 0
	for _, r := range reqs {
		if r.Kind == KindFinal {
			finals++
			if ms := DurationMs(len(r.PCM)); ms > int64(cfg.MaxUtteranceMs)+int64(FrameMs) {
				t.Fatalf("forced commit was too long: %d ms", ms)
			}
		}
	}
	if finals < 3 {
		t.Fatalf("expected the chunker to force commits, got %d finals", finals)
	}
}

func TestChunkerIgnoresBlips(t *testing.T) {
	cfg := DefaultChunkerConfig()
	cfg.PartialIntervalMs = 0
	cfg.PreRollMs = 0
	cfg.MinUtteranceMs = 600
	c := NewChunker(cfg)

	push(t, c, synth(t, 600, 0))
	reqs := push(t, c, synth(t, 100, 0.4)) // a cough
	reqs = append(reqs, push(t, c, synth(t, 1200, 0))...)

	for _, r := range reqs {
		if r.Kind == KindFinal {
			t.Fatalf("a 100 ms blip should not be transcribed (%d ms committed)", DurationMs(len(r.PCM)))
		}
	}
}

func TestFlushCommitsInProgressUtterance(t *testing.T) {
	c := NewChunker(DefaultChunkerConfig())
	push(t, c, synth(t, 600, 0))
	push(t, c, synth(t, 1500, 0.3))

	req, ok := c.Flush()
	if !ok {
		t.Fatal("Flush should commit the in-progress utterance")
	}
	if req.Kind != KindFinal || len(req.PCM) == 0 {
		t.Fatalf("unexpected flush result: %+v", req.Kind)
	}
}
