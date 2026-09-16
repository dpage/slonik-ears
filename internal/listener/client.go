// Package listener contains the room-side half of Slonik Ears: it captures
// audio, has it transcribed, and publishes the text to a server.
package listener

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dpage/slonik-ears/internal/protocol"
	"github.com/gorilla/websocket"
)

// ClientConfig configures the publishing WebSocket client.
type ClientConfig struct {
	// ServerURL is the base address of the server, e.g. http://localhost:8080
	// or wss://ears.example.org. http/https are rewritten to ws/wss.
	ServerURL string
	Room      string
	Token     string
	Meta      protocol.Room
	// MaxQueue is how many finalised segments to hold while disconnected.
	// At a few seconds per segment, the default is over an hour of talk.
	MaxQueue int
	Logger   *slog.Logger
}

// Client maintains a connection to the server, reconnecting as needed, and
// buffers finalised segments so that a flaky venue network costs latency
// rather than transcript.
type Client struct {
	cfg ClientConfig
	log *slog.Logger
	url string

	mu        sync.Mutex
	pending   []protocol.Message
	partial   *protocol.Partial
	status    *protocol.Status
	connected bool
	dropped   int

	notify chan struct{}
}

// NewClient validates the configuration and returns a client. Call Run to
// start it.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.ServerURL == "" {
		return nil, errors.New("listener: server URL is required")
	}
	if !protocol.ValidRoomID(cfg.Room) {
		return nil, fmt.Errorf("listener: %q is not a valid room id (lower case letters, digits, - and _)", cfg.Room)
	}
	if cfg.MaxQueue <= 0 {
		cfg.MaxQueue = 2000
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	u, err := url.Parse(cfg.ServerURL)
	if err != nil {
		return nil, fmt.Errorf("listener: bad server URL: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "ws":
		u.Scheme = "ws"
	case "https", "wss":
		u.Scheme = "wss"
	default:
		return nil, fmt.Errorf("listener: unsupported URL scheme %q", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/publish"
	q := u.Query()
	q.Set("room", cfg.Room)
	u.RawQuery = q.Encode()

	return &Client{
		cfg:    cfg,
		log:    cfg.Logger,
		url:    u.String(),
		notify: make(chan struct{}, 1),
	}, nil
}

// Connected reports whether the client currently has a live connection.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// Queued is the number of finalised segments waiting to be sent.
func (c *Client) Queued() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pending)
}

// PublishFinal queues a committed segment. It never blocks: if the queue is
// full the oldest segment is dropped, on the grounds that the audience would
// rather have the last hour than the first.
func (c *Client) PublishFinal(seg protocol.Segment) {
	c.mu.Lock()
	c.pending = append(c.pending, protocol.Message{Type: protocol.TypeFinal, Segment: &seg})
	if over := len(c.pending) - c.cfg.MaxQueue; over > 0 {
		c.pending = append(c.pending[:0], c.pending[over:]...)
		c.dropped += over
		c.log.Warn("publish queue full; dropped oldest segments", "dropped", c.dropped)
	}
	c.mu.Unlock()
	c.kick()
}

// PublishPartial offers an interim hypothesis. Only the most recent one is
// kept — a partial that is already stale by the time the link recovers is of
// no use to anybody.
func (c *Client) PublishPartial(p protocol.Partial) {
	c.mu.Lock()
	c.partial = &p
	c.mu.Unlock()
	c.kick()
}

// PublishStatus offers listener telemetry, replacing any unsent status.
func (c *Client) PublishStatus(s protocol.Status) {
	c.mu.Lock()
	c.status = &s
	c.mu.Unlock()
	c.kick()
}

func (c *Client) kick() {
	select {
	case c.notify <- struct{}{}:
	default:
	}
}

// Run connects and keeps reconnecting until ctx is cancelled.
func (c *Client) Run(ctx context.Context) error {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := c.session(ctx)
		if ctx.Err() != nil {
			// A cancelled context ends the session with an error that is
			// merely the shutdown being observed from the inside; it is not
			// worth reporting to the caller.
			return nil //nolint:nilerr // deliberate: shutdown is not a failure
		}
		if err != nil {
			c.log.Warn("publisher disconnected", "error", err, "retry_in", backoff.Round(time.Second))
		} else {
			c.log.Info("publisher disconnected", "retry_in", backoff.Round(time.Second))
		}
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil
		}
		if backoff *= 2; backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// session runs one connection from dial to failure.
func (c *Client) session(ctx context.Context) error {
	dialer := &websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
		Proxy:            http.ProxyFromEnvironment,
	}
	header := http.Header{}
	if c.cfg.Token != "" {
		header.Set("Authorization", "Bearer "+c.cfg.Token)
	}

	dialCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	// On a successful upgrade the connection is hijacked and the response
	// body is not a readable body at all, so there is nothing to close; the
	// failure path below does close it.
	conn, resp, err := dialer.DialContext(dialCtx, c.url, header) //nolint:bodyclose // see above
	cancel()
	if err != nil {
		if resp != nil {
			// The server explains itself in the body — "a valid publish token
			// is required" is a great deal more use than a bare 401 status.
			// It must also be closed, or every retry leaks a connection.
			detail := readServerError(resp)
			if detail != "" {
				return fmt.Errorf("connect: %s: %s", resp.Status, detail)
			}
			return fmt.Errorf("connect: %s (%w)", resp.Status, err)
		}
		return fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = conn.Close() }()

	meta := c.cfg.Meta
	hello := protocol.Message{Type: protocol.TypeHello, Version: protocol.Version, Room: &meta}
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteJSON(hello); err != nil {
		return fmt.Errorf("hello: %w", err)
	}

	// Wait for the server's acknowledgement before declaring victory, so that
	// a rejected token surfaces immediately rather than as silence.
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	var ack protocol.Message
	if err := conn.ReadJSON(&ack); err != nil {
		return fmt.Errorf("handshake: %w", err)
	}
	if ack.Type == protocol.TypeError {
		return fmt.Errorf("server refused the connection: %s", ack.Error)
	}
	if ack.Type != protocol.TypeAck {
		return fmt.Errorf("unexpected handshake response %q", ack.Type)
	}

	c.setConnected(true)
	defer c.setConnected(false)
	c.log.Info("connected to server", "url", c.url, "room", c.cfg.Room, "cursor", ack.Cursor)

	sessionCtx, cancelSession := context.WithCancel(ctx)
	defer cancelSession()

	// Reader: the server only speaks to us to report errors, but the read
	// loop is what notices a dropped connection.
	readErr := make(chan error, 1)
	go func() {
		conn.SetReadLimit(64 * 1024)
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		})
		for {
			var msg protocol.Message
			if err := conn.ReadJSON(&msg); err != nil {
				readErr <- err
				return
			}
			_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
			if msg.Type == protocol.TypeError {
				c.log.Error("server reported a problem", "error", msg.Error)
				readErr <- errors.New(msg.Error)
				return
			}
		}
	}()

	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()

	for {
		if err := c.drain(conn); err != nil {
			return err
		}
		select {
		case <-c.notify:
		case <-ping.C:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return fmt.Errorf("ping: %w", err)
			}
		case err := <-readErr:
			return err
		case <-sessionCtx.Done():
			// Say goodbye so the room stops showing as live straight away.
			_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			_ = conn.WriteJSON(protocol.Message{Type: protocol.TypeBye})
			_ = conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return nil
		}
	}
}

