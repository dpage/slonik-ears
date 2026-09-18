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

func TestCleanDropsHallucinatedMusic(t *testing.T) {
	// Straight from a room that had been reset and was sitting in silence:
	// large-v3 invented a lyric and marked it as music. The whole-string
	// annotation check could not see it, because with two ♪ spans there is
	// always a ♪ in the middle.
	for _, s := range []string{
		"♪ Waiting at the master ♪ ♪ Waiting at the master ♪",
		"♪ Waiting at the master ♪ ♪ Waiting at the master ♪ ♪ Waiting at the master ♪",
		"♪ Waiting at the master ♪",
		"♪♪♪",
		"♫ la la la ♫",
		"I can hear 🎵 something 🎵 now",
	} {
		if got := CleanTranscript(s); got != "" {
			t.Errorf("CleanTranscript(%q) = %q, want it discarded", s, got)
		}
	}
}

func TestCleanKeepsSpeechAroundAnAnnotation(t *testing.T) {
	// An annotation in the middle of a sentence is a gap in the speech, not a
	// reason to throw the sentence away.
	for _, tc := range []struct{ in, want string }{
		{"so we ran it [BLANK_AUDIO] overnight", "so we ran it overnight"},
		{"the results (applause) were good", "the results were good"},
		{"[BLANK_AUDIO] and then it worked", "and then it worked"},
		{"[BLANK_AUDIO] [BLANK_AUDIO]", ""},
		{"(applause) (laughter)", ""},
	} {
		if got := CleanTranscript(tc.in); got != tc.want {
			t.Errorf("CleanTranscript(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
