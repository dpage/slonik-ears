package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dpage/slonik-ears/internal/protocol"
)

func seg(seq int64, text string) protocol.Segment {
	return protocol.Segment{Seq: seq, Text: text}
}

func TestArchiveLeavesTheRoomEmptyAndKeepsTheTranscript(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFile(dir, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	s.Append("main-hall", seg(1, "the first talk"))
	if err := s.Archive("main-hall"); err != nil {
		t.Fatal(err)
	}

	// The room now reads as never having had a transcript...
	got, err := s.Load("main-hall")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("the room still has %d segments after being archived", len(got))
	}
	if rooms, _ := s.Rooms(); len(rooms) != 0 {
		t.Errorf("an archived room is still listed: %v", rooms)
	}

	// ...but the talk itself is still on disk.
	archives, err := filepath.Glob(filepath.Join(dir, ArchiveDir, "main-hall-*.jsonl"))
	if err != nil || len(archives) != 1 {
		t.Fatalf("expected one archived file, got %v (%v)", archives, err)
	}
	body, err := os.ReadFile(archives[0])
	if err != nil {
		t.Fatal(err)
	}
	if want := "the first talk"; !contains(string(body), want) {
		t.Errorf("the archived transcript does not contain %q: %s", want, body)
	}
}

func TestAppendAfterArchiveStartsAFreshFile(t *testing.T) {
	// The failure this guards against: renaming a file does not move the open
	// file descriptor pointing at it, so without closing the writer first the
	// next talk is appended to the archive and the two are merged back
	// together at the next restart.
	dir := t.TempDir()
	s, err := NewFile(dir, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	s.Append("main-hall", seg(1, "the first talk"))
	if err := s.Archive("main-hall"); err != nil {
		t.Fatal(err)
	}
	s.Append("main-hall", seg(1, "the second talk"))

	got, err := s.Load("main-hall")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Text != "the second talk" {
		t.Fatalf("expected only the second talk, got %+v", got)
	}
}

func TestArchiveTwiceKeepsBothTranscripts(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFile(dir, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	for _, talk := range []string{"first", "second"} {
		s.Append("main-hall", seg(1, talk))
		if err := s.Archive("main-hall"); err != nil {
			t.Fatal(err)
		}
	}
	// Two archives in the same second must not overwrite one another, which
	// they would if the name were the timestamp alone.
	archives, _ := filepath.Glob(filepath.Join(dir, ArchiveDir, "*.jsonl"))
	if len(archives) != 2 {
		t.Fatalf("expected two archives, got %v", archives)
	}
}

func TestArchiveIsHappyWithNothingToDo(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFile(dir, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if err := s.Archive("never-used"); err != nil {
		t.Errorf("archiving a room with no transcript should be a no-op, got %v", err)
	}
	if err := s.Archive("Not A Valid Id"); err == nil {
		t.Error("an invalid room id should be refused")
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
