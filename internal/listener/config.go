package listener

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dpage/slonik-ears/internal/asr"
	"github.com/dpage/slonik-ears/internal/audio"
	"github.com/dpage/slonik-ears/internal/protocol"
	"gopkg.in/yaml.v3"
)

// Config is everything a listener needs to know. It can come from flags, a
// YAML file, or the environment; flags win, then the environment, then the
// file, then the defaults.
type Config struct {
	// Where the transcript goes.
	Server string `yaml:"server"`
	Room   string `yaml:"room"`
	Token  string `yaml:"token"`

	// How the room is described to attendees.
	Title    string `yaml:"title"`
	Track    string `yaml:"track"`
	Speaker  string `yaml:"speaker"`
	Language string `yaml:"language"`

	// Where the audio comes from.
	Device string `yaml:"device"`
	// Channel selects which input channel or channels of a multichannel
	// device carry the microphone, 1-based as printed on the front of an
	// interface, comma separated. Averaging several is what a genuine stereo
	// microphone wants ("1,2"). Empty means listen briefly and take whichever
	// channel has the most signal.
	Channel string `yaml:"channel"`
	File    string `yaml:"file"`
	Loop    bool   `yaml:"loop"`
	Fast    bool   `yaml:"fast"`

	// Vocabulary is the glossary fed to the model so that it spells the
	// jargon the way the audience does. Setting it replaces the built-in
	// Postgres list rather than adding to it; start from
	// `ears-listener --print-vocabulary` if you want to extend it.
	Vocabulary []string `yaml:"vocabulary"`
	// VocabularyFile is the same thing kept in its own file, one term per
	// line, which is easier to edit and to share between rooms.
	VocabularyFile string `yaml:"vocabulary_file"`
	// NoVocabulary sends no glossary at all, for an event that is not about
	// databases.
	NoVocabulary bool `yaml:"no_vocabulary"`

	// How it gets transcribed.
	Whisper    string `yaml:"whisper_url"`
	Model      string `yaml:"model"`
	APIKey     string `yaml:"api_key"`
	Translate  bool   `yaml:"translate"`
	WhisperVAD bool   `yaml:"whisper_vad"`
	Mock       bool   `yaml:"mock"`

	// Segmentation.
	SilenceMs      int  `yaml:"silence_ms"`
	MinUtteranceMs int  `yaml:"min_utterance_ms"`
	MaxUtteranceMs int  `yaml:"max_utterance_ms"`
	PartialMs      int  `yaml:"partial_ms"`
	NoPartials     bool `yaml:"no_partials"`
	PreRollMs      int  `yaml:"pre_roll_ms"`

	// Voice detection. These decide what counts as somebody talking, and are
	// the settings to reach for when a transcript is either missing speech or
	// full of things nobody said.
	//
	// MinRMS is the one that matters most: an absolute level below which
	// nothing is ever speech. Too high and a quiet speaker is ignored; too low
	// and the room's own noise is sent to the model, which does not answer
	// "there was nothing there" but invents something plausible instead.
	MinRMS      float64 `yaml:"min_rms"`
	StartFactor float64 `yaml:"start_factor"`
	StopFactor  float64 `yaml:"stop_factor"`

	// Timeouts, in seconds on the wire because nobody enjoys YAML durations.
	PartialTimeoutSec int `yaml:"partial_timeout_sec"`
	FinalTimeoutSec   int `yaml:"final_timeout_sec"`

	// Housekeeping.
	Transcript string `yaml:"transcript_file"`
	Record     string `yaml:"record_file"`
	LogLevel   string `yaml:"log_level"`
}

// DefaultConfig returns the out-of-the-box configuration: a local server, a
// local whisper.cpp, and English.
func DefaultConfig() Config {
	ch := audio.DefaultChunkerConfig()
	return Config{
		Server:            "http://localhost:8080",
		Language:          "en",
		Whisper:           "http://127.0.0.1:8081/inference",
		SilenceMs:         ch.SilenceMs,
		MinUtteranceMs:    ch.MinUtteranceMs,
		MaxUtteranceMs:    ch.MaxUtteranceMs,
		PartialMs:         ch.PartialIntervalMs,
		PreRollMs:         ch.PreRollMs,
		MinRMS:            ch.VAD.MinRMS,
		StartFactor:       ch.VAD.StartFactor,
		StopFactor:        ch.VAD.StopFactor,
		PartialTimeoutSec: 8,
		FinalTimeoutSec:   60,
		LogLevel:          "info",
	}
}

