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
	"log/slog"
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
	// Archive sets a room's transcript aside so that the next talk starts from
	// nothing, without destroying the last one. Nothing in this package
	// deletes a transcript: turning a room around between talks is routine,
	// and a button pressed during the wrong one should not cost a speaker
	// their session.
	Archive(roomID string) error
	Close() error
}

// ArchiveDir is where archived transcripts are put, beneath the data
// directory. A subdirectory rather than a naming convention, so that Rooms
// cannot mistake an archive for a live room whatever it ends up being called.
const ArchiveDir = "archive"

// Null is a Store that throws everything away. Used when --data-dir is unset.
type Null struct{}

// Append discards the segment.
func (Null) Append(string, protocol.Segment) {}

// Load returns no history.
func (Null) Load(string) ([]protocol.Segment, error) { return nil, nil }

// Rooms returns no rooms.
func (Null) Rooms() ([]string, error) { return nil, nil }

// Archive has nothing to set aside.
func (Null) Archive(string) error { return nil }

// Close does nothing.
func (Null) Close() error { return nil }

// File writes one JSONL file per room under a directory.
type File struct {
	dir string
	log *slog.Logger

	mu      sync.Mutex
	writers map[string]*roomWriter
	closed  bool
	failed  bool // a write error has already been reported
	stop    chan struct{}
	wg      sync.WaitGroup
}

type roomWriter struct {
	f  *os.File
	bw *bufio.Writer
}

// NewFile opens (creating if needed) a transcript directory. A nil logger
// falls back to the default one.
func NewFile(dir string, log *slog.Logger) (*File, error) {
	if log == nil {
		log = slog.Default()
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	s := &File{
		dir:     dir,
		log:     log,
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

// Append writes a segment. A failure must never take down a live talk, so it
// is logged once — a full disk should be visible to whoever is running the
// event, not silently swallowed — and then tolerated.
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
			s.reportLocked("open transcript file", roomID, err)
			return
		}
		w = &roomWriter{f: f, bw: bufio.NewWriter(f)}
		s.writers[roomID] = w
	}
	if _, err := w.bw.Write(append(line, '\n')); err != nil {
		s.reportLocked("write transcript", roomID, err)
	}
}

// reportLocked logs the first storage failure and stays quiet about the rest,
// so a failing disk does not also flood the log. The caller holds s.mu.
func (s *File) reportLocked(what, roomID string, err error) {
	if s.failed {
		return
	}
	s.failed = true
	s.log.Error("transcript storage is failing; the live transcript continues but is not being saved",
		"operation", what, "room", roomID, "error", err)
}

// Load reads back everything previously written for a room.
func (s *File) Load(roomID string) ([]protocol.Segment, error) {
	if !protocol.ValidRoomID(roomID) {
		return nil, fmt.Errorf("invalid room id %q", roomID)
	}
	s.mu.Lock()
	if w, ok := s.writers[roomID]; ok {
		if err := w.bw.Flush(); err != nil {
			s.reportLocked("flush transcript", roomID, err)
		}
	}
	s.mu.Unlock()

	f, err := os.Open(s.path(roomID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

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

// Archive moves a room's transcript into the archive directory, stamped with
// the time it was set aside, and leaves the room with no transcript at all.
//
// The open writer has to go first. Renaming a file out from under a file
// descriptor does not move the descriptor: it would carry on happily appending
// the next talk to the archived file, which is the opposite of the point.
//
// Closing the writer and renaming the file also have to happen as one
// operation, under the lock throughout. A room is reset while the previous
// speaker may still be finishing a sentence, and a segment that arrives
// between the close and the rename reopens the very file that is about to be
// moved. The new talk's writer then follows the file into the archive, the
// live transcript is never recreated for the life of the process, and at the
// next restart the two talks come back merged into one.
func (s *File) Archive(roomID string) error {
	if !protocol.ValidRoomID(roomID) {
		return fmt.Errorf("invalid room id %q", roomID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if w, ok := s.writers[roomID]; ok {
		if err := w.bw.Flush(); err != nil {
			s.reportLocked("flush transcript", roomID, err)
		}
		_ = w.f.Close()
		delete(s.writers, roomID)
	}

	src := s.path(roomID)
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		return nil // nothing recorded yet, so nothing to set aside
	}

	dir := filepath.Join(s.dir, ArchiveDir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create archive dir: %w", err)
	}
	// Seconds resolution is plenty between talks, but two archives in the same
	// second must not silently overwrite one another.
	stamp := time.Now().UTC().Format("2006-01-02T15-04-05")
	dst := filepath.Join(dir, fmt.Sprintf("%s-%s.jsonl", roomID, stamp))
	for n := 2; ; n++ {
		if _, err := os.Stat(dst); errors.Is(err, os.ErrNotExist) {
			break
		}
		dst = filepath.Join(dir, fmt.Sprintf("%s-%s-%d.jsonl", roomID, stamp, n))
	}
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("archive transcript: %w", err)
	}
	s.log.Info("archived transcript", "room", roomID, "file", dst)
	return nil
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
	for id, w := range s.writers {
		if err := w.bw.Flush(); err != nil {
			s.reportLocked("flush transcript", id, err)
		}
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
