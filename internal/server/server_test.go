package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dpage/slonik-ears/internal/protocol"
	"github.com/dpage/slonik-ears/internal/server"
	"github.com/dpage/slonik-ears/internal/store"
	"github.com/gorilla/websocket"
)

func newTestServer(t *testing.T, mutate func(*server.Config)) (*httptest.Server, *server.Server) {
	t.Helper()
	cfg := server.DefaultConfig()
	cfg.Auth.PublishToken = "publish-secret"
	cfg.Rooms = []server.RoomConfig{{ID: "main-hall", Title: "Main Hall", Track: "Track A"}}
	if mutate != nil {
		mutate(&cfg)
	}
	srv := server.New(cfg, store.Null{}, discardLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, srv
}

func wsURL(ts *httptest.Server, path string) string {
	return "ws" + strings.TrimPrefix(ts.URL, "http") + path
}

func dialPublisher(t *testing.T, ts *httptest.Server, room, token string) *websocket.Conn {
	t.Helper()
	h := http.Header{}
	if token != "" {
		h.Set("Authorization", "Bearer "+token)
	}
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL(ts, "/api/publish?room="+room), h)
	if err != nil {
		status := "no response"
		if resp != nil {
			status = resp.Status
		}
		t.Fatalf("publisher dial failed: %v (%s)", err, status)
	}
	t.Cleanup(func() { conn.Close() })

	if err := conn.WriteJSON(protocol.Message{
		Type:    protocol.TypeHello,
		Version: protocol.Version,
		Room:    &protocol.Room{Title: "Main Hall"},
	}); err != nil {
		t.Fatalf("hello: %v", err)
	}
	var ack protocol.Message
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("ack: %v", err)
	}
	if ack.Type != protocol.TypeAck {
		t.Fatalf("expected an ack, got %q (%s)", ack.Type, ack.Error)
	}
	return conn
}

func readMessage(t *testing.T, conn *websocket.Conn) protocol.Message {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var msg protocol.Message
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read: %v", err)
	}
	return msg
}

func TestPublishReachesViewer(t *testing.T) {
	ts, _ := newTestServer(t, nil)

	viewer, _, err := websocket.DefaultDialer.Dial(wsURL(ts, "/api/watch?room=main-hall"), nil)
	if err != nil {
		t.Fatalf("viewer dial: %v", err)
	}
	defer viewer.Close()
	if snap := readMessage(t, viewer); snap.Type != protocol.TypeSnapshot {
		t.Fatalf("first viewer message was %q", snap.Type)
	}

	pub := dialPublisher(t, ts, "main-hall", "publish-secret")

	// The viewer is told the room went live, then receives the transcript.
	if err := pub.WriteJSON(protocol.Message{
		Type:    protocol.TypeFinal,
		Segment: &protocol.Segment{Text: "good morning everybody", StartMs: 0, EndMs: 1500},
	}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		msg := readMessage(t, viewer)
		if msg.Type == protocol.TypeFinal {
			if msg.Segment.Text != "good morning everybody" {
				t.Fatalf("unexpected text %q", msg.Segment.Text)
			}
			if msg.Segment.Seq != 1 {
				t.Fatalf("seq = %d, want 1", msg.Segment.Seq)
			}
			return
		}
	}
	t.Fatal("the viewer never received the segment")
}

func TestPublishRequiresToken(t *testing.T) {
	ts, _ := newTestServer(t, nil)
	_, resp, err := websocket.DefaultDialer.Dial(wsURL(ts, "/api/publish?room=main-hall"), nil)
	if err == nil {
		t.Fatal("publishing without a token should be refused")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %v", resp)
	}
}

func TestPublishRejectsUnknownRoomWhenAutoRoomsDisabled(t *testing.T) {
	no := false
	ts, _ := newTestServer(t, func(c *server.Config) { c.Auth.AllowAutoRooms = &no })
	h := http.Header{"Authorization": {"Bearer publish-secret"}}
	_, resp, err := websocket.DefaultDialer.Dial(wsURL(ts, "/api/publish?room=side-room"), h)
	if err == nil {
		t.Fatal("expected the connection to be refused")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %v", resp)
	}
}

