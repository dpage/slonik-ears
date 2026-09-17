package listener

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpage/slonik-ears/internal/asr"
)

func TestResolveVocabularyPrecedence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vocabulary.txt")
	const content = `# A glossary for a talk about something else entirely
Kafka

  Debezium
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		cfg  Config
		want []string
	}{
		{
			name: "the built-in glossary when nothing is configured",
			cfg:  Config{},
			want: asr.DefaultVocabulary,
		},
		{
			name: "an inline list replaces the built-in one",
			cfg:  Config{Vocabulary: []string{"Spock"}},
			want: []string{"Spock"},
		},
		{
			name: "a file wins over an inline list",
			cfg:  Config{Vocabulary: []string{"Spock"}, VocabularyFile: path},
			want: []string{"Kafka", "Debezium"},
		},
		{
			name: "turning it off beats everything",
			cfg:  Config{Vocabulary: []string{"Spock"}, VocabularyFile: path, NoVocabulary: true},
			want: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.cfg.ResolveVocabulary()
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestResolveVocabularyReportsAUselessFile(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, []byte("# nothing but a comment\n\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Falling back to the built-in list here would be worse than failing: the
	// operator asked for their own glossary and would have no way of knowing
	// they were not getting it.
	if _, err := (Config{VocabularyFile: empty}).ResolveVocabulary(); err == nil {
		t.Error("a vocabulary file with no terms in it was accepted silently")
	}
	if _, err := (Config{VocabularyFile: filepath.Join(dir, "nope.txt")}).ResolveVocabulary(); err == nil {
		t.Error("a missing vocabulary file was accepted silently")
	}
}

func TestPromptPutsTheGlossaryBeforeTheTranscript(t *testing.T) {
	// The model continues from the end of the prompt, so the recent transcript
	// has to be the last thing it reads: that is what carries a sentence
	// across a chunk boundary. The glossary only needs to be present.
	newEngine := func(t *testing.T, terms []string) *Engine {
		t.Helper()
		cfg := DefaultEngineConfig()
		cfg.Vocabulary = terms
		cfg.Logger = testLogger()
		e, err := NewEngine(cfg, newFakeSource(nil), &asr.Mock{}, &capturePublisher{})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}

	e := newEngine(t, []string{"pgEdge"})
	if got := e.currentPrompt(); got != "Glossary: pgEdge." {
		t.Errorf("with no transcript yet, got %q", got)
	}

	e.pushPrompt("replication lag is manageable")
	if got, want := e.currentPrompt(), "Glossary: pgEdge. replication lag is manageable"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	bare := newEngine(t, nil)
	bare.pushPrompt("no glossary here")
	if got := bare.currentPrompt(); got != "no glossary here" {
		t.Errorf("without a glossary, got %q", got)
	}
}
