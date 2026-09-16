package audio

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// FileSource replays a WAV file as if it were arriving from a microphone,
// pacing the frames in real time. It makes the whole pipeline testable
// without a room, a speaker, or indeed a sound card.
type FileSource struct {
	path   string
	frames chan []float32
	stop   chan struct{}
	once   sync.Once

	mu  sync.Mutex
	err error
}

// OpenFile loads a WAV file, resampling to 16 kHz mono if needed. If realtime
// is false the file is delivered as fast as the consumer accepts it, which is
// what you want in tests.
func OpenFile(path string, realtime bool, loop bool) (*FileSource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("audio: read %s: %w", path, err)
	}
	pcm, rate, err := DecodeWAV(data)
	if err != nil {
		return nil, fmt.Errorf("audio: %s: %w", path, err)
	}
	pcm = Resample(pcm, rate, SampleRate)

	s := &FileSource{
		path:   path,
		frames: make(chan []float32, 32),
		stop:   make(chan struct{}),
	}
	go s.run(pcm, realtime, loop)
	return s, nil
}

func (s *FileSource) run(pcm []float32, realtime, loop bool) {
	defer close(s.frames)
	ticker := time.NewTicker(FrameMs * time.Millisecond)
	defer ticker.Stop()

	for {
		for off := 0; off+FrameSize <= len(pcm); off += FrameSize {
			frame := make([]float32, FrameSize)
			copy(frame, pcm[off:off+FrameSize])
			if realtime {
				select {
				case <-ticker.C:
				case <-s.stop:
					return
				}
			}
			select {
			case s.frames <- frame:
			case <-s.stop:
				return
			}
		}
		if !loop {
			return
		}
		// A moment of silence between loops so the chunker commits the last
		// utterance rather than running the file into itself.
		for i := 0; i < 50; i++ {
			select {
			case s.frames <- make([]float32, FrameSize):
			case <-s.stop:
				return
			}
		}
	}
}

// Frames implements Source.
func (s *FileSource) Frames() <-chan []float32 { return s.frames }

// Name implements Source.
func (s *FileSource) Name() string { return "file: " + s.path }

// Err implements Source.
func (s *FileSource) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Close implements Source.
func (s *FileSource) Close() error {
	s.once.Do(func() { close(s.stop) })
	return nil
}
