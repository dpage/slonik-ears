package listener

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/dpage/slonik-ears/internal/asr"
	"github.com/dpage/slonik-ears/internal/audio"
	"github.com/dpage/slonik-ears/internal/protocol"
)

// fakeSource feeds pre-baked frames and then closes, standing in for a
// microphone that does not exist on a build server.
type fakeSource struct {
	ch chan []float32
}

func newFakeSource(frames [][]float32) *fakeSource {
	s := &fakeSource{ch: make(chan []float32, len(frames))}
	for _, f := range frames {
		s.ch <- f
	}
	close(s.ch)
	return s
}

func (s *fakeSource) Frames() <-chan []float32 { return s.ch }
func (s *fakeSource) Err() error               { return nil }
func (s *fakeSource) Name() string             { return "fake" }
func (s *fakeSource) Close() error             { return nil }

type capturePublisher struct {
	mu       sync.Mutex
	finals   []protocol.Segment
	partials []protocol.Partial
}

func (p *capturePublisher) PublishFinal(seg protocol.Segment) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.finals = append(p.finals, seg)
}

func (p *capturePublisher) PublishPartial(partial protocol.Partial) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.partials = append(p.partials, partial)
}

func (p *capturePublisher) PublishStatus(protocol.Status) {}
func (p *capturePublisher) Connected() bool               { return true }
func (p *capturePublisher) Queued() int                   { return 0 }

func (p *capturePublisher) finalTexts() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.finals))
	for _, f := range p.finals {
		out = append(out, f.Text)
	}
	return out
}

func frames(ms int, amp float64) [][]float32 {
	out := make([][]float32, 0, ms/audio.FrameMs)
	phase := 0.0
	for i := 0; i < ms/audio.FrameMs; i++ {
		f := make([]float32, audio.FrameSize)
		for j := range f {
			f[j] = float32(amp * math.Sin(phase))
			phase += 2 * math.Pi * 180 / audio.SampleRate
		}
		out = append(out, f)
	}
	return out
}

func TestEngineTranscribesEveryUtterance(t *testing.T) {
	var all [][]float32
	all = append(all, frames(600, 0)...)
	all = append(all, frames(1500, 0.3)...)
	all = append(all, frames(1000, 0)...)
	all = append(all, frames(1500, 0.3)...)
	all = append(all, frames(1000, 0)...)

	pub := &capturePublisher{}
	cfg := DefaultEngineConfig()
	cfg.Chunker.PartialIntervalMs = 0
	cfg.Logger = testLogger()

	engine, err := NewEngine(cfg, newFakeSource(all), &asr.Mock{}, pub)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := engine.Run(ctx); err != nil {
		t.Fatal(err)
	}

	texts := pub.finalTexts()
	if len(texts) != 2 {
		t.Fatalf("expected two committed utterances, got %d: %v", len(texts), texts)
	}
	for _, text := range texts {
		if text == "" {
			t.Fatal("published an empty segment")
		}
	}
}

// silentTranscriber stands in for a model whose output the cleaning rules
// discard entirely, which is what "[BLANK_AUDIO]" or a hallucinated "Thank
// you." over room noise comes back as.
type silentTranscriber struct{}

func (silentTranscriber) Transcribe(context.Context, []float32, int, asr.Options) (asr.Result, error) {
	return asr.Result{Raw: "[BLANK_AUDIO]"}, nil
}
func (silentTranscriber) Name() string { return "silent" }
func (silentTranscriber) Close() error { return nil }

func TestEngineClearsThePreviewWhenAFinalHasNoText(t *testing.T) {
	// An utterance the model returns nothing usable for must still clear the
	// preview: otherwise the last partial stays on every screen in the room,
	// greyed out and never confirmed, until somebody speaks again.
	var all [][]float32
	all = append(all, frames(600, 0)...)
	all = append(all, frames(1500, 0.3)...)
	all = append(all, frames(1000, 0)...)

	pub := &capturePublisher{}
	cfg := DefaultEngineConfig()
	cfg.Chunker.PartialIntervalMs = 0
	cfg.Logger = testLogger()

	engine, err := NewEngine(cfg, newFakeSource(all), silentTranscriber{}, pub)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := engine.Run(ctx); err != nil {
		t.Fatal(err)
	}

	if got := pub.finalTexts(); len(got) != 0 {
		t.Fatalf("published a segment for unusable output: %v", got)
	}
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.partials) == 0 {
		t.Fatal("the stale preview was never cleared")
	}
	if last := pub.partials[len(pub.partials)-1]; last.Text != "" {
		t.Fatalf("expected a clearing partial, got %q", last.Text)
	}
}

func TestEngineDropsSilence(t *testing.T) {
	pub := &capturePublisher{}
	cfg := DefaultEngineConfig()
	cfg.Logger = testLogger()

	engine, err := NewEngine(cfg, newFakeSource(frames(5000, 0)), &asr.Mock{}, pub)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := engine.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if got := pub.finalTexts(); len(got) != 0 {
		t.Fatalf("silence produced transcript: %v", got)
	}
}

func TestEngineCommitsTheLastUtteranceOnShutdown(t *testing.T) {
	// The stream ends mid-sentence, which is what happens when the operator
	// stops the listener as the speaker finishes.
	var all [][]float32
	all = append(all, frames(600, 0)...)
	all = append(all, frames(2000, 0.3)...)

	pub := &capturePublisher{}
	cfg := DefaultEngineConfig()
	cfg.Chunker.PartialIntervalMs = 0
	cfg.Logger = testLogger()

	engine, err := NewEngine(cfg, newFakeSource(all), &asr.Mock{Latency: 300 * time.Millisecond}, pub)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := engine.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if got := pub.finalTexts(); len(got) != 1 {
		t.Fatalf("the final utterance was lost on shutdown: %v", got)
	}
}
