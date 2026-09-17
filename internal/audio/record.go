package audio

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"time"
)

// Recorder wraps a Source and writes everything passing through it to a WAV
// file, so a session that behaved oddly can be replayed through --file as many
// times as it takes to work out why. Segmentation is a state machine over the
// audio, and tuning it against a live speaker means asking a person to say the
// same six sentences over and over, which is both slow and unreliable.
//
// This is a debugging tool, not a feature: recording a room full of people is
// a matter for whoever organised the event, and the recording contains every
// word anybody said near the microphone. Keep it out of version control.
type Recorder struct {
	inner  Source
	frames chan []float32
	path   string

	// done closes when the pump has finished, so Close can patch the header
	// knowing nothing more will be written.
	done chan struct{}

	mu      sync.Mutex
	file    *os.File
	written int // bytes of PCM, for the header patch on close
	err     error
}

// RecordTo returns a Source that forwards inner's frames unchanged whilst
// writing them to path as a 16 kHz mono WAV.
func RecordTo(inner Source, path string) (*Recorder, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("audio: create recording: %w", err)
	}
	// A placeholder header, patched with the real lengths by Close.
	if _, err := f.Write(wavHeader(SampleRate, 0)); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("audio: write recording header: %w", err)
	}

	r := &Recorder{
		inner:  inner,
		frames: make(chan []float32, 200),
		path:   path,
		file:   f,
		done:   make(chan struct{}),
	}
	go r.pump()
	return r, nil
}

// pump copies frames through, recording as it goes. Writing to a local file at
// 32 kB/s will not block anything, but a failed write must not take the
// listener down with it: the transcript matters more than the recording.
func (r *Recorder) pump() {
	defer close(r.done)
	defer close(r.frames)
	for frame := range r.inner.Frames() {
		r.write(frame)
		r.frames <- frame
	}
}

func (r *Recorder) write(frame []float32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return
	}
	n, err := r.file.Write(pcmBytes(frame))
	r.written += n
	if err != nil && r.err == nil {
		r.err = fmt.Errorf("audio: writing recording: %w", err)
		_ = r.file.Close()
		r.file = nil
	}
}

// Frames implements Source.
func (r *Recorder) Frames() <-chan []float32 { return r.frames }

// Name implements Source.
func (r *Recorder) Name() string { return r.inner.Name() + " (recording to " + r.path + ")" }

// Err implements Source, preferring the underlying source's error: a failed
// recording is a nuisance, a failed capture is the end of the transcript.
func (r *Recorder) Err() error {
	if err := r.inner.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

// Dropped forwards the underlying source's count, if it keeps one.
func (r *Recorder) Dropped() int64 {
	if d, ok := r.inner.(Dropper); ok {
		return d.Dropped()
	}
	return 0
}

// Close stops capture and finishes the WAV file, patching the two length
// fields that could not be known when the header was written.
func (r *Recorder) Close() error {
	err := r.inner.Close()

	// Closing the source ends the pump, but only once whatever is reading
	// Frames has drained them. If nothing is reading any more, patch the
	// header anyway rather than hanging the shutdown: a WAV whose length field
	// is a fraction of a second short still plays.
	select {
	case <-r.done:
	case <-time.After(2 * time.Second):
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return err
	}
	var sizes [4]byte
	binary.LittleEndian.PutUint32(sizes[:], uint32(36+r.written))
	if _, werr := r.file.WriteAt(sizes[:], 4); werr != nil && err == nil {
		err = werr
	}
	binary.LittleEndian.PutUint32(sizes[:], uint32(r.written))
	if _, werr := r.file.WriteAt(sizes[:], 40); werr != nil && err == nil {
		err = werr
	}
	if cerr := r.file.Close(); cerr != nil && err == nil {
		err = cerr
	}
	r.file = nil
	return err
}
