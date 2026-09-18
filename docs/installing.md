# Installing

Slonik Ears builds into two binaries from one Go module, with a web
application embedded in the server. This page covers the prerequisites,
building from source, and the differences between platforms.

## Prerequisites

Building requires a Go toolchain and Node, and the listener
additionally requires a C compiler because audio capture uses cgo:

- Go 1.27 or later, which the module's `toolchain` directive will fetch
  automatically for a recent Go.
- Node 20 or later, used to build the web application.
- A C compiler and the platform's audio development headers.

On macOS the Xcode command line tools provide everything needed. On
Debian or Ubuntu, install `build-essential` and `libasound2-dev`.

The transcription backend is separate. Install
[whisper.cpp](https://github.com/ggml-org/whisper.cpp), which on macOS
is available through Homebrew:

```bash
brew install go node whisper-cpp
```

The Homebrew formula installs `whisper-server` alongside the command
line tools, so no separate build is required.

## Building from source

Clone the repository and build both binaries into `./bin`:

```bash
git clone https://github.com/dpage/slonik-ears.git
cd slonik-ears
make build
```

The build compiles the web application first and embeds the result in
the server binary, so the server needs no separate static files at
runtime.

To confirm what you built, ask either binary for its version:

```bash
./bin/ears-server --version
./bin/ears-listener --version
```

## Platform notes

The two binaries have different requirements, summarised in the
following table:

| Component | Platforms | Notes |
| --- | --- | --- |
| ears-server | Linux, macOS, Windows, amd64 and arm64 | Pure Go, no cgo, no runtime dependencies |
| ears-listener | macOS, Linux, Windows | Needs cgo for capture: CoreAudio, ALSA or PulseAudio, WASAPI |

Because the listener needs cgo, cross-compiling it requires a cross
toolchain for the target. Building natively on the target machine is
usually less trouble.

The server cross-compiles cleanly, which `make dist` relies on when
producing release archives for other platforms.

## Fetching a model

Models are downloaded into `~/.cache/whisper` and are chosen with the
`MODEL` variable:

```bash
make model MODEL=large-v3
```

Also fetch the voice detection model, which is under a megabyte and
prevents the model inventing text over silence:

```bash
make vad-model
```

The Choosing a Model document explains which model suits which machine.

## Running as a service

The `deploy/` directory contains a LaunchAgent example for macOS. The
listener must run as an agent rather than a daemon, because microphone
access on macOS belongs to a logged-in session and a daemon has none.

A container image of the server is published to the GitHub container
registry, and is usually the easiest way to run the relay on a cloud
instance:

```bash
docker pull ghcr.io/dpage/slonik-ears:latest
```

The Running in a Container document covers it properly. The listener is
deliberately not containerised, since it needs direct access to the
host's audio hardware and to a model server beside it.

## Next Steps

- The Choosing a Model document compares accuracy and speed across
  models and machines.
- The Capturing Audio document explains device selection and input
  levels.
- The Running the Server document covers tokens and persistence.
- The Running in a Container document covers the published image, its
  configuration and its storage.
