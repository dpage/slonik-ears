package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// testBody is a stand-in for a model file, large enough to arrive in several
// reads.
var testBody = []byte(strings.Repeat("not a real model, just test bytes\n", 4096))

func modelFor(url string, body []byte) Model {
	sum := sha256.Sum256(body)
	return Model{
		Name:   "test",
		File:   "ggml-test.bin",
		URL:    url,
		SHA256: hex.EncodeToString(sum[:]),
		Size:   int64(len(body)),
	}
}

// serve returns a server that always answers with body, and a counter of the
// requests it has seen.
func serve(t *testing.T, body []byte) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func assertNoPart(t *testing.T, dir string) {
	t.Helper()
	parts, err := filepath.Glob(filepath.Join(dir, "*.part"))
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 0 {
		t.Errorf("partial files left behind: %v", parts)
	}
}

func TestFetchDownloadsAndVerifies(t *testing.T) {
	srv, _ := serve(t, testBody)
	m := modelFor(srv.URL, testBody)
	dir := t.TempDir()

	var lastDone, lastTotal int64
	path, downloaded, err := Fetch(context.Background(), srv.Client(), m, dir, func(done, total int64) {
		lastDone, lastTotal = done, total
	})
	if err != nil {
		t.Fatal(err)
	}
	if !downloaded {
		t.Error("downloaded = false for a fresh download")
	}
	if want := filepath.Join(dir, m.File); path != want {
		t.Errorf("path = %s, want %s", path, want)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(testBody) {
		t.Error("downloaded bytes differ from the served bytes")
	}
	if lastDone != lastTotal || lastTotal != m.Size {
		t.Errorf("final report was done=%d total=%d, want both %d", lastDone, lastTotal, m.Size)
	}
	assertNoPart(t, dir)
}

func TestFetchSkipsGoodFile(t *testing.T) {
	srv, hits := serve(t, testBody)
	m := modelFor(srv.URL, testBody)
	dir := t.TempDir()

	if _, _, err := Fetch(context.Background(), srv.Client(), m, dir, nil); err != nil {
		t.Fatal(err)
	}
	before := hits.Load()

	_, downloaded, err := Fetch(context.Background(), srv.Client(), m, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if downloaded {
		t.Error("downloaded = true for a file that was already good")
	}
	if after := hits.Load(); after != before {
		t.Errorf("server saw %d more requests, want none", after-before)
	}
}

func TestFetchReplacesCorruptFile(t *testing.T) {
	srv, hits := serve(t, testBody)
	m := modelFor(srv.URL, testBody)
	dir := t.TempDir()

	// Right size, wrong contents.
	corrupt := []byte(strings.Repeat("x", len(testBody)))
	if err := os.WriteFile(filepath.Join(dir, m.File), corrupt, 0o644); err != nil {
		t.Fatal(err)
	}

	path, downloaded, err := Fetch(context.Background(), srv.Client(), m, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !downloaded || hits.Load() != 1 {
		t.Errorf("downloaded = %v after %d requests, want a single fresh download", downloaded, hits.Load())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(testBody) {
		t.Error("the corrupt file was not replaced")
	}
}

func TestFetchReplacesWrongSizedFile(t *testing.T) {
	srv, _ := serve(t, testBody)
	m := modelFor(srv.URL, testBody)
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, m.File), []byte("short"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, downloaded, err := Fetch(context.Background(), srv.Client(), m, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !downloaded {
		t.Error("a file of the wrong size was accepted")
	}
	if got, _ := os.ReadFile(path); string(got) != string(testBody) {
		t.Error("the wrong-sized file was not replaced")
	}
}

func TestFetchHashMismatchFails(t *testing.T) {
	other := []byte(strings.Repeat("y", len(testBody)))
	srv, _ := serve(t, other)
	m := modelFor(srv.URL, testBody)
	dir := t.TempDir()

	_, _, err := Fetch(context.Background(), srv.Client(), m, dir, nil)
	if err == nil {
		t.Fatal("expected a checksum error")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("error %q does not mention the checksum", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, m.File)); !os.IsNotExist(statErr) {
		t.Errorf("a file exists at the final path after a failed download (%v)", statErr)
	}
	assertNoPart(t, dir)
}

func TestFetchInterruptedLeavesNoFinalFile(t *testing.T) {
	// The server promises the whole body, sends half, then waits for the
	// client to go away.
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		_, _ = w.Write(testBody[:len(testBody)/2])
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer broken.Close()

	m := modelFor(broken.URL, testBody)
	dir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, _, err := Fetch(ctx, broken.Client(), m, dir, func(done, total int64) {
		if done > 0 {
			cancel()
		}
	})
	if err == nil {
		t.Fatal("expected an error from an interrupted download")
	}
	if _, statErr := os.Stat(filepath.Join(dir, m.File)); !os.IsNotExist(statErr) {
		t.Errorf("a file exists at the final path after an interrupted download (%v)", statErr)
	}
	assertNoPart(t, dir)

	// A later attempt against a working server starts clean and succeeds.
	srv, _ := serve(t, testBody)
	m.URL = srv.URL
	if _, downloaded, err := Fetch(context.Background(), srv.Client(), m, dir, nil); err != nil || !downloaded {
		t.Fatalf("retry: downloaded=%v err=%v", downloaded, err)
	}
}

func TestFetchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	m := modelFor(srv.URL+"/missing.bin", testBody)
	dir := t.TempDir()

	_, _, err := Fetch(context.Background(), srv.Client(), m, dir, nil)
	if err == nil {
		t.Fatal("expected an error for a 404")
	}
	for _, want := range []string{"404", m.URL} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	assertNoPart(t, dir)
}

func TestFetchCreatesDir(t *testing.T) {
	srv, _ := serve(t, testBody)
	m := modelFor(srv.URL, testBody)
	dir := filepath.Join(t.TempDir(), "a", "b")

	if _, _, err := Fetch(context.Background(), srv.Client(), m, dir, nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", dir)
	}
	if perm := info.Mode().Perm(); perm != 0o755 {
		t.Errorf("directory mode = %o, want 755", perm)
	}
}
