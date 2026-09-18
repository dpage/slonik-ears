# Troubleshooting

This page collects the symptoms that occur during an event, grouped by
where the fault lies. Each entry names the likely cause and what to do
about it.

## Audio and capture issues

These symptoms indicate the listener is not hearing what you expect.

### The transcript is empty but the room shows live

The listener is connected and publishing nothing, which usually means
it is capturing silence. Check three things in order:

1. Confirm microphone permission was granted. On macOS the prompt
    belongs to whichever application ran the command, and dismissing it
    causes capture to record digital silence without any error.
2. Confirm the right device was selected, using `--list-devices`. On a
    Linux host running PulseAudio, a name may have matched a monitor
    source, which is a loopback of what the machine is playing and is
    silent. The listing marks them.
3. Check the level reported at startup, and the heartbeat with
    `--log-level debug`.

If the capture device has stopped delivering anything at all, rather
than delivering silence, the listener says so within ten seconds and the
room page carries the same notice:

```
level=ERROR msg="no audio from the capture device; check that it is
still connected" device="microphone: Scarlett Solo USB"
```

An empty room still produces audio frames, so this means the device
itself has gone: an interface unplugged, or an input taken by another
application. Reconnect it and the listener picks up again by itself,
saying so; nothing needs restarting.

### The input is clipping

The listener warns when the peak reaches full scale. Turn the gain down
on the interface until the peak sits around 0.5. A clipped signal has
had the tops of its waveform removed, and nothing downstream can put
them back.

### A quiet speaker is missed entirely

Raise the gain on the interface first, because more signal helps the
model as well as the detector. If the input is already healthy, lower
`--min-rms` and watch the heartbeat to confirm speech now exceeds the
start threshold.

### Only part of the signal arrives on a multichannel interface

Check which channel the listener chose, which it reports at startup.
The room may have been quiet while it was deciding. Set the channel
explicitly with `--channel`, counting from one as the interface labels
its inputs.

## Transcription issues

These symptoms indicate the audio is fine but the text is not.

### Lines appear that nobody said

The detector is committing chunks of an empty room and the model is
filling them in rather than returning nothing. Raise `--min-rms` until
the heartbeat shows the level during silence sitting below the start
threshold, and enable `--whisper-vad` so the model server rejects
silence itself.

### Sentences arrive chopped in half

The speaker is too quiet for the detector, so it loses them mid-phrase.
Compare the level against the start threshold in the heartbeat: speech
should peak at several times the threshold rather than brushing against
it. Raise the gain before adjusting the detector.

### Text arrives in long delayed bursts

The model is not keeping up. Check the `speed` figure the listener logs
with each segment; below 1 means the model is slower than the speaker.
Move down a model size, or turn off previews with `--no-partials`.

Left alone, the backlog of audio waiting to be transcribed is capped at
eight utterances, and the oldest is dropped to keep the transcript
tracking what is being said now. That is speech nobody will ever read,
so the listener warns each time it happens:

```
level=WARN msg="the model cannot keep up: dropping the oldest audio
waiting to be transcribed" utterances=1 audio_lost=8.4s
```

Seeing that line means the model is the wrong size for the machine, and
no amount of waiting will let it catch up.

### Text arrives many seconds late on a small machine

The model's encoder processes a fixed thirty second window whatever it
is given, which a processor without a GPU feels acutely. Start the
model server with `--audio-ctx 768` and the listener with
`--no-partials`.

### Technical terms are consistently wrong

Add them to the glossary. Start from the built-in list with
`--print-vocabulary`, edit it, and supply it with `--vocabulary`. Terms
near the top of the file reach the model as context; terms lower down
are only corrected afterwards, so move anything being misheard upwards.

## Connection issues

These symptoms indicate a problem between the pieces.

### The listener reports it cannot reach the transcription backend

The model server is not running, or is on a different port. Both
`whisper-server` and the relay default to port 8080, so start the model
server on 8081 explicitly.

### The listener logs publisher disconnected on a loop

The server address is wrong, or the relay is not running. The listener
keeps transcribing and buffers the text, so correcting the address and
restarting loses nothing said so far, provided `--transcript` was set.

### Attendees cannot reach the server

The venue network is isolating wireless clients from one another, which
is common. Run the relay somewhere outside the venue network instead.

### A room shows live but nothing appears after a restart

A second listener took over the room. The first is told and stops, so
check whether an earlier process is still running elsewhere.

## Operational issues

These symptoms concern running the event rather than the audio.

### Ctrl-C does not appear to stop the listener

The listener is finishing the transcription already in flight, or
waiting for the relay to accept the last segments, so the final
sentence of a talk is not lost. It says which. Pressing Ctrl-C again
exits immediately.

### A reset room still shows the previous talk

A viewer that was disconnected during the reset resumes from the
sequence number it held. The server detects that and tells the viewer
to start afresh, so reloading the page resolves it.

### The organiser's page says the controls are switched off

No admin token is configured on the server. Set `EARS_ADMIN_TOKEN` and
restart the server.

### Transcripts disappear when the server restarts

The server was started without `--data-dir`, which keeps transcripts in
memory only.

## Still have questions?

Report a problem or ask a question on the
[issue tracker](https://github.com/dpage/slonik-ears/issues).
