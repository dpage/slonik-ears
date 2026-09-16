package listener

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dpage/slonik-ears/internal/asr"
	"github.com/dpage/slonik-ears/internal/audio"
	"github.com/dpage/slonik-ears/internal/protocol"
)

// Publisher is the subset of Client the engine needs, so tests can substitute
// something simpler.
type Publisher interface {
	PublishFinal(protocol.Segment)
	PublishPartial(protocol.Partial)
	PublishStatus(protocol.Status)
	Connected() bool
	Queued() int
}

// EngineConfig configures the capture-to-text pipeline.
type EngineConfig struct {
	Chunker audio.ChunkerConfig
	// Language is passed to the model; "auto" lets it detect.
	Language string
	// Translate asks for English output regardless of what is spoken.
	Translate bool
	// PromptWords is how much of the preceding transcript is fed back to the
	// model as context. Whisper caps its prompt, and too much of it makes the
	// model repeat itself, so keep this modest.
	PromptWords int
	// PartialTimeout bounds an interim transcription; a partial that takes
	// longer than this is not worth waiting for.
	PartialTimeout time.Duration
	// FinalTimeout bounds a committed transcription.
	FinalTimeout time.Duration
	// TranscriptFile, if set, receives every committed segment as JSONL. It
	// is the local safety net for when the network is having a bad day.
	TranscriptFile string
	// StatusInterval is how often telemetry is published.
	StatusInterval time.Duration
	Logger         *slog.Logger
}

// DefaultEngineConfig returns sensible values for a conference talk.
func DefaultEngineConfig() EngineConfig {
	return EngineConfig{
		Chunker:        audio.DefaultChunkerConfig(),
		Language:       "en",
		PromptWords:    32,
		PartialTimeout: 8 * time.Second,
		FinalTimeout:   60 * time.Second,
		StatusInterval: 3 * time.Second,
	}
}

// Engine reads frames from a Source, segments them, transcribes each segment
// and publishes the text.
type Engine struct {
	cfg    EngineConfig
	log    *slog.Logger
	src    audio.Source
	asr    asr.Transcriber
	pub    Publisher
	chunk  *audio.Chunker
	jsonl  *os.File
	jsonMu sync.Mutex

	// work queue, drained by a single worker so ordering is guaranteed and
	// the model is never asked to do two things at once.
	mu           sync.Mutex
	queuedFinals []audio.Request
	queuedPartia *audio.Request
	wake         chan struct{}
	// busy marks a transcription in flight, so shutdown waits for it rather
	// than cancelling the last sentence of the talk.
	busy atomic.Bool

	// state shared with the status ticker
	stateMu  sync.Mutex
	level    float64
	lastErr  string
	prompt   string
	segments int
}

// NewEngine wires the pipeline together.
func NewEngine(cfg EngineConfig, src audio.Source, transcriber asr.Transcriber, pub Publisher) (*Engine, error) {
	def := DefaultEngineConfig()
	if cfg.PromptWords <= 0 {
		cfg.PromptWords = def.PromptWords
	}
	if cfg.PartialTimeout <= 0 {
		cfg.PartialTimeout = def.PartialTimeout
	}
	if cfg.FinalTimeout <= 0 {
		cfg.FinalTimeout = def.FinalTimeout
	}
	if cfg.StatusInterval <= 0 {
		cfg.StatusInterval = def.StatusInterval
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	e := &Engine{
		cfg:   cfg,
		log:   cfg.Logger,
		src:   src,
		asr:   transcriber,
		pub:   pub,
		chunk: audio.NewChunker(cfg.Chunker),
		wake:  make(chan struct{}, 1),
	}

	if cfg.TranscriptFile != "" {
		f, err := os.OpenFile(cfg.TranscriptFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
		if err != nil {
			return nil, fmt.Errorf("listener: open transcript file: %w", err)
		}
		e.jsonl = f
	}
	return e, nil
}

// Run processes audio until the source ends or ctx is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	defer func() {
		if e.jsonl != nil {
			_ = e.jsonl.Close()
		}
	}()

	var wg sync.WaitGroup
	workerCtx, stopWorker := context.WithCancel(context.Background())
	wg.Add(1)
	go func() {
		defer wg.Done()
		e.worker(workerCtx)
	}()

	wg.Add(1)
	statusCtx, stopStatus := context.WithCancel(context.Background())
	go func() {
		defer wg.Done()
		e.statusLoop(statusCtx)
	}()

	frames := e.src.Frames()
	err := func() error {
		for {
			select {
			case frame, ok := <-frames:
				if !ok {
					return e.src.Err()
				}
				reqs, level := e.chunk.Push(frame)
				e.setLevel(level)
				for _, r := range reqs {
					e.enqueue(r)
				}
			case <-ctx.Done():
				return nil
			}
		}
	}()

	// Commit whatever was mid-sentence when we stopped, then let the worker
	// finish the queue before shutting it down.
	if req, ok := e.chunk.Flush(); ok {
		e.enqueue(req)
	}
	// Give the worker time to finish what is queued and in flight. Without
	// this the last utterance of a talk is lost, which is precisely the one
	// people notice.
	if !e.waitForQueue(e.cfg.FinalTimeout + 5*time.Second) {
		e.log.Warn("gave up waiting for outstanding transcriptions", "pending", e.pendingWork())
	}

	stopStatus()
	stopWorker()
	wg.Wait()
	return err
}

