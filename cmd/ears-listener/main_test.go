package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/dpage/slonik-ears/internal/listener"
)

func parakeetConfig(endpoint string) listener.Config {
	cfg := listener.DefaultConfig()
	cfg.Backend = "parakeet"
	cfg.Parakeet = endpoint
	cfg.FinalTimeoutSec = 2
	return cfg
}

func TestNewTranscriberParakeet(t *testing.T) {
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"text":""}`)); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	tr, err := newTranscriber(parakeetConfig("ws"+strings.TrimPrefix(srv.URL, "http")), log)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	if tr.Name() != "parakeet" {
		t.Errorf("Name = %q", tr.Name())
	}
}

func TestNewTranscriberParakeetUnreachable(t *testing.T) {
	// Take a free port and release it again, so nothing is listening there.
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := srv.Listener.Addr().String()
	srv.Close()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, err := newTranscriber(parakeetConfig("ws://"+addr), log)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "sudo systemctl start sherpa-onnx-server") {
		t.Errorf("the error should say how to start the server: %v", err)
	}
}
