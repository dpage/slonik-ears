package server

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dpage/slonik-ears/internal/protocol"
	"gopkg.in/yaml.v3"
)

// Config is the server's configuration, loaded from YAML and then overlaid
// with environment variables (which is what you want in a container).
type Config struct {
	Event  EventConfig  `yaml:"event"`
	Server ServerConfig `yaml:"server"`
	Auth   AuthConfig   `yaml:"auth"`
	Rooms  []RoomConfig `yaml:"rooms"`
}

// EventConfig is the branding shown to attendees.
type EventConfig struct {
	Name    string `yaml:"name"`
	Tagline string `yaml:"tagline"`
	// Notice is shown under the transcript, e.g. an accessibility disclaimer.
	Notice string `yaml:"notice"`
}

// ServerConfig covers the HTTP listener and storage.
type ServerConfig struct {
	Addr string `yaml:"addr"`
	// BaseURL is the address attendees use, e.g. https://ears.example.org.
	// It is only used to build join links and QR codes.
	BaseURL string `yaml:"base_url"`
	DataDir string `yaml:"data_dir"`
	History int    `yaml:"history"`
	// TrustProxy makes the server honour X-Forwarded-For for logging and
	// rate limiting. Only enable it behind a proxy you control.
	TrustProxy bool `yaml:"trust_proxy"`
	// MaxViewersPerRoom caps concurrent viewers; 0 means unlimited.
	MaxViewersPerRoom int    `yaml:"max_viewers_per_room"`
	TLSCertFile       string `yaml:"tls_cert_file"`
	TLSKeyFile        string `yaml:"tls_key_file"`
}

// AuthConfig holds the shared secrets. Prefer the environment variables in
// production so tokens do not end up in a config file in git.
type AuthConfig struct {
	// PublishToken authorises a listener to publish to any room.
	PublishToken string `yaml:"publish_token"`
	// ViewerPasscode, if set, is required to watch. Leave empty for an open
	// event, which is usually the point.
	ViewerPasscode string `yaml:"viewer_passcode"`
	// AdminToken guards the admin API.
	AdminToken string `yaml:"admin_token"`
	// AllowedOrigins restricts WebSocket origins. Empty means "same host
	// only, plus anything when the request has no Origin header" — see
	// checkOrigin.
	AllowedOrigins []string `yaml:"allowed_origins"`
	// AllowAutoRooms lets a listener create a room that is not in the config.
	// Handy for ad-hoc tracks, off for a locked down conference.
	AllowAutoRooms *bool `yaml:"allow_auto_rooms"`
}

// RoomConfig pre-declares a room so it appears in the lobby before its
// listener connects.
type RoomConfig struct {
	ID       string `yaml:"id"`
	Title    string `yaml:"title"`
	Track    string `yaml:"track"`
	Speaker  string `yaml:"speaker"`
	Language string `yaml:"language"`
	// PublishToken overrides the global token for this room, so each room's
	// operator can be given their own secret.
	PublishToken string `yaml:"publish_token"`
}

// DefaultConfig returns a usable configuration for "just run it on my laptop".
func DefaultConfig() Config {
	allowAuto := true
	return Config{
		Event: EventConfig{
			Name:   "Slonik Ears",
			Notice: "Automatically generated live transcript. Errors are inevitable; please do not quote it verbatim.",
		},
		Server: ServerConfig{
			Addr:    ":8080",
			History: 2000,
		},
		Auth: AuthConfig{AllowAutoRooms: &allowAuto},
	}
}

// LoadConfig reads a YAML config file (optional) and applies environment
// overrides. An empty path skips the file entirely.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read config: %w", err)
		}
		dec := yaml.NewDecoder(strings.NewReader(string(data)))
		dec.KnownFields(true)
		if err := dec.Decode(&cfg); err != nil {
			return cfg, fmt.Errorf("parse config %s: %w", path, err)
		}
	}
	cfg.applyEnv()
	if err := cfg.validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c *Config) applyEnv() {
	str := func(key string, dst *string) {
		if v, ok := os.LookupEnv(key); ok {
			*dst = v
		}
	}
	str("EARS_EVENT_NAME", &c.Event.Name)
	str("EARS_EVENT_TAGLINE", &c.Event.Tagline)
	str("EARS_EVENT_NOTICE", &c.Event.Notice)
	str("EARS_ADDR", &c.Server.Addr)
	str("EARS_BASE_URL", &c.Server.BaseURL)
	str("EARS_DATA_DIR", &c.Server.DataDir)
	str("EARS_TLS_CERT_FILE", &c.Server.TLSCertFile)
	str("EARS_TLS_KEY_FILE", &c.Server.TLSKeyFile)
	str("EARS_PUBLISH_TOKEN", &c.Auth.PublishToken)
	str("EARS_VIEWER_PASSCODE", &c.Auth.ViewerPasscode)
	str("EARS_ADMIN_TOKEN", &c.Auth.AdminToken)

	if v, ok := os.LookupEnv("EARS_HISTORY"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			c.Server.History = n
		}
	}
	if v, ok := os.LookupEnv("EARS_MAX_VIEWERS"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			c.Server.MaxViewersPerRoom = n
		}
	}
	if v, ok := os.LookupEnv("EARS_TRUST_PROXY"); ok {
		c.Server.TrustProxy = truthy(v)
	}
	if v, ok := os.LookupEnv("EARS_ALLOWED_ORIGINS"); ok {
		c.Auth.AllowedOrigins = splitAndTrim(v)
	}
	if v, ok := os.LookupEnv("EARS_ALLOW_AUTO_ROOMS"); ok {
		b := truthy(v)
		c.Auth.AllowAutoRooms = &b
	}
	// PORT is what most PaaS hosts inject.
	if v, ok := os.LookupEnv("PORT"); ok && v != "" {
		c.Server.Addr = ":" + v
	}
}

func (c *Config) validate() error {
	seen := make(map[string]bool, len(c.Rooms))
	for i, r := range c.Rooms {
		if r.ID == "" {
			return fmt.Errorf("rooms[%d]: id is required", i)
		}
		if !protocol.ValidRoomID(r.ID) {
			return fmt.Errorf("rooms[%d]: %q is not a valid room id (use a-z, 0-9, - and _)", i, r.ID)
		}
		if seen[r.ID] {
			return fmt.Errorf("rooms[%d]: duplicate room id %q", i, r.ID)
		}
		seen[r.ID] = true
	}
	if (c.Server.TLSCertFile == "") != (c.Server.TLSKeyFile == "") {
		return fmt.Errorf("tls_cert_file and tls_key_file must be set together")
	}
	return nil
}

// AutoRooms reports whether listeners may create rooms on the fly.
func (c Config) AutoRooms() bool {
	if c.Auth.AllowAutoRooms == nil {
		return true
	}
	return *c.Auth.AllowAutoRooms
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
