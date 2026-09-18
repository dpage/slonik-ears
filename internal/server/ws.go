package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dpage/slonik-ears/internal/hub"
	"github.com/dpage/slonik-ears/internal/protocol"
	"github.com/gorilla/websocket"
)

const (
	// writeWait is how long a single frame may take to flush. Phones on a
	// congested venue network are slow, but not this slow.
	writeWait = 10 * time.Second
	// pongWait is how long we tolerate silence from a peer before assuming it
	// has wandered off.
	pongWait = 70 * time.Second
	// pingPeriod must be comfortably less than pongWait.
	pingPeriod = 25 * time.Second
	// maxPublishMessage caps an inbound listener frame.
	maxPublishMessage = 256 * 1024
)

func (s *Server) upgrader(compress bool) *websocket.Upgrader {
	return &websocket.Upgrader{
		ReadBufferSize:    4096,
		WriteBufferSize:   4096,
		EnableCompression: compress,
		CheckOrigin:       s.checkOrigin,
	}
}

// checkOrigin guards against cross-site WebSocket hijacking. Transcripts are
// public-ish, but publishing is not, so the same rule is applied to both:
// same-origin by default, plus any explicitly allowed origins. Requests with
// no Origin header (native clients such as the listener) are allowed.
func (s *Server) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	for _, a := range s.cfg.Auth.AllowedOrigins {
		if a == "*" {
			return true
		}
		if strings.EqualFold(a, origin) {
			return true
		}
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// ---------------------------------------------------------------- viewers

func (s *Server) handleWatch(w http.ResponseWriter, r *http.Request) {
	if !s.authoriseViewer(r) {
		writeError(w, http.StatusUnauthorized, "a passcode is required")
		return
	}
	roomID := r.URL.Query().Get("room")
	if !protocol.ValidRoomID(roomID) {
		writeError(w, http.StatusBadRequest, "invalid room id")
		return
	}
	room, ok := s.hub.Get(roomID)
	if !ok {
		writeError(w, http.StatusNotFound, "no such room")
		return
	}
	if max := s.cfg.Server.MaxViewersPerRoom; max > 0 && room.Info().Viewers >= max {
		writeError(w, http.StatusServiceUnavailable, "this room is at capacity")
		return
	}

	conn, err := s.upgrader(true).Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade has already written a response
	}
	conn.EnableWriteCompression(true)

	sub := room.Subscribe(queryInt64(r, "since"))
	defer sub.Close()
	s.pumpViewer(conn, sub)
}

func (s *Server) handleLobby(w http.ResponseWriter, r *http.Request) {
	if !s.authoriseViewer(r) {
		writeError(w, http.StatusUnauthorized, "a passcode is required")
		return
	}
	conn, err := s.upgrader(true).Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn.EnableWriteCompression(true)

	sub := s.hub.SubscribeLobby()
	defer sub.Close()
	s.pumpViewer(conn, sub)
}

