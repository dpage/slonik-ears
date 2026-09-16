package asr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dpage/slonik-ears/internal/audio"
)

// WhisperConfig configures the HTTP speech-to-text client.
//
// It speaks the multipart API used by whisper.cpp's `whisper-server`
// (/inference), which is deliberately close to OpenAI's
// /v1/audio/transcriptions, so the same client covers both: set Model and
// APIKey and point Endpoint at a cloud provider instead.
type WhisperConfig struct {
	// Endpoint is the full URL, e.g. http://127.0.0.1:8081/inference.
	Endpoint string
	// APIKey, if set, is sent as a bearer token.
	APIKey string
	// Model is only needed by OpenAI-compatible services; whisper-server
	// already has its model loaded.
	Model string
	// Language is an ISO-639-1 code or "auto".
	Language string
	// Translate requests English output regardless of the spoken language.
	Translate bool
	// Temperature, 0 unless you enjoy surprises.
	Temperature float64
	// Timeout bounds a single request.
	Timeout time.Duration
	// VAD enables whisper-server's own voice activity detection. It needs a
	// VAD model loaded server side, so it is off by default.
	VAD bool
	// HTTPClient is optional; one is created if nil.
	HTTPClient *http.Client
}

// WhisperClient is an HTTP-backed Transcriber.
type WhisperClient struct {
	cfg  WhisperConfig
	http *http.Client
	name string
}

// NewWhisper builds a client. It does not contact the endpoint; use Ping for
// that.
func NewWhisper(cfg WhisperConfig) (*WhisperClient, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("asr: endpoint is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: cfg.Timeout}
	}
	name := "whisper-server"
	if cfg.Model != "" {
		name = "whisper-api(" + cfg.Model + ")"
	}
	return &WhisperClient{cfg: cfg, http: hc, name: name}, nil
}

// Name implements Transcriber.
func (c *WhisperClient) Name() string { return c.name }

// Close implements Transcriber.
func (c *WhisperClient) Close() error {
	c.http.CloseIdleConnections()
	return nil
}

// Ping checks the endpoint is reachable by transcribing a short burst of
// silence. It is worth doing at startup: discovering that whisper-server is
// not running at the moment the speaker says hello is no fun at all.
func (c *WhisperClient) Ping(ctx context.Context) error {
	silence := make([]float32, audio.SampleRate/2)
	_, err := c.Transcribe(ctx, silence, audio.SampleRate, Options{Language: c.cfg.Language})
	return err
}

type whisperResponse struct {
	Text     string `json:"text"`
	Language string `json:"language"`
	Segments []struct {
		Text  string  `json:"text"`
		Start float64 `json:"start"`
		End   float64 `json:"end"`
	} `json:"segments"`
	Error any `json:"error"`
}

// Transcribe implements Transcriber.
func (c *WhisperClient) Transcribe(ctx context.Context, pcm []float32, sampleRate int, opts Options) (Result, error) {
	if len(pcm) == 0 {
		return Result{}, nil
	}
	wav := audio.EncodeWAV(pcm, sampleRate)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "chunk.wav")
	if err != nil {
		return Result{}, err
	}
	if _, err := part.Write(wav); err != nil {
		return Result{}, err
	}

	lang := opts.Language
	if lang == "" {
		lang = c.cfg.Language
	}
	if lang == "" {
		lang = "auto"
	}
	fields := map[string]string{
		"response_format": "verbose_json",
		"temperature":     strconv.FormatFloat(maxFloat(opts.Temperature, c.cfg.Temperature), 'f', -1, 64),
		"language":        lang,
	}
	if c.cfg.Model != "" {
		fields["model"] = c.cfg.Model
	}
	if opts.Prompt != "" {
		fields["prompt"] = opts.Prompt
	}
	if opts.Translate || c.cfg.Translate {
		// whisper-server spells it "translate"; the OpenAI API uses a
		// different endpoint, so only send it to the local server.
		if c.cfg.Model == "" {
			fields["translate"] = "true"
		}
	}
	if c.cfg.VAD && c.cfg.Model == "" {
		fields["vad"] = "true"
	}
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return Result{}, err
		}
	}
	if err := mw.Close(); err != nil {
		return Result{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, &body)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("asr: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return Result{}, fmt.Errorf("asr: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("asr: %s: %s", resp.Status, strings.TrimSpace(truncate(string(raw), 300)))
	}

	var wr whisperResponse
	if err := json.Unmarshal(raw, &wr); err != nil {
		// Some deployments return plain text; treat that as the transcript
		// rather than failing the whole utterance.
		text := strings.TrimSpace(string(raw))
		if text == "" || strings.HasPrefix(text, "{") {
			return Result{}, fmt.Errorf("asr: could not parse response: %w", err)
		}
		return Result{Text: CleanTranscript(text), Took: time.Since(start)}, nil
	}
	if wr.Error != nil {
		return Result{}, fmt.Errorf("asr: backend error: %v", wr.Error)
	}

	res := Result{
		Text:     CleanTranscript(wr.Text),
		Language: wr.Language,
		Took:     time.Since(start),
	}
	for _, s := range wr.Segments {
		text := CleanTranscript(s.Text)
		if text == "" {
			continue
		}
		res.Segments = append(res.Segments, Segment{
			Text:    text,
			StartMs: int64(s.Start * 1000),
			EndMs:   int64(s.End * 1000),
		})
	}
	return res, nil
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
