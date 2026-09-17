// Package asr wraps speech-to-text backends behind a single interface so the
// listener does not care whether transcription happens on the same Mac, on a
// machine in the next room, or in somebody's cloud.
package asr

import (
	"context"
	"time"
)

// Options tunes a single transcription request.
type Options struct {
	// Language is an ISO-639-1 code, or "auto" to let the model decide.
	Language string
	// Prompt is carried over from the previous segment. Whisper uses it as
	// context, which markedly improves continuity of names and jargon across
	// chunk boundaries.
	Prompt string
	// Translate asks the model to translate into English rather than
	// transcribe verbatim.
	Translate bool
	// Temperature of 0 is the sane default for transcription.
	Temperature float64
	// Partial marks a request for an in-progress utterance. Backends may use
	// it to trade accuracy for latency.
	Partial bool
}

// Segment is a timestamped piece of a transcription result.
type Segment struct {
	Text    string
	StartMs int64
	EndMs   int64
}

// Result is what a backend returns for one chunk of audio.
type Result struct {
	Text string
	// Raw is the backend's output before CleanTranscript ran over it. Text
	// being empty whilst Raw is not means the cleaning rules threw the whole
	// utterance away, which is worth seeing in a debug log rather than
	// guessing at.
	Raw      string
	Language string
	Segments []Segment
	// Took is how long the backend needed. The listener logs it so you can
	// tell at a glance whether the model is keeping up with the speaker.
	Took time.Duration
}

// Transcriber turns mono 16 kHz PCM into text.
type Transcriber interface {
	// Transcribe blocks until the audio has been transcribed or ctx expires.
	Transcribe(ctx context.Context, pcm []float32, sampleRate int, opts Options) (Result, error)
	// Name identifies the backend in logs and in the room status shown to
	// organisers.
	Name() string
	// Close releases any resources held by the backend.
	Close() error
}
