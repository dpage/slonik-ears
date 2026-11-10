package listener

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadConfig writes yaml to a temporary file and loads it over the defaults,
// with args standing in for the command line.
func loadConfig(t *testing.T, yaml string, args ...string) Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "listener.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cfg.BindFlags(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	if err := cfg.LoadFile(path, fs); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestDefaultBackendIsWhisper(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Backend != "whisper" {
		t.Errorf("default backend = %q, want whisper", cfg.Backend)
	}
	if cfg.Parakeet != "ws://127.0.0.1:8082" {
		t.Errorf("default parakeet_url = %q", cfg.Parakeet)
	}
}

func TestConfigWithoutBackendKeyIsWhisper(t *testing.T) {
	cfg := loadConfig(t, "room: demo\nwhisper_url: http://127.0.0.1:9000/inference\n")
	if cfg.Backend != "whisper" {
		t.Errorf("backend = %q, want whisper", cfg.Backend)
	}
	if err := cfg.Validate(true); err != nil {
		t.Errorf("an older config should still validate: %v", err)
	}
}

func TestBackendFlagBeatsFile(t *testing.T) {
	cfg := loadConfig(t, "room: demo\nbackend: parakeet\n", "--backend", "whisper")
	if cfg.Backend != "whisper" {
		t.Errorf("backend = %q, want whisper from the flag", cfg.Backend)
	}
	cfg = loadConfig(t, "room: demo\nbackend: parakeet\nparakeet_url: ws://127.0.0.1:9001\n", "--parakeet", "ws://127.0.0.1:9002")
	if cfg.Backend != "parakeet" || cfg.Parakeet != "ws://127.0.0.1:9002" {
		t.Errorf("got backend %q, parakeet %q", cfg.Backend, cfg.Parakeet)
	}
}

func TestValidateBackend(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{"unknown backend names both", func(c *Config) { c.Backend = "bogus" }, "whisper or parakeet"},
		{"parakeet translating", func(c *Config) { c.Backend = "parakeet"; c.Translate = true }, "Parakeet cannot translate; use backend: whisper"},
		{"parakeet without endpoint", func(c *Config) { c.Backend = "parakeet"; c.Parakeet = "" }, "--parakeet"},
		{"parakeet ignores whisper_url", func(c *Config) { c.Backend = "parakeet"; c.Whisper = "" }, ""},
		{"whisper needs whisper_url", func(c *Config) { c.Whisper = "" }, "--whisper is required"},
		{"mock overrides parakeet", func(c *Config) { c.Backend = "parakeet"; c.Parakeet = ""; c.Mock = true }, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := DefaultConfig()
			c.Room = "demo"
			tc.mutate(&c)
			err := c.Validate(true)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("unexpected error: %v", err)
			case tc.wantErr != "" && err == nil:
				t.Error("expected an error")
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Errorf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}
