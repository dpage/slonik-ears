package listener

import (
	"context"
	"math"
	"strings"
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

// TestTheQueueDoesNotGrowWithoutLimit covers a model that cannot keep up with
// the room. The queue used to be unbounded, so every utterance the model was
// too slow for was held in full: roughly a megabyte of audio each, growing for
// as long as the talk lasted, whilst the transcript fell further and further
// behind the speaker with nothing to say so.
func TestTheQueueDoesNotGrowWithoutLimit(t *testing.T) {
	e := &Engine{
		cfg:  DefaultEngineConfig(),
		log:  testLogger(),
		wake: make(chan struct{}, 1),
	}

	for i := range maxQueuedFinals * 3 {
		e.enqueue(audio.Request{
			Kind:    audio.KindFinal,
			StartMs: int64(i) * 5000,
			EndMs:   int64(i)*5000 + 4000,
		})
	}

	e.mu.Lock()
	queued := len(e.queuedFinals)
	oldest := e.queuedFinals[0].StartMs
	e.mu.Unlock()

	if queued != maxQueuedFinals {
		t.Errorf("the queue holds %d utterances, want it capped at %d", queued, maxQueuedFinals)
	}
	// What survives has to be the most recent audio: an audience reading a
	// live transcript needs what is being said now, not what was said two
	// minutes ago.
	wantOldest := int64(maxQueuedFinals*3-maxQueuedFinals) * 5000
	if oldest != wantOldest {
		t.Errorf("the queue kept audio from %dms, want the newest starting at %dms", oldest, wantOldest)
	}
}

// TestASilentCaptureDeviceIsReported covers a microphone that goes away
// mid-talk. Silence still arrives as frames, so a device that stops calling
// back at all leaves the listener sitting there looking healthy and
// transcribing nothing for the rest of the session.
func TestASilentCaptureDeviceIsReported(t *testing.T) {
	src := &stallingSource{ch: make(chan []float32)}
	pub := &capturePublisher{}
	cfg := DefaultEngineConfig()
	cfg.Logger = testLogger()
	e, err := NewEngine(cfg, src, &asr.Mock{}, pub)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = e.Run(ctx)
	}()

	// The device never delivers a frame. Rather than wait out the real
	// timeout, check that the engine has noticed once it has elapsed.
	deadline := time.Now().Add(noAudioTimeout + 3*time.Second)
	for time.Now().Before(deadline) {
		e.stateMu.Lock()
		got := e.lastErr
		e.stateMu.Unlock()
		if got != "" {
			cancel()
			<-done
			if !strings.Contains(got, "no audio") {
				t.Errorf("the engine reported %q, want it to name the missing audio", got)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	<-done
	t.Errorf("a capture device that delivered nothing for %v was never reported", noAudioTimeout)
}

// stallingSource is a microphone that opens and then goes quiet for ever,
// which is what an unplugged USB interface looks like from here.
type stallingSource struct {
	ch chan []float32
}

func (s *stallingSource) Frames() <-chan []float32 { return s.ch }
func (s *stallingSource) Err() error               { return nil }
func (s *stallingSource) Name() string             { return "stalling" }
func (s *stallingSource) Close() error             { return nil }
