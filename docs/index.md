# Slonik Ears

Slonik Ears provides live transcription for events with several rooms
running at once. A listener sits in each room, hears the talk, has the
audio transcribed by a Whisper model, and streams the resulting text to
a server that shares it with anybody watching.

Attendees read the transcript on their own phones, and a stage display
shows the same text in large type beside the speaker. Only the text
crosses the network; the audio never leaves the room it was recorded
in.

The project exists for the conference case, which has a particular
shape: several tracks at once, an audience on a venue network that
blocks devices from talking to each other, and nobody with time to
debug a Python environment ten minutes before the keynote. The whole
system is two Go binaries and a static web application.

## What you need

Running Slonik Ears for real requires three things in each room, plus
one shared service:

- a machine in the room to capture the audio and transcribe it.
- a microphone, ideally fed from the sound desk rather than a laptop.
- a Whisper model server, which normally runs on that same machine.
- a relay server, shared by every room, that the audience can reach.

The relay can run on the same laptop as a single room. A venue network
that isolates clients from each other defeats that arrangement, so plan
to run the relay on a small cloud instance instead.

## What it does not do

Being clear about the limits saves disappointment later:

- Slonik Ears does not identify who is speaking. Several voices at once
  arrive as a single stream of text.
- Slonik Ears does not replace a human captioner where accuracy is a
  legal or contractual requirement.
- Slonik Ears does not translate between languages by default, although
  a model can be asked to translate into English.

## Next Steps

- The Architecture document explains how the pieces fit together and
  where the audio stops.
- The Quick Start document describes how to see the system working in
  two minutes, with no model and no microphone.
- The Installing document covers building the binaries and fetching a
  model.
