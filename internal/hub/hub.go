// Package hub keeps the live state of every room: the committed transcript,
// the in-flight partial, the publishing listener and the subscribed viewers.
//
// It is transport agnostic — the HTTP layer owns the WebSockets, the hub only
// deals in protocol messages.
package hub

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/dpage/slonik-ears/internal/protocol"
)

// Sink receives finalised segments so they can be persisted. It must not
// block for long; the hub calls it while holding no locks.
type Sink interface {
	Append(roomID string, seg protocol.Segment)
}

// Options configures a Hub.
type Options struct {
	// History is how many finalised segments to keep in memory per room for
	// late joiners and reconnects. Roughly 2000 segments is a full day of
	// talking; each is small.
	History int
	// SubscriberQueue is the per-viewer outbound buffer. A viewer that cannot
	// keep up is disconnected and expected to reconnect with its cursor.
	SubscriberQueue int
	// Sink, if set, is notified of every finalised segment.
	Sink Sink
}

func (o Options) withDefaults() Options {
	if o.History <= 0 {
		o.History = 2000
	}
	if o.SubscriberQueue <= 0 {
		o.SubscriberQueue = 256
	}
	return o
}

// Hub owns every room.
type Hub struct {
	opts Options

	mu    sync.RWMutex
	rooms map[string]*Room

	// lobbyMu guards lobby subscribers, which receive room list updates.
	lobbyMu sync.RWMutex
	lobby   map[*Subscriber]struct{}
}

// New returns an empty hub.
func New(opts Options) *Hub {
	return &Hub{
		opts:  opts.withDefaults(),
		rooms: make(map[string]*Room),
		lobby: make(map[*Subscriber]struct{}),
	}
}

// Room is the live state of a single track.
type Room struct {
	hub *Hub

	mu       sync.RWMutex
	info     protocol.Room
	segments []protocol.Segment // ring, oldest first
	partial  *protocol.Partial
	status   *protocol.Status
	subs     map[*Subscriber]struct{}
	// publisherEpoch increments each time a listener takes over the room, so a
	// stale listener's writes can be rejected.
	publisherEpoch int64
	publisherConn  bool
	// pinned* records that a person set this field through the admin API, so
	// that a reconnecting listener repeating its start-up flags cannot quietly
	// undo them.
	pinnedTitle   bool
	pinnedTrack   bool
	pinnedSpeaker bool
}

// Subscriber is a viewer's outbound message queue.
type Subscriber struct {
	ch     chan protocol.Message
	closed bool
	mu     sync.Mutex
	room   *Room
	hub    *Hub
}

// C is the channel of messages to write to the client.
func (s *Subscriber) C() <-chan protocol.Message { return s.ch }

func (s *Subscriber) send(m protocol.Message) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	select {
	case s.ch <- m:
		return true
	default:
		// Slow consumer: drop it. The client reconnects with ?since= and
		// catches up from history, which is cheaper than unbounded buffering.
		s.closed = true
		close(s.ch)
		return false
	}
}

// Close detaches the subscriber from its room or the lobby.
func (s *Subscriber) Close() {
	if s.room != nil {
		s.room.unsubscribe(s)
		return
	}
	if s.hub != nil {
		s.hub.unsubscribeLobby(s)
	}
}

func (s *Subscriber) shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.ch)
	}
}

// ---------------------------------------------------------------- rooms

// Ensure returns the room with the given id, creating it if necessary.
func (h *Hub) Ensure(id string) *Room {
	h.mu.Lock()
	r, ok := h.rooms[id]
	if !ok {
		r = &Room{
			hub:  h,
			info: protocol.Room{ID: id},
			subs: make(map[*Subscriber]struct{}),
		}
		h.rooms[id] = r
	}
	h.mu.Unlock()
	if !ok {
		h.broadcastLobby()
	}
	return r
}

// Get returns a room without creating it.
func (h *Hub) Get(id string) (*Room, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	r, ok := h.rooms[id]
	return r, ok
}

// Remove drops a room entirely, disconnecting its viewers.
func (h *Hub) Remove(id string) bool {
	h.mu.Lock()
	r, ok := h.rooms[id]
	if ok {
		delete(h.rooms, id)
	}
	h.mu.Unlock()
	if !ok {
		return false
	}
	r.mu.Lock()
	subs := make([]*Subscriber, 0, len(r.subs))
	for s := range r.subs {
		subs = append(subs, s)
	}
	r.subs = make(map[*Subscriber]struct{})
	r.mu.Unlock()
	for _, s := range subs {
		s.shutdown()
	}
	h.broadcastLobby()
	return true
}

