package asr

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/dpage/slonik-ears/internal/audio"
)

// ParakeetConfig configures the client for sherpa-onnx's
// `sherpa-onnx-offline-websocket-server`, which is how NVIDIA Parakeet is
// run here.
type ParakeetConfig struct {
	// Endpoint is the WebSocket URL, e.g. ws://127.0.0.1:8082.
	Endpoint string
	// Timeout bounds dialling; 10 seconds if zero. Waiting for a reply is
	// bounded by the caller's context instead.
	Timeout time.Duration
}

// ParakeetClient is a WebSocket-backed Transcriber. It keeps one connection
// open between requests and redials when that connection goes away.
type ParakeetClient struct {
	cfg ParakeetConfig

	// mu serialises requests and guards conn. The server answers one
	// utterance per message in order, so there is nothing to gain from
	// overlapping them on one connection.
	mu   sync.Mutex
	conn *websocket.Conn
}

// NewParakeet builds a client. It does not dial; use Ping for that.
func NewParakeet(cfg ParakeetConfig) (*ParakeetClient, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("asr: endpoint is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &ParakeetClient{cfg: cfg}, nil
}

// Name implements Transcriber.
func (c *ParakeetClient) Name() string { return "parakeet" }

// Close implements Transcriber. It tells the server we are finished (text
// "Done") if a connection is open, then closes it.
func (c *ParakeetClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	_ = c.conn.WriteMessage(websocket.TextMessage, []byte("Done"))
	err := c.conn.Close()
	c.conn = nil
	return err
}

// Ping checks the server is reachable by transcribing half a second of
// silence, as WhisperClient.Ping does.
func (c *ParakeetClient) Ping(ctx context.Context) error {
	silence := make([]float32, audio.SampleRate/2)
	_, err := c.Transcribe(ctx, silence, audio.SampleRate, Options{})
	return err
}

type parakeetResponse struct {
	Text string `json:"text"`
	Lang string `json:"lang"`
}

// Transcribe implements Transcriber.
//
// None of opts is sent: the sherpa-onnx server takes only the sample rate and
// the samples. There is no prompt, language, translation or temperature
// setting per request (the model and its language are fixed when the server
// starts), and no cheaper mode for partial results.
func (c *ParakeetClient) Transcribe(ctx context.Context, pcm []float32, sampleRate int, _ Options) (Result, error) {
	if len(pcm) == 0 {
		return Result{}, nil
	}

	// Header: int32 LE sample rate, int32 LE byte count; then float32 LE
	// samples, all in one binary message.
	msg := make([]byte, 8+4*len(pcm))
	binary.LittleEndian.PutUint32(msg[0:4], uint32(int32(sampleRate)))
	binary.LittleEndian.PutUint32(msg[4:8], uint32(int32(4*len(pcm))))
	for i, s := range pcm {
		binary.LittleEndian.PutUint32(msg[8+4*i:], math.Float32bits(s))
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	start := time.Now()
	var raw []byte
	// A connection kept from an earlier request may have been closed by the
	// server since (it does so after a malformed payload, or when restarted),
	// and the first we hear of it is a failed write or an empty read. That is
	// not the fault of this audio, so drop it and try once on a fresh
	// connection. A connection dialled for this call that fails is reported
	// as it is.
	for attempt := 0; ; attempt++ {
		reused := c.conn != nil
		conn, err := c.dial(ctx)
		if err != nil {
			return Result{}, err
		}
		var cancelled bool
		raw, cancelled, err = c.exchange(ctx, conn, msg)
		if err == nil {
			break
		}
		if cancelled {
			return Result{}, fmt.Errorf("asr: %w", ctx.Err())
		}
		if reused && attempt == 0 && ctx.Err() == nil {
			continue
		}
		var ce *websocket.CloseError
		if errors.As(err, &ce) {
			return Result{}, fmt.Errorf("asr: server closed the connection: %d %s", ce.Code, ce.Text)
		}
		return Result{}, fmt.Errorf("asr: %w", err)
	}

	var pr parakeetResponse
	if err := json.Unmarshal(raw, &pr); err != nil {
		return Result{}, fmt.Errorf("asr: could not parse response: %w", err)
	}
	return Result{
		Text:     CleanTranscript(pr.Text),
		Raw:      strings.TrimSpace(pr.Text),
		Language: pr.Lang,
		Took:     time.Since(start),
	}, nil
}

// dial returns the open connection, connecting first if there is none. The
// caller must hold c.mu.
func (c *ParakeetClient) dial(ctx context.Context) (*websocket.Conn, error) {
	if c.conn != nil {
		return c.conn, nil
	}
	dctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	conn, _, err := websocket.DefaultDialer.DialContext(dctx, c.cfg.Endpoint, nil)
	if err != nil {
		// The dialler may report the network's own timeout a moment before
		// the context notices; callers test for the context's error.
		if derr := dctx.Err(); derr != nil {
			err = derr
		} else if ne := net.Error(nil); errors.As(err, &ne) && ne.Timeout() {
			err = context.DeadlineExceeded
		}
		return nil, fmt.Errorf("asr: connecting to %s: %w", c.cfg.Endpoint, err)
	}
	c.conn = conn
	return conn, nil
}

// exchange sends one request on conn and reads the reply. On any failure the
// connection is closed and forgotten, so the next request dials afresh;
// cancelled reports that ctx ended the exchange. The caller must hold c.mu.
func (c *ParakeetClient) exchange(ctx context.Context, conn *websocket.Conn, msg []byte) (raw []byte, cancelled bool, err error) {
	// The server cannot abort a decode, so cancelling means closing the
	// connection: that unblocks the read below, and it guarantees the late
	// reply can never be taken for the answer to the next request, which
	// dials afresh.
	stop := make(chan struct{})
	fired := make(chan bool, 1)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
			fired <- true
		case <-stop:
			fired <- false
		}
	}()

	err = conn.WriteMessage(websocket.BinaryMessage, msg)
	if err == nil {
		_, raw, err = conn.ReadMessage()
	}
	close(stop)
	cancelled = <-fired
	if cancelled || err != nil {
		// Closed under us, even if the reply beat the cancellation to it,
		// so it cannot be reused.
		_ = conn.Close()
		c.conn = nil
	}
	return raw, cancelled && err != nil, err
}