func (e *Engine) enqueue(r audio.Request) {
	e.mu.Lock()
	switch r.Kind {
	case audio.KindFinal:
		e.queuedFinals = append(e.queuedFinals, r)
		// A pending partial belongs to the utterance we have just committed,
		// so it is now worse than useless.
		e.queuedPartia = nil
	case audio.KindPartial:
		if len(e.queuedFinals) == 0 {
			e.queuedPartia = &r
		}
	}
	e.mu.Unlock()
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

func (e *Engine) next() (audio.Request, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.queuedFinals) > 0 {
		r := e.queuedFinals[0]
		e.queuedFinals = e.queuedFinals[1:]
		return r, true
	}
	if e.queuedPartia != nil {
		r := *e.queuedPartia
		e.queuedPartia = nil
		return r, true
	}
	return audio.Request{}, false
}

// pendingWork counts queued requests plus any transcription in flight.
func (e *Engine) pendingWork() int {
	e.mu.Lock()
	n := len(e.queuedFinals)
	if e.queuedPartia != nil {
		n++
	}
	e.mu.Unlock()
	if e.busy.Load() {
		n++
	}
	return n
}

// waitForQueue blocks until there is no outstanding work, or the limit
// expires. It reports whether the queue drained.
func (e *Engine) waitForQueue(limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if e.pendingWork() == 0 {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return e.pendingWork() == 0
}

// worker transcribes one request at a time. Serialising the work matters:
// whisper saturates the machine, and two concurrent requests simply make both
// of them late.
func (e *Engine) worker(ctx context.Context) {
	for {
		req, ok := e.next()
		if !ok {
			select {
			case <-e.wake:
				continue
			case <-ctx.Done():
				return
			case <-time.After(250 * time.Millisecond):
				continue
			}
		}
		e.transcribe(ctx, req)
	}
}

func (e *Engine) transcribe(ctx context.Context, req audio.Request) {
	e.busy.Store(true)
	defer e.busy.Store(false)

	timeout := e.cfg.FinalTimeout
	if req.Kind == audio.KindPartial {
		timeout = e.cfg.PartialTimeout
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	opts := asr.Options{
		Language:  e.cfg.Language,
		Translate: e.cfg.Translate,
		Prompt:    e.currentPrompt(),
		Partial:   req.Kind == audio.KindPartial,
	}

	res, err := e.asr.Transcribe(reqCtx, req.PCM, audio.SampleRate, opts)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		e.setError(err.Error())
		e.log.Warn("transcription failed",
			"kind", kindName(req.Kind),
			"audio", time.Duration(req.EndMs-req.StartMs)*time.Millisecond,
			"error", err)
		return
	}
	e.setError("")

	text := strings.TrimSpace(res.Text)
	if text == "" {
		if req.Kind == audio.KindPartial {
			e.pub.PublishPartial(protocol.Partial{Text: ""})
		}
		return
	}

	if req.Kind == audio.KindPartial {
		e.pub.PublishPartial(protocol.Partial{Text: text, At: time.Now().UnixMilli()})
		return
	}

	seg := protocol.Segment{
		Text:     text,
		StartMs:  req.StartMs,
		EndMs:    req.EndMs,
		At:       time.Now().UnixMilli(),
		Language: res.Language,
	}
	e.pub.PublishFinal(seg)
	e.appendLocal(seg)
	e.pushPrompt(text)

	e.stateMu.Lock()
	e.segments++
	n := e.segments
	e.stateMu.Unlock()

	e.log.Info("segment",
		"n", n,
		"audio", time.Duration(req.EndMs-req.StartMs)*time.Millisecond,
		"took", res.Took.Round(time.Millisecond),
		"text", truncate(text, 80))
}

// appendLocal writes a segment to the on-disk safety net.
func (e *Engine) appendLocal(seg protocol.Segment) {
	if e.jsonl == nil {
		return
	}
	line, err := json.Marshal(seg)
	if err != nil {
		return
	}
	e.jsonMu.Lock()
	defer e.jsonMu.Unlock()
	_, _ = e.jsonl.Write(append(line, '\n'))
}

func (e *Engine) statusLoop(ctx context.Context) {
	t := time.NewTicker(e.cfg.StatusInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			e.stateMu.Lock()
			level, lastErr := e.level, e.lastErr
			e.stateMu.Unlock()
			e.pub.PublishStatus(protocol.Status{
				Level:          level,
				Listening:      lastErr == "",
				Backend:        e.asr.Name(),
				Detail:         lastErr,
				QueuedSegments: e.pub.Queued(),
			})
		case <-ctx.Done():
			return
		}
	}
}

func (e *Engine) setLevel(l float64) {
	e.stateMu.Lock()
	// Decay slowly so the meter in the browser is readable rather than
	// epileptic.
	if l > e.level {
		e.level = l
	} else {
		e.level = e.level*0.8 + l*0.2
	}
	e.stateMu.Unlock()
}

func (e *Engine) setError(msg string) {
	e.stateMu.Lock()
	e.lastErr = msg
	e.stateMu.Unlock()
}

// pushPrompt keeps the tail of the transcript as context for the next chunk.
func (e *Engine) pushPrompt(text string) {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	words := strings.Fields(e.prompt + " " + text)
	if len(words) > e.cfg.PromptWords {
		words = words[len(words)-e.cfg.PromptWords:]
	}
	e.prompt = strings.Join(words, " ")
}

func (e *Engine) currentPrompt() string {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	return e.prompt
}

func kindName(k audio.RequestKind) string {
	if k == audio.KindPartial {
		return "partial"
	}
	return "final"
}

func truncate(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "..."
}
