# Architecture

Slonik Ears separates the work that must happen near the microphone
from the work that must happen near the audience. This page describes
the pieces, the direction data flows between them, and the reasoning
behind the arrangement.

## The pieces

The system has three processes per room and one shared relay:

- The Whisper model server turns audio into text. It is
  `whisper-server` from whisper.cpp, or any endpoint that speaks the
  same HTTP interface.
- The listener, `ears-listener`, captures audio, decides where each
  utterance begins and ends, sends the audio for transcription, and
  publishes the text.
- The relay, `ears-server`, receives text from every listener, keeps a
  history, and shares the text with viewers. It also serves the web
  application.
- The browser shows the transcript, either on an attendee's phone or on
  a screen beside the stage.

## How data flows

Audio travels the shortest possible distance, and text travels the
rest:

```
   Room A                          Server                      Audience
 ┌──────────────────┐         ┌──────────────────┐         ┌────────────────┐
 │ ears-listener    │         │ ears-server      │         │ phone, laptop  │
 │  mic ─▶ VAD ─▶   │  text   │  rooms, history  │  text   │ stage screen   │
 │  whisper.cpp ────┼────────▶│  fan-out, web    ├────────▶│ /r/main-hall   │
 └──────────────────┘   WSS   └──────────────────┘   WSS   └────────────────┘
   Room B ──────────────────────────▲
   Room C ──────────────────────────┘
```

The listener talks to the model server over HTTP on the loopback
interface, so the audio never reaches a network cable. The listener
talks to the relay over a WebSocket carrying only finished text and
short status messages. Viewers receive that same text over a second
WebSocket.

## Why the audio stays put

Keeping transcription in the room is a deliberate constraint rather
than an accident of implementation. A recording of a conference talk
contains the voices of everybody who asked a question, and those people
did not agree to have their voices sent anywhere. Transcribing locally
means the only thing that leaves the room is text that was going on a
screen regardless.

The arrangement has a practical benefit too. Venue networks are
unreliable and often slow, and streaming audio across one would add
both latency and a new way for the transcript to fail.

## How an utterance is built

The listener does not send a continuous stream to the model. Whisper
works on chunks, so the listener has to decide where one utterance ends
and the next begins:

1. Audio arrives from the capture device in 20 millisecond frames at
    16 kHz, mono.
2. A voice activity detector classifies each frame as speech or not,
    using an adaptive estimate of the room's noise floor.
3. Frames accumulate into an utterance while somebody is speaking.
4. The utterance is committed when the speaker pauses, or when it
    reaches the maximum utterance length.
5. The committed audio goes to the model, and the resulting text is
    published to the relay.

While an utterance is still being spoken, the listener periodically
sends the audio so far for a quick interim transcription. That interim
text appears on screen in a lighter style and is replaced when the
utterance is committed.

## What the relay keeps

The relay holds a bounded history of finished segments per room, in
memory, and can also write them to disk. Each segment receives a
sequence number from the relay when it arrives.

Those sequence numbers matter more than they look. A viewer that
reconnects sends the last number it saw, and receives only what it
missed, which is what makes a phone that has been in somebody's pocket
catch up in one message rather than replaying the whole talk.

## Where the text is shaped

Several stages adjust the text between the model and the reader:

- The listener discards output that is plainly not speech, such as
  hallucinated applause markers or a lone full stop over silence.
- The listener rewrites terms from a glossary so jargon is spelt
  consistently.
- The browser regroups committed segments into sentences, because the
  chunk boundaries fall where the speaker paused rather than where a
  sentence ended.

The committed segments themselves are never rewritten after
publication. A segment whose text changed would break the sequence
number contract that lets viewers resume.

## Next Steps

- The Quick Start document shows the system running without a model or
  a microphone.
- The Capturing Audio document explains how the listener chooses a
  device and a channel.
- The Tuning Transcription document covers the voice activity detector
  and the settings that control it.
