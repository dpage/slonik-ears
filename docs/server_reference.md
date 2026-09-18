# Server Options

This page lists every option `ears-server` accepts. Options may be
given on the command line, in a YAML configuration file, or through the
environment; a command line flag beats the environment, which beats the
configuration file.

## Command line flags

The following table describes the flags the server accepts:

| Flag | Default | Description |
| --- | --- | --- |
| --addr | :8080 | Address to listen on |
| --data-dir | none | Directory for transcript storage |
| --config | none | Path to a YAML configuration file |
| --log-level | info | One of debug, info, warn or error |
| --log-format | text | Either text or json |
| --version | off | Print the version and exit |

Leaving `--data-dir` unset keeps transcripts in memory only, so a
restart loses them.

## Environment variables

The following table describes the environment variables the server
reads:

| Variable | Description |
| --- | --- |
| EARS_ADDR | Address to listen on |
| PORT | Port to listen on, set by the hosting platform |
| EARS_BASE_URL | Public URL, used for generated links and QR codes |
| EARS_DATA_DIR | Directory for transcript storage |
| EARS_EVENT_NAME | Event name shown in the lobby |
| EARS_PUBLISH_TOKEN | Token authorising a listener to publish |
| EARS_VIEWER_PASSCODE | Passcode required to watch, if any |
| EARS_ADMIN_TOKEN | Token guarding the organiser page and admin API |
| EARS_ALLOWED_ORIGINS | Extra origins permitted to open WebSockets |
| EARS_EVENT_TAGLINE | Tagline shown beneath the event name |
| EARS_EVENT_NOTICE | Accessibility notice shown beneath every transcript |
| EARS_HISTORY | Finalised segments retained per room, in memory |
| EARS_MAX_VIEWERS | Cap on concurrent viewers per room |
| EARS_TRUST_PROXY | Take the client address from forwarded headers |
| EARS_ALLOW_AUTO_ROOMS | Allow a listener to create an undeclared room |
| EARS_TLS_CERT_FILE | Certificate for terminating TLS directly |
| EARS_TLS_KEY_FILE | Private key for terminating TLS directly |

The one variable without the `EARS_` prefix is `PORT`, and it is not
ours to rename. Most platform hosts inject it to tell an application
which port to bind, so honouring the name is what allows the server to
be deployed to one without any configuration at all. It is applied
after `EARS_ADDR`, so on a host that sets it, the platform wins.

## Configuration file

The configuration file groups settings under `server`, `auth`, `event`
and `rooms`. A complete example ships in the repository as
`configs/server.example.yaml`, reproduced here:

```yaml
# Slonik Ears server configuration.
#
# Everything here is optional: the server runs with no config at all, provided
# EARS_PUBLISH_TOKEN is set. Environment variables override this file, so keep
# secrets out of it and out of git.

event:
  name: "PGConf Europe 2026"
  tagline: "Live transcripts for every track"
  # Shown under the transcript. Worth keeping: machine transcription is very
  # good and still occasionally very wrong.
  notice: >-
    Automatically generated live transcript. It will contain errors, especially
    with names and technical terms. Please do not quote it verbatim.

server:
  addr: ":8080"
  # The address attendees use. Only needed for QR codes and join links; when
  # it is empty the server works it out from the request.
  base_url: "https://ears.example.org"
  # Transcripts are written here as one JSONL file per room. Leave empty to
  # keep everything in memory and lose it on restart.
  data_dir: "./data"
  # Segments kept in memory per room, for late joiners and reconnects.
  history: 2000
  # Enable only when running behind a reverse proxy you control.
  trust_proxy: false
  # 0 means no limit. A few hundred viewers per room is untroubling; the
  # bandwidth per viewer is a few hundred bytes a second.
  max_viewers_per_room: 0
  # For running TLS directly rather than behind a proxy.
  # tls_cert_file: /etc/ssl/ears/fullchain.pem
  # tls_key_file: /etc/ssl/ears/privkey.pem

auth:
  # Listeners must present this. Set it with EARS_PUBLISH_TOKEN instead of
  # writing it here:  openssl rand -hex 24
  publish_token: ""
  # Leave empty for an open event, which is usually the point of the exercise.
  # Set it (EARS_VIEWER_PASSCODE) for an internal or paid event.
  viewer_passcode: ""
  # Guards the admin API and the organiser page at /admin. Leave it empty and
  # both are switched off, which is the safe default: this is the interface
  # that can clear a talk off every screen in the building.
  admin_token: ""
  # Extra origins allowed to open WebSockets. Same-origin always works, and
  # native clients (the listener) send no Origin header. Add entries here only
  # if you are embedding the transcript in another site.
  allowed_origins: []
  # Whether a listener may create a room that is not listed below. Turn this
  # off for a locked-down event where the room list is decided in advance.
  allow_auto_rooms: true

# Rooms declared here appear in the lobby before their listener connects, and
# can each carry their own publish token so every room operator gets a
# different secret.
rooms:
  - id: main-hall
    title: "Main Hall"
    track: "Track A"
    language: "en"
  - id: seminar-1
    title: "Seminar Room 1"
    track: "Track B"
    language: "en"
    # publish_token: "a-different-secret-for-this-room"
  - id: workshop
    title: "Workshop Room"
    track: "Workshops"
    language: "en"
```

