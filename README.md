# Slonik Ears

[![CI](https://github.com/dpage/slonik-ears/actions/workflows/ci.yml/badge.svg)](https://github.com/dpage/slonik-ears/actions/workflows/ci.yml)
[![Container image](https://github.com/dpage/slonik-ears/actions/workflows/docker.yml/badge.svg)](https://github.com/dpage/slonik-ears/actions/workflows/docker.yml)

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

## Running it for real on a Mac

The whole thing is three processes: a Whisper model server, the relay, and one
listener per room. This walkthrough starts all three on one Mac. Copy and
paste it in order.

### Prerequisites

```bash
brew install go node whisper-cpp
```

`whisper-cpp` is the speech-to-text engine, and the formula installs the HTTP
server that everything below depends on, so `whisper-server` is on your `PATH`
once the install finishes:

```bash
whisper-server --help | head -3
```

If you would rather build [whisper.cpp][whispercpp] from source, put its
`build/bin/whisper-server` on your `PATH` instead.

### Build Slonik Ears

```bash
git clone https://github.com/dpage/slonik-ears.git
cd slonik-ears
make build            # web app, server and listener into ./bin
```

### Fetch a model

```bash
make model MODEL=large-v3     # ~2.9 GB, into ~/.cache/whisper
```

Smallest first: `tiny.en` (78 MB), `base.en` (148 MB), `small.en` (488 MB),
`medium.en` (1.5 GB), `large-v3-turbo` (1.5 GB) and `large-v3` (2.9 GB). Drop
the `.en` for the multilingual variants; the `-q5_0` builds are quantised and
roughly half the size.

**On a recent Mac, use `large-v3`.** Measured on an M2 Max with 30 GPU cores,
replaying a recording of a real talk and scoring against twenty phrases that
were actually spoken:

| model | phrases right | mean speed | slowest chunk |
| --- | --- | --- | --- |
| `small.en` | 17/20 | 9.9× | 0.4× |
| `large-v3-turbo` | 18/20 | 4.0× | 0.4× |
| `large-v3` | **19/20** | 3.9× | **1.0×** |

Speed here is audio duration over time taken, so 3.9× means the model keeps up
with a speaker nearly four times over. `large-v3` was never slower than real
time on that machine, and it was the only one of the three to produce "live
transcript" rather than "life transcript", and "commit timestamps" rather than
"commit tim estamps". The turbo variant is the surprise: no faster than the
full model in practice, and it replaced a quietly spoken sentence with a
repetition of its own prompt, which is the worst way for a transcript to be
wrong.

Smaller machines are a different question. A Raspberry Pi has no GPU worth the
name and a fraction of the memory bandwidth, so `tiny.en` or `base.en` are the
realistic choices there. Whatever you pick, watch the `speed=` figure the
listener logs for each committed segment: below 1 means the model is slower
than the speaker and the queue only grows. `--no-partials` roughly halves the
work if it is marginal.

Measure it in your own room rather than trusting anybody's table of numbers,
this one included: it is one machine, one microphone and one voice.

### Make a publish token

The relay and the listener are separate processes in separate terminals, and
both need the same secret. Generate it once and put it somewhere both can read:

```bash
openssl rand -hex 24 > ~/.ears-token
chmod 600 ~/.ears-token
```

### Terminal 1 — the model server

```bash
whisper-server \
  --model ~/.cache/whisper/ggml-large-v3.bin \
  --host 127.0.0.1 --port 8081 \
  --threads 8
```

**The port matters.** `whisper-server` also defaults to 8080, which is where
the relay wants to live, so give it 8081 explicitly or the second process to
start will fail to bind. `make whisper-server` runs exactly this command.

Leave it running. It loads the model once and then answers requests.

### Terminal 2 — the relay and web app

```bash
cd slonik-ears
export EARS_PUBLISH_TOKEN=$(cat ~/.ears-token)
export EARS_EVENT_NAME="PGConf Europe 2026"

./bin/ears-server --data-dir ./data
```

It prints where attendees can reach it:

```
time=... level=INFO msg="Slonik Ears server started" addr=[::]:8080 rooms_configured=0 auto_rooms=true

  Attendees can watch at:
    http://192.168.1.50:8080
```

Note that address — the listener and the audience both need it. Leave this
running too.

### Terminal 3 — the listener in the room

First, find the microphone and check the model can hear it:

```bash
cd slonik-ears
export EARS_PUBLISH_TOKEN=$(cat ~/.ears-token)

./bin/ears-listener --list-devices
```

```
Capture devices (use --device with the index or part of the name):
  * 0  MacBook Pro Microphone
    1  Scarlett Solo USB
    2  BlackHole 2ch
```

Now rehearse without publishing anything. Say a few sentences; they should
appear in the terminal:

```bash
./bin/ears-listener --room rehearsal --dry-run --device "Scarlett"
```

**macOS will ask for microphone permission the first time**, and it asks on
behalf of whatever is running the command — Terminal, iTerm, or the binary
itself. Grant it under System Settings ▸ Privacy & Security ▸ Microphone. If
you skip this, capture silently records digital silence and you get no
transcript at all.

Happy with what it hears? Stop it with Ctrl-C and go live:

```bash
./bin/ears-listener \
  --room main-hall \
  --title "Main Hall" \
  --track "Track A" \
  --speaker "Dave Page" \
  --device "Scarlett" \
  --server http://192.168.1.50:8080 \
  --transcript ~/transcripts/main-hall.jsonl
```

It picks up `EARS_PUBLISH_TOKEN` from the environment, and defaults to the
Whisper server on `127.0.0.1:8081`. `--transcript` keeps a local copy of every
committed line, which is your insurance against the network.

### Watch it

Open <http://192.168.1.50:8080> on a phone on the same network, or put the
stage display on a screen beside the speaker:

| Page | Who it is for |
| --- | --- |
| `/` | the lobby: every room, live ones first |
| `/r/main-hall` | attendees, on their own devices |
| `/r/main-hall/stage` | a screen beside the stage: huge text and a QR code |
| `/api/rooms/main-hall/transcript?format=txt` | scripted access afterwards (also `srt`, `vtt`, `json`) |

The stage view takes `?lines=6`, `?size=120` (percent) and `?qr=0` if the
projector is smaller, larger or busier than the defaults assume.

### Keep the Mac awake

A machine that sleeps mid-talk stops transcribing. In a fourth terminal, or
before the listener:

```bash
caffeinate -dimsu
```

### More than one room

One listener per room, all pointing at the same relay, each with its own
`--room` id. On the room's own Mac:

```bash
./bin/ears-listener --room seminar-1 --title "Seminar Room 1" --track "Track B" \
  --server http://192.168.1.50:8080
```

Each room needs its own `whisper-server` on its own machine — the model is the
expensive part, and one server will not keep up with several rooms at once.
The relay handles as many rooms as you have listeners; the lobby fills in by
itself as each one connects.

### Stopping

Ctrl-C each terminal. The listener finishes transcribing whatever was being
said, flushes it to the relay, and then exits — so the last sentence of the
talk is not lost. Transcripts stay in `./data` on the relay and in whatever
you passed to `--transcript`.

If it does not stop promptly, press Ctrl-C again and it exits immediately.
That is worth knowing before you need it: a first Ctrl-C shuts down tidily,
which takes a moment when a transcription is still running, and a second one
gives up on tidiness. Either way the listener says what it is waiting for
rather than sitting there silently.

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

## Getting good audio on a Mac

The walkthrough above will work with the built-in microphone, and the results
will be mediocre — a laptop on a lectern hears the room, not the speaker.
Better, in ascending order:

* **A feed from the mixing desk** into a USB audio interface. This is the best
  option by a distance: it is the same signal the PA is amplifying, with no
  room acoustics and no audience in it. Select it with `--device "Scarlett"`
  or whatever `--list-devices` calls it.

  **On a multichannel interface, check which input the listener picked.** A
  device like a TASCAM US-16x08 offers sixteen channels and no mono format at
  all, so it matters which one is read. The listener measures every channel at
  start-up, uses whichever is loudest, and says so:

  ```
  msg="input channels measured" device_channels=16 using=[1] ch1=0.084
  ```

  Override it with `--channel 3` if the microphone is on input 3 and the room
  happened to be quiet whilst it was deciding, or `--channel 1,2` to average a
  genuine stereo pair. It also warns when an input is clipping or nearly
  silent, which is worth reading before blaming the transcript:

  ```
  msg="the input is clipping: turn the gain down on the interface" channel=1 peak=1.015
  ```

  Aim for a peak somewhere around 0.5. Clipped speech transcribes noticeably
  worse and nothing downstream can put back what the converter discarded.
* **A lapel or headset microphone** into the same interface.
* **A virtual device** such as BlackHole or Loopback, if the audio you want is
  already playing on the Mac — a video call, or a media player. macOS will not
  let an application record system output without one. Install it, route the
  audio to it, and use `--device blackhole`.

Two things that cost transcripts if forgotten, both covered in the walkthrough
but worth repeating because they are silent failures: **microphone permission**
belongs to the application running the binary, and a **sleeping Mac** stops
transcribing.

**Running a listener automatically** — there is a LaunchAgent example in
`deploy/`. It must be an agent rather than a daemon, because microphone access
belongs to a logged-in session.

## Teaching it your jargon

Whisper is confident and wrong about exactly the words a technical audience
notices. Left to itself it writes "PG Admin" for pgAdmin, "PG Start statements"
for pg_stat_statements, "PG Dump Hall" for pg_dumpall and "PG Edge" for
pgEdge, which is the sort of thing that makes a transcript look unreliable even
where it is accurate.

The listener carries a glossary to deal with it, used in two ways: the terms go
to the model as context, which measurably helps it *hear* the right words, and
they are also applied to the output afterwards, which makes it *spell* them
consistently. Both are needed, because prompting alone is unreliable in an
interesting way: one glossary fixed "last right wins" into "last-write-wins"
whilst rendering pgEdge as "-pgedge", and a shorter one got pgEdge right and
lost the other. The rewrite is exact rather than fuzzy, so it corrects casing
and spacing ("PG Edge", "pg edge", "-pgedge" all become pgEdge) and leaves
alone anything that merely sounds similar.

A Postgres glossary is built in, so the default behaviour is usually what you
want. To see it, change it, or replace it:

```bash
ears-listener --print-vocabulary > vocabulary.txt   # start from the built-in list
$EDITOR vocabulary.txt                              # one term per line, # for comments
ears-listener --room main-hall --vocabulary vocabulary.txt ...
```

The same thing can live in the config file as `vocabulary_file`, or inline as
`vocabulary:`. Any of those replaces the built-in list rather than adding to
it, so the glossary in force is always exactly what `--print-vocabulary`
prints. `--no-vocabulary` turns it off for an event that is not about
databases.

**Order matters, and the list is longer than the prompt.** The built-in
glossary runs to several hundred terms, covering the client programs, the
system catalogues and statistics views, the common extensions, the pooling and
high-availability tooling, the configuration parameters people say out loud and
a good deal of the vocabulary of a replication or performance talk. The model
will only accept a few hundred characters of prompt, though, so the prompt is
filled from the top of the list downwards and the rest is corrected in the
output only. That is why the list is ordered rather than alphabetical: the
terms said constantly and mangled reliably are at the top. The listener says
which way the split fell when it starts:

```
msg="glossary is longer than the model's prompt allows" in_prompt=48 corrected_only_afterwards=299
```

If something of yours is coming out *misheard* rather than merely misspelt,
move it nearer the top of your own copy. If it is only misspelt, its position
does not matter.

Ordinary words are left to the model's own capitalisation, so "logical
replication" in the glossary cannot strip the capital off a sentence beginning
with it, and terms written in capitals never rewrite lower-case prose, which is
what stops a glossary containing GIN, HOT and TOAST from turning gin, hot and
toast into index internals.

## Configuration

Both programs take flags, an optional YAML file, and environment variables, in
that order of precedence. Annotated examples live in `configs/`.

The variables worth knowing:

| Variable | Used by | Meaning |
| --- | --- | --- |
| `EARS_PUBLISH_TOKEN` | both | the secret a listener needs to publish |
| `EARS_VIEWER_PASSCODE` | server | optional passcode for attendees |
| `EARS_ADMIN_TOKEN` | server | guards the admin API and the organiser page at `/admin`; unset means both are off |
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
* **The voice detector adapts downwards readily and upwards reluctantly**, and
  not at all until the room has been quiet for half a second. The quiet frames
  within a sentence are not silence, and letting them into the estimate lets a
  speaker gradually talk themselves into being ignored. `docs/OPERATING.md`
  covers tuning it against a recording of the room you will actually use.

## Development

```bash
make test      # Go tests
make race      # the same, under the race detector
make lint      # gofmt, go vet, golangci-lint, and the web app's type check
make vuln      # govulncheck and npm audit
make smoke     # drive the attendee views in a real browser
cd web && npm run dev    # the web app with hot reload, proxying to :8080
```

`make lint`, `make vuln` and `make smoke` run what CI runs, so a green local
run means a green pull request.

The server can be built without cgo; the listener needs it for audio capture
(`CGO_ENABLED=1`), which is handled by `make listener`. A cgo-free build of
the listener still works with `--file` for replaying recordings.

### What CI does

| Workflow | When | What |
| --- | --- | --- |
| `ci.yml` | every push and pull request | gofmt, vet, golangci-lint; tests under the race detector on Linux and macOS; `govulncheck` and `npm audit`; the web build; and an end-to-end run that publishes a transcript through a real server and then drives the attendee views in a real browser |
| `docker.yml` | main, tags, and changes to the Dockerfile | builds the image, runs it, checks it serves the app and is not running as root, then publishes a multi-architecture image to GHCR |
| `release.yml` | a `v*` tag | cross-compiles the server for Linux, macOS and Windows, builds the listener natively on each platform that needs cgo, and attaches tarballs and `SHA256SUMS` to a GitHub release |

Dependabot groups its updates weekly, so a quiet week produces one pull
request rather than nine.

### Cutting a release

```bash
git tag -a v0.1.0 -m "First release"
git push origin v0.1.0
```

`release.yml` does the rest. Run it from the Actions tab first if you want to
check the packaging without publishing anything — a manual run builds
everything and stops short of creating the release.

## Licence

The [PostgreSQL Licence](LICENSE) (SPDX: `PostgreSQL`), same as Postgres itself.

[whispercpp]: https://github.com/ggml-org/whisper.cpp