// BindFlags registers command line flags against this config.
func (c *Config) BindFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.Server, "server", c.Server, "ears-server base URL")
	fs.StringVar(&c.Room, "room", c.Room, "room id to publish to (lower case letters, digits, - and _)")
	fs.StringVar(&c.Token, "token", c.Token, "publish token (or set EARS_PUBLISH_TOKEN)")

	fs.StringVar(&c.Title, "title", c.Title, "room title shown to attendees")
	fs.StringVar(&c.Track, "track", c.Track, "track name, for grouping rooms in the lobby")
	fs.StringVar(&c.Speaker, "speaker", c.Speaker, "speaker name shown to attendees")
	fs.StringVar(&c.Language, "language", c.Language, `spoken language as an ISO-639-1 code, or "auto"`)

	fs.StringVar(&c.Device, "device", c.Device, "capture device index, id, or part of its name (default: system default)")
	fs.StringVar(&c.Channel, "channel", c.Channel, "input channel(s) of a multichannel device, 1-based and comma separated (default: whichever is loudest)")
	fs.StringVar(&c.File, "file", c.File, "replay a WAV file instead of capturing audio (for testing)")
	fs.BoolVar(&c.Loop, "loop", c.Loop, "loop the replayed file")
	fs.BoolVar(&c.Fast, "fast", c.Fast, "replay the file as fast as possible rather than in real time")

	fs.StringVar(&c.Whisper, "whisper", c.Whisper, "transcription endpoint (whisper.cpp server, or an OpenAI-compatible URL)")
	fs.StringVar(&c.Model, "model", c.Model, "model name, for OpenAI-compatible endpoints only")
	fs.StringVar(&c.APIKey, "api-key", c.APIKey, "bearer token for the transcription endpoint (or set EARS_API_KEY)")
	fs.BoolVar(&c.Translate, "translate", c.Translate, "translate to English rather than transcribing verbatim")
	fs.BoolVar(&c.WhisperVAD, "whisper-vad", c.WhisperVAD, "ask the whisper server to apply its own VAD (needs a VAD model loaded there)")
	fs.BoolVar(&c.Mock, "mock", c.Mock, "use the mock transcriber: invents text, needs no model")

	fs.StringVar(&c.VocabularyFile, "vocabulary", c.VocabularyFile, "file of terms the model should spell correctly, one per line (replaces the built-in Postgres glossary)")
	fs.BoolVar(&c.NoVocabulary, "no-vocabulary", c.NoVocabulary, "send no glossary to the model at all")

	fs.IntVar(&c.SilenceMs, "silence-ms", c.SilenceMs, "silence that ends an utterance")
	fs.IntVar(&c.MinUtteranceMs, "min-utterance-ms", c.MinUtteranceMs, "ignore utterances shorter than this")
	fs.IntVar(&c.MaxUtteranceMs, "max-utterance-ms", c.MaxUtteranceMs, "commit an utterance at least this often")
	fs.IntVar(&c.PartialMs, "partial-ms", c.PartialMs, "how often to refresh the live preview")
	fs.BoolVar(&c.NoPartials, "no-partials", c.NoPartials, "disable live previews, halving the load on the model")
	fs.IntVar(&c.PreRollMs, "pre-roll-ms", c.PreRollMs, "audio kept from before speech is detected")

	fs.Float64Var(&c.MinRMS, "min-rms", c.MinRMS, "absolute level below which nothing is ever speech; raise it if the room's noise is being transcribed, lower it if a quiet speaker is missed")
	fs.Float64Var(&c.StartFactor, "start-factor", c.StartFactor, "how far above the estimated noise floor a frame must be to start an utterance")
	fs.Float64Var(&c.StopFactor, "stop-factor", c.StopFactor, "the lower threshold that sustains an utterance once it has started")

	fs.IntVar(&c.PartialTimeoutSec, "partial-timeout", c.PartialTimeoutSec, "seconds to wait for a preview transcription")
	fs.IntVar(&c.FinalTimeoutSec, "final-timeout", c.FinalTimeoutSec, "seconds to wait for a committed transcription")

	fs.StringVar(&c.Transcript, "transcript", c.Transcript, "append committed segments to this JSONL file as a local backup")
	fs.StringVar(&c.Record, "record", c.Record, "record the captured audio to this WAV file for debugging, and replay it later with --file")
	fs.StringVar(&c.LogLevel, "log-level", c.LogLevel, "log level: debug, info, warn or error")
}