The remainder of this page describes each key in turn. The shorter
example below shows only the structure:

```yaml
server:
  addr: ":8080"
  base_url: "https://ears.example.org"
  data_dir: "/var/lib/slonik-ears"
  history: 2000
  trust_proxy: true
  max_viewers_per_room: 0
  tls_cert_file: ""
  tls_key_file: ""

event:
  name: "Slonik Ears"
  tagline: ""
  notice: "Automatically generated live transcript."

auth:
  publish_token: ""
  viewer_passcode: ""
  admin_token: ""
  allowed_origins: []
  allow_auto_rooms: true

rooms:
  - id: main-hall
    title: Main Hall
    track: Track A
    speaker: ""
    language: en
```

## Server settings

The following table describes the keys beneath `server`:

| Key | Default | Description |
| --- | --- | --- |
| addr | :8080 | Address to listen on |
| base_url | none | Public URL for generated links and QR codes |
| data_dir | none | Directory for transcript storage |
| history | 2000 | Finalised segments retained per room, in memory |
| trust_proxy | false | Take the client address from forwarded headers |
| max_viewers_per_room | 0 | Cap on concurrent viewers, 0 for unlimited |
| tls_cert_file | none | Certificate for terminating TLS directly |
| tls_key_file | none | Private key for terminating TLS directly |

## Event settings

The following table describes the keys beneath `event`, which control
what attendees see around the transcript:

| Key | Default | Description |
| --- | --- | --- |
| name | Slonik Ears | Event name shown in the lobby |
| tagline | none | Short line shown beneath the event name |
| notice | see below | Accessibility notice shown beneath every transcript |

The notice defaults to a sentence explaining that the transcript is
generated automatically, that errors are inevitable, and that it should
not be quoted verbatim. Agree the wording with the organisers before
the event rather than during it.

## Authentication settings

The following table describes the keys beneath `auth`:

| Key | Default | Description |
| --- | --- | --- |
| publish_token | none | Token authorising a listener to publish |
| viewer_passcode | none | Passcode required to watch, empty for open |
| admin_token | none | Token guarding the organiser page and admin API |
| allowed_origins | empty | Extra origins permitted to open WebSockets |
| allow_auto_rooms | true | Allow a listener to create an undeclared room |

A listener is refused only when both the global `publish_token` and the
room's own `publish_token` are empty, so a room carrying its own token
still accepts listeners on a server that sets no global one. Leaving
both empty means the server refuses that room rather than accepting
anonymous publication.

Leaving `admin_token` empty switches the administrative interface off.

## Room settings

The following table describes the keys beneath each entry in `rooms`:

| Key | Description |
| --- | --- |
| id | Room identifier, used in URLs |
| title | Room title shown to attendees |
| track | Track name, used to group rooms in the lobby |
| speaker | Speaker name shown to attendees |
| language | Spoken language as an ISO-639-1 code |
| publish_token | Token for this room, overriding the global one |

Declaring rooms populates the lobby before any listener connects.
