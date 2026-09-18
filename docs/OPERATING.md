# Running an event

Notes from thinking about what actually goes wrong on the day. Adapt freely.

## A week before

* Decide where the server runs. If the venue's Wi-Fi isolates clients — assume
  it does until proven otherwise — it needs to be somewhere public, and DNS
  and a certificate need to exist before the morning of the event.
* Pick the room ids. They appear in URLs (`/r/main-hall`), so keep them short
  and memorable. Declare them in the server config so the lobby is populated
  before anything starts.
* Generate the publish token, or one per room:

  ```bash
  openssl rand -hex 24
  ```

* Decide about the audio feed. A lapel mic into a laptop's built-in
  microphone across a room is the worst case and will read like it. A feed
  from the desk into a USB interface is the best.

## The day before

* On each room's Mac: install the binaries, install `whisper-cpp`, download the
  model, and **run it once**. The microphone permission prompt is the classic
  way to lose the first ten minutes of a keynote.
* Test with `--dry-run`, standing where the speaker will stand, with the room
  empty and again with somebody talking at the back. Adjust `--device` and the
  microphone gain rather than hoping.
* Check the model keeps up. If the listener's log shows `took` creeping past
  the length of the audio being transcribed, move down a model size or turn
  off previews with `--no-partials`.
* Decide the accessibility notice wording with the organisers and put it in
  the server config; it appears under every transcript.

## On the day

Per room, in order:

```bash
# 1. the model server
whisper-server --model ~/.cache/whisper/ggml-large-v3.bin --port 8081 --threads 8

# 2. keep the Mac awake
caffeinate -dimsu &

# 3. the listener
ears-listener --config /usr/local/etc/slonik-ears/listener.yaml \
              --speaker "Speaker Name" \
              --transcript ~/transcripts/main-hall.jsonl
```

Between talks, use the organiser's page at `/admin` to change the speaker and
clear the transcript without going near the machine at the back of the room;
see "Turning a room around between talks" below. The same thing over the API,
if you would rather script it:

```bash
curl -X PUT https://ears.example.org/api/admin/rooms/main-hall \
     -H "Authorization: Bearer $EARS_ADMIN_TOKEN" \
     -H 'Content-Type: application/json' \
     -d '{"title":"Main Hall","speaker":"Someone Else","track":"Track A"}'

curl -X POST https://ears.example.org/api/admin/rooms/main-hall/reset \
     -H "Authorization: Bearer $EARS_ADMIN_TOKEN"
```

Put the stage display on `/r/main-hall/stage`. Its QR code sends the audience
to the same transcript on their own phones, which saves reading a URL aloud.

## While it is running

* The lobby shows every room and whether it is live. A room that has gone
  quiet shows "Idle" within moments of its listener disconnecting.
* The room page shows what the listener last reported, including any backend
  error, so "why has it stopped?" is answerable from a phone at the back.
* `GET /healthz` is there for whatever monitoring you already have.

## Afterwards

Transcripts are in the server's data directory as one JSONL file per room, and
are downloadable as text, SRT, VTT or JSON:

```bash
curl -O https://ears.example.org/api/rooms/main-hall/transcript?format=txt
```

There are two plain text forms, and it is worth knowing which you want. The
**Download text** button on the room page writes the file in the browser,
grouped into sentences exactly as the page displays it, which is the one to
give a speaker. The `?format=txt` endpoint above emits one line per committed
segment, cut where the speaker paused rather than where the sentence ended;
that is the form to script against, and the one whose lines line up with the
JSONL file, the SRT cues and their timings.

Speakers generally appreciate being offered theirs, and equally appreciate
being told that it is a machine transcript with the errors that implies.

## Things that will go wrong, and what to do

