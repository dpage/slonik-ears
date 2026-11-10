package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dpage/slonik-ears/internal/models"
)

// fakeCatalogue points the lookup and vad seams at a test server, so no test
// touches Hugging Face, and restores them afterwards.
func fakeCatalogue(t *testing.T) {
	t.Helper()
	body := []byte(strings.Repeat("not a real model, just test bytes\n", 4096))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	sum := sha256.Sum256(body)
	entry := func(name string) models.Model {
		return models.Model{
			Name:   name,
			File:   "ggml-" + name + ".bin",
			URL:    srv.URL + "/" + name,
			SHA256: hex.EncodeToString(sum[:]),
			Size:   int64(len(body)),
		}
	}
	origLookup, origVAD := lookup, vad
	t.Cleanup(func() { lookup, vad = origLookup, origVAD })
	lookup = func(name string) (models.Model, error) {
		switch name {
		case "test", "silero-v5.1.2":
			return entry(name), nil
		}
		_, err := origLookup(name)
		return models.Model{}, err
	}
	vad = func() models.Model { return entry("silero-v5.1.2") }
}

func TestFetchModelsFetchesModelAndVAD(t *testing.T) {
	fakeCatalogue(t)
	dir := t.TempDir()
	var out bytes.Buffer
	if err := fetchModels(context.Background(), "test", dir, &out, false, http.DefaultClient); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"ggml-test.bin", "ggml-silero-v5.1.2.bin"} {
		p := filepath.Join(dir, f)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
		if !strings.Contains(out.String(), p) {
			t.Errorf("output does not mention %s:\n%s", p, out.String())
		}
	}
}

func TestFetchModelsReportsExistingFiles(t *testing.T) {
	fakeCatalogue(t)
	dir := t.TempDir()
	var out bytes.Buffer
	for i := 0; i < 2; i++ {
		out.Reset()
		if err := fetchModels(context.Background(), "test", dir, &out, false, http.DefaultClient); err != nil {
			t.Fatal(err)
		}
	}
	if n := strings.Count(out.String(), "already present: "); n != 2 {
		t.Errorf("want 2 'already present' lines, got %d:\n%s", n, out.String())
	}
}

func TestFetchModelsVADNameFetchesOnlyVAD(t *testing.T) {
	fakeCatalogue(t)
	dir := t.TempDir()
	var out bytes.Buffer
	if err := fetchModels(context.Background(), "silero-v5.1.2", dir, &out, false, http.DefaultClient); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "ggml-silero-v5.1.2.bin" {
		t.Errorf("want only the VAD file, got %v", entries)
	}
}

func TestFetchModelsFetchesBundleWithoutVAD(t *testing.T) {
	body := []byte(strings.Repeat("not a real model, just test bytes\n", 64))
	var requested []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	sum := sha256.Sum256(body)
	file := func(f string) models.Model {
		return models.Model{
			Name:   "test-bundle/" + f,
			File:   f,
			URL:    srv.URL + "/" + f,
			SHA256: hex.EncodeToString(sum[:]),
			Size:   int64(len(body)),
		}
	}
	origBundle, origVAD := lookupBundle, vad
	t.Cleanup(func() { lookupBundle, vad = origBundle, origVAD })
	lookupBundle = func(name string) (models.Bundle, bool) {
		if name == "test-bundle" {
			return models.Bundle{Name: name, Files: []models.Model{file("encoder.onnx"), file("tokens.txt")}}, true
		}
		return models.Bundle{}, false
	}
	vad = func() models.Model {
		t.Error("the VAD model must not be looked up for a bundle")
		return models.Model{}
	}

	dir := t.TempDir()
	var out bytes.Buffer
	if err := fetchModels(context.Background(), "test-bundle", dir, &out, false, http.DefaultClient); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"encoder.onnx", "tokens.txt"} {
		if _, err := os.Stat(filepath.Join(dir, "test-bundle", f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	if len(requested) != 2 {
		t.Errorf("requested %v, want exactly the two bundle files", requested)
	}

	out.Reset()
	if err := fetchModels(context.Background(), "test-bundle", dir, &out, false, http.DefaultClient); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out.String(), "already present: "); n != 2 || len(requested) != 2 {
		t.Errorf("second run: %d 'already present' lines, %d requests:\n%s", n, len(requested), out.String())
	}
}

func TestFetchModelsUnknownName(t *testing.T) {
	fakeCatalogue(t)
	dir := t.TempDir()
	var out bytes.Buffer
	err := fetchModels(context.Background(), "no-such-model", dir, &out, false, http.DefaultClient)
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, n := range models.Names() {
		if !strings.Contains(err.Error(), n) {
			t.Errorf("error does not list %q: %v", n, err)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("nothing should be written, got %v", entries)
	}
}

func TestFetchModelsNonTerminalOutput(t *testing.T) {
	fakeCatalogue(t)
	var out bytes.Buffer
	if err := fetchModels(context.Background(), "test", t.TempDir(), &out, false, http.DefaultClient); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "\r") {
		t.Errorf("non-terminal output contains a carriage return: %q", out.String())
	}
	if !strings.Contains(out.String(), "100%") {
		t.Errorf("expected a 100%% line:\n%s", out.String())
	}
}
