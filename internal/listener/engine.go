package listener

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dpage/slonik-ears/internal/asr"
	"github.com/dpage/slonik-ears/internal/audio"
	"github.com/dpage/slonik-ears/internal/protocol"
)

// maxShutdownWait caps how long Run waits for outstanding transcriptions once
// the source has ended. Long enough for a chunk that is nearly done; short
// enough that Ctrl-C feels like it did something.
const maxShutdownWait = 15 * time.Second

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
	// Vocabulary is the glossary of terms the audience would notice getting
	// mangled. It is used twice: as a prompt, which helps the model hear the
	// right words, and as a rewrite over the output, which makes it spell them
	// consistently. Neither alone is sufficient; see asr.Canonicaliser.
	Vocabulary []string
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

	// vocabPrompt is the rendered glossary, and canon the rewrite built from
	// the same terms. Both are fixed for the life of the engine.
	vocabPrompt string
	canon       *asr.Canonicaliser

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
		canon: asr.NewCanonicaliser(cfg.Vocabulary),
	}

	prompt, dropped := asr.VocabularyPrompt(cfg.Vocabulary)
	e.vocabPrompt = prompt
	if len(dropped) > 0 {
		// Not a warning: the built-in glossary is deliberately longer than the
		// prompt can hold, because the two uses have different appetites. The
		// prompt takes what fits from the top of the list and helps the model
		// hear those terms; the rewrite covers all of them regardless. Worth
		// saying plainly, though, so that somebody whose term is coming out
		// misheard rather than misspelt can move it up the list.
		e.log.Info("glossary is longer than the model's prompt allows",
			"in_prompt", len(cfg.Vocabulary)-len(dropped),
			"corrected_only_afterwards", len(dropped),
			"budget_chars", asr.PromptBudget)
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
//
// The worker and status contexts are deliberately NOT derived from ctx: when
// the operator stops the listener we want capture to stop immediately but the
// transcription already in flight to finish, so the last sentence of the talk
// still reaches the audience. They are cancelled below, once the queue has
// drained.
//
//nolint:contextcheck // see above: shutdown ordering is the whole point
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
		// The instrumentation below is why this loop is longer than it looks
		// like it should be. The failure people actually hit is "it went quiet
		// and I have no idea why", and from the audience's seat a detector
		// that has decided the room is silent, a capture ring that has
		// overflowed, and a model whose output the cleaning rules discarded
		// are indistinguishable. Everything here is debug level bar the
		// dropped-frames warning, which is not a diagnostic so much as news.
		dropper, _ := e.src.(audio.Dropper)
		speaking := false
		lastDropped := int64(0)
		heartbeat := time.NewTicker(time.Second)
		defer heartbeat.Stop()

		for {
			select {
			case frame, ok := <-frames:
				if !ok {
					return e.src.Err()
				}
				reqs, level := e.chunk.Push(frame)
				e.setLevel(level)
				if now := e.chunk.Speaking(); now != speaking {
					speaking = now
					e.logSpeechEdge(now, level)
				}
				for _, r := range reqs {
					e.log.Debug("chunk ready",
						"kind", kindName(r.Kind),
						"audio", time.Duration(r.EndMs-r.StartMs)*time.Millisecond,
						"at", time.Duration(r.StartMs)*time.Millisecond,
						"peak", round4(r.Level))
					e.enqueue(r)
				}
			case <-heartbeat.C:
				if dropper != nil {
					if d := dropper.Dropped(); d > lastDropped {
						// Not debug level: dropped frames are audio that no
						// longer exists, and the operator wants to know whilst
						// the talk is still going on.
						e.log.Warn("dropped captured audio: transcription is not keeping up with the room",
							"frames", d,
							"audio_lost", time.Duration(d*audio.FrameMs)*time.Millisecond)
						lastDropped = d
					}
				}
				e.logHeartbeat()
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
	// Give the worker time to finish what is queued and in flight: without
	// this the last utterance of a talk is lost, which is precisely the one
	// people notice. Bounded well below FinalTimeout, though — the point is to
	// catch a transcription that is nearly done, not to wait out a model that
	// has stopped answering while somebody stands there pressing Ctrl-C.
	budget := e.cfg.FinalTimeout
	if budget > maxShutdownWait {
		budget = maxShutdownWait
	}
	if pending := e.pendingWork(); pending > 0 {
		e.log.Info("finishing the last utterance before stopping",
			"pending", pending, "waiting_up_to", budget)
	}
	if !e.waitForQueue(budget) {
		e.log.Warn("gave up waiting for outstanding transcriptions", "pending", e.pendingWork())
	}

	stopStatus()
	stopWorker()
	wg.Wait()
	return err
}

// logSpeechEdge records the detector changing its mind, with the numbers that
// decided it. A start threshold that has crept up to meet the speaker's level
// is what a room that has gone silent on you looks like from here.
//
// It reads chunker state directly, so it may only be called from the frame
// loop that owns it.
func (e *Engine) logSpeechEdge(speaking bool, level float64) {
	if !e.log.Enabled(context.Background(), slog.LevelDebug) {
		return
	}
	start, stop := e.chunk.Thresholds()
	msg := "speech ended"
	if speaking {
		msg = "speech started"
	}
	e.log.Debug(msg,
		"level", round4(level),
		"noise_floor", round4(e.chunk.NoiseFloor()),
		"start_threshold", round4(start),
		"stop_threshold", round4(stop),
		"at", e.chunk.Elapsed().Round(time.Millisecond))
}

// logHeartbeat says, once a second, what the pipeline believes is going on.
// Read against the wall clock it distinguishes "nobody is talking" from "the
// detector cannot hear you" from "the model is hopelessly behind".
//
// It reads chunker state directly, so it may only be called from the frame
// loop that owns it.
func (e *Engine) logHeartbeat() {
	if !e.log.Enabled(context.Background(), slog.LevelDebug) {
		return
	}
	start, _ := e.chunk.Thresholds()
	e.stateMu.Lock()
	level, segments := e.level, e.segments
	e.stateMu.Unlock()
	e.mu.Lock()
	finals, partial := len(e.queuedFinals), e.queuedPartia != nil
	e.mu.Unlock()

	e.log.Debug("heartbeat",
		"at", e.chunk.Elapsed().Round(time.Second),
		"level", round4(level),
		"noise_floor", round4(e.chunk.NoiseFloor()),
		"start_threshold", round4(start),
		"speaking", e.chunk.Speaking(),
		"utterance", time.Duration(e.chunk.UtteranceMs())*time.Millisecond,
		"voiced", time.Duration(e.chunk.VoicedMs())*time.Millisecond,
		"queued_finals", finals,
		"queued_partial", partial,
		"transcribing", e.busy.Load(),
		"segments", segments,
		"connected", e.pub.Connected(),
		"undelivered", e.pub.Queued())
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

	audioLen := time.Duration(req.EndMs-req.StartMs) * time.Millisecond
	e.log.Debug("transcribing",
		"kind", kindName(req.Kind),
		"audio", audioLen,
		"samples", len(req.PCM),
		"timeout", timeout,
		"prompt", truncate(opts.Prompt, 60))

	res, err := e.asr.Transcribe(reqCtx, req.PCM, audio.SampleRate, opts)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		// The detail goes to the log, not to the room. What the engine
		// publishes is shown to every attendee on their own phone, and the
		// error from an HTTP client carries the backend's URL: on a room
		// machine that is harmless localhost, but pointed at a server across
		// the venue or at a cloud endpoint it puts an internal hostname, or
		// an upstream provider's error payload, on three hundred screens.
		e.setError(publicTranscriptionError(req.Kind))
		e.log.Warn("transcription failed",
			"kind", kindName(req.Kind),
			"audio", time.Duration(req.EndMs-req.StartMs)*time.Millisecond,
			"error", err)
		return
	}
	e.setError("")

	// Rewrite before anything else sees the text, so the preview, the
	// committed segment, the prompt fed back to the model and the local
	// transcript file all agree on how the jargon is spelt. A partial that
	// says "PG Edge" and a final that says "pgEdge" reads, to an audience, as
	// the thing changing its mind.
	text := e.canon.Apply(strings.TrimSpace(res.Text))
	if text == "" {
		// Clear the preview whichever kind this was. A final that produces no
		// text used to return here without touching the partial, which left
		// the last preview on every screen in the room, greyed out and
		// unconfirmed, until somebody said something else: the audience sees a
		// half sentence that never resolves.
		e.pub.PublishPartial(protocol.Partial{Text: ""})
		if raw := strings.TrimSpace(res.Raw); raw != "" {
			// The model did say something and the cleaning rules threw it
			// away. Worth seeing, since that is the difference between a deaf
			// microphone and an over-eager filter.
			e.log.Debug("discarded by cleaning",
				"kind", kindName(req.Kind), "audio", audioLen, "raw", truncate(raw, 120))
		} else {
			e.log.Debug("no text",
				"kind", kindName(req.Kind), "audio", audioLen,
				"took", res.Took.Round(time.Millisecond))
		}
		return
	}

	if req.Kind == audio.KindPartial {
		e.log.Debug("partial",
			"audio", audioLen,
			"took", res.Took.Round(time.Millisecond),
			"text", truncate(text, 80))
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

	// "speed" is audio duration over wall clock: below 1 the model is slower
	// than the person speaking, the queue only grows, and the answer is a
	// smaller model or --no-partials.
	speed := 0.0
	if res.Took > 0 {
		speed = audioLen.Seconds() / res.Took.Seconds()
	}
	e.log.Info("segment",
		"n", n,
		"audio", audioLen,
		"took", res.Took.Round(time.Millisecond),
		"speed", math.Round(speed*10)/10,
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

// publicTranscriptionError is what an attendee is told when the model server
// is not answering: enough for somebody in the room to tell the organisers
// that something is wrong, and nothing about where the machine sits or what
// it is called.
func publicTranscriptionError(kind audio.RequestKind) string {
	if kind == audio.KindPartial {
		return "the transcription service is slow to respond"
	}
	return "the transcription service is not responding"
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
	tail := e.prompt
	e.stateMu.Unlock()

	switch {
	case e.vocabPrompt == "":
		return tail
	case tail == "":
		return e.vocabPrompt
	default:
		return e.vocabPrompt + " " + tail
	}
}

func kindName(k audio.RequestKind) string {
	if k == audio.KindPartial {
		return "partial"
	}
	return "final"
}

// round4 keeps audio levels readable in a log line: they are small numbers and
// seventeen significant figures of them help nobody.
func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func truncate(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "..."
}
