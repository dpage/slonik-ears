package asr

import "testing"

func TestCanonicaliseFixesWhatWhisperActuallyProduced(t *testing.T) {
	// Every "got" here came out of whisper.cpp on a real recording, which is
	// why they are an odd-looking set: "-pgedge" with a leading hyphen is not
	// the sort of mistake you would think to invent.
	c := NewCanonicaliser(DefaultVocabulary)
	for _, tc := range []struct{ got, want string }{
		{"PG Edge runs multi-master replication.", "pgEdge runs multi-master replication."},
		{"-pgedge runs multi-master replication", "pgEdge runs multi-master replication"},
		{"pgedge and PGEdge and pg edge", "pgEdge and pgEdge and pgEdge"},
		{"We use PG Admin for that.", "We use pgAdmin for that."},
		{"Check PG stat statements first.", "Check pg_stat_statements first."},
		{"the P SQL prompt", "the psql prompt"},
		{"It uses hot stand by, then fails over.", "It uses hot standby, then fails over."},
		{"Check PG HBA conf too.", "Check pg_hba.conf too."},
	} {
		if got := c.Apply(tc.got); got != tc.want {
			t.Errorf("Apply(%q)\n got %q\nwant %q", tc.got, got, tc.want)
		}
	}
}

func TestCanonicaliseLeavesOrdinarySpeechAlone(t *testing.T) {
	// The rewrite has to be timid. Anything that reduces to different letters
	// is a different word, and a transcript that quietly edits what somebody
	// said is worse than one with a misspelling in it.
	c := NewCanonicaliser(DefaultVocabulary)
	for _, s := range []string{
		"I walked into a wall in the hall.",
		"The dump all went well, apparently.",
		"Postgres's own documentation says otherwise.",
		"We talked about partitioned tables and vacuuming.",
		"That is a subscription service, not a database.",
		"",
	} {
		if got := c.Apply(s); got != s {
			t.Errorf("Apply(%q) rewrote it to %q", s, got)
		}
	}
}

func TestCanonicaliseLeavesSentenceCapitalsAlone(t *testing.T) {
	// The glossary lists "logical replication" and "failover" in lower case
	// because that is how they are written mid-sentence, but they are ordinary
	// English rather than names. Imposing the glossary's case on them took the
	// capital off the front of a sentence, which an audience notices
	// immediately and which is worse than the problem being solved.
	c := NewCanonicaliser(DefaultVocabulary)
	for _, s := range []string{
		"Logical replication between Postgres 18 and Postgres 19.",
		"Failover happened before anybody noticed.",
		"Subscription state is worth checking.",
		"Partitioning helps here.",
	} {
		if got := c.Apply(s); got != s {
			t.Errorf("Apply(%q) rewrote it to %q", s, got)
		}
	}

	// Identifiers are a different matter: their spelling is deliberate and is
	// imposed wherever they appear, start of sentence included.
	for _, tc := range []struct{ got, want string }{
		{"PG Edge is lower case even here.", "pgEdge is lower case even here."},
		{"PG stat statements tells you.", "pg_stat_statements tells you."},
	} {
		if got := c.Apply(tc.got); got != tc.want {
			t.Errorf("Apply(%q)\n got %q\nwant %q", tc.got, got, tc.want)
		}
	}
}

func TestIdentifierLike(t *testing.T) {
	for _, term := range []string{"pgEdge", "pgAdmin", "pg_dumpall", "WAL", "MVCC", "pg_hba.conf", "TimescaleDB", "Postgres 18"} {
		if !identifierLike(term) {
			t.Errorf("%q should be treated as an identifier", term)
		}
	}
	for _, term := range []string{"logical replication", "failover", "subscription", "Postgres", "Slonik Ears", "psql"} {
		if identifierLike(term) {
			t.Errorf("%q should be treated as ordinary words", term)
		}
	}
}

func TestCanonicalisePrefersTheLongestTerm(t *testing.T) {
	// "pg_stat" must not claim the first two words and leave "statements"
	// stranded behind it.
	c := NewCanonicaliser([]string{"pg_stat", "pg_stat_statements"})
	if got, want := c.Apply("Run PG stat statements now"), "Run pg_stat_statements now"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCanonicaliserIsUsableWhenEmpty(t *testing.T) {
	// A nil canonicaliser is what --no-vocabulary produces, and it has to be
	// safe to call rather than something every caller must check.
	var c *Canonicaliser
	if got := c.Apply("left exactly as it was"); got != "left exactly as it was" {
		t.Errorf("got %q", got)
	}
	if NewCanonicaliser(nil) != nil {
		t.Error("an empty glossary should produce no canonicaliser at all")
	}
	if NewCanonicaliser([]string{"  ", ""}) != nil {
		t.Error("a glossary of blanks should produce no canonicaliser at all")
	}
}
