package asr

import (
	"strings"
	"testing"
)

func TestDefaultVocabularyIsLongButWellOrdered(t *testing.T) {
	// The built-in glossary is deliberately far longer than the prompt can
	// hold, because the prompt and the rewrite have different appetites. What
	// matters is that the terms most worth prompting are near the top, where
	// they will actually get there.
	if len(DefaultVocabulary) < 200 {
		t.Errorf("the built-in glossary has only %d terms; it is meant to be comprehensive", len(DefaultVocabulary))
	}

	prompt, dropped := VocabularyPrompt(DefaultVocabulary)
	if len(prompt) > PromptBudget {
		t.Errorf("rendered prompt is %d characters, over the %d budget", len(prompt), PromptBudget)
	}
	if len(dropped) == 0 {
		t.Error("nothing was dropped, so either the glossary shrank or the budget grew")
	}

	// These are the ones Whisper reliably mangles into something an audience
	// notices, so they have to be in the part that reaches the model.
	for _, term := range []string{
		"pgEdge", "Spock", "PostgreSQL", "Postgres", "psql", "pgAdmin",
		"pg_dump", "pg_dumpall", "pg_stat_statements", "pg_stat_activity",
		"pg_hba.conf", "postgresql.conf", "pgBouncer", "pgBackRest",
		"logical replication", "Slonik Ears",
	} {
		if !strings.Contains(prompt, term) {
			t.Errorf("%q did not make it into the prompt; move it up vocabulary.txt", term)
		}
	}
}

func TestDefaultVocabularyIsEntirelyCoveredByTheRewrite(t *testing.T) {
	// Whatever misses the prompt is still corrected afterwards, which is the
	// reason a long list costs nothing.
	c := NewCanonicaliser(DefaultVocabulary)
	if c == nil {
		t.Fatal("no canonicaliser was built from the built-in glossary")
	}
	for _, term := range []string{"pg_stat_progress_vacuum", "shared_buffers", "pgRouting", "CloudNativePG"} {
		if got := c.Apply(strings.ToLower(strings.ReplaceAll(term, "_", " "))); got != term {
			t.Errorf("a term from the tail of the glossary was not corrected: got %q, want %q", got, term)
		}
	}
}

func TestDefaultVocabularyHasNoAmbiguousRewrites(t *testing.T) {
	// Postgres is full of acronyms that are also ordinary words. If any of
	// them could rewrite lower-case prose, a talk that mentions gin or toast
	// acquires index internals it never had.
	c := NewCanonicaliser(DefaultVocabulary)
	for _, s := range []string{
		"a gin and tonic, and some toast",
		"the hot aisle was warm",
		"I got the gist of it",
		"we sat in the seg unit",
		"the cube root of it",
		"a bloom filter, they said",
	} {
		if got := c.Apply(s); got != s {
			t.Errorf("Apply(%q) rewrote it to %q", s, got)
		}
	}
}

func TestParseVocabulary(t *testing.T) {
	terms := ParseVocabulary("# a comment\n\npgEdge\n  Spock  \npgEdge\n# another\nWAL\n")
	want := []string{"pgEdge", "Spock", "WAL"}
	if len(terms) != len(want) {
		t.Fatalf("got %v, want %v", terms, want)
	}
	for i := range want {
		if terms[i] != want[i] {
			t.Fatalf("got %v, want %v", terms, want)
		}
	}
}

func TestVocabularyPromptDropsWholeTermsAndReportsThem(t *testing.T) {
	long := make([]string, 0, 100)
	for i := 0; i < 100; i++ {
		long = append(long, "reasonably_long_identifier_number_"+string(rune('a'+i%26)))
	}
	prompt, dropped := VocabularyPrompt(long)
	if len(prompt) > PromptBudget {
		t.Errorf("prompt is %d characters, over the %d budget", len(prompt), PromptBudget)
	}
	if len(dropped) == 0 {
		t.Fatal("a hundred long terms fitted in the budget, which cannot be right")
	}
	// A truncated term would teach the model to misspell things, which is the
	// opposite of the point.
	for _, term := range strings.Split(strings.TrimSuffix(strings.TrimPrefix(prompt, "Glossary: "), "."), ", ") {
		if !strings.HasPrefix(term, "reasonably_long_identifier_number_") {
			t.Errorf("term %q was cut in half rather than dropped whole", term)
		}
	}
}

func TestVocabularyPromptIgnoresBlanks(t *testing.T) {
	prompt, _ := VocabularyPrompt([]string{"  ", "pgEdge", "", "  Spock  "})
	if prompt != "Glossary: pgEdge, Spock." {
		t.Errorf("got %q", prompt)
	}
}

func TestVocabularyPromptIsEmptyForNoTerms(t *testing.T) {
	if prompt, _ := VocabularyPrompt(nil); prompt != "" {
		t.Errorf("expected no prompt at all, got %q", prompt)
	}
}
