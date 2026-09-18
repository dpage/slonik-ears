package asr

import (
	"regexp"
	"strings"
	"unicode"
)

// Whisper models hallucinate when fed silence, applause or room noise. The
// same handful of phrases come up again and again — they are artefacts of the
// subtitle corpora the model was trained on. Rather than show an audience
// "Thanks for watching!" in the middle of a talk about replication, drop any
// chunk whose entire output is one of these.
//
// Only whole-output matches are dropped, so a speaker who genuinely says
// "thank you" mid-sentence is unaffected.
var junkOutputs = map[string]bool{
	"thank you":                     true,
	"thank you.":                    true,
	"thanks for watching":           true,
	"thanks for watching!":          true,
	"thank you for watching":        true,
	"thank you very much":           true,
	"please subscribe":              true,
	"subscribe to my channel":       true,
	"you":                           true,
	"bye":                           true,
	"bye bye":                       true,
	"okay":                          true,
	"ok":                            true,
	"so":                            true,
	"the":                           true,
	"oh":                            true,
	"um":                            true,
	"uh":                            true,
	"hmm":                           true,
	"mm":                            true,
	"mm-hmm":                        true,
	"transcription by castingwords": true,
}

// bracketed matches a whole-string non-speech annotation such as
// "[BLANK_AUDIO]", "(applause)" or "♪♪♪".
var bracketed = regexp.MustCompile(`^[\[\(\*♪<]+[^\]\)\*♪>]*[\]\)\*♪>]+$`)

// annotationSpan matches a non-speech annotation wherever it sits in a line,
// rather than only when it is the whole of it.
//
// Whisper does not confine itself to one per line, and the whole-string check
// above cannot see a second: its middle section is defined as containing no
// closing marker, and with two spans there always is one. So "[BLANK_AUDIO]
// [BLANK_AUDIO]" was caught but "♪ Waiting at the master ♪ ♪ Waiting at the
// master ♪" was not, and went up on a screen at the front of a room.
var annotationSpan = regexp.MustCompile(`[\[\(][^\]\)]*[\]\)]|♪[^♪]*♪`)

// musicalNote catches the markers Whisper uses for singing and music. Handed a
// silent room, a large model will invent song lyrics and label them as music;
// there is no conference transcript that wants them, because either the model
// is hallucinating or somebody is playing a video, and in neither case are the
// words the speaker's.
var musicalNote = regexp.MustCompile(`[♪♫♬♩🎵🎶]`)

// CleanTranscript tidies a raw model output and returns "" for anything that
// is plainly not speech from the room.
func CleanTranscript(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// whisper.cpp prefixes segments with a space; collapse all whitespace
	// (including the newlines that separate its internal segments) to single
	// spaces so the transcript flows as prose.
	s = strings.Join(strings.Fields(s), " ")

	// Strip whole-string non-speech annotations, including repeats such as
	// "[BLANK_AUDIO] [BLANK_AUDIO]".
	fields := strings.Fields(s)
	allBracketed := len(fields) > 0
	for _, f := range fields {
		if !bracketed.MatchString(f) {
			allBracketed = false
			break
		}
	}
	if allBracketed || bracketed.MatchString(s) {
		return ""
	}

	// Output with no letters or digits anywhere in it is not a transcript of
	// anything. large-v3 answers a chunk of room tone with a lone full stop
	// often enough to matter, and a line reading "." at the front of the hall
	// is worse than no line at all. Checked directly rather than through
	// normalise, which keeps hyphens on purpose and so would let "--" past.
	if !hasWordCharacter(s) {
		return ""
	}

	// Anything marked as music goes, whole line and all. The marked span is
	// not the speaker talking, and what surrounds it is invariably more of the
	// same hallucination rather than a sentence worth rescuing.
	if musicalNote.MatchString(s) {
		return ""
	}

	// Annotations elsewhere in the line are dropped and the real speech either
	// side of them kept: "so we ran it [BLANK_AUDIO] overnight" is a sentence
	// with a gap in it, not a non-speech event.
	if annotationSpan.MatchString(s) {
		s = strings.Join(strings.Fields(annotationSpan.ReplaceAllString(s, " ")), " ")
		if !hasWordCharacter(s) {
			return ""
		}
	}

	if junkOutputs[normalise(s)] {
		return ""
	}
	// A model stuck in a loop is worse than no transcript at all.
	if looksRepetitive(s) {
		return ""
	}
	return s
}

// looksRepetitive reports whether the text is mostly one short phrase said
// over and over, which is what whisper produces when it loses its footing —
// "so, so, so, so, so, so" and similar. Go's regexp has no backreferences, so
// this counts repeated n-grams directly.
func looksRepetitive(s string) bool {
	words := strings.Fields(strings.ToLower(s))
	for i := range words {
		words[i] = trimPunct(words[i])
	}
	if len(words) < 6 {
		return false
	}
	const minRuns = 6 // a phrase repeated this many times in a row is a loop
	for n := 1; n <= 4 && n*minRuns <= len(words); n++ {
		runs := 1
		for i := n; i+n <= len(words); i += n {
			if equalWords(words[i-n:i], words[i:i+n]) {
				runs++
				if runs >= minRuns {
					return true
				}
			} else {
				runs = 1
			}
		}
	}
	return false
}

func equalWords(a, b []string) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// hasWordCharacter reports whether there is anything in s that could be part
// of a spoken word.
func hasWordCharacter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// normalise lowercases and drops punctuation for junk comparison.
func normalise(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(strings.Join(strings.Fields(b.String()), " "))
}

func trimPunct(s string) string {
	return strings.TrimFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}
