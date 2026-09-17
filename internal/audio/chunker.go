package audio

import "time"

// RequestKind distinguishes a low-latency interim transcription from the
// committed one.
type RequestKind int

const (
	// KindPartial is an in-progress utterance: transcribe it quickly, show it
	// greyed out, and expect it to change.
	KindPartial RequestKind = iota
	// KindFinal is a complete utterance: transcribe it properly and commit.
	KindFinal
)

// Request is a chunk of audio the chunker wants transcribed.
type Request struct {
	Kind RequestKind
	// PCM is a private copy, safe to hand to another goroutine.
	PCM []float32
	// StartMs and EndMs are offsets from the start of the session.
	StartMs int64
	EndMs   int64
	// Level is the recent peak level, forwarded to the room status display.
	Level float64
}

// ChunkerConfig tunes utterance segmentation.
type ChunkerConfig struct {
	// SilenceMs is how much quiet ends an utterance. Too short and sentences
	// get chopped mid-clause (hurting accuracy, since whisper uses context);
	// too long and the audience reads yesterday's news.
	SilenceMs int
	// MinUtteranceMs ignores coughs, door slams and chair scrapes.
	MinUtteranceMs int
	// MaxUtteranceMs forces a commit for speakers who never pause for breath.
	MaxUtteranceMs int
	// PartialIntervalMs is how often an in-progress utterance is re-run for a
	// live preview. Set to 0 to disable partials entirely, which halves the
	// load on the model.
	PartialIntervalMs int
	// PreRollMs of audio is kept from before speech was detected, so the
	// first consonant is not clipped off.
	PreRollMs int
	// MaxPartialWindowMs caps how much audio a partial re-transcribes, so a
	// long sentence does not make previews progressively slower.
	MaxPartialWindowMs int
	// VAD tunes the detector.
	VAD VADConfig
}

// DefaultChunkerConfig is a reasonable starting point for a conference talk.
func DefaultChunkerConfig() ChunkerConfig {
	return ChunkerConfig{
		SilenceMs:          650,
		MinUtteranceMs:     400,
		MaxUtteranceMs:     14000,
		PartialIntervalMs:  900,
		PreRollMs:          320,
		MaxPartialWindowMs: 12000,
		VAD:                DefaultVADConfig(),
	}
}

// Chunker turns a continuous stream of frames into transcription requests. It
// is a pure state machine: no goroutines, no clocks, no I/O, which makes it
// straightforward to test with synthetic audio.
type Chunker struct {
	cfg ChunkerConfig
	vad *VAD

	preRoll    []float32 // ring of audio from before speech started
	preRollMax int

	utterance    []float32
	utteranceAt  int64 // session offset of utterance[0]
	speaking     bool
	silenceMs    int64
	voicedMs     int64
	sincePartial int64

	elapsedMs int64
	peak      float64
}

// NewChunker returns a chunker. Zero-valued config fields get defaults.
func NewChunker(cfg ChunkerConfig) *Chunker {
	def := DefaultChunkerConfig()
	if cfg.SilenceMs <= 0 {
		cfg.SilenceMs = def.SilenceMs
	}
	if cfg.MinUtteranceMs <= 0 {
		cfg.MinUtteranceMs = def.MinUtteranceMs
	}
	if cfg.MaxUtteranceMs <= 0 {
		cfg.MaxUtteranceMs = def.MaxUtteranceMs
	}
	if cfg.PreRollMs < 0 {
		cfg.PreRollMs = def.PreRollMs
	}
	if cfg.MaxPartialWindowMs <= 0 {
		cfg.MaxPartialWindowMs = def.MaxPartialWindowMs
	}
	return &Chunker{
		cfg:        cfg,
		vad:        NewVAD(cfg.VAD),
		preRollMax: cfg.PreRollMs * SampleRate / 1000,
	}
}

// Speaking reports whether the chunker currently believes somebody is talking.
func (c *Chunker) Speaking() bool { return c.speaking }

// NoiseFloor is the detector's current estimate of the room.
func (c *Chunker) NoiseFloor() float64 { return c.vad.NoiseFloor() }

// Thresholds are the levels speech must currently reach to start and to carry
// on, which is what a debug log needs in order to explain a silent transcript.
func (c *Chunker) Thresholds() (start, stop float64) { return c.vad.Thresholds() }

// UtteranceMs is how much audio is held in the utterance being built, and
// VoicedMs how much of it the detector counted as speech. A commit is refused
// when VoicedMs falls below MinUtteranceMs, so logging both explains a chunk
// that was captured and then quietly dropped.
func (c *Chunker) UtteranceMs() int64 { return DurationMs(len(c.utterance)) }

// VoicedMs is the voiced portion of the utterance being built.
func (c *Chunker) VoicedMs() int64 { return c.voicedMs }

// Elapsed is the session time consumed so far.
func (c *Chunker) Elapsed() time.Duration { return time.Duration(c.elapsedMs) * time.Millisecond }

