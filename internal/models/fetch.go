package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// Fetch makes sure the model's file exists in dir with the right contents and
// returns its path. A file that is already there with the expected size and
// SHA-256 is left alone (downloaded is false); anything else is replaced by a
// fresh download. The download is written to a .part file and renamed into
// place only once its checksum matches, so the final path never holds a
// partial or unverified file. report, which may be nil, is called with the
// bytes received so far and the expected total.
func Fetch(ctx context.Context, client *http.Client, m Model, dir string, report func(done, total int64)) (path string, downloaded bool, err error) {
	path = filepath.Join(dir, m.File)
	if fileMatches(path, m) {
		return path, false, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, fmt.Errorf("create %s: %w", dir, err)
	}

	part := path + ".part"
	if err := download(ctx, client, m, part, report); err != nil {
		_ = os.Remove(part)
		return "", false, err
	}
	if err := os.Rename(part, path); err != nil {
		_ = os.Remove(part)
		return "", false, fmt.Errorf("move %s into place: %w", m.File, err)
	}
	return path, true, nil
}

// fileMatches reports whether path holds exactly the model's bytes. The size is
// checked first so that an obviously wrong file is not read through.
func fileMatches(path string, m Model) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != m.Size {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	return hex.EncodeToString(h.Sum(nil)) == m.SHA256
}

// download streams the model's URL into part (truncating any earlier attempt)
// and checks the checksum. The caller removes part on error.
func download(ctx context.Context, client *http.Client, m Model, part string, report func(done, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.URL, nil)
	if err != nil {
		return fmt.Errorf("download %s: %w", m.URL, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", m.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", m.URL, resp.Status)
	}

	out, err := os.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", part, err)
	}

	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(out, h, &progress{total: m.Size, report: report}), resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("download %s: %w", m.URL, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("write %s: %w", part, closeErr)
	}

	if got := hex.EncodeToString(h.Sum(nil)); got != m.SHA256 {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", m.File, got, m.SHA256)
	}
	return nil
}

// progress is an io.Writer that discards its input and tells report how much
// has gone past.
type progress struct {
	done, total int64
	report      func(done, total int64)
}

func (p *progress) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	if p.report != nil {
		p.report(p.done, p.total)
	}
	return len(b), nil
}
