//go:build cgo

package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

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
			Monitor: IsMonitorName(infos[i].Name()),
		})
	}
	return out, nil
}

// MicSource captures live audio from a hardware (or virtual) input device.
//
// It deliberately does NOT ask miniaudio to deliver mono. A multichannel
// interface generally offers no mono format at all — a TASCAM US-16x08 offers
// 16 channels and nothing else — so a request for one channel is satisfied by
// averaging every channel the device has. With a microphone on input 1 and the
// other fifteen inputs unplugged and silent, that average is the microphone
// divided by sixteen: 24 dB thrown away before the detector ever sees it,
// measured as a peak of 0.025 where the real input was 0.42.
//
// That was not a hypothetical. It made speech sit a whisker above the voice
// detector's threshold, chopped a quarter of all utterances mid-word, and had
// everybody reaching for the gain knob on perfectly good equipment. It is also
// exactly the setup this project recommends, a feed from the desk into a USB
// interface, so it was the normal case rather than an exotic one.
//
// So capture happens at the device's native channel count and only the
// channels carrying the microphone are mixed down here.
type MicSource struct {
	name    string
	frames  chan []float32
	ctx     *malgo.AllocatedContext
	device  *malgo.Device
	partial []float32

	// channels is what the device actually delivers; selected is the 0-based
	// subset averaged into the mono stream.
	channels int
	selected []int
	// scratch is the decoded interleaved buffer, reused so that the realtime
	// callback allocates nothing per buffer.
	scratch []float32

	// calibration is non-nil whilst the loudest channel is still being
	// chosen, accumulating energy per channel.
	calibrating atomic.Bool
	calMu       sync.Mutex
	calEnergy   []float64
	calPeak     []float64
	calClipped  []int
	calFrames   int

	mu      sync.Mutex
	closed  bool
	dropped atomic.Int64
	err     error
}

// OpenMic starts capturing from the named device. An empty selector uses the
// system default; otherwise the selector matches a device by index ("2"), by
// exact id, or by a case-insensitive substring of its name ("blackhole").
func OpenMic(selector string) (*MicSource, error) {
	return OpenMicChannels(selector, nil)
}

// OpenMicChannels is OpenMic with an explicit choice of input channels, given
// 1-based as a person reads them off the front of an interface. More than one
// is averaged, which is what a genuine stereo microphone wants ("1,2"). An
// empty list means listen to every channel for a moment and take whichever is
// carrying the most signal.
func OpenMicChannels(selector string, wanted []int) (*MicSource, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("audio: init context: %w", err)
	}

	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatF32
	// Zero means "whatever the device has". Asking for one channel would make
	// miniaudio average them all, which is the bug this function exists to
	// avoid; the mixing down happens in onFrames instead, over the channels
	// that actually carry the microphone.
	cfg.Capture.Channels = 0
	cfg.SampleRate = SampleRate
	cfg.Alsa.NoMMap = 1

	chosen := "system default"
	if selector != "" {
		infos, err := ctx.Devices(malgo.Capture)
		if err != nil {
			_ = ctx.Uninit()
			ctx.Free()
			return nil, fmt.Errorf("audio: enumerate capture devices: %w", err)
		}
		devices := make([]Device, 0, len(infos))
		for i := range infos {
			devices = append(devices, Device{
				ID:      infos[i].ID.String(),
				Name:    infos[i].Name(),
				Monitor: IsMonitorName(infos[i].Name()),
			})
		}
		idx, selErr := SelectDevice(devices, selector)
		if selErr != nil {
			_ = ctx.Uninit()
			ctx.Free()
			return nil, selErr
		}
		if idx >= 0 {
			cfg.Capture.DeviceID = infos[idx].ID.Pointer()
			chosen = infos[idx].Name()
		}
	}

	s := &MicSource{
		name: chosen,
		// Roughly four seconds of slack. If the consumer falls this far
		// behind, something is badly wrong and dropping is kinder than
		// growing without bound.
		frames: make(chan []float32, 200),
		ctx:    ctx,
	}

	// The Stop callback matters as much as the Data one. Without it, a device
	// that goes away mid-talk (a USB interface unplugged, an input reclaimed
	// by another application) simply stops calling back: the frames channel
	// stays open and empty, Err stays nil, and the listener sits there looking
	// healthy whilst transcribing silence for the rest of the session.
	device, err := malgo.InitDevice(ctx.Context, cfg, malgo.DeviceCallbacks{
		Data: s.onFrames,
		Stop: s.onStop,
	})
	if err != nil {
		_ = ctx.Uninit()
		ctx.Free()
		return nil, fmt.Errorf("audio: open capture device: %w", err)
	}
	s.device = device
	s.channels = int(device.CaptureChannels())
	if s.channels < 1 {
		s.channels = 1
	}

	s.selected, err = resolveChannels(wanted, s.channels)
	if err != nil {
		device.Uninit()
		_ = ctx.Uninit()
		ctx.Free()
		return nil, err
	}
	if s.selected == nil {
		// Nothing was asked for, so listen before deciding.
		s.calEnergy = make([]float64, s.channels)
		s.calPeak = make([]float64, s.channels)
		s.calClipped = make([]int, s.channels)
		s.calibrating.Store(true)
	}

	if err := device.Start(); err != nil {
		device.Uninit()
		_ = ctx.Uninit()
		ctx.Free()
		return nil, fmt.Errorf("audio: start capture: %w", err)
	}
	return s, nil
}