// LoadFile applies a YAML config, leaving any value that was given explicitly
// on the command line alone.
func (c *Config) LoadFile(path string, fs *flag.FlagSet) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read listener config: %w", err)
	}
	fromFile := DefaultConfig()
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&fromFile); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	explicit := map[string]bool{}
	if fs != nil {
		fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	}

	// Rebuild from the file, then put back anything the command line set.
	cmdline := *c
	*c = fromFile
	overlay := func(name string, apply func()) {
		if explicit[name] {
			apply()
		}
	}
	overlay("server", func() { c.Server = cmdline.Server })
	overlay("room", func() { c.Room = cmdline.Room })
	overlay("token", func() { c.Token = cmdline.Token })
	overlay("title", func() { c.Title = cmdline.Title })
	overlay("track", func() { c.Track = cmdline.Track })
	overlay("speaker", func() { c.Speaker = cmdline.Speaker })
	overlay("language", func() { c.Language = cmdline.Language })
	overlay("device", func() { c.Device = cmdline.Device })
	overlay("channel", func() { c.Channel = cmdline.Channel })
	overlay("file", func() { c.File = cmdline.File })
	overlay("loop", func() { c.Loop = cmdline.Loop })
	overlay("fast", func() { c.Fast = cmdline.Fast })
	overlay("whisper", func() { c.Whisper = cmdline.Whisper })
	overlay("model", func() { c.Model = cmdline.Model })
	overlay("api-key", func() { c.APIKey = cmdline.APIKey })
	overlay("translate", func() { c.Translate = cmdline.Translate })
	overlay("whisper-vad", func() { c.WhisperVAD = cmdline.WhisperVAD })
	overlay("mock", func() { c.Mock = cmdline.Mock })
	overlay("vocabulary", func() { c.VocabularyFile = cmdline.VocabularyFile })
	overlay("no-vocabulary", func() { c.NoVocabulary = cmdline.NoVocabulary })
	overlay("silence-ms", func() { c.SilenceMs = cmdline.SilenceMs })
	overlay("min-utterance-ms", func() { c.MinUtteranceMs = cmdline.MinUtteranceMs })
	overlay("max-utterance-ms", func() { c.MaxUtteranceMs = cmdline.MaxUtteranceMs })
	overlay("partial-ms", func() { c.PartialMs = cmdline.PartialMs })
	overlay("no-partials", func() { c.NoPartials = cmdline.NoPartials })
	overlay("pre-roll-ms", func() { c.PreRollMs = cmdline.PreRollMs })
	overlay("min-rms", func() { c.MinRMS = cmdline.MinRMS })
	overlay("start-factor", func() { c.StartFactor = cmdline.StartFactor })
	overlay("stop-factor", func() { c.StopFactor = cmdline.StopFactor })
	overlay("partial-timeout", func() { c.PartialTimeoutSec = cmdline.PartialTimeoutSec })
	overlay("final-timeout", func() { c.FinalTimeoutSec = cmdline.FinalTimeoutSec })
	overlay("transcript", func() { c.Transcript = cmdline.Transcript })
	overlay("record", func() { c.Record = cmdline.Record })
	overlay("log-level", func() { c.LogLevel = cmdline.LogLevel })
	return nil
}

// ApplyEnv fills in anything still empty from the environment. Secrets belong
// here rather than in a config file or a shell history.
func (c *Config) ApplyEnv() {
	if v := os.Getenv("EARS_SERVER"); v != "" && c.Server == DefaultConfig().Server {
		c.Server = v
	}
	if v := os.Getenv("EARS_ROOM"); v != "" && c.Room == "" {
		c.Room = v
	}
	if v := os.Getenv("EARS_PUBLISH_TOKEN"); v != "" && c.Token == "" {
		c.Token = v
	}
	if v := os.Getenv("EARS_WHISPER_URL"); v != "" && c.Whisper == DefaultConfig().Whisper {
		c.Whisper = v
	}
	if v := os.Getenv("EARS_API_KEY"); v != "" && c.APIKey == "" {
		c.APIKey = v
	}
}

