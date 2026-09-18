package asr

import "testing"

func TestCleanTranscriptDropsJunk(t *testing.T) {
	junk := []string{
		"[BLANK_AUDIO]",
		"[ Silence ]",
		"(applause)",
		"♪♪♪",
		"Thank you.",
		"Thanks for watching!",
		"  you  ",
		"so so so so so so so so",
		"[BLANK_AUDIO] [BLANK_AUDIO]",
	}
	for _, in := range junk {
		if got := CleanTranscript(in); got != "" {
			t.Errorf("CleanTranscript(%q) = %q, want it dropped", in, got)
		}
	}
}

func TestCleanTranscriptKeepsSpeech(t *testing.T) {
	keep := map[string]string{
		"  Hello   and welcome\nto the talk. ":      "Hello and welcome to the talk.",
		"Thank you for coming, let us begin.":       "Thank you for coming, let us begin.",
		"So, the thing about replication slots is…": "So, the thing about replication slots is…",
		"Yes, yes, that is right.":                  "Yes, yes, that is right.",
	}
	for in, want := range keep {
		if got := CleanTranscript(in); got != want {
			t.Errorf("CleanTranscript(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCommonPrefixWords(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"the quick brown fox", "the quick brown dog", 3},
		{"the quick", "The Quick!", 2},
		{"", "anything", 0},
		{"same", "same", 1},
	}
	for _, c := range cases {
		if got := CommonPrefixWords(c.a, c.b); got != c.want {
			t.Errorf("CommonPrefixWords(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestCleanDropsOutputWithNoWordsInIt(t *testing.T) {
	// large-v3 answers trailing room tone with a lone full stop, and the
	// stage display would otherwise show it as a line of transcript.
	for _, s := range []string{".", ". .", "...", " -- ", "?!", "…", ","} {
		if got := CleanTranscript(s); got != "" {
			t.Errorf("CleanTranscript(%q) = %q, want it discarded", s, got)
		}
	}
	// Anything with a word or a number in it is still speech.
	for _, s := range []string{"No.", "18.", "A.", "pg_dump."} {
		if CleanTranscript(s) == "" {
			t.Errorf("CleanTranscript(%q) discarded real speech", s)
		}
	}
}
