// Command ears-listener runs in a room: it captures the audio, has it
// transcribed by a Whisper backend, and streams the text to an ears-server.
//
// One listener per room; point them all at the same server and a multi-track
// conference appears in the lobby by itself.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dpage/slonik-ears/internal/asr"
	"github.com/dpage/slonik-ears/internal/audio"
	"github.com/dpage/slonik-ears/internal/listener"
	"github.com/dpage/slonik-ears/internal/protocol"
	"github.com/dpage/slonik-ears/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ears-listener:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath  = flag.String("config", "", "path to a YAML config file (optional; flags win)")
		listDevices = flag.Bool("list-devices", false, "list audio capture devices and exit")
		showVersion = flag.Bool("version", false, "print the version and exit")
		dryRun      = flag.Bool("dry-run", false, "transcribe to the terminal without connecting to a server")
	)
	cfg := listener.DefaultConfig()
	cfg.BindFlags(flag.CommandLine)
	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Println("ears-listener", version.String())
		return nil
	}
	if *listDevices {
		return printDevices()
	}

	if *configPath != "" {
		if err := cfg.LoadFile(*configPath, flag.CommandLine); err != nil {
			return err
		}
	}
	cfg.ApplyEnv()
	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)

	if err := cfg.Validate(*dryRun); err != nil {
		return err
	}

	// ---- audio source
	src, err := openSource(cfg, log)
	if err != nil {
		return err
	}
	defer src.Close()
	log.Info("audio source ready", "source", src.Name())

	// ---- transcription backend
	transcriber, err := newTranscriber(cfg, log)
	if err != nil {
		return err
	}
	defer transcriber.Close()

	// ---- publisher
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var (
		pub        listener.Publisher
		client     *listener.Client
		clientDone = make(chan struct{})
	)
	// The publisher outlives the signal context on purpose: when the operator
	// presses Ctrl-C we stop capturing, but we still want the last utterance
	// to reach the server before the process goes away.
	clientCtx, stopClient := context.WithCancel(context.Background())
	defer stopClient()

	if *dryRun {
		pub = listener.NewConsolePublisher(os.Stdout)
		close(clientDone)
	} else {
		c, err := listener.NewClient(listener.ClientConfig{
			ServerURL: cfg.Server,
			Room:      cfg.Room,
			Token:     cfg.Token,
			Meta: protocol.Room{
				ID:       cfg.Room,
				Title:    cfg.Title,
				Track:    cfg.Track,
				Speaker:  cfg.Speaker,
				Language: cfg.Language,
			},
			Logger: log,
		})
		if err != nil {
			return err
		}
		client = c
		go func() {
			defer close(clientDone)
			_ = client.Run(clientCtx)
		}()
		pub = client
	}

	engine, err := listener.NewEngine(cfg.EngineConfig(log), src, transcriber, pub)
	if err != nil {
		return err
	}

	log.Info("listening",
		"room", cfg.Room,
		"server", cfg.Server,
		"backend", transcriber.Name(),
		"language", cfg.Language)

	runErr := engine.Run(ctx)

	if client != nil {
		if !client.Flush(15 * time.Second) {
			log.Warn("some segments could not be delivered before shutdown", "queued", client.Queued())
		}
		stopClient()
		select {
		case <-clientDone:
		case <-time.After(3 * time.Second):
		}
	}

	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		return runErr
	}
	log.Info("listener stopped")
	return nil
}

func openSource(cfg listener.Config, log *slog.Logger) (audio.Source, error) {
	if cfg.File != "" {
		return audio.OpenFile(cfg.File, !cfg.Fast, cfg.Loop)
	}
	mic, err := audio.OpenMic(cfg.Device)
	if err != nil {
		return nil, fmt.Errorf("%w\n\nOn macOS, the first run needs microphone permission: the prompt appears for\nthe application running this command (Terminal, iTerm, or the binary itself).\nGrant it under System Settings > Privacy & Security > Microphone, then try again.\nUse --list-devices to see what is available", err)
	}
	return mic, nil
}

func newTranscriber(cfg listener.Config, log *slog.Logger) (asr.Transcriber, error) {
	if cfg.Mock {
		log.Warn("using the mock transcription backend: the text is invented, not heard")
		return &asr.Mock{Latency: 200 * time.Millisecond}, nil
	}
	client, err := asr.NewWhisper(asr.WhisperConfig{
		Endpoint:  cfg.Whisper,
		APIKey:    cfg.APIKey,
		Model:     cfg.Model,
		Language:  cfg.Language,
		Translate: cfg.Translate,
		VAD:       cfg.WhisperVAD,
		Timeout:   cfg.FinalTimeoutDuration(),
	})
	if err != nil {
		return nil, err
	}
	// Fail loudly at startup rather than silently during the opening keynote.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		return nil, fmt.Errorf("cannot reach the transcription backend at %s: %w\n\nStart one with:\n  whisper-server --model ~/.cache/whisper/ggml-small.en.bin --port 8081\n(install it with `brew install whisper-cpp`), or pass --mock to try the rest of\nthe system without a model", cfg.Whisper, err)
	}
	log.Info("transcription backend ready", "backend", client.Name(), "endpoint", cfg.Whisper)
	return client, nil
}

func printDevices() error {
	devices, err := audio.ListDevices()
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		fmt.Println("No capture devices found.")
		return nil
	}
	fmt.Println("Capture devices (use --device with the index or part of the name):")
	for i, d := range devices {
		marker := " "
		if d.Default {
			marker = "*"
		}
		fmt.Printf("  %s %d  %s\n", marker, i, d.Name)
	}
	fmt.Println("\n  * = system default")
	return nil
}

func newLogger(level string) *slog.Logger {
	lvl := slog.LevelInfo
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}

func usage() {
	fmt.Fprintf(os.Stderr, `ears-listener %s — transcribes a room and streams it to an ears-server

Usage:
  ears-listener --room main-hall --server https://ears.example.org --token SECRET

Flags:
`, version.String())
	flag.PrintDefaults()
	fmt.Fprint(os.Stderr, `
Environment:
  EARS_SERVER, EARS_ROOM, EARS_PUBLISH_TOKEN, EARS_WHISPER_URL, EARS_API_KEY

Examples:
  # Try the whole system with no model and no microphone:
  ears-listener --room demo --mock --dry-run

  # A real room, transcribed locally by whisper.cpp:
  whisper-server --model ~/.cache/whisper/ggml-small.en.bin --port 8081 &
  ears-listener --room main-hall --title "Main Hall" --token "$EARS_PUBLISH_TOKEN"
`)
}