// resolveChannels validates a 1-based channel list against what the device
// has. It returns nil when nothing was asked for, meaning "work it out".
func resolveChannels(wanted []int, have int) ([]int, error) {
	if len(wanted) == 0 {
		return nil, nil
	}
	seen := make(map[int]bool, len(wanted))
	out := make([]int, 0, len(wanted))
	for _, ch := range wanted {
		if ch < 1 || ch > have {
			return nil, fmt.Errorf("audio: this device has %d input channel(s), so channel %d does not exist", have, ch)
		}
		if seen[ch] {
			continue
		}
		seen[ch] = true
		out = append(out, ch-1)
	}
	return out, nil
}

// ChannelLevels reports the RMS of each input channel gathered during
// calibration, so the listener can say where it found the signal and where it
// did not. It returns nil once calibration has finished being interesting.
func (s *MicSource) ChannelLevels() []float64 {
	s.calMu.Lock()
	defer s.calMu.Unlock()
	if s.calFrames == 0 {
		return nil
	}
	out := make([]float64, len(s.calEnergy))
	for i, e := range s.calEnergy {
		out[i] = math.Sqrt(e / float64(s.calFrames))
	}
	return out
}

// ChannelPeaks reports the highest absolute sample seen on each channel during
// calibration, and how many samples of each hit full scale. A peak of 1.0 is
// not a strong signal, it is a clipped one.
func (s *MicSource) ChannelPeaks() (peaks []float64, clippedFraction []float64) {
	s.calMu.Lock()
	defer s.calMu.Unlock()
	if s.calFrames == 0 {
		return nil, nil
	}
	peaks = make([]float64, len(s.calPeak))
	clippedFraction = make([]float64, len(s.calClipped))
	copy(peaks, s.calPeak)
	for i, n := range s.calClipped {
		clippedFraction[i] = float64(n) / float64(s.calFrames)
	}
	return peaks, clippedFraction
}

// Channels reports how many input channels the device delivers.
func (s *MicSource) Channels() int { return s.channels }

// SelectedChannels reports the 1-based channels being mixed into the
// transcript, once they have been settled on.
func (s *MicSource) SelectedChannels() []int {
	s.calMu.Lock()
	defer s.calMu.Unlock()
	out := make([]int, 0, len(s.selected))
	for _, c := range s.selected {
		out = append(out, c+1)
	}
	return out
}

// finishCalibration picks the channel carrying the most signal. Called from
// the audio callback, which is why it does no logging and no allocation
// beyond the one slice.
func (s *MicSource) finishCalibration() {
	s.calMu.Lock()
	defer s.calMu.Unlock()
	if s.selected != nil {
		return
	}
	best, bestEnergy := 0, -1.0
	for i, e := range s.calEnergy {
		if e > bestEnergy {
			best, bestEnergy = i, e
		}
	}
	// A room that was silent throughout gives nothing to go on, and channel 1
	// is where a single microphone is nearly always plugged.
	s.selected = []int{best}
	s.calibrating.Store(false)
}

