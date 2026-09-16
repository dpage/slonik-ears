# Slonik Ears — live event transcription
#
#   make            build everything into ./bin
#   make demo       run a server and a fake listener, no model or microphone needed
#   make test       run the Go tests
#   make model      download a Whisper model

SHELL      := /bin/bash
GO         ?= go
NPM        ?= npm
BIN        := bin
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null)
LDFLAGS    := -X github.com/dpage/slonik-ears/internal/version.Version=$(VERSION) \
              -X github.com/dpage/slonik-ears/internal/version.Commit=$(COMMIT)

# Which model `make model` fetches. tiny.en (78 MB) through medium.en (1.5 GB);
# small.en is the usual compromise for live captioning on Apple silicon.
MODEL      ?= small.en
MODEL_DIR  ?= $(HOME)/.cache/whisper
MODEL_FILE := $(MODEL_DIR)/ggml-$(MODEL).bin
MODEL_URL  := https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-$(MODEL).bin

.PHONY: all build web server listener test race lint fmt vet tidy clean demo run-server run-listener model whisper-server help

all: build

## build: build the web app and both binaries
build: web server listener

## web: build the attendee web app into web/dist (embedded by the server)
web:
	cd web && $(NPM) ci --no-audit --no-fund && $(NPM) run build

## server: build the relay server (embeds whatever is in web/dist)
server:
	CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN)/ears-server ./cmd/ears-server

## listener: build the room listener (needs cgo for audio capture)
listener:
	CGO_ENABLED=1 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN)/ears-listener ./cmd/ears-listener

## test: run the Go tests
test:
	$(GO) test ./...

## race: run the Go tests under the race detector
race:
	$(GO) test -race ./...

## lint: vet the Go code, check formatting, and type check the web app
lint: vet
	@unformatted=$$(gofmt -l cmd internal web); \
	if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi
	cd web && $(NPM) run typecheck

vet:
	$(GO) vet ./...

fmt:
	gofmt -w cmd internal web

tidy:
	$(GO) mod tidy

## demo: a server plus a fake listener, so you can see the whole thing work
demo: server
	@echo "Starting a demo on http://localhost:8080 — press Ctrl-C to stop."
	@EARS_PUBLISH_TOKEN=demo-token EARS_EVENT_NAME="Slonik Ears demo" \
		./$(BIN)/ears-server --addr :8080 & \
	SERVER=$$!; \
	sleep 1; \
	$(GO) run ./cmd/ears-listener --room demo --title "Demo Room" --track "Demo" \
		--server http://localhost:8080 --token demo-token --mock --file testdata/sample.wav --loop; \
	kill $$SERVER

## run-server: run the server with transcripts persisted to ./data
run-server: server
	./$(BIN)/ears-server --data-dir ./data

## run-listener: run a listener against a local server and whisper-server
run-listener: listener
	./$(BIN)/ears-listener --room main-hall --server http://localhost:8080

## model: download a Whisper model into ~/.cache/whisper
model: $(MODEL_FILE)

$(MODEL_FILE):
	@mkdir -p $(MODEL_DIR)
	@echo "Fetching ggml-$(MODEL).bin ..."
	curl -L --progress-bar -o $(MODEL_FILE).part $(MODEL_URL)
	@mv $(MODEL_FILE).part $(MODEL_FILE)
	@echo "Saved to $(MODEL_FILE)"

## whisper-server: start whisper.cpp's server with the downloaded model
whisper-server: $(MODEL_FILE)
	whisper-server --model $(MODEL_FILE) --port 8081 --host 127.0.0.1 --threads 8

clean:
	rm -rf $(BIN) web/dist/assets web/dist/index.html

## help: list the targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | awk -F: '{printf "  \033[36m%-14s\033[0m%s\n", $$1, $$2}'
