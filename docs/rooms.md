# Managing Rooms

Between talks a room needs a new speaker name and a clean transcript.
This page describes the organiser's page that does both, and the
alternative for anybody who would rather not run with an admin token.

## Enabling the organiser's page

The page lives at `/admin` and is guarded by its own token:

```bash
export EARS_ADMIN_TOKEN=$(openssl rand -hex 24)
```

Without a token the page says so and the controls stay switched off.
That is deliberate, because this is the interface that can clear a talk
off every screen in the building, so it is unavailable by default
rather than open. The page is not linked from the lobby for the same
reason.

## What the page offers

The page lists one card per room, whether or not a listener is
currently connected to it:

![The organiser's page, showing a card for each room with editable title,
speaker and track fields above buttons to save, reset, remove or view the
room](img/screens/admin.png)

Each room carries three controls, covering what comes up between
speakers:

- Save details changes the title, speaker and track. The room
  identifier, its URL and its QR code do not move, so anything already
  on a screen or in somebody's pocket keeps working.
- Reset for next talk empties the transcript, leaving the room ready for
  the next speaker.
- Remove room takes the room out of the lobby entirely, for a track
  that has finished for the day. A listener still running in that room
  is told the room has gone and stops, rather than reconnecting and
  recreating it.

## What a reset does

Viewers watching at the time see the screen clear where they stand.
They are not disconnected, so a stage display does not flicker through
a reconnection in front of an audience. A viewer that was away and
reconnects afterwards is told to start afresh rather than resuming into
the previous talk.

A listener that is already running carries straight on into the next
talk without being restarted, which is the point: the room turns around
without anybody visiting the machine at the back of it.

The sequence numbers carry on climbing rather than starting again from
one, so the first line of the afternoon talk might be numbered 481.
That is what lets the server tell a returning viewer which talk it is
holding: were the numbering to restart, line 50 of this talk and line
50 of the last one would be the same request, and a phone that slept
through the break would be handed the remainder of a talk it had never
seen the start of.

## Nothing is destroyed

Both destructive controls take two clicks, and neither deletes
anything. The previous transcript moves to an archive directory
alongside the live ones, named after the room and the time it was set
aside:

```
data/archive/main-hall-2026-09-18T09-30-00.jsonl
```

A button pressed during the wrong talk therefore costs a moment rather
than a speaker's session. Tidying the archive is a job for whoever
tidies the disk.

## Changing a room identifier

The page deliberately cannot change a room's identifier. That
identifier is printed on the QR code, sitting in the URL bar of every
phone in the room, and configured on the listener at the back of it, so
moving it would strand all three at once.

## The alternative

If you would rather not run with an admin token, use a room per talk
instead. Start each listener with a new identifier:

```bash
ears-listener --room main-hall-2 --title "Main Hall" --speaker "Sam Roe"
```

Each talk then gets its own transcript and its own URL, the stage QR
code updates itself, and the speaker ends up with a file containing
only their own talk.

## Next Steps

- The HTTP API document describes the same operations for scripting.
- The Running the Server document covers tokens and transcript storage.
- The Troubleshooting document lists what to check when a room behaves
  oddly.
