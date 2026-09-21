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

The documentation is built with MkDocs and the Material theme. The
versions are pinned, so install them from the requirements file and
serve the site locally:

```bash
pip install -r requirements-docs.txt
mkdocs serve
```

Building with `mkdocs build --strict` fails on a broken link or a page
missing from the navigation, which is what continuous integration
checks on every pull request that touches the documentation.

Publication happens automatically. A change to `docs/`, `mkdocs.yml` or
the requirements file that lands on `main` rebuilds the site and
publishes it to
[dpage.github.io/slonik-ears](https://dpage.github.io/slonik-ears/).
A change to the code alone does not, since the code cannot alter the
rendered site.

## Continuous integration

Four workflows run against the repository, and between them they cover
everything `make lint`, `make vuln` and `make smoke` do locally.

The following table describes what each workflow does and when:

| Workflow | When | What |
| --- | --- | --- |
| `ci.yml` | Every push and pull request that touches something other than the documentation | Formatting, vet and golangci-lint; the tests under the race detector on Linux and macOS; govulncheck and npm audit; the web build; and an end to end run that publishes a transcript through a real server and drives the attendee views in a real browser. |
| `docker.yml` | Main, tags, and changes to the Dockerfile | Builds the image, runs it, checks that it serves the application and is not running as root, then publishes a multi-architecture image to the GitHub container registry. |
| `docs.yml` | Changes to the documentation | Builds the site with `--strict`, and publishes it to GitHub Pages when the change lands on `main`. |
| `release.yml` | A tag beginning with `v` | Cross-compiles the server for Linux, macOS and Windows, builds the listener natively on each platform that needs cgo, and attaches the tarballs and `SHA256SUMS` to a GitHub release. A tag carrying a suffix, such as `-rc1`, publishes as a pre-release. |

Dependabot groups its updates weekly, so a quiet week produces one pull
request rather than nine.

## Cutting a release

A release is made by tagging. Push an annotated tag and the release
workflow does the rest:

```bash
git tag -a v1.0.1 -m "Release 1.0.1"
git push origin v1.0.1
```

The version the binaries report comes from `git describe`, so the tag
is the only place a release number is written; nothing in the tree
needs editing beyond the changelog.

A tag carrying a suffix, such as `v1.1.0-rc1`, publishes as a GitHub
pre-release rather than as the current release, which is how a
candidate is handed round for smoke testing before the bare tag is
pushed to the same commit. A tag that has been pushed is never moved:
correct a mistake with a new tag at the right commit.

To check the packaging without publishing anything, run `release.yml`
manually from the Actions tab; a manual run builds every artifact and
stops short of creating the release.

## Contributing

Contributions are welcome. Work on a branch, open a pull request
against `main`, and make sure the checks above pass first.

Report problems on the
[issue tracker](https://github.com/dpage/slonik-ears/issues).
