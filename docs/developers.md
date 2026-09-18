# Developer Resources

This page covers working on Slonik Ears itself: the layout of the code,
the checks that run before a change lands, and how to build the
documentation.

## Repository layout

The following table describes what lives where:

| Path | Contents |
| --- | --- |
| cmd/ears-server | The relay binary |
| cmd/ears-listener | The room listener binary |
| internal/protocol | The JSON wire format shared by both ends |
| internal/audio | Capture, voice detection and utterance segmentation |
| internal/asr | Transcription client, output cleaning, glossary |
| internal/listener | The capture-to-text engine and publisher |
| internal/hub | Rooms, history, cursors and viewer fan-out |
| internal/server | HTTP, WebSockets, authentication, exports |
| internal/store | Transcript persistence |
| web | The attendee web application |

## Running the checks

The Makefile targets mirror what continuous integration runs, so a
green run locally means a green pull request:

```bash
make lint
make test
make race
make vuln
make web-test
make smoke
```

The smoke target drives the attendee and stage views in a real browser
against a running demo, which catches the faults that unit tests do
not: a transcript that scrolls off screen, a QR code that never loaded,
controls drawn on top of one another.

## Testing without a microphone

Two mechanisms make the system testable on a machine with no audio
hardware:

- `--mock` substitutes a transcriber that invents text, so the rest of
  the system can be exercised without a model.
- `--file` replays a WAV file instead of opening a capture device, with
  `--loop` and `--fast` for longer or quicker runs.

Combining them, as `make demo` does, exercises everything from
segmentation to the browser without hardware.

## Tuning against a recording

The audio package contains a measuring instrument rather than a
pass-or-fail test, which sweeps segmentation settings over a recording
and reports how much of it each would have committed:

```bash
EARS_TUNE_WAV=/tmp/room.wav go test ./internal/audio -run Segmentation -v
```

Recordings are never committed to the repository. They contain
somebody's voice, and often a roomful of other people's.

## Regenerating the sample recording

The sample the smoke test replays is generated rather than recorded,
and can be rebuilt:

```bash
EARS_WRITE_FIXTURE=1 go test ./internal/audio -run GenerateFixture
```

The generated sample deliberately contains real pauses. An earlier
sample had none, which meant a fault in deciding where silence falls
could not possibly show up in testing.

## Web application tests

The web application's unit tests run under Node's own test runner
against the TypeScript directly, so they need no additional
dependency:

```bash
make web-test
```

## Building the documentation

The documentation is built with MkDocs and the Material theme. Install
both, then serve the site locally:

```bash
pip install mkdocs-material
mkdocs serve
```

Building with `mkdocs build --strict` fails on broken links, which is
what continuous integration checks.

## Contributing

Contributions are welcome. Work on a branch, open a pull request
against `main`, and make sure the checks above pass first.

Report problems on the
[issue tracker](https://github.com/dpage/slonik-ears/issues).
