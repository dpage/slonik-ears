# Slonik Ears

Live transcription for events. A listener sits in each room, hears the talk,
has it transcribed by a Whisper model, and streams the text to a server that
fans it out to anybody watching — on their own phone, or on a screen beside
the stage.

It is built for the conference case: several rooms at once, an audience on a
venue network that blocks devices from talking to each other, and nobody with
the time to debug a Python environment ten minutes before the keynote. The
whole thing is two Go binaries and a static web app.

```
   Room A                          Server                      Audience
 ┌──────────────────┐         ┌──────────────────┐         ┌────────────────┐
 │ ears-listener    │         │ ears-server      │         │ phone, laptop  │
 │  mic ─▶ VAD ─▶   │  text   │  rooms, history  │  text   │ stage screen   │
 │  whisper.cpp ────┼────────▶│  fan-out, web    ├────────▶│ /r/main-hall   │
 └──────────────────┘   WSS   └──────────────────┘   WSS   └────────────────┘
   Room B ──────────────────────────▲
   Room C ──────────────────────────┘        Run the server on the same laptop
                                             for one room, or on a cloud box
                                             when the venue's Wi-Fi isolates
                                             clients — which it usually does.
```

Only text crosses the network. Audio never leaves the room.

### Platforms

| | Runs on | Notes |
| --- | --- | --- |
| `ears-server` | Linux, macOS, Windows; amd64 and arm64 | pure Go, no cgo, no dependencies. Cross-compile it for an EC2 box with `make dist`. |
| `ears-listener` | macOS, Linux, Windows | needs cgo for audio capture: CoreAudio on macOS, ALSA/PulseAudio/JACK on Linux, WASAPI on Windows |

The listener is written with a Mac in each room in mind — that is where the
microphone permissions, the virtual audio devices and the Metal-accelerated
models are least trouble — but nothing about it is macOS-only. The server is
happiest on a small Linux instance well away from the venue's network.

## Try it in two minutes

No model, no microphone, no patience required:

```bash
make demo
```

That builds the server, starts it on <http://localhost:8080>, and runs a
listener with a fake transcriber against a sample recording. Open the page,
click into the demo room, and try the stage display.

## Running it for real

### 1. Install a Whisper backend

Transcription happens over HTTP against [whisper.cpp][whispercpp]'s own
server, so nothing needs to be linked into these binaries and there is no
Python anywhere:

```bash
brew install whisper-cpp          # provides whisper-server, whisper-cli, …
make model MODEL=small.en         # ~488 MB, into ~/.cache/whisper
make whisper-server               # starts it on 127.0.0.1:8081
```

Models, smallest first: `tiny.en` (78 MB), `base.en` (148 MB), `small.en`
(488 MB), `medium.en` (1.5 GB), and the multilingual `large-v3-turbo-q5_0`
(574 MB). Drop the `.en` for multilingual variants. On Apple silicon the
Homebrew build uses Metal, and `small.en` is the usual compromise between
accuracy and keeping up with a fast speaker — but measure it in your room
rather than trusting anybody's table of numbers, including this one:

```bash
ears-listener --room test --dry-run
```

`--dry-run` prints the transcript to the terminal instead of publishing it, so
you can hear what the model hears before an audience does.

### 2. Start the server

```bash
export EARS_PUBLISH_TOKEN=$(openssl rand -hex 24)
make server
./bin/ears-server --data-dir ./data
```

It prints the LAN addresses attendees can use. For a single room on one
machine, that is the whole job.

### 3. Start a listener in each room

```bash
./bin/ears-listener \
  --room main-hall --title "Main Hall" --track "Track A" \
  --server http://192.168.1.50:8080 \
  --token "$EARS_PUBLISH_TOKEN"
```

Each room gets its own `--room` id and its own listener; they all publish to
the same server, and the lobby fills in by itself. A listener that loses the
network keeps transcribing and buffers the text until it can reconnect.

### Where to watch

| Page | Who it is for |
| --- | --- |
| `/` | the lobby: every room, live ones first |
| `/r/main-hall` | attendees, on their own devices |
| `/r/main-hall/stage` | a screen beside the stage: huge text and a QR code |
| `/api/rooms/main-hall/transcript?format=txt` | the speaker, afterwards (also `srt`, `vtt`, `json`) |

The stage view takes `?lines=6`, `?size=120` (percent) and `?qr=0` if the
projector is smaller, larger or busier than the defaults assume.

## When the venue network fights back

Most conference Wi-Fi isolates clients, so attendees cannot reach a laptop on
stage no matter how many times you read out the IP address. The fix is to run
`ears-server` somewhere public and point every listener at it:

```bash
# On a small cloud box — the relay is text only, so it needs very little
EARS_PUBLISH_TOKEN=... EARS_BASE_URL=https://ears.example.org \
  ears-server --data-dir /var/lib/ears
```

Put a reverse proxy in front of it for TLS (`EARS_TRUST_PROXY=true`), or point
`tls_cert_file`/`tls_key_file` at a certificate and let the server do it.
There is a `Dockerfile` and a `docker-compose.yml` in `deploy/`.

Bandwidth is genuinely negligible, because only text crosses the network.
Measured against a live room with previews enabled, one viewer costs about
**160 bytes per second, or 0.6 MB per viewer-hour** over a compressed
WebSocket. A 300-person room running all day is therefore under 2 GB — inside
AWS's 100 GB per month free egress allowance, never mind the per-GB rate after
it.

