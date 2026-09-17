package asr

import (
	_ "embed"
	"fmt"
	"strings"
	"unicode"
)

// PromptBudget caps the rendered vocabulary, in characters.
//
// Whisper will only accept so much prompt: whisper.cpp keeps the last
// n_text_ctx/2 tokens, which is 224 for every model worth running, and quietly
// drops the rest. Technical identifiers tokenise badly, since "pg_dumpall"
// costs four or five tokens where an ordinary word costs one, so the practical
// ceiling is lower than a character count suggests. This leaves room for the
// tail of the transcript, which shares the same budget and is the more
// valuable of the two: a glossary fixes spellings, but recent context is what
// keeps sentences continuous across a chunk boundary.
const PromptBudget = 600

// defaultVocabularyFile is the built-in glossary, kept as a text file rather
// than a Go slice because it is a long list that people will want to read,
// copy and edit. It is embedded so the binary stays self-contained: a listener
// copied onto a machine in a room has no repository to read it from.
//
//go:embed vocabulary.txt
var defaultVocabularyFile string

// DefaultVocabulary is the glossary the listener uses when nothing else is
// configured. It exists because Whisper is confident and wrong about exactly
// the words a Postgres audience notices: left to itself it writes "PG Admin"
// for pgAdmin, "PG Start statements" for pg_stat_statements, "PG Dump Hall"
// for pg_dumpall and "PG Edge" for pgEdge.
//
// It is far longer than the prompt can hold, which is deliberate rather than
// careless. The list does two jobs with different appetites: the prompt takes
// what fits from the top and helps the model hear those terms, whilst the
// rewrite over the output has no such limit and covers every entry. So the
// file is ordered, most valuable first, and the tail costs nothing but still
// earns its keep.
var DefaultVocabulary = ParseVocabulary(defaultVocabularyFile)

// ParseVocabulary reads a glossary: one term per line, with blank lines and
// # comments ignored so a list can explain itself, and duplicates dropped so
// that a repeated term does not quietly consume prompt budget twice. Order is
// preserved, because order is what decides who gets into the prompt.
func ParseVocabulary(s string) []string {
	var terms []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || seen[line] {
			continue
		}
		seen[line] = true
		terms = append(terms, line)
	}
	return terms
}

// VocabularyPrompt renders a glossary into something a Whisper model will
// treat as context.
//
// A bare list of words works less well than a sentence that reads like the
// transcript it precedes, because the model is continuing text rather than
// consulting a dictionary, so the terms are presented as prose. Anything over
// PromptBudget is dropped at a term boundary and reported, since silently
// discarding half of somebody's glossary and letting them wonder why their
// product name is still spelt wrong would be unkind.
func VocabularyPrompt(terms []string) (prompt string, dropped []string) {
	var kept []string
	used := len("Glossary: .")
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		cost := len(term) + 2 // ", "
		if used+cost > PromptBudget {
			dropped = append(dropped, term)
			continue
		}
		used += cost
		kept = append(kept, term)
	}
	if len(kept) == 0 {
		return "", dropped
	}
	return fmt.Sprintf("Glossary: %s.", strings.Join(kept, ", ")), dropped
}

// Canonicaliser rewrites known terms in a transcript to their configured
// spelling.
//
// The prompt alone is not enough, and measurement is what settled that. A
// glossary reliably helps the model *hear* the right words: with one, "last
// right wins" became "last-write-wins" and "Stonakiers" became "Slonik Ears",
// which no amount of post-processing could have recovered. What it does not do
// reliably is *spell* them. The same glossary that fixed those two rendered
// pgEdge as "-pgedge", and a shorter one that got pgEdge right lost
// "last-write-wins" instead. Whisper is producing likely text, not consulting
// a dictionary, so it will keep making that class of mistake.
//
// Casing and spacing, though, can be fixed after the fact and without
// guesswork: "PG Edge", "pg edge", "PGEdge" and "-pgedge" all reduce to the
// same letters as pgEdge, so they can be mapped back to it exactly. What this
// cannot repair is a term the model misheard rather than misspelt, since "PG
// Dump Hall" has an h in it that pg_dumpall does not, and inventing a fuzzy
// match confident enough to bridge that gap would eventually rewrite something
// the speaker really said. Those remain the prompt's job.
//
// There is one more distinction to draw, and getting it wrong is how a
// spelling fixer starts damaging the transcript. A glossary holds two kinds of
// entry: identifiers, whose spelling is deliberate and should be imposed
// wherever they appear (pgEdge is never Pgedge, not even at the start of a
// sentence), and ordinary words and phrases that are only listed to help the
// model hear them (logical replication, failover, subscription). Imposing the
// glossary's lower case on the second kind rewrote "Logical replication
// between Postgres 18 and 19" as "logical replication between...", stripping
// the capital off the front of a sentence.
//
// So a term's spelling is imposed only when the term is identifier-like, or
// when the model has clearly broken one apart: "P SQL" spans two words where
// psql spans one, and no ordinary phrase does that by accident.
type Canonicaliser struct {
	// byLetters maps a term reduced to its bare letters and digits to the
	// spelling that should appear in the transcript.
	byLetters map[string]string
	// window is the longest term in words, and so how far ahead to look.
	window int
}

