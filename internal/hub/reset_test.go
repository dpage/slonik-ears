package hub_test

import (
	"testing"

	"github.com/dpage/slonik-ears/internal/hub"
	"github.com/dpage/slonik-ears/internal/protocol"
)

func TestResetEmptiesTheRoomAndKeepsNumberingClimbing(t *testing.T) {
	h := hub.New(hub.Options{History: 100})
	room := h.Ensure("main-hall")
	epoch := room.AttachPublisher(protocol.Room{Title: "Main Hall"})

	if _, err := room.AddSegment(epoch, protocol.Segment{Text: "the first talk"}); err != nil {
		t.Fatal(err)
	}
	room.Reset()

	// The transcript goes, but the numbering does not go back to the start:
	// a viewer that was away across the reset has to be able to tell the new
	// talk's segment 1 from the old talk's segment 1, and it only has the
	// number to go on.
	if got := room.Info().Cursor; got != 1 {
		t.Errorf("cursor after reset is %d, want it left where the last talk ended", got)
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
	if seg.Seq != 2 {
		t.Errorf("the next talk was numbered from %d, want the numbering to carry on at 2", seg.Seq)
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
	if msg.Cursor != 5 {
		t.Errorf("snapshot cursor is %d, want the reset to have left it at 5", msg.Cursor)
	}
	if len(msg.Segments) != 0 {
		t.Errorf("the reset snapshot carried %d segments of the old talk", len(msg.Segments))
	}
}

func TestAViewerReturningLateIntoTheNextTalkIsStillToldToStartOver(t *testing.T) {
	// The case the old cursor test missed. A phone sleeps at segment 5 of the
	// morning talk; the room is reset; the afternoon speaker gets past five
	// segments before the phone wakes. Judging "has this been reset" by
	// comparing the viewer's position with the room's cursor said no, because
	// the new talk had already climbed past it, and the reader ended up with
	// the two talks run together and the second one missing its opening.
	h := hub.New(hub.Options{History: 100})
	room := h.Ensure("main-hall")
	epoch := room.AttachPublisher(protocol.Room{})
	for range 5 {
		if _, err := room.AddSegment(epoch, protocol.Segment{Text: "the morning talk"}); err != nil {
			t.Fatal(err)
		}
	}
	room.Reset()
	for range 8 {
		if _, err := room.AddSegment(epoch, protocol.Segment{Text: "the afternoon talk"}); err != nil {
			t.Fatal(err)
		}
	}

	sub := room.Subscribe(5) // where the phone got to before the break
	defer sub.Close()

	msg := <-sub.C()
	if !msg.Reset {
		t.Fatal("a viewer resuming into the middle of the next talk was not told to start over")
	}
	if len(msg.Segments) != 8 {
		t.Errorf("the viewer was sent %d segments, want the whole of the new talk", len(msg.Segments))
	}
}

func TestAViewerBehindTheRetainedHistoryIsToldToStartOver(t *testing.T) {
	// Same failure, different cause: on a long talk the history is trimmed
	// past where the viewer got to, so the segments in between are gone for
	// good. Sending what is left would leave a silent hole in the middle of
	// the transcript with nothing to mark it.
	h := hub.New(hub.Options{History: 5})
	room := h.Ensure("main-hall")
	epoch := room.AttachPublisher(protocol.Room{})
	for range 20 {
		if _, err := room.AddSegment(epoch, protocol.Segment{Text: "a long talk"}); err != nil {
			t.Fatal(err)
		}
	}

	sub := room.Subscribe(2)
	defer sub.Close()

	msg := <-sub.C()
	if !msg.Reset {
		t.Error("a viewer behind the retained history was not told to start over")
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

func TestResetKeepsTheRoomLookingLive(t *testing.T) {
	// Live and Viewers are computed rather than stored, so a snapshot built
	// from a bare copy of the room's info claims nothing is publishing and
	// nobody is watching. A viewer receiving that carries on receiving
	// transcript whilst its badge reads "waiting for room", directly above the
	// words the speaker is saying.
	h := hub.New(hub.Options{History: 10})
	room := h.Ensure("main-hall")
	epoch := room.AttachPublisher(protocol.Room{Title: "Main Hall"})

	sub := room.Subscribe(0)
	defer sub.Close()
	<-sub.C() // the snapshot sent on subscribing

	room.Reset()

	msg := <-sub.C()
	if msg.Room == nil {
		t.Fatal("the reset snapshot carried no room")
	}
	if !msg.Room.Live {
		t.Error("the room was reported as not live whilst a listener was connected")
	}
	if msg.Room.Viewers != 1 {
		t.Errorf("viewers reported as %d, want 1", msg.Room.Viewers)
	}

	// And it really is still live: the listener carries on publishing.
	if _, err := room.AddSegment(epoch, protocol.Segment{Text: "still here"}); err != nil {
		t.Fatalf("the publisher was locked out: %v", err)
	}
}