// Validate checks the configuration makes sense before any audio is captured.
func (c Config) Validate(dryRun bool) error {
	if c.Room == "" {
		return fmt.Errorf("--room is required (for example: --room main-hall)")
	}
	if !protocol.ValidRoomID(c.Room) {
		suggestion := protocol.NormaliseRoomID(c.Room)
		if suggestion == "" {
			return fmt.Errorf("--room %q is not usable: use lower case letters, digits, - and _", c.Room)
		}
		return fmt.Errorf("--room %q is not valid: try --room %s", c.Room, suggestion)
	}
	if !dryRun {
		if c.Server == "" {
			return fmt.Errorf("--server is required")
		}
		if c.Token == "" {
			return fmt.Errorf("a publish token is required: pass --token or set EARS_PUBLISH_TOKEN")
		}
	}
	if !c.Mock && c.Whisper == "" {
		return fmt.Errorf("--whisper is required unless --mock is used")
	}
	if c.File != "" && c.Device != "" {
		return fmt.Errorf("--file and --device are mutually exclusive")
	}
	if _, err := c.Channels(); err != nil {
		return err
	}
	if c.Record != "" && c.File != "" {
		return fmt.Errorf("--record and --file are mutually exclusive: the audio is already in a file")
	}
	return nil
}

// ResolveVocabulary works out the glossary this listener will actually send,
// in order of precedence: nothing at all if it has been turned off, the
// contents of a vocabulary file if one is named, an inline list from the
// config file if there is one, and otherwise the built-in Postgres glossary.
//
// Each of these replaces rather than extends the one below it, which is the
// behaviour that makes "why is my term still spelt wrong" answerable: the
// effective list is whatever `--print-vocabulary` shows, with no merging to
// reason about. Extending the default means starting from a copy of it, which
// is what that flag is for.
func (c Config) ResolveVocabulary() ([]string, error) {
	switch {
	case c.NoVocabulary:
		return nil, nil
	case c.VocabularyFile != "":
		terms, err := readVocabularyFile(c.VocabularyFile)
		if err != nil {
			return nil, err
		}
		return terms, nil
	case len(c.Vocabulary) > 0:
		return c.Vocabulary, nil
	default:
		return asr.DefaultVocabulary, nil
	}
}

// readVocabularyFile reads one term per line, ignoring blank lines and
// comments so that a glossary can explain itself.
func readVocabularyFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read vocabulary: %w", err)
	}
	terms := asr.ParseVocabulary(string(data))
	if len(terms) == 0 {
		return nil, fmt.Errorf("vocabulary file %s has no terms in it", path)
	}
	return terms, nil
}

// Channels parses the --channel list into 1-based channel numbers.
func (c Config) Channels() ([]int, error) {
	if strings.TrimSpace(c.Channel) == "" {
		return nil, nil
	}
	var out []int
	for _, part := range strings.Split(c.Channel, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("--channel %q is not usable: give channel numbers as they are printed on the interface, such as 1 or 1,2", c.Channel)
		}
		out = append(out, n)
	}
	return out, nil
}

// VADConfig builds the detector's settings, leaving anything unset at its
// default.
func (c Config) VADConfig() audio.VADConfig {
	vad := audio.DefaultVADConfig()
	if c.MinRMS > 0 {
		vad.MinRMS = c.MinRMS
	}
	if c.StartFactor > 0 {
		vad.StartFactor = c.StartFactor
	}
	if c.StopFactor > 0 {
		vad.StopFactor = c.StopFactor
	}
	return vad
}

// EngineConfig converts to the engine's configuration. The glossary is passed
// in already resolved, because reading it can fail on a bad file and that is
// better reported at startup than here.
func (c Config) EngineConfig(log *slog.Logger, vocabulary []string) EngineConfig {
	partial := c.PartialMs
	if c.NoPartials {
		partial = 0
	}
	return EngineConfig{
		Chunker: audio.ChunkerConfig{
			SilenceMs:         c.SilenceMs,
			MinUtteranceMs:    c.MinUtteranceMs,
			MaxUtteranceMs:    c.MaxUtteranceMs,
			PartialIntervalMs: partial,
			PreRollMs:         c.PreRollMs,
			VAD:               c.VADConfig(),
		},
		Language:       c.Language,
		Translate:      c.Translate,
		Vocabulary:     vocabulary,
		PartialTimeout: time.Duration(c.PartialTimeoutSec) * time.Second,
		FinalTimeout:   time.Duration(c.FinalTimeoutSec) * time.Second,
		TranscriptFile: c.Transcript,
		Logger:         log,
	}
}

// FinalTimeoutDuration is the ASR client's request timeout.
func (c Config) FinalTimeoutDuration() time.Duration {
	return time.Duration(c.FinalTimeoutSec) * time.Second
}
