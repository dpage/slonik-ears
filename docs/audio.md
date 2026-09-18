# Capturing Audio

The quality of the transcript depends more on the audio reaching the
model than on the model itself. This page covers choosing a source,
selecting the right device and channel, and setting the level.

## Choosing a source

The options, in ascending order of how well they work:

- A laptop's built-in microphone hears the room rather than the
  speaker, and reads like it.
- A lapel or headset microphone into a USB audio interface is a large
  improvement.
- A feed from the mixing desk into a USB audio interface is the best
  option by a distance, because it carries the same signal the public
  address system is amplifying, with no room acoustics and no audience.
- A virtual device such as BlackHole or Loopback captures audio already
  playing on the machine, which is how you transcribe a video call.

## Listing devices

Ask the listener what it can see before configuring anything:

```bash
ears-listener --list-devices
```

The output numbers each device and marks the system default. On Linux
hosts running PulseAudio it also marks monitor sources, which are
loopbacks of an output rather than anything the machine can hear.

Select a device by index, by exact identifier, or by part of its name:

```bash
ears-listener --room main-hall --device "Scarlett"
```

A monitor is only selected when nothing else matches the name given, or
when the name itself mentions a monitor. Capturing a monitor on purpose
remains possible, and is how a video call gets transcribed.

## Multichannel interfaces

A multichannel interface needs more care than it first appears. Many
offer no mono format at all: a sixteen input interface offers sixteen
channels and nothing else, so a request for one channel is answered by
averaging every channel the device has. With a microphone on input one
and the rest unplugged and silent, that average is the microphone
divided by sixteen.

The listener therefore captures at the device's native channel count
and mixes down only the channels carrying the microphone. With nothing
specified it listens to every channel for two seconds and uses whichever
is loudest, then reports what it found:

```
msg="input channels measured" device_channels=16 using=[1] ch1=0.084
```

Override that when the room was quiet while it was deciding, or when
you know the wiring:

```bash
ears-listener --room main-hall --device "US-16x08" --channel 3
```

Averaging several channels suits a genuine stereo microphone, and is
requested as a comma separated list such as `--channel 1,2`.

## Setting the level

The listener reports the input peak at startup and warns when the level
is unhelpful:

```
msg="the input is clipping: turn the gain down on the interface" channel=1 peak=1.015
```

Aim for a peak around 0.5. A peak of 1.0 is not a strong signal, it is
a damaged one: the converter has run out of headroom and the tops of
the waveform are gone. Whisper transcribes clipped speech noticeably
worse, and nothing downstream can restore what the interface discarded.

A very quiet input earns the opposite warning. Raise the gain on the
interface before reaching for the detector's settings, because more
signal helps the model as well as the detector.

## Microphone permission on macOS

macOS asks for microphone permission on first use, and the prompt
belongs to whichever application runs the command, whether that is
Terminal, another terminal emulator, or the binary itself. Grant it
under System Settings, Privacy and Security, Microphone.

Skipping the prompt does not produce an error. Capture records digital
silence instead, and the room shows as live with an empty transcript.

## Recording for later analysis

Segmentation is a state machine over the audio, so tuning it against a
live speaker means asking somebody to repeat themselves. The listener
can record what it captured instead:

```bash
ears-listener --room main-hall --record /tmp/room.wav
```

A recording contains every word said near the microphone, by anybody.
Whether to make one at a real event is a question for whoever is
running the event, and the file belongs nowhere near a source
repository.

## Next Steps

- The Tuning Transcription document explains how to use a recording to
  choose detector settings.
- The Running a Listener document covers the remaining options.
- The Troubleshooting document lists the symptoms of a misconfigured
  input.
