# Running the Server

The server relays text from every listener to every viewer, keeps the
transcript history, and serves the web application. This page covers
where to run it, how to secure it, and how transcripts are stored.

## Where to run it

For a single room, the server can run on the same machine as the
listener. For anything larger, run it somewhere the audience can reach.

Venue networks commonly isolate wireless clients from one another,
which prevents phones from reaching a laptop on the same network. Assume
that is the case until proven otherwise, and put the server on a small
cloud instance with a name in DNS and a certificate in place before the
morning of the event.

## Starting the server

The server needs a publish token before any listener can connect. Create
one and start the server with a data directory:

```bash
export EARS_PUBLISH_TOKEN=$(openssl rand -hex 24)
export EARS_EVENT_NAME="Example Conference 2026"
ears-server --data-dir ./data
```

The server prints the address attendees should use:

```
msg="Slonik Ears server started" addr=[::]:8080 rooms_configured=0 auto_rooms=true

  Attendees can watch at:
    http://192.0.2.10:8080
```

Without a publish token the server refuses every listener, rather than
running an open relay that anybody on the venue network can write to.

## Tokens and access

Three separate secrets control the three kinds of access, and each has
a different audience:

- The publish token authorises a listener to send transcript. Set it
  with `EARS_PUBLISH_TOKEN`, and give it only to the machines in the
  rooms.
- The viewer passcode, `EARS_VIEWER_PASSCODE`, is required to watch.
  Leave it empty for a public event, which is usually the point.
- The admin token, `EARS_ADMIN_TOKEN`, guards the organiser's page and
  the administrative API. Leaving it unset switches both off entirely
  rather than leaving them open.

A room may also carry its own publish token in the configuration file,
so each room's operator can be given a separate secret.

## Storing transcripts

Passing `--data-dir` writes one file per room, as JSON objects one per
line, and reloads them when the server restarts. Without it, a restart
loses the morning's talks.

Archived transcripts, created when a room is reset or removed, are
written to an `archive` subdirectory with a timestamp in the name.
Nothing in Slonik Ears deletes a transcript.

## Pre-declaring rooms

Rooms appear automatically when a listener connects, which suits an
ad-hoc setup. For a planned event, declare them in a configuration file
so the lobby is populated before anything starts:

```yaml
rooms:
  - id: main-hall
    title: Main Hall
    track: Track A
    language: en
  - id: side-room
    title: Side Room
    track: Track B
```

Setting `allow_auto_rooms` to false then prevents a listener creating
any room that is not declared, which suits a locked down conference.

The Server Options document reproduces the complete example
configuration, which also ships in the repository as
`configs/server.example.yaml`.

## Behind a proxy

When the server runs behind a reverse proxy, set `trust_proxy` so that
client addresses in the logs reflect the forwarded header rather than
the proxy. Set `base_url` so generated links and QR codes point at the
public name rather than the internal one.

The server can also terminate TLS itself, given `tls_cert_file` and
`tls_key_file`, which avoids a proxy for a simple deployment.

## Checking it is healthy

The server answers `GET /healthz` with its version, uptime and room
count, which suits whatever monitoring you already run.

## Next Steps

- The Running a Listener document covers the other half of the system.
- The Managing Rooms document explains the organiser's page and how to
  turn a room around between talks.
- The Server Options document lists every flag, environment variable
  and configuration key.
