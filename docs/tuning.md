# Tuning Transcription

Where one utterance ends and the next begins is decided by a state
machine over the audio. This page explains what the settings do and how
to choose them by measurement rather than instinct.

## How the detector works

A voice activity detector classifies each 20 millisecond frame as
speech or not, comparing the frame's level against two thresholds:

- The start threshold decides whether a new utterance begins. A frame
  must exceed the larger of the absolute floor and the noise floor
  multiplied by the start factor.
- The stop threshold, which is lower, decides whether an utterance
  already under way continues. The gap gives hysteresis, so a quiet
  syllable does not chop a sentence in half.

The noise floor is an estimate of the room, updated while nobody is
speaking. It falls readily and rises reluctantly, and does not move at
all until the room has been quiet for half a second. Those asymmetries
exist because rising is the dangerous direction: every rise makes
speech harder to detect, which produces more frames that look like
noise, which raises the floor again.

## The setting that matters most

The absolute floor, `--min-rms`, sits beneath the adaptive threshold
and decides what can never be speech. It defaults to 0.012.

Set it too high and a quiet speaker is ignored entirely. Set it too low
and the room's own noise is committed and sent to the model, which does
not answer that there was nothing there. The model invents something
plausible instead, which is where stray lines such as "Thank you." and
hallucinated song lyrics come from.

Measured over a recording with known silent and spoken stretches on a
correctly levelled input, an empty room sits between 0.0016 and 0.005
while speech runs from 0.06 to 0.25. The default sits in that gap.

## Diagnosing by log

Run the listener with `--log-level debug` and watch the heartbeat,
which reports the current level against the thresholds once a second:

```
msg=heartbeat level=0.0173 noise_floor=0.0011 start_threshold=0.0034 speaking=true
```

Speech should peak at several times the start threshold rather than
brushing against it. If the level during silence sits above the start
threshold, the detector is committing an empty room and the transcript
will acquire lines nobody said.

## Tuning against a recording

Tuning against a live speaker means asking somebody to repeat
themselves. Record the room instead, then replay it as often as
necessary.

Capture a recording that contains both silence and speech:

```bash
ears-listener --room main-hall --record /tmp/room.wav
```

Replay it through the whole pipeline to judge the transcript:

```bash
ears-listener --room test --dry-run --file /tmp/room.wav --fast \
  --no-partials
```

Or sweep the detector's settings over the recording and see how much of
it each would have sent to the model:

```bash
EARS_TUNE_WAV=/tmp/room.wav go test ./internal/audio -run Segmentation -v
```

Aim for committed audio a little above the amount of real speech in the
recording. Well under it means sentences are being lost; well over it
means silence is being sent to the model.

Judge the result by reading the transcript, not only by the totals. On
one recording the aggregate favoured a higher floor, while replaying
end to end showed that the higher value truncated a sentence spoken
deliberately quietly.

## Utterance length

Three settings decide how audio is divided, and the right values depend
on what the machine can keep up with:

- `--silence-ms`, 650 by default, is how much quiet ends an utterance.
  Shortening it feels snappier at the cost of more, shorter chunks and
  slightly worse accuracy, because the model gets less context.
- `--max-utterance-ms`, 14000 by default, forces a commit for a speaker
  who never pauses.
- `--min-utterance-ms`, 400 by default, judges an utterance by how much
  of it was actually voiced, so a cough is discarded rather than
  transcribed.

On a machine without a GPU, longer utterances are markedly more
efficient, because the model's cost barely depends on how much audio it
receives. The Choosing a Model document explains why.

## Letting the model reject silence

The model server can run its own voice detection, which returns nothing
for a chunk containing no speech rather than inventing text. Load the
detection model on the server and enable it on the listener:

```bash
ears-listener --room main-hall --whisper-vad
```

On one measurement a silent chunk returned nothing in 31 milliseconds,
where without detection it returned a full stop in 490 milliseconds. The
transcript stops acquiring inventions and the machine stops spending
time on them.

## Next Steps

- The Capturing Audio document covers the input levels these thresholds
  are measured against.
- The Choosing a Model document explains the relationship between
  utterance length and cost.
- The Troubleshooting document lists the symptoms these settings fix.
