# Listener Options

This page lists every option `ears-listener` accepts. Options may be
given on the command line, in a YAML configuration file, or through the
environment. A command line flag beats both of the others, and a value
in the configuration file beats the environment.

## Configuration file

Every option below can be set in a YAML file passed with `--config`. A
complete example ships in the repository as
`configs/listener.example.yaml`, reproduced here:

```yaml
# Slonik Ears listener configuration — one of these per room.
#
# Anything given on the command line wins over this file, and
# EARS_PUBLISH_TOKEN wins over an empty token here.

server: "https://ears.example.org"
room: "main-hall"
# Better set through the environment: EARS_PUBLISH_TOKEN
token: ""

title: "Main Hall"
track: "Track A"
speaker: ""
# ISO-639-1, or "auto" to detect. Naming the language is both faster and more
# accurate than letting the model guess, so name it if you know it.
language: "en"

# Capture device: an index, a device id, or part of the name as shown by
# `ears-listener --list-devices`. Empty means the system default input.
device: ""
# Which input channel of a multichannel interface carries the microphone,
# 1-based as printed on the front of the box. Empty means measure every channel
# at start-up and use whichever is loudest, which the listener logs.
#
# This matters more than it sounds. A sixteen-input interface offers no mono
# format, so a request for one channel is answered by averaging all sixteen —
# a single microphone divided by sixteen. Several channels here are averaged
# together, which is what a genuine stereo microphone wants: "1,2".
channel: ""

# Transcription backend. This is whisper.cpp's own server:
#   whisper-server --model ~/.cache/whisper/ggml-small.en.bin --port 8081
whisper_url: "http://127.0.0.1:8081/inference"
# Only needed for OpenAI-compatible cloud endpoints, which also want an
# api_key (or EARS_API_KEY).
model: ""
api_key: ""
translate: false
# Ask the model server to apply its own voice detection, so a chunk containing
# no speech comes back empty rather than being invented. This needs a VAD
# model loaded on that server; without one it refuses the request.
whisper_vad: false

# The glossary handed to the model so it spells your jargon the way the
# audience does. Left alone, the built-in Postgres list applies, which is what
# turns "PG Admin" into pgAdmin, "PG Start statements" into pg_stat_statements
# and "PG Dump Hall" into pg_dumpall.
#
# The easiest way to extend it is to start from a copy:
#
#   ears-listener --print-vocabulary > vocabulary.txt
#
# then edit that and point vocabulary_file at it. Whichever of these you use
# REPLACES the built-in list rather than adding to it, so that the effective
# glossary is always exactly what --print-vocabulary shows.
#
# vocabulary_file: "vocabulary.txt"
# vocabulary:
#   - "pgEdge"
#   - "pg_stat_statements"
#
# Order matters. The model accepts only a few hundred characters of prompt, so
# it is filled from the top of the list downwards; terms below the cut are
# still corrected in the transcript, they just do not help the model hear them.
# Put anything you particularly care about near the top. The listener logs
# which way the split fell when it starts.
# no_vocabulary: false

# Segmentation. The defaults suit a conference talk; shorten silence_ms for a
# snappier feel at the cost of more, shorter chunks (and slightly worse
# accuracy, since the model gets less context per chunk).
silence_ms: 650
min_utterance_ms: 400
max_utterance_ms: 14000
partial_ms: 900
no_partials: false
pre_roll_ms: 320

# Voice detection: what counts as somebody talking. min_rms is the one to reach
# for. Below it nothing is ever speech, so too high ignores a quiet speaker and
# too low sends the room's own noise to the model — which does not answer "there
# was nothing there" but invents something, which is where stray "Thank you."
# lines and hallucinated song lyrics come from.
#
# Measured over a recording with known silent and spoken stretches, on a
# correctly levelled input: the empty room sits at 0.0016 to 0.005 and speech
# runs 0.06 to 0.25. The default sits in the gap. Run the listener with
# --log-level debug and compare "level" against "start_threshold" in the
# heartbeat if you need to move it.
min_rms: 0.012
start_factor: 3.0
stop_factor: 1.8

# A local copy of every committed line, in case the network has other ideas.
transcript_file: "main-hall.jsonl"

# Record the captured audio to a WAV file, for working out afterwards why a
# room transcribed badly. A recording holds every word said near the
# microphone, by anybody, so treat making one as the organisers' decision and
# keep the file out of version control.
record_file: ""

log_level: "info"
```

## Identity and connection

The following table describes the options that identify the room and
locate the server:

