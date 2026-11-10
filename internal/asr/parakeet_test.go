package asr

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// fakeSherpa is the server half of sherpa-onnx's offline WebSocket protocol:
// it parses the 8-byte header, accumulates samples until the announced byte
// count is reached and then hands the request to reply.
type fakeSherpa struct {
	*httptest.Server
	upgrades atomic.Int32
	requests atomic.Int32 // across all connections
}

// request is what the fake server decoded from one utterance.
type request struct {
	sampleRate int
	byteCount  int
	samples    []float32
}

// newFakeSherpa starts a server; reply is called with the zero-based
// request number across all connections and may write to conn or return the JSON
// to send as the reply ("" sends nothing).
func newFakeSherpa(t *testing.T, reply func(n int, req request, conn *websocket.Conn) string) *fakeSherpa {
	t.Helper()
	f := &fakeSherpa{}
	up := websocket.Upgrader{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		f.upgrades.Add(1)

		var buf []byte
		var req request
		for {
			typ, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if typ == websocket.TextMessage {
				return // "Done"
			}
			if buf == nil {
				if len(data) < 8 {
					t.Errorf("first message is %d bytes, want at least the 8-byte header", len(data))
					return
				}
				req.sampleRate = int(int32(binary.LittleEndian.Uint32(data[0:4])))
				req.byteCount = int(int32(binary.LittleEndian.Uint32(data[4:8])))
				data = data[8:]
			}
			buf = append(buf, data...)
			if len(buf) < req.byteCount {
				continue
			}
			req.samples = nil
			for i := 0; i+4 <= len(buf); i += 4 {
				req.samples = append(req.samples, math.Float32frombits(binary.LittleEndian.Uint32(buf[i:])))
			}
			buf = nil
			out := reply(int(f.requests.Add(1))-1, req, conn)
			if out == "" {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte(out)); err != nil {
				return
			}
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeSherpa) wsURL() string { return "ws" + strings.TrimPrefix(f.URL, "http") }

func newTestParakeet(t *testing.T, f *fakeSherpa) *ParakeetClient {
	t.Helper()
	c, err := NewParakeet(ParakeetConfig{Endpoint: f.wsURL(), Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestParakeetSendsHeaderAndSamples(t *testing.T) {
	got := make(chan request, 1)
	f := newFakeSherpa(t, func(_ int, req request, _ *websocket.Conn) string {
		got <- req
		return `{"lang":"","text":" Hello there."}`
	})
	c := newTestParakeet(t, f)

	res, err := c.Transcribe(context.Background(), []float32{0.5, -0.25}, 16000, Options{})
	if err != nil {
		t.Fatal(err)
	}
	req := <-got
	if req.sampleRate != 16000 || req.byteCount != 8 {
		t.Errorf("header = rate %d, bytes %d; want 16000, 8", req.sampleRate, req.byteCount)
	}
	if len(req.samples) != 2 || req.samples[0] != 0.5 || req.samples[1] != -0.25 {
		t.Errorf("samples = %v", req.samples)
	}
	if res.Text != "Hello there." || res.Raw != "Hello there." {
		t.Errorf("result = %+v", res)
	}
}

func TestParakeetReusesConnection(t *testing.T) {
	f := newFakeSherpa(t, func(int, request, *websocket.Conn) string { return `{"text":"hi"}` })
	c := newTestParakeet(t, f)
	for i := 0; i < 2; i++ {
		if _, err := c.Transcribe(context.Background(), []float32{0.1}, 16000, Options{}); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.upgrades.Load(); n != 1 {
		t.Errorf("upgrades = %d, want 1", n)
	}
}

func TestParakeetRedialsAfterServerClose(t *testing.T) {
	f := newFakeSherpa(t, func(n int, _ request, conn *websocket.Conn) string {
		if n == 0 {
			// Reply, then close, as the server does after a bad payload.
			_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"text":"one"}`))
			_ = conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"))
			return ""
		}
		return `{"text":"two"}`
	})
	c := newTestParakeet(t, f)

	if _, err := c.Transcribe(context.Background(), []float32{0.1}, 16000, Options{}); err != nil {
		t.Fatal(err)
	}
	// The kept connection is dead by now; the call must redial and resend
	// rather than lose this utterance.
	res, err := c.Transcribe(context.Background(), []float32{0.1}, 16000, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "two" {
		t.Errorf("Text = %q, want two", res.Text)
	}
	if n := f.upgrades.Load(); n != 2 {
		t.Errorf("upgrades = %d, want 2", n)
	}
}

func TestParakeetCancelDoesNotLeakLateReply(t *testing.T) {
	f := newFakeSherpa(t, func(n int, _ request, _ *websocket.Conn) string {
		if n == 0 {
			time.Sleep(300 * time.Millisecond)
			return `{"text":"first"}`
		}
		return `{"text":"second"}`
	})
	c := newTestParakeet(t, f)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.Transcribe(ctx, []float32{0.1}, 16000, Options{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}

	res, err := c.Transcribe(context.Background(), []float32{0.1}, 16000, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "second" {
		t.Errorf("Text = %q, want second", res.Text)
	}
}

func TestParakeetCloseReasonInError(t *testing.T) {
	f := newFakeSherpa(t, func(_ int, _ request, conn *websocket.Conn) string {
		_ = conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseMessageTooBig, "Payload is too large!"))
		return ""
	})
	c := newTestParakeet(t, f)

	_, err := c.Transcribe(context.Background(), []float32{0.1}, 16000, Options{})
	if err == nil || !strings.Contains(err.Error(), "Payload is too large!") {
		t.Errorf("err = %v, want it to carry the close reason", err)
	}
}

func TestParakeetEmptyAudio(t *testing.T) {
	f := newFakeSherpa(t, func(int, request, *websocket.Conn) string { return `{"text":"x"}` })
	c := newTestParakeet(t, f)

	res, err := c.Transcribe(context.Background(), nil, 16000, Options{})
	if err != nil || res.Text != "" || res.Raw != "" || res.Took != 0 {
		t.Errorf("got %+v, %v; want zero Result and nil", res, err)
	}
	if n := f.upgrades.Load(); n != 0 {
		t.Errorf("upgrades = %d, want 0", n)
	}
}

func TestParakeetLeadingFullStopCleaned(t *testing.T) {
	f := newFakeSherpa(t, func(int, request, *websocket.Conn) string {
		return `{"text":". So how do we get there?"}`
	})
	c := newTestParakeet(t, f)

	res, err := c.Transcribe(context.Background(), []float32{0.1}, 16000, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "So how do we get there?" {
		t.Errorf("Text = %q", res.Text)
	}
}

func TestParakeetKeepsShortUtterances(t *testing.T) {
	f := newFakeSherpa(t, func(int, request, *websocket.Conn) string {
		return `{"text":" Thank you."}`
	})
	c := newTestParakeet(t, f)

	res, err := c.Transcribe(context.Background(), []float32{0.1}, 16000, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "Thank you." {
		t.Errorf("Text = %q, want %q", res.Text, "Thank you.")
	}
}

func TestNewParakeetNeedsEndpoint(t *testing.T) {
	if _, err := NewParakeet(ParakeetConfig{}); err == nil {
		t.Error("expected an error for an empty endpoint")
	}
}

func TestParakeetDialHonoursContext(t *testing.T) {
	// A server that accepts the TCP connection but never completes the
	// WebSocket handshake.
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer slow.Close()
	c, err := NewParakeet(ParakeetConfig{Endpoint: "ws" + strings.TrimPrefix(slow.URL, "http"), Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.Transcribe(ctx, []float32{0.1}, 16000, Options{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("deadline: err = %v, want deadline exceeded", err)
	}

	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if _, err := c.Transcribe(ctx, []float32{0.1}, 16000, Options{}); !errors.Is(err, context.Canceled) {
		t.Errorf("cancel: err = %v, want cancelled", err)
	}
}