// Rooms returns a snapshot of every room, ordered by track then id so the
// lobby does not jump around between refreshes.
func (h *Hub) Rooms() []protocol.Room {
	h.mu.RLock()
	rooms := make([]*Room, 0, len(h.rooms))
	for _, r := range h.rooms {
		rooms = append(rooms, r)
	}
	h.mu.RUnlock()

	out := make([]protocol.Room, 0, len(rooms))
	for _, r := range rooms {
		out = append(out, r.Info())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Live != out[j].Live {
			return out[i].Live // live rooms first
		}
		if out[i].Track != out[j].Track {
			return out[i].Track < out[j].Track
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// ---------------------------------------------------------------- lobby

// SubscribeLobby returns a subscriber that receives room list updates.
func (h *Hub) SubscribeLobby() *Subscriber {
	s := &Subscriber{ch: make(chan protocol.Message, h.opts.SubscriberQueue), hub: h}
	h.lobbyMu.Lock()
	h.lobby[s] = struct{}{}
	h.lobbyMu.Unlock()
	s.send(protocol.Message{Type: protocol.TypeRooms, Rooms: h.Rooms(), ServerTime: nowMs()})
	return s
}

func (h *Hub) unsubscribeLobby(s *Subscriber) {
	h.lobbyMu.Lock()
	delete(h.lobby, s)
	h.lobbyMu.Unlock()
	s.shutdown()
}

func (h *Hub) broadcastLobby() {
	h.lobbyMu.RLock()
	if len(h.lobby) == 0 {
		h.lobbyMu.RUnlock()
		return
	}
	subs := make([]*Subscriber, 0, len(h.lobby))
	for s := range h.lobby {
		subs = append(subs, s)
	}
	h.lobbyMu.RUnlock()

	msg := protocol.Message{Type: protocol.TypeRooms, Rooms: h.Rooms(), ServerTime: nowMs()}
	var dead []*Subscriber
	for _, s := range subs {
		if !s.send(msg) {
			dead = append(dead, s)
		}
	}
	for _, s := range dead {
		h.unsubscribeLobby(s)
	}
}

// ---------------------------------------------------------------- room state

// Info returns a copy of the room's public metadata.
func (r *Room) Info() protocol.Room {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info := r.info
	info.Viewers = len(r.subs)
	info.Live = r.publisherConn
	return info
}

// Status returns the last listener telemetry, if any.
func (r *Room) Status() *protocol.Status {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.status == nil {
		return nil
	}
	s := *r.status
	return &s
}

// AttachPublisher marks the room live and returns an epoch token. A second
// listener connecting to the same room takes over: the previous one is
// considered stale and its writes are ignored.
func (r *Room) AttachPublisher(meta protocol.Room) int64 {
	r.mu.Lock()
	r.publisherEpoch++
	epoch := r.publisherEpoch
	r.publisherConn = true
	// A listener describes itself on every hello, which is how a room gets its
	// title in the first place. It must not, however, undo an organiser who has
	// since retitled the room for the next speaker: that person is holding the
	// more recent information, and the listener is only repeating the flags it
	// happened to be started with.
	if meta.Title != "" && !r.pinnedTitle {
		r.info.Title = meta.Title
	}
	if meta.Track != "" && !r.pinnedTrack {
		r.info.Track = meta.Track
	}
	if meta.Speaker != "" && !r.pinnedSpeaker {
		r.info.Speaker = meta.Speaker
	}
	if meta.Language != "" {
		r.info.Language = meta.Language
	}
	r.info.StartedAt = nowMs()
	r.info.LastActivity = nowMs()
	r.partial = nil
	r.mu.Unlock()

	r.broadcastState()
	r.hub.broadcastLobby()
	return epoch
}

// DetachPublisher marks the room no longer live if epoch is still current.
func (r *Room) DetachPublisher(epoch int64) {
	r.mu.Lock()
	if r.publisherEpoch != epoch {
		r.mu.Unlock()
		return
	}
	r.publisherConn = false
	r.partial = nil
	r.status = nil
	r.mu.Unlock()

	r.broadcastState()
	r.hub.broadcastLobby()
}

// SetMetadata updates the room description from config or the admin API.
func (r *Room) SetMetadata(meta protocol.Room) {
	r.setMetadata(meta, false)
}

// SetOperatorMetadata is the same, but records that a person set these fields
// deliberately, so a reconnecting listener does not undo their work.
//
// Without this the organiser's page was of very little use: a listener sends
// its --title and --speaker in every hello, so retitling a room for the next
// speaker lasted exactly until the machine at the back of the room reconnected
// — a restart, a dropped network, a lid closing — and then silently reverted
// to whatever that listener was started with.
func (r *Room) SetOperatorMetadata(meta protocol.Room) {
	r.setMetadata(meta, true)
}

func (r *Room) setMetadata(meta protocol.Room, byOperator bool) {
	r.mu.Lock()
	if meta.Title != "" {
		r.info.Title = meta.Title
		r.pinnedTitle = r.pinnedTitle || byOperator
	}
	if meta.Track != "" {
		r.info.Track = meta.Track
		r.pinnedTrack = r.pinnedTrack || byOperator
	}
	if meta.Speaker != "" {
		r.info.Speaker = meta.Speaker
		r.pinnedSpeaker = r.pinnedSpeaker || byOperator
	}
	if meta.Language != "" {
		r.info.Language = meta.Language
	}
	r.mu.Unlock()
	r.broadcastState()
	r.hub.broadcastLobby()
}

// Cursor is the sequence number of the most recent committed segment.
func (r *Room) Cursor() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.info.Cursor
}

// ---------------------------------------------------------------- transcript

// ErrStalePublisher is returned when a listener that has been superseded tries
// to publish.
var ErrStalePublisher = fmt.Errorf("publisher superseded by a newer listener")

// AddSegment commits a segment. The hub assigns the sequence number so that
// cursors are authoritative even across listener restarts.
func (r *Room) AddSegment(epoch int64, seg protocol.Segment) (protocol.Segment, error) {
	r.mu.Lock()
	if epoch != 0 && r.publisherEpoch != epoch {
		r.mu.Unlock()
		return protocol.Segment{}, ErrStalePublisher
	}
	r.info.Cursor++
	seg.Seq = r.info.Cursor
	if seg.At == 0 {
		seg.At = nowMs()
	}
	if seg.ID == "" {
		seg.ID = fmt.Sprintf("%s-%d", r.info.ID, seg.Seq)
	}
	if seg.Language == "" {
		seg.Language = r.info.Language
	}
	r.info.LastActivity = seg.At
	r.segments = append(r.segments, seg)
	if n := len(r.segments) - r.hub.opts.History; n > 0 {
		r.segments = append(r.segments[:0], r.segments[n:]...)
	}
	r.partial = nil
	r.mu.Unlock()

	r.broadcast(protocol.Message{Type: protocol.TypeFinal, Segment: &seg, Cursor: seg.Seq})
	if sink := r.hub.opts.Sink; sink != nil {
		sink.Append(r.info.ID, seg)
	}
	return seg, nil
}

// Restore seeds the room's history from persistent storage at startup. It does
// not broadcast and does not write back to the sink.
func (r *Room) Restore(segs []protocol.Segment) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.segments = append(r.segments, segs...)
	if n := len(r.segments) - r.hub.opts.History; n > 0 {
		r.segments = append(r.segments[:0], r.segments[n:]...)
	}
	for _, s := range segs {
		if s.Seq > r.info.Cursor {
			r.info.Cursor = s.Seq
		}
		if s.At > r.info.LastActivity {
			r.info.LastActivity = s.At
		}
	}
}

// Reset empties a room for the next talk: the transcript goes, the sequence
// numbering starts again from one, and any viewer still watching is brought
// back to an empty screen.
//
// The publisher's epoch is deliberately left alone, so a listener that is
// already connected carries straight on into the new talk without needing to
// be restarted. That is the case this exists for: the room turns around
// between speakers whilst the machine at the back of the room keeps running.
func (r *Room) Reset() {
	r.mu.Lock()
	r.segments = nil
	r.partial = nil
	r.info.Cursor = 0
	r.info.StartedAt = nowMs()
	r.info.LastActivity = nowMs()
	subs := make([]*Subscriber, 0, len(r.subs))
	for s := range r.subs {
		subs = append(subs, s)
	}
	// Live and Viewers are computed rather than stored, so a bare copy of
	// r.info reports a room that nobody is watching and nothing is publishing
	// to. Sent to a viewer that is quite happily receiving transcript, that
	// reads as "waiting for room" on a badge sitting directly above the words
	// arriving from the speaker.
	info := r.info
	info.Viewers = len(r.subs)
	info.Live = r.publisherConn
	r.mu.Unlock()

	// Viewers are sent a fresh snapshot rather than being disconnected: a
	// stage display should go blank and stay connected, not flicker through a
	// reconnection in front of an audience.
	for _, s := range subs {
		s.send(protocol.Message{
			Type:       protocol.TypeSnapshot,
			Version:    protocol.Version,
			Room:       &info,
			Reset:      true,
			Cursor:     0,
			ServerTime: nowMs(),
		})
	}
	r.hub.broadcastLobby()
}

// SetPartial publishes the in-flight hypothesis.
func (r *Room) SetPartial(epoch int64, p protocol.Partial) error {
	r.mu.Lock()
	if epoch != 0 && r.publisherEpoch != epoch {
		r.mu.Unlock()
		return ErrStalePublisher
	}
	if p.At == 0 {
		p.At = nowMs()
	}
	p.AfterSeq = r.info.Cursor
	r.partial = &p
	r.info.LastActivity = p.At
	r.mu.Unlock()

	r.broadcast(protocol.Message{Type: protocol.TypePartial, Partial: &p, Cursor: p.AfterSeq})
	return nil
}

// ClearPartial drops any in-flight hypothesis, e.g. when speech stops without
// producing a usable transcript.
func (r *Room) ClearPartial(epoch int64) {
	r.mu.Lock()
	if epoch != 0 && r.publisherEpoch != epoch {
		r.mu.Unlock()
		return
	}
	had := r.partial != nil
	r.partial = nil
	cursor := r.info.Cursor
	r.mu.Unlock()
	if had {
		empty := protocol.Partial{AfterSeq: cursor, At: nowMs()}
		r.broadcast(protocol.Message{Type: protocol.TypePartial, Partial: &empty, Cursor: cursor})
	}
}

// SetStatus records listener telemetry and forwards it to viewers.
func (r *Room) SetStatus(epoch int64, st protocol.Status) {
	r.mu.Lock()
	if epoch != 0 && r.publisherEpoch != epoch {
		r.mu.Unlock()
		return
	}
	r.status = &st
	r.mu.Unlock()
	r.broadcast(protocol.Message{Type: protocol.TypeStatus, Status: &st})
}

// History returns every retained segment with Seq > since, plus the current
// partial and cursor.
func (r *Room) History(since int64) ([]protocol.Segment, *protocol.Partial, int64) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []protocol.Segment
	for _, s := range r.segments {
		if s.Seq > since {
			out = append(out, s)
		}
	}
	var p *protocol.Partial
	if r.partial != nil {
		cp := *r.partial
		p = &cp
	}
	return out, p, r.info.Cursor
}

