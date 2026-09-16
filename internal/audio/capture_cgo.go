//go:build cgo

package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gen2brain/malgo"
)

// ListDevices enumerates capture devices.
//
// On macOS the list contains the built-in microphone, any USB or Bluetooth
// input, and — if installed — virtual devices such as BlackHole or Loopback.
// A virtual device is how you capture the room's PA feed or the audio of a
// video call, since macOS will not let an application record system output
// directly.
func ListDevices() ([]Device, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("audio: init context: %w", err)
	}
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()

	infos, err := ctx.Devices(malgo.Capture)
	if err != nil {
		return nil, fmt.Errorf("audio: enumerate capture devices: %w", err)
	}
	out := make([]Device, 0, len(infos))
	for i := range infos {
		out = append(out, Device{
			ID:      infos[i].ID.String(),
			Name:    infos[i].Name(),
			Default: infos[i].IsDefault != 0,
		})
	}
	return out, nil
}

// MicSource captures live audio from a hardware (or virtual) input device.
type MicSource struct {
	name    string
	frames  chan []float32
	ctx     *malgo.AllocatedContext
	device  *malgo.Device
	partial []float32

	mu      sync.Mutex
	closed  bool
	dropped atomic.Int64
	err     error
}

// OpenMic starts capturing from the named device. An empty selector uses the
// system default; otherwise the selector matches a device by index ("2"), by
// exact id, or by a case-insensitive substring of its name ("blackhole").
func OpenMic(selector string) (*MicSource, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("audio: init context: %w", err)
	}

	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatF32
	cfg.Capture.Channels = 1
	cfg.SampleRate = SampleRate
	// miniaudio resamples and downmixes internally, so whatever the device
	// natively offers arrives here as 16 kHz mono float32.
	cfg.Alsa.NoMMap = 1

	chosen := "system default"
	if selector != "" {
		infos, err := ctx.Devices(malgo.Capture)
		if err != nil {
			_ = ctx.Uninit()
			ctx.Free()
			return nil, fmt.Errorf("audio: enumerate capture devices: %w", err)
		}
		idx := -1
		if n, convErr := strconv.Atoi(selector); convErr == nil && n >= 0 && n < len(infos) {
			idx = n
		} else {
			for i := range infos {
				if strings.EqualFold(infos[i].ID.String(), selector) ||
					strings.Contains(strings.ToLower(infos[i].Name()), strings.ToLower(selector)) {
					idx = i
					break
				}
			}
		}
		if idx < 0 {
			_ = ctx.Uninit()
			ctx.Free()
			return nil, fmt.Errorf("audio: no capture device matches %q (try --list-devices)", selector)
		}
		cfg.Capture.DeviceID = infos[idx].ID.Pointer()
		chosen = infos[idx].Name()
	}

	s := &MicSource{
		name: chosen,
		// Roughly four seconds of slack. If the consumer falls this far
		// behind, something is badly wrong and dropping is kinder than
		// growing without bound.
		frames: make(chan []float32, 200),
		ctx:    ctx,
	}

	device, err := malgo.InitDevice(ctx.Context, cfg, malgo.DeviceCallbacks{Data: s.onFrames})
	if err != nil {
		_ = ctx.Uninit()
		ctx.Free()
		return nil, fmt.Errorf("audio: open capture device: %w", err)
	}
	s.device = device

	if err := device.Start(); err != nil {
		device.Uninit()
		_ = ctx.Uninit()
		ctx.Free()
		return nil, fmt.Errorf("audio: start capture: %w", err)
	}
	return s, nil
}

// onFrames runs on miniaudio's realtime thread. It must not allocate much,
// must not block, and must never panic.
func (s *MicSource) onFrames(_, input []byte, frameCount uint32) {
	if frameCount == 0 || len(input) < 4 {
		return
	}
	n := len(input) / 4
	for i := 0; i < n; i++ {
		v := math.Float32frombits(binary.LittleEndian.Uint32(input[i*4 : i*4+4]))
		s.partial = append(s.partial, v)
		if len(s.partial) == FrameSize {
			frame := make([]float32, FrameSize)
			copy(frame, s.partial)
			s.partial = s.partial[:0]
			select {
			case s.frames <- frame:
			default:
				s.dropped.Add(1)
			}
		}
	}
}

// Frames implements Source.
func (s *MicSource) Frames() <-chan []float32 { return s.frames }

// Name implements Source.
func (s *MicSource) Name() string { return "microphone: " + s.name }

// Err implements Source.
func (s *MicSource) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Dropped reports how many frames were discarded because the consumer could
// not keep up. Anything other than zero deserves investigation.
func (s *MicSource) Dropped() int64 { return s.dropped.Load() }

// Close implements Source.
func (s *MicSource) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	if s.device != nil {
		s.device.Uninit() // stops the device and waits for the callback to finish
	}
	if s.ctx != nil {
		_ = s.ctx.Uninit()
		s.ctx.Free()
	}
	close(s.frames)
	return nil
}
