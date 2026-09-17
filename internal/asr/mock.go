package asr

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"
)

// Mock is a Transcriber that invents plausible text from the audio's length
// and loudness. It exists so the whole pipeline — capture, chunking, publish,
// fan-out, browser — can be exercised on a machine with no model, no GPU and
// no microphone, which is exactly the machine CI runs on.
type Mock struct {
	// Latency simulates model think time.
	Latency time.Duration

	mu sync.Mutex
	// next is where the following utterance starts in mockWords. Only a
	// committed segment advances it, which is what makes an interim result a
	// prefix of the final it grows into.
	next int
}

var mockWords = strings.Fields(`the elephant in the room is replication lag but postgres 
handles it rather well these days provided you remember to monitor the slot and not let it 
grow unbounded over a long weekend which is how most of us learned about it in the first place`)

// Name implements Transcriber.
func (m *Mock) Name() string { return "mock" }

// Close implements Transcriber.
func (m *Mock) Close() error { return nil }

// Transcribe implements Transcriber.
func (m *Mock) Transcribe(ctx context.Context, pcm []float32, sampleRate int, opts Options) (Result, error) {
	if m.Latency > 0 {
		select {
		case <-time.After(m.Latency):
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	}
	if len(pcm) == 0 || sampleRate <= 0 {
		return Result{}, nil
	}
	// Silence in, silence out: this keeps the VAD and the junk filter honest.
	var peak float64
	for _, s := range pcm {
		peak = math.Max(peak, math.Abs(float64(s)))
	}
	if peak < 0.01 {
		return Result{}, nil
	}

	seconds := float64(len(pcm)) / float64(sampleRate)
	words := int(seconds * 2.5)
	if words < 1 {
		words = 1
	}

	// A real model transcribes the same utterance twice: once part-way
	// through for the live preview, and again when the speaker pauses. The
	// second result extends the first rather than replacing it with something
	// unrelated. Reproduce that by letting only a committed segment move the
	// cursor on, so successive previews of one utterance grow from the same
	// starting word, and successive utterances read as continuous prose.
	m.mu.Lock()
	start := m.next
	if !opts.Partial {
		m.next = (m.next + words) % len(mockWords)
	}
	m.mu.Unlock()

	out := make([]string, 0, words)
	for i := 0; i < words; i++ {
		out = append(out, mockWords[(start+i)%len(mockWords)])
	}
	text := strings.Join(out, " ")
	if !opts.Partial {
		text += "."
	}
	return Result{Text: text, Language: "en", Took: m.Latency}, nil
}
