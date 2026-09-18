package asr_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dpage/slonik-ears/internal/asr"
)

// captureFields records the multipart fields a request carried.
func captureFields(t *testing.T, cfg asr.WhisperConfig) map[string]string {
	t.Helper()
	got := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		for k, v := range r.MultipartForm.Value {
			if len(v) > 0 {
				got[k] = v[0]
			}
		}
		_, _ = io.WriteString(w, `{"text":"hello"}`)
	}))
	t.Cleanup(srv.Close)

	cfg.Endpoint = srv.URL
	c, err := asr.NewWhisper(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Transcribe(context.Background(), make([]float32, 1600), 16000, asr.Options{Language: "en"}); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestVADRequestsPlainJSON(t *testing.T) {
	// whisper.cpp 1.9.4 exits when asked for verbose_json with its own voice
	// detection enabled and no speech in the audio: it leaves zero segments,
	// then looks up a language that was never detected. The listener health
	// checks the backend with half a second of silence at start-up, so this
	// took the model server down before a word had been transcribed.
	got := captureFields(t, asr.WhisperConfig{VAD: true})
	if got["vad"] != "true" {
		t.Errorf("vad field is %q, want true", got["vad"])
	}
	if got["response_format"] != "json" {
		t.Errorf("response_format is %q; verbose_json crashes whisper.cpp when its VAD finds nothing", got["response_format"])
	}
}

func TestWithoutVADTheRicherFormatIsStillUsed(t *testing.T) {
	got := captureFields(t, asr.WhisperConfig{})
	if got["response_format"] != "verbose_json" {
		t.Errorf("response_format is %q, want verbose_json when VAD is off", got["response_format"])
	}
	if _, ok := got["vad"]; ok {
		t.Error("vad was sent when it was not asked for")
	}
}

func TestVADIsNotSentToACloudEndpoint(t *testing.T) {
	// Only whisper.cpp understands the field; a model name means this is an
	// OpenAI-compatible service, which would reject or ignore it.
	got := captureFields(t, asr.WhisperConfig{VAD: true, Model: "whisper-1"})
	if _, ok := got["vad"]; ok {
		t.Error("vad was sent to an OpenAI-compatible endpoint")
	}
	if got["response_format"] != "verbose_json" {
		t.Errorf("response_format is %q, want verbose_json for a cloud endpoint", got["response_format"])
	}
	if !strings.Contains(got["model"], "whisper-1") {
		t.Errorf("model is %q", got["model"])
	}
}
