package server

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// The annotated example in configs/ is what people copy, and nothing else
// reads it, so it can drift away from the code without anybody noticing. The
// two directions of drift need different checks: a key the struct no longer
// has is caught by LoadConfig, which decodes with KnownFields set, and a
// field the example never mentions is caught by walking the struct.
const serverExample = "../../configs/server.example.yaml"

func TestServerExampleConfigLoads(t *testing.T) {
	// LoadConfig applies the environment afterwards, and a developer's shell
	// may well have EARS_ variables exported for a real event. They have to
	// be removed rather than emptied, because applyEnv honours a variable
	// that is set to an empty string.
	for _, key := range []string{"EARS_PUBLISH_TOKEN", "EARS_ADDR", "EARS_DATA_DIR"} {
		if old, ok := os.LookupEnv(key); ok {
			if err := os.Unsetenv(key); err != nil {
				t.Fatalf("unset %s: %v", key, err)
			}
			t.Cleanup(func() { _ = os.Setenv(key, old) })
		}
	}

	cfg, err := LoadConfig(serverExample)
	if err != nil {
		t.Fatalf("the example config does not load: %v", err)
	}
	if cfg.Server.Addr == "" {
		t.Error("the example config left the listen address empty")
	}
}

func TestServerExampleConfigMentionsEveryOption(t *testing.T) {
	assertEveryYAMLKeyIsMentioned(t, reflect.TypeOf(Config{}), serverExample)
}

// assertEveryYAMLKeyIsMentioned walks a config struct and fails for any YAML
// key the example file never names. A key that is deliberately shown only as
// a commented-out line still counts as mentioned, because the point is that
// somebody copying the file can see the option exists.
func assertEveryYAMLKeyIsMentioned(t *testing.T, typ reflect.Type, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Base(path), err)
	}
	text := string(data)

	for _, key := range yamlKeys(typ, map[reflect.Type]bool{}) {
		// Matches "key:" at the start of a line, with or without a comment
		// marker in front of it, so a commented-out example counts.
		mentioned := regexp.MustCompile(`(?m)^[\t ]*#?[\t -]*` + regexp.QuoteMeta(key) + `:`)
		if !mentioned.MatchString(text) {
			t.Errorf("%s never mentions %q; add it, even as a commented-out line",
				filepath.Base(path), key)
		}
	}
}

// yamlKeys returns every yaml tag name in a struct and the structs it
// contains, following slices and pointers. The seen set stops a recursive
// type from looping.
func yamlKeys(typ reflect.Type, seen map[reflect.Type]bool) []string {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || seen[typ] {
		return nil
	}
	seen[typ] = true

	var keys []string
	for i := range typ.NumField() {
		field := typ.Field(i)
		tag, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
		if tag == "" || tag == "-" {
			continue
		}
		keys = append(keys, tag)
		keys = append(keys, yamlKeys(field.Type, seen)...)
	}
	return keys
}
