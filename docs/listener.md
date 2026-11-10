# Running a Listener

One listener runs in each room. It captures the audio, has it
transcribed, and publishes the text to the server. This page covers
starting one, describing the room, and the local safety net.

## Starting a listener

A listener needs a room identifier, the server address, and the publish
token. The token is read from the environment so it stays out of shell
history:

```bash
export EARS_PUBLISH_TOKEN=$(cat ~/.ears-token)
ears-listener \
  --room main-hall \
  --title "Main Hall" \
  --track "Track A" \
  --speaker "Alex Roe" \
  --device "Scarlett" \
  --server https://ears.example.org \
  --transcript ~/transcripts/main-hall.jsonl
```

Room identifiers appear in URLs, so keep them short and memorable. They
may contain lower case letters, digits, hyphens and underscores.

## Describing the room

Three settings control what attendees see, and all three can be changed
later from the organiser's page without restarting anything:

- `--title` is the room name shown at the top of the page.
- `--track` groups rooms together in the lobby.
- `--speaker` names whoever is presenting.

A listener sends these values every time it connects. Values set from
the organiser's page take precedence and survive a reconnection, so a
listener restarting mid-event will not overwrite them.

Every option can also be set in a YAML file passed with `--config`. The
Listener Options document reproduces the complete example
configuration, which ships in the repository as
`configs/listener.example.yaml`.

## Rehearsing without publishing

Before an event, check what the microphone hears without connecting to
a server at all:

```bash
ears-listener --room rehearsal --dry-run --device "Scarlett"
```

Committed text prints to the terminal. Stand where the speaker will
stand, try it with the room empty and again with somebody talking at
the back, and adjust the device and gain rather than hoping.

## Keeping a local copy

Passing `--transcript` appends every committed segment to a file as
JSON objects, one per line. That file is the insurance against the
network: if the server is unreachable, the listener keeps transcribing
and buffers the text, and nothing said so far is lost.

## Using Parakeet

The listener uses Whisper by default. Setting the backend to `parakeet`
sends the audio to the sherpa-onnx project's WebSocket server instead,
which runs NVIDIA's Parakeet model on the processor alone:

```bash
ears-listener --room main-hall --backend parakeet \
  --parakeet ws://127.0.0.1:8082
```

The same choice in the configuration file uses two keys:

```yaml
backend: parakeet
parakeet_url: "ws://127.0.0.1:8082"
```

On Debian, the `sherpa-onnx-server` package runs the server as a systemd
service; the Installing on Debian document covers fetching its model and
starting it. Parakeet keeps the transcript well under a second behind
the speaker on a small room machine, but it cannot translate and covers
fewer languages than Whisper. The Choosing a Model document compares
the two backends.

## Using a remote or cloud model

The listener talks to whatever transcription endpoint it is given. The
default is a local whisper.cpp server:

```bash
ears-listener --room main-hall --whisper http://127.0.0.1:8081/inference
```

The same client speaks the OpenAI compatible transcription interface,
so a cloud service works by setting a model name and an API key:

```bash
export EARS_API_KEY=...
ears-listener --room main-hall \
  --whisper https://api.example.com/v1/audio/transcriptions \
  --model whisper-1
```

Sending audio to a remote service means the audio leaves the room,
which is the one property the local arrangement exists to preserve.
Consider carefully whether the speakers and the audience would expect
that.

## Teaching it your jargon

Whisper is confident and wrong about technical vocabulary. A glossary
corrects it, and a Postgres glossary is built in.

To see the glossary in force, change it, or replace it:

```bash
ears-listener --print-vocabulary > vocabulary.txt
ears-listener --room main-hall --vocabulary vocabulary.txt
```

The file holds one term per line, and lines beginning with a hash are
comments. Whichever source you use replaces the built-in list rather
than adding to it, so the glossary in force is always exactly what
`--print-vocabulary` shows. Use `--no-vocabulary` to send none at all.

Order matters, because the model accepts only a few hundred characters
of prompt. The prompt is filled from the top of the list downwards, and
terms below that cut are still corrected in the transcript but do not
help the model hear them. The listener reports the split at startup.

Parakeet accepts no prompt, so under the Parakeet backend the glossary
only corrects the spelling of terms in the transcript; it cannot help
the model hear them.

## Stopping cleanly

Pressing Ctrl-C stops capture immediately but lets the transcription
already in flight finish, so the last sentence of a talk is not lost.
The listener says what it is waiting for. Pressing Ctrl-C a second time
exits at once.

## Next Steps

- The Capturing Audio document covers device and channel selection in
  detail.
- The Managing Rooms document explains turning a room around between
  talks.
- The Listener Options document lists every flag and configuration key.
