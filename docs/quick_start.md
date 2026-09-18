# Quick Start

This page shows Slonik Ears working in a couple of minutes, without a
model, a microphone, or any patience. Use it to confirm the build works
and to see what the attendee and stage views look like.

## Running the demo

The repository ships a demo target that builds everything and starts a
relay with a fake listener attached.

Clone the repository and run the demo from its root:

```bash
git clone https://github.com/dpage/slonik-ears.git
cd slonik-ears
make demo
```

The demo builds the web application, the server, and the listener, then
starts a relay on <http://localhost:8080> and a listener that replays a
sample recording through a transcriber that invents its text. Nothing
is downloaded and no microphone is opened.

## What to look at

Open <http://localhost:8080> and work through the views.

The lobby lists every room, with the live ones first, so an attendee
who scanned a code in the corridor can find the talk they are in:

![The lobby, listing two live rooms and one idle room as cards carrying the
track, the speaker and the number of lines transcribed](img/screens/lobby.png)

The room page shows the transcript as an attendee sees it, with reading
controls for text size, theme and timestamps. Committed text is
regrouped into sentences, and the unconfirmed line at the end appears
in a lighter style:

![The attendee view of a room, showing the transcript as paragraphs with the
reading controls above and share, download and subtitle buttons
below](img/screens/room.png)

The stage display at `/r/demo/stage` shows a few lines in large type
with a QR code for the audience, which the Accessibility document
illustrates.

Press Ctrl-C in the terminal to stop the demo.

## Trying a real model

To hear your own voice transcribed, you need a model server and a
microphone. The short version, on a Mac with Homebrew, is to install
whisper.cpp and fetch a model:

```bash
brew install whisper-cpp
make model MODEL=large-v3
make vad-model
```

Start the model server in one terminal:

```bash
make whisper-server
```

Then run a listener that prints to the terminal rather than publishing
anywhere, which needs no token and no relay:

```bash
make build
./bin/ears-listener --room rehearsal --dry-run
```

Say a few sentences. They appear in the terminal as they are committed.

On macOS the first run prompts for microphone permission, and the
prompt belongs to whichever application is running the command. If you
dismiss it, capture records digital silence and produces no transcript
at all.

## Next Steps

- The Installing document covers building and installing properly,
  including on Linux.
- The Choosing a Model document compares models and explains what each
  machine can realistically run.
- The Running the Server document describes tokens, persistence, and
  putting the relay somewhere the audience can reach.