| Flag | Config key | Default | Description |
| --- | --- | --- | --- |
| --room | room | none | Room identifier, lower case letters, digits, hyphen and underscore |
| --title | title | none | Room title shown to attendees |
| --track | track | none | Track name, used to group rooms in the lobby |
| --speaker | speaker | none | Speaker name shown to attendees |
| --language | language | en | Spoken language as an ISO-639-1 code, or auto |
| --server | server | http://localhost:8080 | Base URL of the relay |
| --token | token | none | Publish token, better supplied by the environment |
| --config | none | none | Path to a YAML configuration file |

## Audio capture

The following table describes the options controlling where audio comes
from:

| Flag | Config key | Default | Description |
| --- | --- | --- | --- |
| --device | device | system default | Device index, identifier, or part of its name |
| --channel | channel | loudest | Input channels, 1-based, comma separated to average |
| --list-devices | none | off | List capture devices and exit |
| --record | record_file | none | Record captured audio to a WAV file for debugging |
| --file | file | none | Replay a WAV file instead of capturing |
| --loop | loop | off | Loop the replayed file |
| --fast | fast | off | Replay as fast as possible rather than in real time |

## Transcription backend

The following table describes the options selecting and controlling the
model:

| Flag | Config key | Default | Description |
| --- | --- | --- | --- |
| --whisper | whisper_url | http://127.0.0.1:8081/inference | Transcription endpoint |
| --model | model | none | Model name, for OpenAI compatible endpoints only |
| --api-key | api_key | none | Bearer token for the transcription endpoint |
| --translate | translate | off | Translate to English rather than transcribe verbatim |
| --whisper-vad | whisper_vad | off | Ask the model server to apply its own voice detection |
| --mock | mock | off | Use the mock transcriber, which invents text |
| --final-timeout | final_timeout_sec | 60 | Seconds to wait for a committed transcription |
| --partial-timeout | partial_timeout_sec | 8 | Seconds to wait for a preview transcription |

Enabling `--whisper-vad` requires a voice detection model loaded on the
model server. Without one, the server rejects the request.

## Segmentation

The following table describes the options deciding where an utterance
begins and ends:

| Flag | Config key | Default | Description |
| --- | --- | --- | --- |
| --silence-ms | silence_ms | 650 | Silence that ends an utterance |
| --min-utterance-ms | min_utterance_ms | 400 | Ignore utterances shorter than this |
| --max-utterance-ms | max_utterance_ms | 14000 | Commit an utterance at least this often |
| --partial-ms | partial_ms | 900 | How often to refresh the live preview |
| --no-partials | no_partials | off | Disable previews, halving the load on the model |
| --pre-roll-ms | pre_roll_ms | 320 | Audio kept from before speech is detected |

## Voice detection

The following table describes the options controlling what counts as
speech:

| Flag | Config key | Default | Description |
| --- | --- | --- | --- |
| --min-rms | min_rms | 0.012 | Absolute level below which nothing is ever speech |
| --start-factor | start_factor | 3.0 | How far above the noise floor a frame must be to start |
| --stop-factor | stop_factor | 1.8 | The lower threshold that sustains an utterance |

## Glossary

The following table describes the options controlling the terms sent to
the model and corrected in the output:

| Flag | Config key | Default | Description |
| --- | --- | --- | --- |
| --vocabulary | vocabulary_file | built-in | File of terms, one per line |
| --no-vocabulary | no_vocabulary | off | Send no glossary at all |
| --print-vocabulary | none | off | Print the glossary in force and exit |

An inline `vocabulary` list in the configuration file is also accepted.
Whichever source is used replaces the built-in list rather than adding
to it.

## Output and diagnostics

The following table describes the remaining options:

| Flag | Config key | Default | Description |
| --- | --- | --- | --- |
| --transcript | transcript_file | none | Append committed segments to a JSONL file |
| --dry-run | none | off | Transcribe to the terminal without connecting |
| --log-level | log_level | info | One of debug, info, warn or error |
| --version | none | off | Print the version and exit |

## Environment variables

The following table describes the environment variables the listener
reads:

| Variable | Equivalent | Description |
| --- | --- | --- |
| EARS_SERVER | --server | Base URL of the relay |
| EARS_ROOM | --room | Room identifier |
| EARS_PUBLISH_TOKEN | --token | Publish token |
| EARS_WHISPER_URL | --whisper | Transcription endpoint |
| EARS_API_KEY | --api-key | Bearer token for the transcription endpoint |

An environment variable applies only when the corresponding value is
still unset, whether that value would have come from the command line
or from the configuration file. For `EARS_SERVER` and
`EARS_WHISPER_URL` the test is against the built-in default, so passing
that default explicitly on the command line does not suppress the
environment variable.