// allCaps reports whether a term contains capital letters and no small ones,
// which is how an acronym is told from a name.
func allCaps(term string) bool {
	return term != strings.ToLower(term) && term == strings.ToUpper(term)
}

// identifierLike reports whether a term's exact spelling is deliberate, rather
// than ordinary English that happens to be worth prompting the model with. An
// internal capital, an underscore, a digit, a dot, or being written entirely
// in capitals all say "this is a name, spell it this way".
//
// It judges each word separately, so that an ordinary Title Case phrase such
// as "Slonik Ears" is not mistaken for an identifier on account of the capital
// letter starting its second word.
func identifierLike(term string) bool {
	for _, word := range strings.Fields(term) {
		if word == strings.ToUpper(word) && word != strings.ToLower(word) {
			return true // WAL, MVCC, JSONB
		}
		for i, r := range word {
			switch {
			case r == '_' || r == '.' || unicode.IsDigit(r):
				return true
			case i > 0 && unicode.IsUpper(r):
				return true // pgEdge, TimescaleDB, PostGIS
			}
		}
	}
	return false
}

// NewCanonicaliser builds a rewriter for a glossary. It returns nil when there
// is nothing to do, which callers may use directly.
func NewCanonicaliser(terms []string) *Canonicaliser {
	c := &Canonicaliser{byLetters: make(map[string]string), window: 1}
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		key := letters(term)
		if key == "" {
			continue
		}
		// First spelling wins, so a glossary listing both "Postgres" and
		// "postgres" behaves predictably rather than by map iteration order.
		if _, seen := c.byLetters[key]; !seen {
			c.byLetters[key] = term
		}
		if n := lookahead(term); n > c.window {
			c.window = n
		}
	}
	if len(c.byLetters) == 0 {
		return nil
	}
	return c
}

// Apply rewrites any known term in s to its configured spelling, leaving
// everything else, punctuation included, exactly as the model produced it.
func (c *Canonicaliser) Apply(s string) string {
	if c == nil || s == "" {
		return s
	}
	words := strings.Split(s, " ")
	out := make([]string, 0, len(words))

	for i := 0; i < len(words); {
		matched := false
		// Longest match first, so "logical replication" is not shadowed by a
		// glossary that also contains "replication".
		for n := min(c.window, len(words)-i); n >= 1; n-- {
			phrase := words[i : i+n]
			canonical, ok := c.byLetters[letters(strings.Join(phrase, " "))]
			if !ok {
				continue
			}
			// Leave ordinary words as the model capitalised them unless it has
			// visibly split an identifier apart.
			if !identifierLike(canonical) && n == len(strings.Fields(canonical)) {
				break
			}
			// An all-capitals term never rewrites text containing lower case.
			// Postgres has a fine collection of acronyms that are also
			// ordinary words, and without this rule a glossary listing GIN,
			// HOT and TOAST would turn gin, hot and toast into index
			// internals wherever anybody said them. Somebody who writes
			// "M.V.C.C." still gets MVCC, because that has no lower case in
			// it either.
			if allCaps(canonical) && strings.ToUpper(strings.Join(phrase, " ")) != strings.Join(phrase, " ") {
				break
			}
			// Keep whatever punctuation surrounded the phrase: an opening
			// quote or a closing comma is the model's business, not ours.
			out = append(out, leadingPunct(phrase[0])+canonical+trailingPunct(phrase[n-1]))
			i += n
			matched = true
			break
		}
		if !matched {
			out = append(out, words[i])
			i++
		}
	}
	return strings.Join(out, " ")
}

// maxLookahead bounds the search window, since the cost of widening it is
// paid on every word of every transcript.
const maxLookahead = 6

// lookahead estimates how many transcript words a term might be spread over.
//
// It cannot be the term's word count: "pg_stat_statements" is one word, and
// the model writes it as three. Counting the pieces an identifier is built
// from gets much closer, and one more is added for a model that also splits a
// piece in half, which is how "hot standby" arrives as "hot stand by". Being
// generous here is cheap and safe, because a match still requires the letters
// to agree exactly.
func lookahead(term string) int {
	pieces := 1
	var prev rune
	for i, r := range term {
		switch {
		case !unicode.IsLetter(r) && !unicode.IsDigit(r):
			pieces++
		case i > 0 && unicode.IsUpper(r) && unicode.IsLower(prev):
			pieces++ // a camelCase join, as in pgEdge
		}
		prev = r
	}
	return min(pieces+1, maxLookahead)
}

// letters reduces a term to its lower-case letters and digits, which is the
// form in which "PG Edge" and "pgEdge" are the same thing.
func letters(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// leadingPunct returns the run of punctuation at the start of a word, less any
// stray hyphen: Whisper prefixes a term with one often enough ("-pgedge") that
// keeping it would defeat the point of the rewrite.
func leadingPunct(word string) string {
	runes := []rune(word)
	i := 0
	for i < len(runes) && !unicode.IsLetter(runes[i]) && !unicode.IsDigit(runes[i]) {
		i++
	}
	return strings.TrimLeft(string(runes[:i]), "-")
}

// trailingPunct returns the run of punctuation at the end of a word.
func trailingPunct(word string) string {
	runes := []rune(word)
	i := len(runes)
	for i > 0 && !unicode.IsLetter(runes[i-1]) && !unicode.IsDigit(runes[i-1]) {
		i--
	}
	return string(runes[i:])
}
