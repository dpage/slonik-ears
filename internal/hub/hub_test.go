package hub

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dpage/slonik-ears/internal/protocol"
)

func recv(t *testing.T, sub *Subscriber) protocol.Message {
	t.Helper()
	select {
	case m, ok := <-sub.C():
		if !ok {
			t.Fatal("subscriber channel closed unexpectedly")
		}
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a message")
		return protocol.Message{}
	}
}

func TestSubscriberGetsSnapshotThenUpdates(t *testing.T) {
	h := New(Options{})
	room := h.Ensure("main-hall")
	epoch := room.AttachPublisher(protocol.Room{Title: "Main Hall"})

	if _, err := room.AddSegment(epoch, protocol.Segment{Text: "before you joined"}); err != nil {
		t.Fatalf("AddSegment: %v", err)
	}

	sub := room.Subscribe(0)
	defer sub.Close()

	snap := recv(t, sub)
	if snap.Type != protocol.TypeSnapshot {
		t.Fatalf("first message was %q, want a snapshot", snap.Type)
	}
	if len(snap.Segments) != 1 || snap.Segments[0].Text != "before you joined" {
		t.Fatalf("snapshot did not carry history: %+v", snap.Segments)
	}
	if snap.Room == nil || snap.Room.Title != "Main Hall" {
		t.Fatalf("snapshot did not carry room metadata: %+v", snap.Room)
	}

	if _, err := room.AddSegment(epoch, protocol.Segment{Text: "and this is live"}); err != nil {
		t.Fatalf("AddSegment: %v", err)
	}
	live := recv(t, sub)
	if live.Type != protocol.TypeFinal || live.Segment.Text != "and this is live" {
		t.Fatalf("unexpected live message: %+v", live)
	}
	if live.Segment.Seq != 2 {
		t.Fatalf("sequence numbers should be assigned by the hub, got %d", live.Segment.Seq)
	}
}

func TestResumeFromCursorSkipsWhatTheViewerHas(t *testing.T) {
	h := New(Options{})
	room := h.Ensure("hall")
	epoch := room.AttachPublisher(protocol.Room{})
	for _, text := range []string{"one", "two", "three"} {
		if _, err := room.AddSegment(epoch, protocol.Segment{Text: text}); err != nil {
			t.Fatal(err)
		}
	}

	sub := room.Subscribe(2)
	defer sub.Close()
	snap := recv(t, sub)
	if len(snap.Segments) != 1 || snap.Segments[0].Text != "three" {
		t.Fatalf("resume replayed the wrong segments: %+v", snap.Segments)
	}
	if snap.Cursor != 3 {
		t.Fatalf("cursor = %d, want 3", snap.Cursor)
	}
}

func TestPartialIsClearedByFinal(t *testing.T) {
	h := New(Options{})
	room := h.Ensure("hall")
	epoch := room.AttachPublisher(protocol.Room{})

	if err := room.SetPartial(epoch, protocol.Partial{Text: "half a thou"}); err != nil {
		t.Fatal(err)
	}
	sub := room.Subscribe(0)
	defer sub.Close()
	snap := recv(t, sub)
	if snap.Partial == nil || snap.Partial.Text != "half a thou" {
		t.Fatalf("snapshot should include the in-flight partial: %+v", snap.Partial)
	}

	if _, err := room.AddSegment(epoch, protocol.Segment{Text: "half a thought"}); err != nil {
		t.Fatal(err)
	}
	recv(t, sub) // the final

	_, partial, _ := room.History(0)
	if partial != nil {
		t.Fatalf("committing a segment should clear the partial, got %+v", partial)
	}
}