// onFrames runs on miniaudio's realtime thread. It must not allocate much,
// must not block, and must never panic.
//
// The input is interleaved across the device's channels, so a sample for the
// chosen channel appears once every s.channels samples. Averaging across all
// of them, which is what asking miniaudio for mono would have done, divides a
// single live microphone by the number of sockets on the box.
// onStop records that the device has stopped. Closing the frames channel is
// deliberately left to Close, which is the only place that owns it and which
// waits for the data callback to finish first; the watchdog in the listener
// is what turns this into something the operator sees.
func (s *MicSource) onStop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.err != nil {
		return // an expected stop, on the way out through Close
	}
	s.err = errors.New("audio: the capture device stopped: it may have been unplugged or taken by another application")
}

func (s *MicSource) onFrames(_, input []byte, frameCount uint32) {
	if frameCount == 0 || len(input) < 4 {
		return
	}
	nch := s.channels
	if nch < 1 {
		nch = 1
	}
	total := len(input) / 4
	sampleAt := func(i int) float64 {
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(input[i*4 : i*4+4])))
	}

	if s.calibrating.Load() {
		// Measure every channel, and pass nothing on yet: two seconds of
		// nothing at start-up is invisible, whereas two seconds of the wrong
		// channel would be committed to the transcript.
		s.calMu.Lock()
		for f := 0; f+nch <= total; f += nch {
			for c := 0; c < nch && c < len(s.calEnergy); c++ {
				v := sampleAt(f + c)
				s.calEnergy[c] += v * v
				if a := math.Abs(v); a > s.calPeak[c] {
					s.calPeak[c] = a
				}
				// Full scale in a float32 capture means the converter ran out
				// of headroom, so the waveform has had its top taken off.
				if math.Abs(v) >= 0.999 {
					s.calClipped[c]++
				}
			}
			s.calFrames++
		}
		enough := s.calFrames >= int(CalibrationWindow.Seconds())*SampleRate
		s.calMu.Unlock()
		if enough {
			s.finishCalibration()
		}
		return
	}

	s.scratch = decodeF32(input, s.scratch[:0])
	s.partial = MixDown(s.scratch, nch, s.selected, s.partial)
	for len(s.partial) >= FrameSize {
		frame := make([]float32, FrameSize)
		copy(frame, s.partial[:FrameSize])
		s.partial = append(s.partial[:0], s.partial[FrameSize:]...)
		select {
		case s.frames <- frame:
		default:
			s.dropped.Add(1)
		}
	}
}

// decodeF32 reads little-endian float32 samples into dst, reusing its capacity
// so the realtime callback does not allocate on every buffer.
func decodeF32(input []byte, dst []float32) []float32 {
	for i := 0; i+4 <= len(input); i += 4 {
		dst = append(dst, math.Float32frombits(binary.LittleEndian.Uint32(input[i:i+4])))
	}
	return dst
}

// MixDown reduces interleaved multichannel audio to mono, averaging only the
// selected 0-based channels and appending the result to dst.
//
// Averaging only the chosen channels is the whole point. Averaging all of them
// — which is what a device driver does when asked for mono it cannot provide —
// divides a single live microphone by the number of inputs on the box, whether
// or not anything is plugged into them.
func MixDown(interleaved []float32, channels int, selected []int, dst []float32) []float32 {
	if channels < 1 {
		channels = 1
	}
	if len(selected) == 0 {
		selected = []int{0}
	}
	for f := 0; f+channels <= len(interleaved); f += channels {
		var sum float64
		var n int
		for _, c := range selected {
			if c >= 0 && c < channels {
				sum += float64(interleaved[f+c])
				n++
			}
		}
		if n == 0 {
			n = 1
		}
		dst = append(dst, float32(sum/float64(n)))
	}
	return dst
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

// AwaitChannelChoice blocks until the loudest input channel has been settled
// on, or the limit expires. It reports whether the choice was made in time.
//
// It exists so the listener can say in its log which channel it is listening
// to and what every other channel measured. That one line is the difference
// between "the transcript is poor" and "the microphone is on input 3".
func (s *MicSource) AwaitChannelChoice(limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if !s.calibrating.Load() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return !s.calibrating.Load()
}
