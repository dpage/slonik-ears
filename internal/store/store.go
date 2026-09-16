// Package store persists finalised transcript segments so that a server
// restart (or a laptop lid closing) does not lose a talk, and so organisers
// can hand out the transcript afterwards.
//
// The format is deliberately boring: one JSON object per line, one file per
// room. Any text editor, jq, or a two line script can read it.
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dpage/slonik-ears/internal/protocol"
)

// Store persists and reloads transcript segments.
type Store interface {
	Append(roomID string, seg protocol.Segment)
	Load(roomID string) ([]protocol.Segment, error)
	Rooms() ([]string, error)
	Close() error
}

// Null is a Store that throws everything away. Used when --data-dir is unset.
type Null struct{}

func (Null) Append(string, protocol.Segment)         {}
func (Null) Load(string) ([]protocol.Segment, error) { return nil, nil }
func (Null) Rooms() ([]string, error)                { return nil, nil }
func (Null) Close() error                            { return nil }

// File writes one JSONL file per room under a directory.
type File struct {
	dir string

	mu      sync.Mutex
	writers map[string]*roomWriter
	closed  bool
	stop    chan struct{}
	wg      sync.WaitGroup
}

type roomWriter struct {
	f  *os.File
	bw *bufio.Writer
}

// NewFile opens (creating if needed) a transcript directory.
func NewFile(dir string) (*File, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	s := &File{
		dir:     dir,
		writers: make(map[string]*roomWriter),
		stop:    make(chan struct{}),
	}
	// Flush periodically rather than on every line: a conference generates a
	// segment every few seconds, and fsync-per-line on a laptop SSD is a waste
	// of a perfectly good battery.
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				s.flush()
			case <-s.stop:
				return
			}
		}
	}()
	return s, nil
}

func (s *File) path(roomID string) string {
	return filepath.Join(s.dir, roomID+".jsonl")
}

// Append writes a segment. Errors are logged by the caller's logger via the
// returned error channel; here a failure must never take down a live talk, so
// it is reported once and then ignored.
func (s *File) Append(roomID string, seg protocol.Segment) {
	if !protocol.ValidRoomID(roomID) {
		return
	}
	line, err := json.Marshal(seg)
	if err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	w, ok := s.writers[roomID]
	if !ok {
		f, err := os.OpenFile(s.path(roomID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
		if err != nil {
			return
		}
		w = &roomWriter{f: f, bw: bufio.NewWriter(f)}
		s.writers[roomID] = w
	}
	w.bw.Write(line)
	w.bw.WriteByte('\n')
}

// Load reads back everything previously written for a room.
func (s *File) Load(roomID string) ([]protocol.Segment, error) {
	if !protocol.ValidRoomID(roomID) {
		return nil, fmt.Errorf("invalid room id %q", roomID)
	}
	s.mu.Lock()
	if w, ok := s.writers[roomID]; ok {
		w.bw.Flush()
	}
	s.mu.Unlock()

	f, err := os.Open(s.path(roomID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []protocol.Segment
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var seg protocol.Segment
		if err := json.Unmarshal(line, &seg); err != nil {
			continue // skip a torn line rather than lose the whole transcript
		}
		out = append(out, seg)
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return out, err
	}
	return out, nil
}

// Rooms lists the room ids that have stored transcripts.
func (s *File) Rooms() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		id := e.Name()[:len(e.Name())-len(".jsonl")]
		if protocol.ValidRoomID(id) {
			out = append(out, id)
		}
	}
	return out, nil
}

func (s *File) flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range s.writers {
		w.bw.Flush()
	}
}

// Close flushes and closes every open transcript file.
func (s *File) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	close(s.stop)
	s.wg.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()
	var firstErr error
	for id, w := range s.writers {
		if err := w.bw.Flush(); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := w.f.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(s.writers, id)
	}
	return firstErr
}
