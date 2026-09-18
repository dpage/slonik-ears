# Listener Options

This page lists every option `ears-listener` accepts. Options may be
given on the command line, in a YAML configuration file, or through the
environment. A command line flag beats both of the others, and a value
in the configuration file beats the environment.

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
