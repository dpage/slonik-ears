package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/dpage/slonik-ears/internal/models"
)

// Seams for the tests, which point them at a local server.
var (
	lookup       = models.Lookup
	lookupBundle = models.LookupBundle
	vad          = models.VAD
)

// defaultModelDir is where whisper.cpp users conventionally keep models, and
// where the Makefile and the documentation expect them.
func defaultModelDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot work out the default model directory (use --model-dir): %w", err)
	}
	return filepath.Join(home, ".cache", "whisper"), nil
}

// stderrIsTerminal reports whether progress can be drawn in place.
func stderrIsTerminal() bool {
	fi, err := os.Stderr.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// fetchModels downloads the named model into dir. A Whisper model is fetched
// together with the Silero voice-activity-detection model that the listener's
// recommended setup uses; a Silero name fetches only itself. Files already in
// place with the right checksum are left alone. A Parakeet bundle name fetches
// that bundle's files into dir/<name>/ and nothing else, since the Parakeet
// listener does not use the Silero model.
func fetchModels(ctx context.Context, name, dir string, out io.Writer, isTerminal bool, client *http.Client) error {
	if b, ok := lookupBundle(name); ok {
		for _, f := range b.Files {
			if err := fetchOne(ctx, f, filepath.Join(dir, b.Name), out, isTerminal, client); err != nil {
				return err
			}
		}
		return nil
	}
	m, err := lookup(name)
	if err != nil {
		return err
	}
	wanted := []models.Model{m}
	if !strings.HasPrefix(name, "silero-") {
		wanted = append(wanted, vad())
	}
	for _, w := range wanted {
		if err := fetchOne(ctx, w, dir, out, isTerminal, client); err != nil {
			return err
		}
	}
	return nil
}

func fetchOne(ctx context.Context, m models.Model, dir string, out io.Writer, isTerminal bool, client *http.Client) error {
	const mb = 1 << 20
	lastTenth := -1
	report := func(done, total int64) {
		if total <= 0 {
			return
		}
		pct := int(done * 100 / total)
		if lastTenth < 0 {
			fmt.Fprintf(out, "fetching %s (%.1f MB)\n", m.File, float64(total)/mb)
		}
		if isTerminal {
			fmt.Fprintf(out, "\r  %3d%%  %.1f / %.1f MB", pct, float64(done)/mb, float64(total)/mb)
			lastTenth = pct / 10
			return
		}
		if tenth := pct / 10; tenth > lastTenth {
			lastTenth = tenth
			fmt.Fprintf(out, "  %3d%%  %.1f / %.1f MB\n", tenth*10, float64(done)/mb, float64(total)/mb)
		}
	}
	path, downloaded, err := models.Fetch(ctx, client, m, dir, report)
	if isTerminal && lastTenth >= 0 {
		fmt.Fprintln(out)
	}
	if err != nil {
		return fmt.Errorf("fetching %s: %w", m.Name, err)
	}
	if downloaded {
		fmt.Fprintf(out, "saved: %s\n", path)
	} else {
		fmt.Fprintf(out, "already present: %s\n", path)
	}
	return nil
}
