package audio

import (
	"fmt"
	"os"
	"testing"
)

// TestSegmentationAgainstRecording replays a real recording through the
// chunker and reports what it did with it. It is a measuring instrument rather
// than a pass/fail test, so it only runs when pointed at a file:
//
//	EARS_TUNE_WAV=/tmp/room.wav go test ./internal/audio -run Segmentation -v
//
// Capture one with `ears-listener --record`. Recordings stay out of the
// repository: they are somebody's voice, and often a roomful of other
// people's, so the fixture here is deliberately something you supply rather
// than something we ship.
func TestSegmentationAgainstRecording(t *testing.T) {
	path := os.Getenv("EARS_TUNE_WAV")
	if path == "" {
		t.Skip("set EARS_TUNE_WAV to a recording to measure segmentation against it")
	}
	pcm := loadRecording(t, path)

	t.Logf("%s: %s of audio", path, fmtMs(DurationMs(len(pcm))))
	t.Log("")
	t.Log("   min_rms    rise  hangover  utterances  committed  longest  coverage  splits/min")
	// The interesting range is narrow. Above roughly 0.001 the absolute floor
	// sits on top of ordinary speech and swallows most of it; below roughly
	// 0.0003 the detector stops finding any silence at all, commits the room
	// tone between sentences, and the model hallucinates over it. A rise rate
	// equal to AdaptRate is the old symmetric behaviour, kept in the sweep so
	// the ratchet it caused stays visible.
	for _, minRMS := range []float64{0.004, 0.002, 0.001, 0.0008, 0.0005, 0.0003} {
		for _, hangover := range []int{0, 500} {
			for _, rise := range []float64{0.02, 0.002} {
				cfg := DefaultChunkerConfig()
				cfg.PartialIntervalMs = 0
				cfg.VAD.MinRMS = minRMS
				cfg.VAD.HangoverMs = hangover
				cfg.VAD.AdaptRiseRate = rise
				r := replay(cfg, pcm)
				t.Log(r.line(fmt.Sprintf("%9.4f  %6.3f  %8d", minRMS, rise, hangover)))
			}
		}
	}
}

type replayResult struct {
	total      int64 // ms of audio replayed
	committed  int64 // ms of audio inside committed utterances
	longest    int64
	utterances int
}

func (r replayResult) line(prefix string) string {
	coverage := 0.0
	splits := 0.0
	if r.total > 0 {
		coverage = float64(r.committed) / float64(r.total) * 100
		splits = float64(r.utterances) / (float64(r.total) / 60000)
	}
	return fmt.Sprintf("%s  %10d  %9s  %7s  %7.1f%%  %10.1f",
		prefix, r.utterances, fmtMs(r.committed), fmtMs(r.longest), coverage, splits)
}

// replay feeds a recording through a chunker one frame at a time, exactly as
// the engine does, and totals up what came out.
func replay(cfg ChunkerConfig, pcm []float32) replayResult {
	c := NewChunker(cfg)
	var res replayResult
	for i := 0; i+FrameSize <= len(pcm); i += FrameSize {
		reqs, _ := c.Push(pcm[i : i+FrameSize])
		for _, r := range reqs {
			if r.Kind == KindFinal {
				res.record(r)
			}
		}
	}
	if r, ok := c.Flush(); ok {
		res.record(r)
	}
	res.total = DurationMs(len(pcm))
	return res
}

func (r *replayResult) record(req Request) {
	d := DurationMs(len(req.PCM))
	r.utterances++
	r.committed += d
	if d > r.longest {
		r.longest = d
	}
}

func loadRecording(t *testing.T, path string) []float32 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read recording: %v", err)
	}
	pcm, rate, err := DecodeWAV(data)
	if err != nil {
		t.Fatalf("decode recording: %v", err)
	}
	return Resample(pcm, rate, SampleRate)
}

func fmtMs(ms int64) string {
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}
