# Changelog

All notable changes to Slonik Ears are recorded here. The format is
based on [Keep a Changelog](https://keepachangelog.com/), and the
project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Documentation site covering installation, operation, tuning and the
  HTTP API.
- An organiser's page for changing a room's details, clearing its
  transcript between talks, and removing a room.
- A glossary of technical terms, sent to the model as context and
  applied to its output, with the list replaceable per deployment.
- Per-channel selection for multichannel capture interfaces, including
  automatic detection of which input carries the microphone.
- Recording of captured audio for offline tuning, and a sweep over
  segmentation settings to support it.

### Changed

- The transcript is grouped into sentences rather than into the chunks
  the segmenter committed, both on screen and in the download.
- Voice detection thresholds were re-measured against a correctly
  levelled input.

### Fixed

- Capture on a multichannel interface no longer averages every channel,
  which attenuated a single microphone by the number of inputs.
- A device selector no longer matches a monitor source in preference to
  a real input.
- Resetting a room no longer leaves viewers reporting that the room is
  not live.
- Output consisting only of music markers or punctuation is discarded
  rather than shown.
