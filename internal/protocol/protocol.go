// Package protocol defines the JSON messages exchanged between the listener
// (publisher), the server and the browser (viewer).
//
// Both directions use the same envelope so the TypeScript client only needs a
// single discriminated union. Everything is JSON over a WebSocket; timestamps
// are Unix milliseconds UTC.
package protocol

// Version is the wire protocol version. It is bumped when an incompatible
// change is made, so that an old listener talking to a new server fails
// loudly instead of subtly.
const Version = 1

// Message types sent by a listener to the server.
const (
	TypeHello   = "hello"   // listener announces the room it is publishing
	TypePartial = "partial" // interim hypothesis, replaces the previous partial
	TypeFinal   = "final"   // committed segment, never revised
	TypeStatus  = "status"  // listener health (audio level, backend state)
	TypeBye     = "bye"     // listener is shutting down cleanly
)

// Message types sent by the server to a viewer.
const (
	TypeSnapshot  = "snapshot"   // full(ish) state on connect or resume
	TypeRoomState = "room_state" // room metadata / liveness changed
	TypeRooms     = "rooms"      // lobby: the list of known rooms
	TypeError     = "error"      // fatal for this connection
	TypeAck       = "ack"        // server accepted a hello
)

// Message is the single envelope used in both directions. Unused fields are
// omitted so the wire stays small enough for a phone on venue Wi-Fi.
type Message struct {
	Type    string `json:"type"`
	Version int    `json:"version,omitempty"`

	Room     *Room     `json:"room,omitempty"`
	Rooms    []Room    `json:"rooms,omitempty"`
	Segment  *Segment  `json:"segment,omitempty"`
	Segments []Segment `json:"segments,omitempty"`
	Partial  *Partial  `json:"partial,omitempty"`
	Status   *Status   `json:"status,omitempty"`

	// Cursor is the highest segment sequence number the server holds for the
	// room. Viewers send it back on reconnect to resume without replaying the
	// whole talk.
	Cursor int64 `json:"cursor,omitempty"`

	Error      string `json:"error,omitempty"`
	ServerTime int64  `json:"serverTime,omitempty"`
}

// Room is the public description of a track/room.
type Room struct {
	ID       string `json:"id"`
	Title    string `json:"title,omitempty"`
	Track    string `json:"track,omitempty"`
	Speaker  string `json:"speaker,omitempty"`
	Language string `json:"language,omitempty"`

	// Live is true while a listener is connected and publishing.
	Live bool `json:"live"`
	// Viewers is the current number of connected browsers.
	Viewers int `json:"viewers"`
	// Cursor is the latest segment sequence number in the room.
	Cursor int64 `json:"cursor"`
	// StartedAt is when the current listener session began.
	StartedAt int64 `json:"startedAt,omitempty"`
	// LastActivity is the time of the most recent transcript update.
	LastActivity int64 `json:"lastActivity,omitempty"`
}

// Segment is a committed piece of transcript. Once published with a given Seq
// its text never changes, which is what lets viewers resume from a cursor.
type Segment struct {
	Seq  int64  `json:"seq"`
	ID   string `json:"id"`
	Text string `json:"text"`
	// StartMs/EndMs are offsets from the start of the listener session.
	StartMs int64 `json:"startMs"`
	EndMs   int64 `json:"endMs"`
	// At is the wall-clock time the segment was committed.
	At       int64  `json:"at"`
	Language string `json:"language,omitempty"`
}

// Partial is the in-flight hypothesis for the utterance currently being
// spoken. It is replaced wholesale by the next partial, and superseded by the
// Segment whose Seq is AfterSeq+1.
type Partial struct {
	Text     string `json:"text"`
	AfterSeq int64  `json:"afterSeq"`
	At       int64  `json:"at"`
}

// Status carries listener telemetry so an organiser can see at a glance that a
// room is actually hearing something.
type Status struct {
	// Level is the recent peak audio level, 0..1.
	Level float64 `json:"level"`
	// Listening is false when capture has stopped or the ASR backend is down.
	Listening bool `json:"listening"`
	// Backend names the transcription backend in use, e.g. "whisper-server".
	Backend string `json:"backend,omitempty"`
	// Detail is a human readable note, usually the last error.
	Detail string `json:"detail,omitempty"`
	// QueuedSegments is how many finals are buffered locally awaiting upload.
	QueuedSegments int `json:"queuedSegments,omitempty"`
}