// pumpViewer writes hub messages to a browser until either side gives up.
func (s *Server) pumpViewer(conn *websocket.Conn, sub *hub.Subscriber) {
	defer func() { _ = conn.Close() }()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn.SetReadLimit(4096)
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(pongWait))
		})
		for {
			// Viewers are read-only; this loop exists to notice disconnects and
			// to keep the pong handler running.
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-sub.C():
			if !ok {
				_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
				_ = conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "falling behind; please reconnect"))
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteJSON(msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

// ---------------------------------------------------------------- publisher

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	roomID := r.URL.Query().Get("room")
	if !protocol.ValidRoomID(roomID) {
		writeError(w, http.StatusBadRequest, "invalid room id")
		return
	}
	if !s.authorisePublisher(r, roomID) {
		s.log.Warn("rejected publisher", "room", roomID, "ip", s.clientIP(r))
		writeError(w, http.StatusUnauthorized, "a valid publish token is required")
		return
	}
	room, exists := s.hub.Get(roomID)
	if !exists {
		if !s.cfg.AutoRooms() {
			writeError(w, http.StatusForbidden, "this room is not configured and automatic rooms are disabled")
			return
		}
		room = s.hub.Ensure(roomID)
	}

	conn, err := s.upgrader(false).Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	conn.SetReadLimit(maxPublishMessage)

	ip := s.clientIP(r)
	s.log.Info("listener connected", "room", roomID, "ip", ip)

	// The first message must be a hello so we know what the room is called.
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	var hello protocol.Message
	if err := conn.ReadJSON(&hello); err != nil || hello.Type != protocol.TypeHello {
		_ = writeWS(conn, protocol.Message{Type: protocol.TypeError, Error: "expected a hello message"})
		return
	}
	if hello.Version != 0 && hello.Version != protocol.Version {
		_ = writeWS(conn, protocol.Message{
			Type:  protocol.TypeError,
			Error: "protocol version mismatch: upgrade the listener or the server so they match",
		})
		s.log.Warn("listener protocol mismatch", "room", roomID, "listener", hello.Version, "server", protocol.Version)
		return
	}
	meta := protocol.Room{}
	if hello.Room != nil {
		meta = *hello.Room
	}
	epoch := room.AttachPublisher(meta)
	defer room.DetachPublisher(epoch)

	info := room.Info()
	if err := writeWS(conn, protocol.Message{
		Type:       protocol.TypeAck,
		Version:    protocol.Version,
		Room:       &info,
		Cursor:     room.Cursor(),
		ServerTime: time.Now().UnixMilli(),
	}); err != nil {
		return
	}

	// Keepalive pings on a separate goroutine. WriteControl rather than
	// WriteMessage: gorilla allows only one concurrent writer, and the read
	// loop below replies to a rejected message on this same connection, so a
	// ping landing at that moment would interleave with it and emit a corrupt
	// frame. WriteControl is the one write method documented as safe to call
	// concurrently, which is exactly why it exists.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		t := time.NewTicker(pingPeriod)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
					return
				}
			case <-stop:
				return
			}
		}
	}()

	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		var msg protocol.Message
		if err := conn.ReadJSON(&msg); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				s.log.Info("listener disconnected", "room", roomID, "error", err)
			} else {
				s.log.Info("listener disconnected", "room", roomID)
			}
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))

		if err := s.applyPublished(room, epoch, msg); err != nil {
			if errors.Is(err, hub.ErrStalePublisher) {
				_ = writeWS(conn, protocol.Message{
					Type:  protocol.TypeError,
					Error: "another listener has taken over this room",
				})
				s.log.Warn("stale listener rejected", "room", roomID, "ip", ip)
				return
			}
			_ = writeWS(conn, protocol.Message{Type: protocol.TypeError, Error: err.Error()})
		}
		if msg.Type == protocol.TypeBye {
			return
		}
	}
}

func (s *Server) applyPublished(room *hub.Room, epoch int64, msg protocol.Message) error {
	switch msg.Type {
	case protocol.TypeFinal:
		if msg.Segment == nil || strings.TrimSpace(msg.Segment.Text) == "" {
			return nil
		}
		seg := *msg.Segment
		seg.Text = strings.TrimSpace(seg.Text)
		_, err := room.AddSegment(epoch, seg)
		return err
	case protocol.TypePartial:
		if msg.Partial == nil {
			return nil
		}
		return room.SetPartial(epoch, protocol.Partial{Text: strings.TrimSpace(msg.Partial.Text)})
	case protocol.TypeStatus:
		if msg.Status != nil {
			room.SetStatus(epoch, *msg.Status)
		}
		return nil
	case protocol.TypeHello:
		if msg.Room != nil {
			room.SetMetadata(*msg.Room)
		}
		return nil
	case protocol.TypeBye:
		room.ClearPartial(epoch)
		return nil
	default:
		return errors.New("unknown message type: " + msg.Type)
	}
}

func writeWS(conn *websocket.Conn, msg protocol.Message) error {
	_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}
