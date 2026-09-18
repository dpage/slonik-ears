package hub_test

import (
	"testing"

	"github.com/dpage/slonik-ears/internal/hub"
	"github.com/dpage/slonik-ears/internal/protocol"
)

func TestResetEmptiesTheRoomAndRestartsNumbering(t *testing.T) {
	h := hub.New(hub.Options{History: 100})
	room := h.Ensure("main-hall")
	epoch := room.AttachPublisher(protocol.Room{Title: "Main Hall"})

	if _, err := room.AddSegment(epoch, protocol.Segment{Text: "the first talk"}); err != nil {
		t.Fatal(err)
	}
	room.Reset()

	if got := room.Info().Cursor; got != 0 {
		t.Errorf("cursor after reset is %d, want 0", got)
	}
	if got := room.All(); len(got) != 0 {
		t.Errorf("the transcript survived the reset: %v", got)
	}

	// The listener at the back of the room is still connected and carries
	// straight on into the next talk: the epoch is deliberately untouched, so
	// nobody has to go and restart anything between speakers.
	seg, err := room.AddSegment(epoch, protocol.Segment{Text: "the second talk"})
	if err != nil {
		t.Fatalf("the publisher was locked out by the reset: %v", err)
	}
	if seg.Seq != 1 {
		t.Errorf("numbering restarted at %d, want 1", seg.Seq)
	}
}

func TestAViewerReturningAfterAResetIsToldToStartOver(t *testing.T) {
	// A phone that was in somebody's pocket throughout the reset comes back
	// asking to resume from a cursor the room no longer has. Without being
	// told, it would hold the previous speaker's words on screen and discard
	// the new talk as segments it had already seen.
	h := hub.New(hub.Options{History: 100})
	room := h.Ensure("main-hall")
	epoch := room.AttachPublisher(protocol.Room{})
	for i := 0; i < 5; i++ {
		if _, err := room.AddSegment(epoch, protocol.Segment{Text: "a line"}); err != nil {
			t.Fatal(err)
		}
	}
	room.Reset()

	sub := room.Subscribe(5) // resuming from where the last talk got to
	defer sub.Close()

	msg := <-sub.C()
	if msg.Type != protocol.TypeSnapshot {
		t.Fatalf("first message is %q, want a snapshot", msg.Type)
	}
	if !msg.Reset {
		t.Error("the snapshot did not tell the viewer to start over")
	}
	if msg.Cursor != 0 {
		t.Errorf("snapshot cursor is %d, want 0", msg.Cursor)
	}
}

func TestAnOrdinaryResumeIsNotTreatedAsAReset(t *testing.T) {
	// The guard has to be narrow: a viewer resuming normally, from a cursor
	// the room still holds, must not have its transcript thrown away.
	h := hub.New(hub.Options{History: 100})
	room := h.Ensure("main-hall")
	epoch := room.AttachPublisher(protocol.Room{})
	for i := 0; i < 5; i++ {
		if _, err := room.AddSegment(epoch, protocol.Segment{Text: "a line"}); err != nil {
			t.Fatal(err)
		}
	}

	for _, since := range []int64{0, 3, 5} {
		sub := room.Subscribe(since)
		msg := <-sub.C()
		if msg.Reset {
			t.Errorf("resuming from %d was treated as a reset", since)
		}
		sub.Close()
	}
}

func TestAnOperatorsRetitleSurvivesAListenerReconnecting(t *testing.T) {
	// A listener repeats its --title and --speaker in every hello. Retitling a
	// room for the next speaker used to last only until the machine at the back
	// of the room reconnected, at which point it silently reverted.
	h := hub.New(hub.Options{History: 10})
	room := h.Ensure("main-hall")
	room.AttachPublisher(protocol.Room{Title: "Main Hall", Speaker: "First Speaker", Track: "Track A"})

	room.SetOperatorMetadata(protocol.Room{Title: "Keynote", Speaker: "Second Speaker"})

	// The listener drops and comes back, still configured with the old flags.
	room.AttachPublisher(protocol.Room{Title: "Main Hall", Speaker: "First Speaker", Track: "Track A"})

	got := room.Info()
	if got.Title != "Keynote" {
		t.Errorf("title reverted to %q", got.Title)
	}
	if got.Speaker != "Second Speaker" {
		t.Errorf("speaker reverted to %q", got.Speaker)
	}
	// Track was never touched by the operator, so the listener still owns it.
	if got.Track != "Track A" {
		t.Errorf("track is %q, want the listener's value", got.Track)
	}
}

func TestAListenerStillNamesARoomNobodyHasEdited(t *testing.T) {
	h := hub.New(hub.Options{History: 10})
	room := h.Ensure("main-hall")
	room.AttachPublisher(protocol.Room{Title: "Main Hall", Speaker: "First Speaker"})
	if got := room.Info(); got.Title != "Main Hall" || got.Speaker != "First Speaker" {
		t.Fatalf("a listener should name an unedited room: %+v", got)
	}
	// And a later listener with different flags still wins, until a person
	// intervenes.
	room.AttachPublisher(protocol.Room{Title: "Second Hall", Speaker: "Someone Else"})
	if got := room.Info(); got.Title != "Second Hall" || got.Speaker != "Someone Else" {
		t.Errorf("an unpinned room should follow its listener: %+v", got)
	}
}
