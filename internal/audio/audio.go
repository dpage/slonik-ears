// Package audio handles capture, framing, voice activity detection and the
// segmentation of a continuous microphone stream into utterances that are
// worth sending to a speech-to-text model.
package audio

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

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
	// Monitor marks a loopback of something being played rather than anything
	// being listened to. PulseAudio offers one alongside every output, named
	// after it, and it is almost never what somebody selecting a microphone by
	// name has in mind.
	Monitor bool
}

// IsMonitorName reports whether a device name describes a loopback of an
// output rather than a real input.
func IsMonitorName(name string) bool {
	return strings.Contains(strings.ToLower(name), "monitor of ")
}

// SelectDevice resolves a --device selector against the available inputs and
// returns an index into devices, or -1 for "the system default".
//
// The selector is an index, an exact device id, or part of a name. It is the
// last of those that needs care, because on a PulseAudio host every output has
// a monitor source named after it. Ask for "Anker" and the first substring
// match is "Monitor of Anker PowerConf S330", a loopback of whatever the
// machine is playing. It opens without complaint and is silent, so the symptom
// is a listener that runs happily for ten minutes and transcribes nothing.
//
// A real input therefore always beats a monitor, and a monitor is chosen only
// when nothing else matches or when the selector asks for one in so many
// words.
func SelectDevice(devices []Device, selector string) (int, error) {
	if selector == "" {
		return -1, nil // the system default, left to the audio layer
	}
	if n, err := strconv.Atoi(selector); err == nil {
		if n < 0 || n >= len(devices) {
			return 0, fmt.Errorf("audio: there is no capture device %d (try --list-devices)", n)
		}
		return n, nil
	}

	lower := strings.ToLower(selector)
	wantsMonitor := strings.Contains(lower, "monitor")
	fallback := -1
	for i, d := range devices {
		if strings.EqualFold(d.ID, selector) {
			return i, nil
		}
		if !strings.Contains(strings.ToLower(d.Name), lower) {
			continue
		}
		if d.Monitor && !wantsMonitor {
			if fallback < 0 {
				fallback = i // remembered, but any real input displaces it
			}
			continue
		}
		return i, nil
	}
	if fallback >= 0 {
		return fallback, nil
	}
	return 0, fmt.Errorf("audio: no capture device matches %q (try --list-devices)", selector)
}

// DurationMs converts a sample count to milliseconds.
func DurationMs(samples int) int64 {
	return int64(samples) * 1000 / SampleRate
}
