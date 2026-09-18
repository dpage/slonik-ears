# Choosing a Model

The model decides both how accurate the transcript is and whether the
machine can keep up with the speaker. This page gives measured figures
for two very different machines so you can choose without guessing.

## Available models

Models are fetched with `make model MODEL=<name>` into
`~/.cache/whisper`. The following table lists the usual choices and
their sizes:

| Model | Size | Notes |
| --- | --- | --- |
| tiny.en | 78 MB | English only, lowest accuracy, fastest |
| base.en | 148 MB | English only, a clear step up from tiny |
| small.en | 488 MB | English only, a reasonable compromise |
| medium.en | 1.5 GB | English only, slow without a GPU |
| large-v3-turbo | 1.5 GB | Multilingual, fewer decoder layers |
| large-v3 | 2.9 GB | Multilingual, the most accurate available |

Dropping the `.en` suffix selects the multilingual variant of a model.
Builds with a `-q5_0` suffix are quantised and roughly half the size.

## On a machine with a GPU

Use `large-v3`. Measured on an Apple M2 Max with 30 GPU cores,
replaying a recording of a real talk and scoring against twenty phrases
that were actually spoken:

| Model | Phrases correct | Mean speed | Slowest chunk |
| --- | --- | --- | --- |
| small.en | 17 of 20 | 9.9x | 0.4x |
| large-v3-turbo | 18 of 20 | 4.0x | 0.4x |
| large-v3 | 19 of 20 | 3.9x | 1.0x |

Speed is the duration of the audio divided by the time taken, so 3.9x
means the model keeps up with a speaker nearly four times over. On that
machine `large-v3` never fell below real time, and it was the only one
of the three to produce "live transcript" rather than "life transcript".

The turbo variant is the surprise. It is no faster than the full model
in practice, and it replaced a quietly spoken sentence with a
repetition of its own prompt, which is the worst way for a transcript
to be wrong.

## On a machine without a GPU

Small machines are a different proposition. Measured on a Raspberry Pi
4 Model B, four cores at 1.5 GHz, transcribing through the model server
with voice detection enabled:

| Model and options | Audio | Time | Effective speed |
| --- | --- | --- | --- |
| tiny.en | 4 s | 6.8 s | 0.6x |
| tiny.en with audio-ctx 768 | 4 s | 2.9 s | 1.4x |
| tiny.en with audio-ctx 768 | 8 s | 3.4 s | 2.3x |
| base.en with audio-ctx 768 | 8 s | 7.6 s | 1.0x |

Two conclusions follow. The first is that `tiny.en` is the ceiling on a
Pi 4, because `base.en` only reaches about real time and so has nothing
in hand for a fast speaker. The second is that a Pi 4 is workable but
never comfortable, and a faster machine is worth the money if the
transcript matters.

## Why a short utterance costs as much as a long one

Whisper's encoder always processes a thirty second window, padding
shorter audio with silence. The cost therefore barely depends on how
much audio you send. On that Pi, two seconds cost 5.5 seconds and
sixteen seconds cost 6.3 seconds.

A GPU absorbs the fixed cost and nobody notices. A CPU does not, and
since the listener commits an utterance every time the speaker pauses,
the machine spends its life paying a thirty second bill for four
seconds of speech.

Limiting the encoder context recovers most of that. Start the model
server with `--audio-ctx 768`, which covers about fifteen seconds:

```bash
whisper-server \
  --model ~/.cache/whisper/ggml-tiny.en.bin \
  --host 127.0.0.1 --port 8081 --threads 4 \
  --audio-ctx 768 \
  --vad --vad-model ~/.cache/whisper/ggml-silero-v5.1.2.bin
```

Fifteen seconds sits just above the fourteen second maximum utterance,
so nothing is truncated. Lower the value further only if you lower
`--max-utterance-ms` to match.

## The constraint is the processor

On the same Pi, transcribing one clip with one, two, three and four
threads took 27.1, 14.3, 10.6 and 8.4 seconds. That is 81 percent
scaling efficiency, which says the cores are the limit rather than the
memory system. A model occupies 75 to 142 MB against 4 GB of RAM, so
capacity never enters into the question.

## Watching whether the model keeps up

The listener logs a `speed` figure with every committed segment. A
value below 1 means the model is slower than the speaker and the queue
will only grow.

If the figure is marginal, `--no-partials` roughly halves the work by
turning off the interim previews.

## Next Steps

- The Capturing Audio document explains how to give the model a clean
  signal, which matters more than the model choice.
- The Tuning Transcription document covers the settings that decide
  where an utterance begins and ends.
- The Troubleshooting document lists the symptoms of a model that
  cannot keep up.
