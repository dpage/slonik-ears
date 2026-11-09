package models

import (
	"slices"
	"strings"
	"testing"
)

func TestLookupKnownValues(t *testing.T) {
	m, err := Lookup("large-v3")
	if err != nil {
		t.Fatal(err)
	}
	if want := "64d182b440b98d5203c4f9bd541544d84c605196c4f7b845dfa11fb23594d1e2"; m.SHA256 != want {
		t.Errorf("large-v3 SHA256 = %s, want %s", m.SHA256, want)
	}
	if m.Size != 3095033483 {
		t.Errorf("large-v3 Size = %d, want 3095033483", m.Size)
	}

	m, err = Lookup("small.en")
	if err != nil {
		t.Fatal(err)
	}
	if want := "c6138d6d58ecc8322097e0f987c32f1be8bb0a18532a3f88f734d1bbf9c41e5d"; m.SHA256 != want {
		t.Errorf("small.en SHA256 = %s, want %s", m.SHA256, want)
	}

	v := VAD()
	if v.Name != "silero-v5.1.2" || v.File != "ggml-silero-v5.1.2.bin" {
		t.Errorf("VAD() = %s / %s, want silero-v5.1.2 / ggml-silero-v5.1.2.bin", v.Name, v.File)
	}
	if want := "29940d98d42b91fbd05ce489f3ecf7c72f0a42f027e4875919a28fb4c04ea2cf"; v.SHA256 != want {
		t.Errorf("VAD SHA256 = %s, want %s", v.SHA256, want)
	}
}

func TestURLsArePinned(t *testing.T) {
	pins := []string{
		"/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/",
		"/resolve/9ffd54a1e1ee413ddf265af9913beaf518d1639b/",
	}
	for _, name := range Names() {
		m, err := Lookup(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(m.URL, "/resolve/main/") {
			t.Errorf("%s: URL follows main: %s", name, m.URL)
		}
		if !slices.ContainsFunc(pins, func(p string) bool { return strings.Contains(m.URL, p) }) {
			t.Errorf("%s: URL is not pinned to a known commit: %s", name, m.URL)
		}
	}
}

func TestLookupUnknownListsNames(t *testing.T) {
	_, err := Lookup("huge")
	if err == nil {
		t.Fatal("expected an error for an unknown model")
	}
	for _, want := range []string{"large-v3", "tiny.en"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not list %s", err, want)
		}
	}
}

func TestNoLargeV1OrV2(t *testing.T) {
	for _, name := range Names() {
		if strings.HasPrefix(name, "large-v1") || strings.HasPrefix(name, "large-v2") {
			t.Errorf("unexpected model %s", name)
		}
	}
}

func TestNamesAreSorted(t *testing.T) {
	if names := Names(); !slices.IsSorted(names) {
		t.Errorf("Names() is not sorted: %v", names)
	}
}