// Push feeds one frame and returns any requests it produced (at most one
// final plus one partial). Level is the frame's RMS.
func (c *Chunker) Push(frame []float32) (reqs []Request, level float64) {
	speech, level := c.vad.Push(frame)
	frameMs := DurationMs(len(frame))
	c.elapsedMs += frameMs
	if p := Peak(frame); p > c.peak {
		c.peak = p
	}

	if !c.speaking && !speech {
		// Idle: keep a short pre-roll so the start of the next word survives.
		c.preRoll = append(c.preRoll, frame...)
		if over := len(c.preRoll) - c.preRollMax; over > 0 {
			c.preRoll = append(c.preRoll[:0], c.preRoll[over:]...)
		}
		return nil, level
	}

	if !c.speaking {
		// Speech just started: adopt the pre-roll as the head of the utterance.
		c.speaking = true
		c.voicedMs = 0
		c.silenceMs = 0
		c.sincePartial = 0
		c.utterance = append(c.utterance[:0], c.preRoll...)
		c.utteranceAt = c.elapsedMs - frameMs - DurationMs(len(c.preRoll))
		if c.utteranceAt < 0 {
			c.utteranceAt = 0
		}
		c.preRoll = c.preRoll[:0]
	}

	c.utterance = append(c.utterance, frame...)
	c.sincePartial += frameMs
	if speech {
		c.voicedMs += frameMs
		c.silenceMs = 0
	} else {
		c.silenceMs += frameMs
	}

	utteranceMs := DurationMs(len(c.utterance))
	endOfSpeech := c.silenceMs >= int64(c.cfg.SilenceMs)
	tooLong := utteranceMs >= int64(c.cfg.MaxUtteranceMs)

	if endOfSpeech || tooLong {
		req, ok := c.commit(tooLong && !endOfSpeech)
		if ok {
			reqs = append(reqs, req)
		}
		return reqs, level
	}

	if c.cfg.PartialIntervalMs > 0 &&
		c.sincePartial >= int64(c.cfg.PartialIntervalMs) &&
		utteranceMs >= int64(c.cfg.MinUtteranceMs) {
		c.sincePartial = 0
		reqs = append(reqs, Request{
			Kind:    KindPartial,
			PCM:     c.partialWindow(),
			StartMs: c.utteranceAt,
			EndMs:   c.elapsedMs,
			Level:   c.takePeak(),
		})
	}
	return reqs, level
}

// Flush ends any in-progress utterance, e.g. when the source is closing. The
// returned request may be empty.
func (c *Chunker) Flush() (Request, bool) {
	if !c.speaking {
		return Request{}, false
	}
	return c.commit(true)
}

// commit finalises the current utterance. carryOn keeps the tail of the audio
// as the head of the next utterance when a speaker is simply not pausing.
func (c *Chunker) commit(carryOn bool) (Request, bool) {
	utterance := c.utterance
	startMs := c.utteranceAt
	endMs := c.elapsedMs
	voicedMs := c.voicedMs

	// Trim the hangover silence back to a short tail: the model does not need
	// it, and it is audio we would otherwise upload for nothing.
	const keepTailMs = 250
	if c.silenceMs > keepTailMs {
		drop := int((c.silenceMs - keepTailMs) * SampleRate / 1000)
		if drop > 0 && drop < len(utterance) {
			utterance = utterance[:len(utterance)-drop]
			endMs -= c.silenceMs - keepTailMs
		}
	}

	c.speaking = false
	c.silenceMs = 0
	c.voicedMs = 0
	c.sincePartial = 0
	c.preRoll = c.preRoll[:0]
	c.utterance = nil

	if carryOn {
		// Hand the last moment of audio to the next utterance so a word
		// straddling the cut is not lost entirely.
		tail := c.cfg.PreRollMs * SampleRate / 1000
		if tail > 0 && len(utterance) > tail {
			c.preRoll = append(c.preRoll, utterance[len(utterance)-tail:]...)
		}
	}

	// Judge the length by how much of it was actually voiced: an utterance
	// that is mostly the silence that ended it is a cough, not a sentence.
	if voicedMs < int64(c.cfg.MinUtteranceMs) {
		return Request{}, false
	}
	pcm := make([]float32, len(utterance))
	copy(pcm, utterance)
	return Request{
		Kind:    KindFinal,
		PCM:     pcm,
		StartMs: startMs,
		EndMs:   endMs,
		Level:   c.takePeak(),
	}, true
}

// partialWindow returns a copy of the tail of the current utterance, bounded
// by MaxPartialWindowMs.
func (c *Chunker) partialWindow() []float32 {
	max := c.cfg.MaxPartialWindowMs * SampleRate / 1000
	src := c.utterance
	if max > 0 && len(src) > max {
		src = src[len(src)-max:]
	}
	out := make([]float32, len(src))
	copy(out, src)
	return out
}

func (c *Chunker) takePeak() float64 {
	p := c.peak
	c.peak = 0
	return p
}