func TestAutoRoomIsCreatedOnPublish(t *testing.T) {
	ts, srv := newTestServer(t, nil)
	dialPublisher(t, ts, "side-room", "publish-secret")
	if _, ok := srv.Hub().Get("side-room"); !ok {
		t.Fatal("publishing to an unknown room should create it when auto rooms are enabled")
	}
}

func TestViewerPasscode(t *testing.T) {
	ts, _ := newTestServer(t, func(c *server.Config) { c.Auth.ViewerPasscode = "sesame" })

	resp, err := http.Get(ts.URL + "/api/rooms")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a passcode, got %d", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/api/rooms?k=sesame")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with the right passcode, got %d", resp.StatusCode)
	}
}

func TestTranscriptExport(t *testing.T) {
	ts, srv := newTestServer(t, nil)
	room := srv.Hub().Ensure("main-hall")
	epoch := room.AttachPublisher(protocol.Room{Title: "Main Hall"})
	for _, text := range []string{"first line", "second line"} {
		if _, err := room.AddSegment(epoch, protocol.Segment{Text: text, StartMs: 0, EndMs: 1000}); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct{ format, contains string }{
		{"txt", "first line"},
		{"srt", "00:00:00,000 --> "},
		{"vtt", "WEBVTT"},
		{"json", `"second line"`},
	} {
		resp, err := http.Get(ts.URL + "/api/rooms/main-hall/transcript?format=" + tc.format)
		if err != nil {
			t.Fatal(err)
		}
		body := readAll(t, resp)
		if !strings.Contains(body, tc.contains) {
			t.Errorf("%s export did not contain %q:\n%s", tc.format, tc.contains, body)
		}
	}
}

func TestRoomsEndpointListsConfiguredRooms(t *testing.T) {
	ts, _ := newTestServer(t, nil)
	resp, err := http.Get(ts.URL + "/api/rooms")
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Rooms []protocol.Room `json:"rooms"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(payload.Rooms) != 1 || payload.Rooms[0].ID != "main-hall" || payload.Rooms[0].Title != "Main Hall" {
		t.Fatalf("unexpected room list: %+v", payload.Rooms)
	}
}

func TestQRCodeIsRendered(t *testing.T) {
	ts, _ := newTestServer(t, nil)
	resp, err := http.Get(ts.URL + "/api/rooms/main-hall/qr.png")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content type %q", ct)
	}
}

func TestSecondListenerTakesOverRoom(t *testing.T) {
	ts, _ := newTestServer(t, nil)
	first := dialPublisher(t, ts, "main-hall", "publish-secret")
	dialPublisher(t, ts, "main-hall", "publish-secret")

	// The superseded listener is told so, rather than left publishing into
	// the void.
	_ = first.WriteJSON(protocol.Message{Type: protocol.TypeFinal, Segment: &protocol.Segment{Text: "am I still here?"}})
	_ = first.SetReadDeadline(time.Now().Add(5 * time.Second))
	var msg protocol.Message
	if err := first.ReadJSON(&msg); err != nil {
		t.Fatalf("expected an error message, got %v", err)
	}
	if msg.Type != protocol.TypeError {
		t.Fatalf("expected an error message, got %q", msg.Type)
	}
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String()
}

func TestPublicConfigUsesJSONNames(t *testing.T) {
	ts, _ := newTestServer(t, func(c *server.Config) {
		c.Event.Name = "PGConf"
		c.Event.Tagline = "Elephants, mostly"
	})
	resp, err := http.Get(ts.URL + "/api/config")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var payload struct {
		Event struct {
			Name    string `json:"name"`
			Tagline string `json:"tagline"`
		} `json:"event"`
		RequiresKey bool `json:"requiresKey"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	// The web app reads these names; Go's defaults would have exported
	// "Name" and broken it silently.
	if payload.Event.Name != "PGConf" || payload.Event.Tagline != "Elephants, mostly" {
		t.Fatalf("event config did not round trip: %+v", payload.Event)
	}
	if payload.RequiresKey {
		t.Fatal("requiresKey should be false when no passcode is configured")
	}
}