// All returns the complete retained transcript, for downloads.
func (r *Room) All() []protocol.Segment {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]protocol.Segment, len(r.segments))
	copy(out, r.segments)
	return out
}

// ---------------------------------------------------------------- viewers

// Subscribe attaches a viewer and primes it with a snapshot from since.
func (r *Room) Subscribe(since int64) *Subscriber {
	s := &Subscriber{ch: make(chan protocol.Message, r.hub.opts.SubscriberQueue), room: r}

	r.mu.Lock()
	r.subs[s] = struct{}{}
	r.mu.Unlock()

	segs, partial, cursor := r.History(since)
	info := r.Info()
	s.send(protocol.Message{
		Type:    protocol.TypeSnapshot,
		Version: protocol.Version,
		Room:    &info,
		// A viewer asking to resume from beyond where the room now is has
		// lived through a reset and is still holding the last talk. Tell it to
		// start over rather than merge. Deciding this from the cursor rather
		// than by announcing the reset means it self-heals: a phone that was
		// in somebody's pocket throughout gets the same treatment when it
		// eventually reconnects.
		Reset:      since > cursor,
		Segments:   segs,
		Partial:    partial,
		Cursor:     cursor,
		Status:     r.Status(),
		ServerTime: nowMs(),
	})
	r.hub.broadcastLobby()
	return s
}

func (r *Room) unsubscribe(s *Subscriber) {
	r.mu.Lock()
	delete(r.subs, s)
	r.mu.Unlock()
	s.shutdown()
	r.hub.broadcastLobby()
}

func (r *Room) broadcast(m protocol.Message) {
	r.mu.RLock()
	subs := make([]*Subscriber, 0, len(r.subs))
	for s := range r.subs {
		subs = append(subs, s)
	}
	r.mu.RUnlock()

	var dead []*Subscriber
	for _, s := range subs {
		if !s.send(m) {
			dead = append(dead, s)
		}
	}
	for _, s := range dead {
		r.unsubscribe(s)
	}
}

func (r *Room) broadcastState() {
	info := r.Info()
	r.broadcast(protocol.Message{Type: protocol.TypeRoomState, Room: &info, ServerTime: nowMs()})
}

func nowMs() int64 { return time.Now().UnixMilli() }
