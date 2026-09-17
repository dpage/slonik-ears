package asr

import (
	"strings"
	"testing"
)

func TestDefaultVocabularyFitsItsOwnBudget(t *testing.T) {
	// A shipped default that trips its own truncation warning on first run
	// would be a poor advertisement for the warning.
	prompt, dropped := VocabularyPrompt(DefaultVocabulary)
	if len(dropped) > 0 {
		t.Errorf("the built-in glossary does not fit in %d characters; dropped %v", PromptBudget, dropped)
	}
	if len(prompt) > PromptBudget {
		t.Errorf("rendered prompt is %d characters, over the %d budget", len(prompt), PromptBudget)
	}
	for _, term := range []string{"pgEdge", "pg_stat_statements", "pgAdmin", "pg_dumpall"} {
		if !strings.Contains(prompt, term) {
			t.Errorf("%q is missing from the glossary, and it is one of the ones Whisper gets wrong", term)
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
