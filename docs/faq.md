# Frequently Asked Questions

This page answers the questions that come up when somebody first
considers using Slonik Ears at an event.

## Does the audio leave the room?

No. The listener sends audio to a model server over the loopback
interface, and only the resulting text crosses the network.

The exception is deliberate: pointing the listener at a cloud
transcription service sends audio to that service. Consider whether the
speakers and the audience would expect that before doing so.

## Does it identify who is speaking?

No. Several voices arrive as a single stream of text without
attribution, so a panel discussion reads as one continuous transcript.

## What languages does it support?

The model decides. English-only models carry an `.en` suffix and are
faster; the multilingual models handle many languages and are selected
with `--language`, or with `auto` to let the model decide.

A model can also translate into English rather than transcribing
verbatim, using `--translate`.

## Can it run without a GPU?

Yes, within limits. A Raspberry Pi 4 manages the smallest model at
roughly 1.4 times real time with the right settings, which works but
leaves nothing in hand. The Choosing a Model document gives measured
figures.

## How much bandwidth does a viewer use?

Roughly 160 bytes per second, or about 0.6 MB per viewer-hour, over a
compressed WebSocket. A venue network carrying a few hundred viewers is
carrying very little traffic.

## What happens when the network fails?

The listener keeps transcribing and buffers the text, then delivers it
when the connection returns. Setting `--transcript` also writes every
committed segment to a local file, so nothing said is lost even if the
relay never comes back.

## Can the audience join late?

Yes. A viewer connecting partway through receives the transcript so
far, bounded by how much history the server retains, which defaults to
2000 segments per room.

## Do I need to restart anything between talks?

No. The organiser's page changes the speaker and clears the transcript
while the listener keeps running, which the Managing Rooms document
describes.

## Are transcripts deleted when a room is reset?

No. The previous transcript is archived alongside the live ones with a
timestamp in the name. Nothing in Slonik Ears deletes a transcript.

## Can I give a speaker their transcript afterwards?

Yes. The room page offers a download that reads as the audience saw it,
grouped into sentences. The API also offers text, JSON, SRT and VTT for
scripted access.

## Still have questions?

Report a problem or ask a question on the
[issue tracker](https://github.com/dpage/slonik-ears/issues).
