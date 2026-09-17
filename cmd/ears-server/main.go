// Command ears-server is the Slonik Ears relay and web server.
//
// It accepts live transcripts from one listener per room over a WebSocket,
// fans them out to everybody watching, and serves the attendee web app. Run
// it on the same laptop for a single room, or on a small cloud box when the
// venue's network will not let devices talk to each other — which, at a
// conference, is most of the time.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dpage/slonik-ears/internal/server"
	"github.com/dpage/slonik-ears/internal/signals"
	"github.com/dpage/slonik-ears/internal/store"
	"github.com/dpage/slonik-ears/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ears-server:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath  = flag.String("config", "", "path to a YAML config file (optional)")
		addr        = flag.String("addr", "", "address to listen on (overrides config; default :8080)")
		dataDir     = flag.String("data-dir", "", "directory for transcript storage (empty keeps transcripts in memory only)")
		logLevel    = flag.String("log-level", "info", "log level: debug, info, warn or error")
		logFormat   = flag.String("log-format", "text", "log format: text or json")
		showVersion = flag.Bool("version", false, "print the version and exit")
	)
	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Println("ears-server", version.String())
		return nil
	}

	log := newLogger(*logLevel, *logFormat)
	slog.SetDefault(log)

	cfg, err := server.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	if *addr != "" {
		cfg.Server.Addr = *addr
	}
	if *dataDir != "" {
		cfg.Server.DataDir = *dataDir
	}
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8080"
	}

	var st store.Store = store.Null{}
	if cfg.Server.DataDir != "" {
		fs, err := store.NewFile(cfg.Server.DataDir, log)
		if err != nil {
			return err
		}
		defer func() { _ = fs.Close() }()
		st = fs
		log.Info("persisting transcripts", "dir", cfg.Server.DataDir)
	} else {
		log.Warn("no data directory configured; transcripts are lost on restart (set --data-dir to keep them)")
	}

	srv := server.New(cfg, st, log)

	httpServer := &http.Server{
		Addr:    cfg.Server.Addr,
		Handler: srv.Handler(),
		// WebSockets are long lived, so there is deliberately no write
		// timeout here; the handlers set per-message deadlines instead.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", cfg.Server.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Server.Addr, err)
	}

	banner(log, cfg, ln.Addr().String())

	errCh := make(chan error, 1)
	go func() {
		if cfg.Server.TLSCertFile != "" {
			errCh <- httpServer.ServeTLS(ln, cfg.Server.TLSCertFile, cfg.Server.TLSKeyFile)
			return
		}
		errCh <- httpServer.Serve(ln)
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// The same escape hatch as the listener: the first interrupt drains
	// connections, a second one leaves immediately.
	defer signals.ExitOnSecondInterrupt(os.Stderr, "ears-server: interrupted again — exiting now.")()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}
	return nil
}

// banner prints the URLs an organiser actually needs, including the machine's
// LAN addresses, so nobody has to go hunting in System Settings for them.
func banner(log *slog.Logger, cfg server.Config, listenAddr string) {
	scheme := "http"
	if cfg.Server.TLSCertFile != "" {
		scheme = "https"
	}
	log.Info("Slonik Ears server started",
		"version", version.String(),
		"addr", listenAddr,
		"rooms_configured", len(cfg.Rooms),
		"auto_rooms", cfg.AutoRooms())

	if cfg.Auth.PublishToken == "" {
		allHaveTokens := len(cfg.Rooms) > 0
		for _, r := range cfg.Rooms {
			if r.PublishToken == "" {
				allHaveTokens = false
			}
		}
		if !allHaveTokens {
			log.Warn("no publish token configured: listeners will be refused. Set EARS_PUBLISH_TOKEN or auth.publish_token")
		}
	}

	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return
	}
	fmt.Fprintln(os.Stderr, "\n  Attendees can watch at:")
	if cfg.Server.BaseURL != "" {
		fmt.Fprintf(os.Stderr, "    %s\n", strings.TrimRight(cfg.Server.BaseURL, "/"))
	}
	for _, ip := range localAddresses() {
		fmt.Fprintf(os.Stderr, "    %s://%s\n", scheme, net.JoinHostPort(ip, port))
	}
	fmt.Fprintln(os.Stderr)
}

func localAddresses() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.IsLoopback() || ipnet.IP.IsLinkLocalUnicast() {
				continue
			}
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				out = append(out, ip4.String())
			}
		}
	}
	return out
}

func newLogger(level, format string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}
	if strings.EqualFold(format, "json") {
		return slog.New(slog.NewJSONHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stderr, opts))
}

func usage() {
	fmt.Fprintf(os.Stderr, `ears-server %s — live transcript relay for events

Usage:
  ears-server [flags]

Flags:
`, version.String())
	flag.PrintDefaults()
	fmt.Fprint(os.Stderr, `
Environment:
  EARS_ADDR, EARS_BASE_URL, EARS_DATA_DIR, EARS_EVENT_NAME, EARS_PUBLISH_TOKEN,
  EARS_VIEWER_PASSCODE, EARS_ADMIN_TOKEN, EARS_ALLOWED_ORIGINS, PORT

Example:
  EARS_PUBLISH_TOKEN=$(openssl rand -hex 16) ears-server --data-dir ./data
`)
}
