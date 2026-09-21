# Accessibility

A live transcript is an accessibility feature before it is anything
else, and treating it as one changes several decisions. This page
describes what the web application does, and the limits worth stating
to an audience.

## What the attendee view provides

The transcript page is built for someone relying on it to follow a
talk:

- The transcript is an ARIA live region, announced politely by a screen
  reader as lines arrive rather than interrupting. Only additions are
  announced: committed text is regrouped into sentences as it arrives,
  and announcing changes as well would have a reader hear the same
  growing sentence from its beginning several times a second.
- Text size and contrast are adjustable from the page, and the choice
  is remembered between visits.
- The page follows the speaker automatically, and stops following when
  the reader scrolls back to re-read something.
- A control returns the reader to the live position when they are
  ready.
- Interim text appears in a lighter style so a guess is never mistaken
  for a quotation.

On a phone, which is where most of the audience reads, the controls
collapse behind a single Display button so that the transcript keeps
the screen:

![The attendee view on a phone, showing three paragraphs of transcript with
the last line in a lighter italic style, and share, download and subtitle
buttons below](img/screens/room-phone.png){ width="320" }

## What the stage display provides

The display beside the stage shows a few lines in large type, sized for
the back of the room. It carries a QR code so anybody who prefers to
read on their own device can pick up the same transcript without a URL
being read aloud.

![The stage display: two sentences in large white type on black, the most
recent line in grey, and a QR code captioned "Follow along on your
phone"](img/screens/stage.png)

The number of lines, the text size and whether the QR code appears are
all adjustable from the URL, which the Managing Rooms document
describes.

## Stating the limits

Machine transcription is an aid rather than a substitute for a human
captioner where accuracy is a legal or contractual requirement. Say so
to the audience rather than leaving them to discover it.

Two limits are worth naming specifically:

- Slonik Ears does not identify speakers, so a panel discussion arrives
  as one stream of text without attribution.
- Accuracy falls with poor audio, several people talking at once, and
  technical vocabulary the glossary does not cover.

The server configuration carries an accessibility notice that appears
beneath every transcript. Agree the wording with the organisers before
the event rather than during it.

## Getting the audio right matters most

No accessibility feature compensates for a poor signal. A feed from the
sound desk transcribes far better than a microphone across a room, and
the difference is larger than any choice of model.

## Next Steps

- The Capturing Audio document explains how to get a clean signal.
- The Managing Rooms document covers the stage display options.
- The Choosing a Model document compares accuracy across models.
