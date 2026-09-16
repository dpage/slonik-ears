package store

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dpage/slonik-ears/internal/protocol"
)

func TestFileStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFile(dir, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		s.Append("main-hall", protocol.Segment{Seq: int64(i), Text: "line", At: 1000})
	}
	s.Append("side-room", protocol.Segment{Seq: 1, Text: "elsewhere"})

	got, err := s.Load("main-hall")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("loaded %d segments, want 3", len(got))
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// A fresh store over the same directory sees everything.
	reopened, err := NewFile(dir, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	rooms, err := reopened.Rooms()
	if err != nil {
		t.Fatal(err)
	}
	if len(rooms) != 2 {
		t.Fatalf("rooms = %v, want two", rooms)
	}
	again, err := reopened.Load("main-hall")
	if err != nil || len(again) != 3 {
		t.Fatalf("reload: %d segments, err %v", len(again), err)
	}
}

func TestFileStoreRejectsSillyRoomIDs(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFile(dir, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.Append("../../etc/passwd", protocol.Segment{Text: "nope"})
	if _, err := s.Load("../../etc/passwd"); err == nil {
		t.Fatal("expected an invalid room id to be rejected")
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*"))
	if len(matches) != 0 {
		t.Fatalf("no files should have been created, found %v", matches)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestFileStoreReportsWriteFailures(t *testing.T) {
	dir := t.TempDir()

	var logged strings.Builder
	log := slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelError}))
	s, err := NewFile(dir, log)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Make the room's file impossible to create by putting a directory in its
	// place: the transcript must keep flowing, but somebody has to be told.
	if err := os.Mkdir(filepath.Join(dir, "doomed.jsonl"), 0o750); err != nil {
		t.Fatal(err)
	}
	s.Append("doomed", protocol.Segment{Seq: 1, Text: "into the void"})

	if !strings.Contains(logged.String(), "transcript storage is failing") {
		t.Fatalf("a storage failure must be reported, got: %q", logged.String())
	}

	// A second failure must not repeat the complaint.
	before := logged.Len()
	s.Append("doomed", protocol.Segment{Seq: 2, Text: "still nowhere"})
	if logged.Len() != before {
		t.Error("the failure should be reported once, not on every segment")
	}
}