The relay is idle between utterances and holds only text in memory, so the
smallest instance any provider sells is ample: a `t4g.nano` or equivalent, at
a few pounds a month. Do check current prices, and size for TLS handshakes and
concurrent sockets rather than for CPU.

## macOS notes

* **Microphone permission.** The first run triggers a permission prompt for
  whatever is running the binary — Terminal, iTerm, or the binary itself.
  Grant it under System Settings ▸ Privacy & Security ▸ Microphone. Without
  it, capture silently produces silence.
* **Capturing the room's PA rather than a laptop microphone** gives markedly
  better results. Either take a feed from the mixing desk into a USB audio
  interface, or install a virtual device such as BlackHole or Loopback and
  select it with `--device blackhole`. macOS will not let an application
  record system audio without one.
* **`--list-devices`** shows what is available, with the default marked.
* **Stop the Mac sleeping** for the duration: `caffeinate -dimsu` alongside the
  listener, or Settings ▸ Displays ▸ Advanced.
* **Running it automatically** — there is a LaunchAgent example in `deploy/`.
  It must be an agent rather than a daemon, because microphone access belongs
  to a logged-in session.

## Configuration

Both programs take flags, an optional YAML file, and environment variables, in
that order of precedence. Annotated examples live in `configs/`.

The variables worth knowing:

| Variable | Used by | Meaning |
| --- | --- | --- |
| `EARS_PUBLISH_TOKEN` | both | the secret a listener needs to publish |
| `EARS_VIEWER_PASSCODE` | server | optional passcode for attendees |
| `EARS_ADMIN_TOKEN` | server | guards the admin API |
| `EARS_BASE_URL` | server | public address, for QR codes and join links |
| `EARS_DATA_DIR` | server | where transcripts are written |
| `EARS_SERVER` | listener | which server to publish to |
| `EARS_WHISPER_URL` | listener | transcription endpoint |
| `EARS_API_KEY` | listener | bearer token for a cloud transcription endpoint |

### Using a cloud transcription service instead

The listener speaks the OpenAI-compatible transcription API, so any service
offering it will do — set `--whisper`, `--model` and `--api-key`. Worth
remembering that this sends the room's audio to a third party, which is a
conversation to have with the speakers and, depending on the audience, the
legal department. Local whisper.cpp keeps everything in the building.

## Security

* Publishing always requires a token; a server with no token configured
  refuses every listener rather than quietly accepting anybody on the network.
* Per-room tokens let each room's operator have a different secret.
* Watching is open by default, because that is normally the point. Set a
  viewer passcode for an internal event.
* WebSocket origins are restricted to the server's own host unless
  `allowed_origins` says otherwise.
* Transcripts are stored as plain JSONL under the data directory. They are a
  record of what people said in a room: treat them accordingly, and think
  about whether you want `--data-dir` set at all for a sensitive session.

## Accessibility

This is, in large part, an accessibility tool, so the attendee view tries to
behave:

* the transcript is an ARIA live region, announced politely rather than
  interrupting;
* text size, three themes (including high contrast) and timestamps are the
  reader's choice, and are remembered;
* scrolling follows the speaker but yields the moment the reader scrolls back,
  offering a way to return;
* animation respects `prefers-reduced-motion`;
* interim, unconfirmed text is visually distinct from committed text, so a
  guess is never mistaken for a quote.

Machine transcription is an aid, not a substitute for a human captioner where
one is required.

## How it fits together

| Package | What it does |
| --- | --- |
| `internal/protocol` | the JSON wire format, shared by both ends |
| `internal/audio` | capture, voice activity detection, utterance segmentation |
| `internal/asr` | the Whisper client, output cleaning, and a mock backend |
| `internal/listener` | the capture-to-text engine and the publishing client |
| `internal/hub` | rooms, transcript history, cursors, viewer fan-out |
| `internal/store` | JSONL persistence |
| `internal/server` | HTTP and WebSocket surface, auth, exports, QR codes |
| `web/` | the React and TypeScript attendee app, embedded into the server |

A few decisions worth knowing about:

* **Sequence numbers come from the server.** A viewer reconnecting sends the
  last one it saw and gets only what it missed, which matters on a phone that
  has been in somebody's pocket.
* **Interim results are explicitly provisional.** The listener re-transcribes
  the current utterance every so often for a live preview, then transcribes it
  properly once the speaker pauses. Only the second one is committed.
* **The tail of the transcript is fed back as context.** Whisper does much
  better on names and jargon when it knows what was just said.
* **Segments are queued locally when the network drops**, written to disk if
  `--transcript` is set, and flushed before shutdown.

## Development

```bash
make test      # Go tests
make race      # the same, under the race detector
make lint      # vet, gofmt, and the web app's type check
cd web && npm run dev    # the web app with hot reload, proxying to :8080
```

The server can be built without cgo; the listener needs it for audio capture
(`CGO_ENABLED=1`), which is handled by `make listener`. A cgo-free build of
the listener still works with `--file` for replaying recordings.

## Licence

The [PostgreSQL Licence](LICENSE) (SPDX: `PostgreSQL`), same as Postgres itself.

[whispercpp]: https://github.com/ggml-org/whisper.cpp
