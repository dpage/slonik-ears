# HTTP API

The server exposes a small HTTP and WebSocket interface, used by the
web application and available for scripting. This page lists the
endpoints, what they need, and what they return.

## Authentication

Three kinds of caller exist, and each proves itself differently:

- A viewer supplies the passcode, when one is configured, as a `k`
  query parameter, an `X-Ears-Key` header, or a session cookie obtained
  from the session endpoint.
- A listener supplies the publish token as a bearer token or as a
  `token` query parameter.
- An administrator supplies the admin token as a bearer token or as a
  session cookie obtained from the admin session endpoint.

Both session endpoints throttle repeated failures from one address and
answer 429 once the budget is spent. Only failures count against it, so
a caller presenting a correct credential is unaffected until somebody
else at the same address has exhausted the budget by guessing. It
refills continuously rather than needing to be cleared: a viewer
passcode allows twenty failures a minute, and an exhausted budget
admits another attempt about three seconds later; the admin token
allows ten failures every five minutes, so the equivalent wait is
around thirty seconds.

## Public endpoints

The following table describes the endpoints available to viewers:

| Method and path | Description |
| --- | --- |
| GET /healthz | Version, uptime and room count |
| GET /api/config | Event name, version and authentication state |
| POST /api/session | Exchange a viewer passcode for a cookie |
| GET /api/rooms | List all rooms |
| GET /api/rooms/{id} | Describe one room |
| GET /api/rooms/{id}/transcript | Download the transcript |
| GET /api/rooms/{id}/qr.png | QR code linking to the room |

The transcript endpoint accepts a `format` parameter of `txt`, `json`,
`srt` or `vtt`, defaulting to `txt`; `text` is accepted as a synonym
for `txt`. Passing `timestamps=0` omits the timestamps from the text
format.

The QR code endpoint accepts a `size` parameter, in pixels, honoured
between 64 and 2048.

The text produced by this endpoint contains one line per committed
segment, cut where the speaker paused. The download offered by the room
page instead groups the transcript into sentences, matching what was on
screen. Use this endpoint when the lines need to correspond to the
subtitle cues or the stored transcript.

## WebSocket endpoints

The following table describes the streaming endpoints:

| Path | Caller | Description |
| --- | --- | --- |
| GET /api/watch | Viewer | Subscribe to one room's transcript |
| GET /api/lobby | Viewer | Subscribe to the list of rooms |
| GET /api/publish | Listener | Publish transcript to one room |

A viewer subscribing to `/api/watch` supplies `room` and optionally
`since`, the highest sequence number it already holds, and receives
only what it missed. A server response carrying a reset flag means what
the viewer holds does not join up with what it is about to be sent, so
it should discard the transcript rather than merge. That covers a room
reset between talks, and a long talk whose retained history has been
trimmed past where the viewer got to.

Sequence numbers climb for the life of a room and do not restart when
it is reset, so the first line of the afternoon talk might be numbered
481. A client tracking a cursor can therefore rely on the number alone
to tell whether it is holding the current talk.

A listener publishing to `/api/publish` may be sent an error carrying a
`fatal` flag, which means reconnecting will not help: the room has been
removed, another listener has taken it over, or the two protocol
versions do not match. A client receiving one should stop rather than retry,
because coming back would recreate a room an organiser has just removed
or take the room off the listener that superseded it.

## Administrative endpoints

The following table describes the endpoints guarded by the admin token:

| Method and path | Description |
| --- | --- |
| POST /api/admin/session | Exchange the admin token for a cookie |
| GET /api/admin/rooms | List rooms as an administrator |
| PUT /api/admin/rooms/{id} | Change a room's title, track or speaker |
| POST /api/admin/rooms/{id}/reset | Archive the transcript and empty the room |
| DELETE /api/admin/rooms/{id} | Remove the room and archive its transcript |

The administrative endpoints return 401 when the admin token is absent
or wrong, and also when no admin token is configured on the server,
because the interface is then switched off rather than open. The
session endpoint is the exception: it answers 403 when the server has
no admin token configured, distinguishing "this server does not offer
these controls" from "that token is wrong".

A successful reset or metadata change returns the room as JSON. A
successful delete returns 204 with no body.

## Examples

Changing the speaker between talks takes a PUT with the fields to
change:

```bash
curl -X PUT https://ears.example.org/api/admin/rooms/main-hall \
     -H "Authorization: Bearer $EARS_ADMIN_TOKEN" \
     -H 'Content-Type: application/json' \
     -d '{"title":"Main Hall","speaker":"Sam Roe","track":"Track A"}'
```

Clearing the transcript for the next talk takes a POST with no body:

```bash
curl -X POST https://ears.example.org/api/admin/rooms/main-hall/reset \
     -H "Authorization: Bearer $EARS_ADMIN_TOKEN"
```

Downloading a transcript afterwards needs no token unless a viewer
passcode is configured:

```bash
curl -O "https://ears.example.org/api/rooms/main-hall/transcript?format=txt"
```

## Cross-origin requests

The API sets permissive cross-origin headers for GET requests so a
venue can embed the transcript in its own site without a proxy. The
`allowed_origins` setting restricts which origins may open a WebSocket;
leaving it empty permits same-origin connections and native clients,
which send no origin header.
