# Changelog

All notable changes to Slonik Ears are recorded here. The format is
based on [Keep a Changelog](https://keepachangelog.com/), and the
project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2026-09-21

The first release. Everything below is new, so the list describes what
Slonik Ears does rather than what changed.

### Added

- A relay server, `ears-server`, which receives transcript from every
  room, keeps a history, and serves the attendee web application from
  inside its own binary. It has no runtime dependencies.
- A room listener, `ears-listener`, which captures audio, decides where
  each utterance begins and ends, has it transcribed, and publishes the
  text. The audio never leaves the room it was recorded in.
- Transcription through whisper.cpp's own server, or through any
  endpoint offering the same OpenAI-compatible interface, and a mock
  backend for rehearsing the rest of the system with no model at all.
- An attendee view with the reader in charge: text size, three themes
  including high contrast, and optional timestamps, all remembered.
  Committed text is regrouped into sentences, and the unconfirmed line
  is visually distinct so a guess is never mistaken for a quotation.
- Accessibility throughout the attendee view: the transcript is an ARIA
  live region announcing only additions, scrolling follows the speaker
  but yields the moment a reader scrolls back, and animation respects
  `prefers-reduced-motion`.
- A stage display showing the last few lines in large type beside the
  speaker, with a QR code so anybody can pick the transcript up on
  their own phone without a URL being read aloud.
- A lobby listing every room, live ones first.
- An organiser's page at `/admin` for retitling a room, naming the next
  speaker, clearing the transcript between talks and removing a room
  for the day. Nothing there destroys a transcript: a reset archives
  the previous talk first.
- Resumption by sequence number, so a phone that has been in somebody's
  pocket receives only what it missed, and is told to start afresh when
  what it holds no longer joins up.
- Transcript export as plain text, JSON, SubRip and WebVTT, plus an
  optional local JSONL copy written by the listener as insurance
  against the network.
- A glossary of technical terms, used twice: as context for the model,
  which helps it hear the right words, and as a rewrite over its
  output, which makes it spell them consistently. A Postgres glossary
  is built in and can be extended or replaced per deployment.
- Capture from multichannel interfaces, measuring every input at
  start-up to find the one carrying the microphone, averaging a genuine
  stereo pair on request, and warning when an input is clipping or
  nearly silent.
- Recording of captured audio, and an offline sweep over the
  segmentation settings, for tuning against a recording of the room
  rather than against a live audience.
- Separate secrets for the three kinds of access: a publish token per
  server or per room, an optional passcode for viewers, and an admin
  token that switches the organiser's page on. Repeated failed
  sign-ins from one address are throttled.
- Transcript persistence to disk, reloaded at startup, with archives
  kept when a room is reset or removed.
- A container image of the server, published for amd64 and arm64, and a
  documentation site covering installation, running an event, choosing
  a model, capturing audio, tuning and the HTTP API.

### Known limitations

- Slonik Ears does not identify speakers, so several voices arrive as
  one stream of text.
- It is an aid rather than a substitute for a human captioner where
  accuracy is a legal or contractual requirement.
- The listener needs cgo for audio capture, so it is built natively for
  each platform; the server is pure Go and cross-compiles freely.