func TestStalePublisherIsRejected(t *testing.T) {
	h := New(Options{})
	room := h.Ensure("hall")
	first := room.AttachPublisher(protocol.Room{})
	second := room.AttachPublisher(protocol.Room{}) // a second listener takes over

	if _, err := room.AddSegment(first, protocol.Segment{Text: "from the old listener"}); err == nil {
		t.Fatal("the superseded listener should be refused")
	}
	if _, err := room.AddSegment(second, protocol.Segment{Text: "from the new one"}); err != nil {
		t.Fatalf("the current listener should be accepted: %v", err)
	}
}

func TestSlowSubscriberIsDropped(t *testing.T) {
	h := New(Options{SubscriberQueue: 4})
	room := h.Ensure("hall")
	epoch := room.AttachPublisher(protocol.Room{})

	sub := room.Subscribe(0) // never read from; the queue will fill
	for i := 0; i < 50; i++ {
		_, _ = room.AddSegment(epoch, protocol.Segment{Text: "filler"})
	}

	drained := 0
	for range sub.C() {
		drained++
	}
	if drained == 0 {
		t.Fatal("expected some messages before the subscriber was dropped")
	}
	if got := room.Info().Viewers; got != 0 {
		t.Fatalf("a dropped subscriber should be unsubscribed, viewers = %d", got)
	}
}

func TestLobbyReflectsRooms(t *testing.T) {
	h := New(Options{})
	sub := h.SubscribeLobby()
	defer sub.Close()
	if first := recv(t, sub); first.Type != protocol.TypeRooms {
		t.Fatalf("lobby should open with a room list, got %q", first.Type)
	}

	h.Ensure("new-room")
	update := recv(t, sub)
	found := false
	for _, r := range update.Rooms {
		if r.ID == "new-room" {
			found = true
		}
	}
	if !found {
		t.Fatalf("lobby was not told about the new room: %+v", update.Rooms)
	}
}

func TestSinkReceivesFinals(t *testing.T) {
	var got []protocol.Segment
	h := New(Options{Sink: sinkFn(func(_ string, s protocol.Segment) { got = append(got, s) })})
	room := h.Ensure("hall")
	epoch := room.AttachPublisher(protocol.Room{})
	_, _ = room.AddSegment(epoch, protocol.Segment{Text: "persist me"})

	if len(got) != 1 || got[0].Text != "persist me" {
		t.Fatalf("sink got %+v", got)
	}
}

type sinkFn func(string, protocol.Segment)

func (f sinkFn) Append(room string, seg protocol.Segment) { f(room, seg) }

// TestRemovedRoomRefusesItsPublisher guards the case where an organiser takes
// a room out of the lobby whilst the machine at the back of that room is still
// running. The publisher holds its *Room directly, so without a check it
// carries on committing segments to a room nobody can see, and the sink
// recreates the transcript file that removing the room had just archived.
func TestRemovedRoomRefusesItsPublisher(t *testing.T) {
	sink := &countingSink{}
	h := New(Options{History: 100, Sink: sink})
	room := h.Ensure("main-hall")
	epoch := room.AttachPublisher(protocol.Room{Title: "Main Hall"})

	if _, err := room.AddSegment(epoch, protocol.Segment{Text: "before"}); err != nil {
		t.Fatalf("publishing to a live room failed: %v", err)
	}
	before := sink.count()

	if !h.Remove("main-hall") {
		t.Fatal("the room was not removed")
	}

	_, err := room.AddSegment(epoch, protocol.Segment{Text: "after"})
	if !errors.Is(err, ErrRoomRemoved) {
		t.Errorf("publishing to a removed room returned %v, want ErrRoomRemoved", err)
	}
	if err := room.SetPartial(epoch, protocol.Partial{Text: "still going"}); !errors.Is(err, ErrRoomRemoved) {
		t.Errorf("a partial to a removed room returned %v, want ErrRoomRemoved", err)
	}
	if got := sink.count(); got != before {
		t.Errorf("a removed room wrote %d more segments to storage", got-before)
	}
}

type countingSink struct {
	mu sync.Mutex
	n  int
}

func (c *countingSink) Append(string, protocol.Segment) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
}

func (c *countingSink) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}
