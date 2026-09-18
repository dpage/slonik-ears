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

// TestArchiveIsAtomicAgainstAConcurrentAppend is the concurrent version of
// TestAppendAfterArchiveStartsAFreshFile. Closing the writer is not enough on
// its own: a segment arriving between the close and the rename reopens the
// file that is about to be moved, so the descriptor follows it into the
// archive and every later segment is written there instead.
//
// The observable symptom is that the live transcript never comes back. A
// speaker still finishing a sentence when the organiser presses Reset is
// enough to trigger it.
func TestArchiveIsAtomicAgainstAConcurrentAppend(t *testing.T) {
	const marker = "after the reset"

	for attempt := range 200 {
		dir := t.TempDir()
		s, err := NewFile(dir, testLogger())
		if err != nil {
			t.Fatal(err)
		}

		s.Append("main-hall", seg(1, "the first talk"))

		// One goroutine keeps publishing while the other archives, which is
		// what a reset mid-sentence looks like.
		done := make(chan struct{})
		go func() {
			defer close(done)
			for i := range 50 {
				s.Append("main-hall", seg(int64(i+2), "still talking"))
			}
		}()
		if err := s.Archive("main-hall"); err != nil {
			t.Fatal(err)
		}
		<-done

		// Archive has returned and the appender has finished, so this one
		// lands unambiguously after the rename.
		s.Append("main-hall", seg(999, marker))
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}

		// Whatever landed either side of the rename, the segments written
		// after it must be in the live file and not in the archive.
		live, err := os.ReadFile(filepath.Join(dir, "main-hall.jsonl"))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		archives, err := filepath.Glob(filepath.Join(dir, ArchiveDir, "main-hall-*.jsonl"))
		if err != nil || len(archives) == 0 {
			t.Fatalf("attempt %d: expected an archived file, got %v (%v)", attempt, archives, err)
		}
		archived, err := os.ReadFile(archives[0])
		if err != nil {
			t.Fatal(err)
		}

		lines := strings.Count(string(live), "\n")
		archivedLines := strings.Count(string(archived), "\n")
		if lines+archivedLines != 52 {
			t.Fatalf("attempt %d: %d segments survived, expected 52 (%d live, %d archived)",
				attempt, lines+archivedLines, lines, archivedLines)
		}
		// The segment appended after Archive returned must be in the live
		// file. Asserting it this way rather than on the concurrent writes is
		// deliberate: the appender may legitimately get all fifty in before
		// the rename, which leaves the live file empty through no fault of
		// the code, and a test that reads that as a failure fails at random
		// on a loaded machine. Once Archive has returned the rename has
		// definitely happened, so where the next segment lands is a fact
		// about the implementation rather than about the scheduler — and it
		// is exactly what the descriptor following the rename would break.
		if !strings.Contains(string(live), marker) {
			t.Fatalf("attempt %d: a segment appended after the reset did not reach the "+
				"live transcript (%d live, %d archived); the writer followed the rename "+
				"into the archive", attempt, lines, archivedLines)
		}
		if strings.Contains(string(archived), marker) {
			t.Fatalf("attempt %d: a segment appended after the reset was written into "+
				"the archive", attempt)
		}
	}
}
