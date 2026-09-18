# Running an Event

This page is a checklist rather than a reference: what to decide in
advance, what to do on the day, and what to hand over afterwards. It
assumes the software is already installed and working.

## A week before

Several decisions are much cheaper to make early than on the morning:

- Decide where the server runs. If the venue network isolates wireless
  clients, and assume it does until proven otherwise, the server needs
  to be somewhere public, with a name in DNS and a certificate in place
  well before the event.
- Pick the room identifiers. They appear in URLs, so keep them short
  and memorable, and declare them in the server configuration so the
  lobby is populated before anything starts.
- Generate the publish token, or one per room, and decide who holds it.
- Decide about the audio feed. A laptop microphone across a room is the
  worst case and will read like it; a feed from the sound desk into a
  USB interface is the best.
- Agree the accessibility notice wording with the organisers, and put
  it in the server configuration.

Generate a token with a command that produces something unguessable:

```bash
openssl rand -hex 24
```

## The day before

Everything below is easier to fix the day before than ten minutes
before the keynote:

1. Install the binaries, the model server and a model on each room's
   machine, and run the listener once. The microphone permission prompt
   on macOS is the classic way to lose the first ten minutes of a talk.
2. Test with `--dry-run`, standing where the speaker will stand, with
   the room empty and again with somebody talking at the back.
3. Check the input level reported at startup, and adjust the gain on
   the interface rather than hoping.
4. Check the model keeps up by watching the `speed` figure the listener
   logs. Below 1 means it is slower than the speaker.
5. Confirm a phone on the venue network can reach the server, using the
   network the audience will actually be on.

## On the day

Each room needs three processes, started in this order:

1. Start the model server, giving it the model and a voice detection
   model:

   ```bash
   whisper-server \
     --model ~/.cache/whisper/ggml-large-v3.bin \
     --host 127.0.0.1 --port 8081 --threads 8 \
     --vad --vad-model ~/.cache/whisper/ggml-silero-v5.1.2.bin
   ```

2. Stop the machine sleeping, because a machine that sleeps mid-talk
   stops transcribing:

   ```bash
   caffeinate -dimsu &
   ```

3. Start the listener for the room:

   ```bash
   ears-listener --config /usr/local/etc/slonik-ears/listener.yaml \
                 --speaker "Alex Roe" \
                 --transcript ~/transcripts/main-hall.jsonl
   ```

Put the stage display on the room's stage URL. Its QR code sends the
audience to the same transcript on their own phones, which saves
reading a URL aloud.

## While it is running

Three things are worth watching during the day:

- The lobby shows every room and whether it is live. A room that has
  gone quiet shows as idle within moments of its listener disconnecting.
- The room page shows what the listener last reported, including any
  backend error, so the question of why it has stopped is answerable
  from a phone at the back of the hall.
- The health endpoint suits whatever monitoring you already run.

Between talks, use the organiser's page to change the speaker and clear
the transcript. The Managing Rooms document describes it.

## Afterwards

Transcripts are in the server's data directory, one file per room, and
are downloadable as text, subtitles or JSON.

Speakers generally appreciate being offered theirs, and equally
appreciate being told that it is a machine transcript with the errors
that implies.

## Next Steps

- The Managing Rooms document covers turning a room around between
  talks.
- The Troubleshooting document lists what goes wrong and what to do.
- The Accessibility document describes what to tell the audience.
