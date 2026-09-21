package listener

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dpage/slonik-ears/internal/protocol"
	"github.com/gorilla/websocket"
)

// TestAFatalRefusalStopsTheListener covers the two cases where reconnecting
// makes things worse rather than better. A listener told its room has been
// removed used to come back a second later and, with automatic rooms on,
// recreate the room an organiser had just taken out of the lobby. A listener
// told it had been superseded used to take the room back off the listener that
// superseded it, whereupon the pair of them fought over the room for the rest
// of the event.
func TestAFatalRefusalStopsTheListener(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
	}{
		{"removed room", "this room has been removed by an organiser"},
		{"superseded", "another listener has taken over this room"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var connections atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				connections.Add(1)
				conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer func() { _ = conn.Close() }()
				var hello protocol.Message
				if err := conn.ReadJSON(&hello); err != nil {
					return
				}
				_ = conn.WriteJSON(protocol.Message{
					Type:  protocol.TypeError,
					Error: tc.text,
					Fatal: true,
				})
			}))
			defer srv.Close()

			c, err := NewClient(ClientConfig{
				ServerURL: "http://" + srv.Listener.Addr().String(),
				Room:      "main-hall",
				Token:     "t",
				Logger:    testLogger(),
			})
			if err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			runErr := c.Run(ctx)

			if !errors.Is(runErr, ErrFatal) {
				t.Errorf("Run returned %v, want it to report ErrFatal and stop", runErr)
			}
			if ctx.Err() != nil {
				t.Error("Run kept reconnecting until the test timed out instead of giving up")
			}
			if got := connections.Load(); got != 1 {
				t.Errorf("the listener connected %d times; it should not have come back at all", got)
			}
		})
	}
}
