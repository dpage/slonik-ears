package listener

import (
	"flag"
	"io"
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
// has is caught by LoadFile, which decodes with KnownFields set, and a field
// the example never mentions is caught by walking the struct.
const listenerExample = "../../configs/listener.example.yaml"

func TestListenerExampleConfigLoads(t *testing.T) {
	cfg := DefaultConfig()

	// A throwaway flag set with nothing visited, so every value comes from
	// the file rather than from a command line.
	fs := flag.NewFlagSet("example", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cfg.BindFlags(fs)

	if err := cfg.LoadFile(listenerExample, fs); err != nil {
		t.Fatalf("the example config does not load: %v", err)
	}
	if cfg.Room == "" {
		t.Error("the example config left the room empty")
	}
}

func TestListenerExampleConfigMentionsEveryOption(t *testing.T) {
	assertEveryYAMLKeyIsMentioned(t, reflect.TypeOf(Config{}), listenerExample)
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
