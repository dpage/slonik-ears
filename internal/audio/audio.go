// Package audio handles capture, framing, voice activity detection and the
// segmentation of a continuous microphone stream into utterances that are
// worth sending to a speech-to-text model.
package audio

import "time"

// SampleRate is the rate everything in this project works at. Whisper models
// expect 16 kHz mono, so there is no reason to carry anything else around.
const SampleRate = 16000

// FrameMs is the granularity of voice activity detection. 20 ms is small
// enough to react quickly and large enough that the arithmetic is free.
const FrameMs = 20

// FrameSize is the number of samples in one VAD frame.
const FrameSize = SampleRate * FrameMs / 1000

// CalibrationWindow is how long a multichannel device is listened to before
// deciding which of its inputs carries the microphone. Long enough to catch a
// syllable or two, short enough that nobody notices it at start-up.
const CalibrationWindow = 2 * time.Second

// Source produces mono float32 frames at SampleRate.
type Source interface {
	// Frames returns the channel of audio frames. It is closed when the
	// source ends (end of file, device removed, or Close).
	Frames() <-chan []float32
	// Err returns the error that ended the stream, if any.
	Err() error
	// Name describes the source for logs.
	Name() string
	// Close stops capture and releases the device.
	Close() error
}

// Dropper is implemented by sources that can fall behind their consumer. A
// live microphone can, because the device carries on producing audio whether
// or not anything is reading it; a file replayed from disk cannot. Anything
// other than zero means audio was discarded before it was ever transcribed.
type Dropper interface {
	Dropped() int64
}

// Device is a capture device offered by the host.
type Device struct {
	ID      string
	Name    string
	Default bool
}

// DurationMs converts a sample count to milliseconds.
func DurationMs(samples int) int64 {
	return int64(samples) * 1000 / SampleRate
}
