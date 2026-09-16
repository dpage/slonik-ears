package version

import "testing"

func TestStringDoesNotRepeatTheCommit(t *testing.T) {
	defer func(v, c string) { Version, Commit = v, c }(Version, Commit)

	Version, Commit = "016af4f", "016af4f"
	if got := String(); got != "016af4f" {
		t.Errorf("got %q, want the bare commit", got)
	}

	Version, Commit = "v1.2.0", "abc1234"
	if got := String(); got != "v1.2.0 (abc1234)" {
		t.Errorf("got %q", got)
	}
}