| Symptom | Likely cause |
| --- | --- |
| Transcript is empty but the room shows live | microphone permission, or the wrong `--device`; check the level in the listener's status |
| Text arrives in long delayed bursts | the model is not keeping up: smaller model, or `--no-partials` |
| Occasional "Thank you." or "[BLANK_AUDIO]" | Whisper hallucinating on silence; the cleaner catches the common cases, and a better audio feed catches the rest |
| Attendees cannot reach the server | client isolation on the venue network: this is what the hosted relay is for |
| Room shows live but nothing appears after a restart | a second listener took over the room; the first is told and stops |
| Listener logs "publisher disconnected ... connection refused" on a loop | wrong `--server` address, or the relay is not running. The listener keeps transcribing and buffers the text, so fix the address and restart it — nothing said so far is lost if `--transcript` was set |
| Ctrl-C does not seem to stop the listener | it is finishing the last transcription, or waiting on the relay. It says which. Press Ctrl-C again to exit immediately |
| Lines nobody said: "Thank you.", or ♪ song lyrics ♪ | the detector is committing chunks of an empty room, and the model fills them in rather than returning nothing. Raise `--min-rms` until the heartbeat's `level` during silence sits below `start_threshold` |
| A quiet speaker is missed entirely | the opposite: lower `--min-rms`. Check first that the input is not simply too quiet, which the start-up channel report will tell you |
| Text arrives many seconds late on a small machine | the encoder always processes a thirty second window whatever you send it, which a CPU feels and a GPU does not. Start `whisper-server` with `--audio-ctx 768` and the listener with `--no-partials` |
| Nothing is transcribed at all, but everything looks healthy | on a PulseAudio host, `--device` may have matched a monitor: a loopback of what the machine is playing, which is silent. `--list-devices` marks them |
| Sentences arrive chopped in half, or a long one stops partway through | the speaker is too quiet for the detector. Run with `--log-level debug` and compare `level` against `start_threshold` in the heartbeat: speech should peak at several times the threshold rather than brushing against it. Raise the gain on the interface first, and only then reach for the detector's settings |

## Turning a room around between talks

Give the server an admin token and the organiser's page at `/admin` handles it:

```bash
export EARS_ADMIN_TOKEN=$(openssl rand -hex 24)
```

Without one the page says so and the controls stay switched off, which is
deliberate: this is the interface that can clear a talk off every screen in the
building, so it is unavailable by default rather than open. It is not linked
from the lobby either, for the same reason.

For each room it offers the three things that come up between speakers:

* **Save details** changes the title, speaker and track. The room's id, its URL
  and its QR code do not move, so anything already on a screen or in somebody's
  pocket carries on working.
* **Reset for next talk** empties the transcript and starts the numbering over.
  Viewers watching at the time see the screen clear where they stand; they are
  not disconnected, so a stage display does not flicker through a reconnection
  in front of an audience. A listener already running carries straight on into
  the next talk without being restarted.
* **Remove room** takes it out of the lobby entirely, for a track that has
  finished for the day.

The last two each take two clicks, and neither destroys anything: the previous
transcript is moved to `<data-dir>/archive/<room>-<timestamp>.jsonl`, so a
button pressed during the wrong talk costs a moment rather than a speaker's
session. Nothing in Slonik Ears deletes a transcript, and tidying the archive is
a job for whoever tidies the disk.

If you would rather not run with an admin token at all, the alternative is a
room per talk: `--room main-hall-2` and so on. Each gets its own transcript and
its own URL, the stage QR code updates itself, and the speaker ends up with a
file containing only their own talk.

## Tuning segmentation for a particular room

Where a sentence ends is decided by a state machine over the audio, so the only
honest way to tune it is against a recording of the room it will be used in.
The listener will make you one:

```bash
ears-listener --room main-hall --record /tmp/room.wav ...   # for debugging
```

That recording can then be replayed as often as you like without anybody having
to say anything again, either through the whole pipeline:

```bash
ears-listener --room test --dry-run --file /tmp/room.wav --fast --no-partials
```

or through the segmenter alone, which sweeps the detector's settings and
reports how much of the recording each one would have sent to the model:

```bash
EARS_TUNE_WAV=/tmp/room.wav go test ./internal/audio -run Segmentation -v
```

Aim for committed audio a little above the amount of real speech in the
recording. Well under it means sentences are being lost; well over it means
silence is being sent to the model, and silence is what Whisper hallucinates
over.

A recording captures every word said near the microphone, by anybody. Whether
to make one at a real event is a question for whoever is running the event
rather than for whoever is holding the laptop, and the file belongs nowhere
near a git repository.