// drain writes everything currently queued. Finals go first and in order; the
// partial and status are only ever the latest.
func (c *Client) drain(conn *websocket.Conn) error {
	for {
		c.mu.Lock()
		var msg protocol.Message
		switch {
		case len(c.pending) > 0:
			msg = c.pending[0]
			c.pending = c.pending[1:]
		case c.partial != nil:
			msg = protocol.Message{Type: protocol.TypePartial, Partial: c.partial}
			c.partial = nil
		case c.status != nil:
			msg = protocol.Message{Type: protocol.TypeStatus, Status: c.status}
			c.status = nil
		default:
			c.mu.Unlock()
			return nil
		}
		c.mu.Unlock()

		_ = conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
		if err := conn.WriteJSON(msg); err != nil {
			// Put a lost final back at the head of the queue; a lost partial
			// is not worth the trouble.
			if msg.Type == protocol.TypeFinal {
				c.mu.Lock()
				c.pending = append([]protocol.Message{msg}, c.pending...)
				c.mu.Unlock()
			}
			return fmt.Errorf("write: %w", err)
		}
	}
}

// readServerError pulls a short, human-readable reason out of a refused
// handshake and closes the body.
func readServerError(resp *http.Response) string {
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil || len(body) == 0 {
		return ""
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Error != "" {
		return payload.Error
	}
	return strings.TrimSpace(string(body))
}

func (c *Client) setConnected(v bool) {
	c.mu.Lock()
	c.connected = v
	c.mu.Unlock()
}

// Flush waits until every queued segment has been written to the server, or
// the timeout expires. It reports whether the queue drained. Call it before
// shutting down, otherwise the last thing the speaker said never arrives.
func (c *Client) Flush(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c.Queued() == 0 && c.Connected() {
			// One more moment for the write to reach the wire.
			time.Sleep(50 * time.Millisecond)
			if c.Queued() == 0 {
				return true
			}
		}
		c.kick()
		time.Sleep(50 * time.Millisecond)
	}
	return c.Queued() == 0
}
