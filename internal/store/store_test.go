package store

import (
	"path/filepath"
	"testing"

	"github.com/dpage/slonik-ears/internal/protocol"
)

func TestFileStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFile(dir)
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
	reopened, err := NewFile(dir)
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
	s, err := NewFile(dir)
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
