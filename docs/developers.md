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
rendered site. The same workflow builds the apt repository, which is
covered under Maintaining the Debian packages below.

## Continuous integration

Four workflows run against the repository, and between them they cover
everything `make lint`, `make vuln` and `make smoke` do locally.

The following table describes what each workflow does and when:

| Workflow | When | What |
| --- | --- | --- |
| `ci.yml` | Every push and pull request that touches something other than the documentation | Formatting, vet and golangci-lint; the tests under the race detector on Linux and macOS; govulncheck and npm audit; the web build; and an end to end run that publishes a transcript through a real server and drives the attendee views in a real browser. |
| `docker.yml` | Main, tags, and changes to the Dockerfile | Builds the image, runs it, checks that it serves the application and is not running as root, then publishes a multi-architecture image to the GitHub container registry. |
| `site.yml` | Changes to the documentation or to `packaging/apt`, and after every release | Builds the site with `--strict` and, off a pull request, the signed apt repository under `/apt/`, then publishes both to GitHub Pages when the change lands on `main`. |
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

## Maintaining the Debian packages

The `.deb` files are built by `release.yml` and attached to each release.
`site.yml` then downloads the `.deb` files from every stable release
(pre-releases and drafts are left out), signs an apt repository over
them with `packaging/apt/build-repo.sh`, and publishes it at
`/apt/` beside the documentation. `release.yml` starts `site.yml` by
hand after it creates a release, since a release made with the default
token does not trigger other workflows.

### Creating the signing key

The repository is signed with a key that only the `APT_SIGNING_KEY`
secret holds. Make a key with no passphrase, because the workflow has
nobody to type one. The identity below is only an example; use your own:

```bash
gpg --batch --passphrase '' --quick-gen-key \
  'Slonik Ears Packaging <packaging@example.org>' ed25519 sign never
gpg --list-secret-keys --keyid-format long packaging@example.org
gpg --armor --export-secret-keys <fingerprint> | gh secret set APT_SIGNING_KEY
```

Then delete the key from your keyring, or export it to somewhere kept
offline first if you want to be able to reissue it; nothing in the
repository should ever hold it. The public half is regenerated from the
secret on every build and published as `/apt/slonik-ears.asc`, so there
is nothing to copy by hand.

Until the secret exists, `site.yml` prints a warning, skips the apt step
and publishes the documentation as before. Once it is set, run
`site.yml` from the Actions tab, or cut a release, to publish the
repository. Replacing the key means users must download the new
`slonik-ears.asc`, so treat it as a rare event.

### Updating whisper.cpp

The packaged whisper.cpp is pinned in `packaging/whisper-server/VERSION`.
To move to a new release, set `WHISPER_CPP_TAG` and
`WHISPER_CPP_COMMIT`, and reset `DEB_VERSION` to `<new version>-0slonik1`.
`build.sh` reads the file, so it needs no other edit, but check that its
build flags still match the new release. The `-0slonikN` suffix makes the
version sort below any whisper.cpp package Debian might one day ship.
Any change that alters the package without changing whisper.cpp, such as
a new unit file or default setting, bumps the number to `-0slonik2` and
so on, because the apt pool must never hold two different files under
one name.

### Refreshing the model checksums

The model catalogue in `internal/models/catalogue.go` pins two Hugging
Face commits, and each file's checksum and size come from the tree API
at those commits. To move to newer models, take the new commit hashes
from the model pages, update the commits named in the comment at the top
of the table, and regenerate the entries from:

```bash
curl -s https://huggingface.co/api/models/ggerganov/whisper.cpp/tree/<commit>
curl -s https://huggingface.co/api/models/ggml-org/whisper-vad/tree/<commit>
```

Each entry's `lfs.oid` is its SHA-256 and `lfs.size` its size. Never use
`resolve/main` in a URL, since the file behind it can change under a
checksum.

## Contributing

Contributions are welcome. Work on a branch, open a pull request
against `main`, and make sure the checks above pass first.

Report problems on the
[issue tracker](https://github.com/dpage/slonik-ears/issues).
