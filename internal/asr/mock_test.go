package asr

import (
	"context"
	"math"
	"strings"
	"testing"
)

// speech returns audio loud enough for the mock to treat as speech.
func speech(seconds float64) []float32 {
	n := int(16000 * seconds)
	pcm := make([]float32, n)
	for i := range pcm {
		pcm[i] = float32(0.3 * math.Sin(float64(i)/10))
	}
	return pcm
}

// A live preview must grow into the segment that replaces it. Anything else
// makes the display flick to unrelated words mid-sentence, which is what a
// counter shared between previews and commits used to do.
func TestMockPartialIsAPrefixOfItsFinal(t *testing.T) {
	m := &Mock{}
	ctx := context.Background()

	first, err := m.Transcribe(ctx, speech(1.5), 16000, Options{Partial: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Transcribe(ctx, speech(2.5), 16000, Options{Partial: true})
	if err != nil {
		t.Fatal(err)
	}
	final, err := m.Transcribe(ctx, speech(3.0), 16000, Options{})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(second.Text, first.Text) {
		t.Errorf("the second preview should extend the first:\n  first:  %q\n  second: %q", first.Text, second.Text)
	}
	committed := strings.TrimSuffix(final.Text, ".")
	if !strings.HasPrefix(committed, second.Text) {
		t.Errorf("the committed segment should extend the last preview:\n  preview:   %q\n  committed: %q", second.Text, committed)
	}
}

// Successive utterances should read on from one another rather than repeating.
func TestMockAdvancesBetweenUtterances(t *testing.T) {
	m := &Mock{}
	ctx := context.Background()

	one, _ := m.Transcribe(ctx, speech(2.0), 16000, Options{})
	two, _ := m.Transcribe(ctx, speech(2.0), 16000, Options{})

	if one.Text == two.Text {
		t.Fatalf("two utterances produced identical text: %q", one.Text)
	}
	if strings.Fields(one.Text)[0] == strings.Fields(two.Text)[0] {
		t.Errorf("the cursor did not advance after a committed segment:\n  %q\n  %q", one.Text, two.Text)
	}
}

// Previews must not consume words the committed segment then skips.
func TestMockPreviewDoesNotAdvanceTheCursor(t *testing.T) {
	withPreview := &Mock{}
	ctx := context.Background()
	_, _ = withPreview.Transcribe(ctx, speech(1.0), 16000, Options{Partial: true})
	afterPreview, _ := withPreview.Transcribe(ctx, speech(2.0), 16000, Options{})

	noPreview := &Mock{}
	direct, _ := noPreview.Transcribe(ctx, speech(2.0), 16000, Options{})

	if afterPreview.Text != direct.Text {
		t.Errorf("a preview changed the committed text:\n  with preview: %q\n  without:      %q", afterPreview.Text, direct.Text)
	}
}
