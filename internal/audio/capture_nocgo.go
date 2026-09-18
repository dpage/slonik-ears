//go:build !cgo

package audio

import (
	"errors"
	"time"
)

// ErrNoCapture is returned when the binary was built without cgo, which
// miniaudio requires. Build with CGO_ENABLED=1 to capture live audio; a
// cgo-free build can still replay WAV files, which is enough for testing the
// server end of the system.
var ErrNoCapture = errors.New("audio: live capture needs a cgo build (CGO_ENABLED=1)")

// ListDevices is unavailable without cgo.
func ListDevices() ([]Device, error) { return nil, ErrNoCapture }

// MicSource is unavailable without cgo.
type MicSource struct{}

// OpenMicChannels is unavailable without cgo.
func OpenMicChannels(string, []int) (*MicSource, error) { return nil, ErrNoCapture }

// Channels implements the diagnostic the listener logs at start-up.
func (*MicSource) Channels() int { return 0 }

// SelectedChannels implements the diagnostic the listener logs at start-up.
func (*MicSource) SelectedChannels() []int { return nil }

// ChannelLevels implements the diagnostic the listener logs at start-up.
func (*MicSource) ChannelLevels() []float64 { return nil }

// ChannelPeaks implements the diagnostic the listener logs at start-up.
func (*MicSource) ChannelPeaks() ([]float64, []float64) { return nil, nil }

// AwaitChannelChoice implements the diagnostic the listener logs at start-up.
func (*MicSource) AwaitChannelChoice(time.Duration) bool { return true }

// Frames implements Source.
func (*MicSource) Frames() <-chan []float32 { return nil }

// Name implements Source.
func (*MicSource) Name() string { return "unavailable" }

// Err implements Source.
func (*MicSource) Err() error { return ErrNoCapture }

// Dropped implements the diagnostic used by the listener.
func (*MicSource) Dropped() int64 { return 0 }

// Close implements Source.
func (*MicSource) Close() error { return nil }
