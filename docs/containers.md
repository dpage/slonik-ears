# Running in a Container

A container image of the server is published with every change to the
main branch, which is usually the easiest way to put the relay on a
cloud instance. This page covers pulling the image, running it, giving
it its configuration and its storage, and upgrading it later.

Only the server is containerised. The listener needs a microphone and a
Whisper backend on the same machine as the speaker, so it is installed
directly on the machine in the room; the Installing document covers
that.

## What the image contains

The image holds one statically linked binary and the web application
embedded inside it, on top of Alpine:

- `ears-server` is the entry point, so any flag passed to `docker run`
  after the image name reaches the server.
- The web application is compiled into the binary, so there is no
  separate web server and no static files to mount.
- The server runs as the unprivileged user `ears`, with user id 10001.
  It never runs as root.
- `/data` is declared as a volume, and `EARS_DATA_DIR` already points
  at it.
- Port 8080 is exposed, and `EARS_ADDR` already points at it.

## Pulling the image

The image is published to the GitHub container registry and can be
pulled without authenticating.

Pull the current build:

```bash
docker pull ghcr.io/dpage/slonik-ears:latest
```

The following table describes the tags that are published:

| Tag | Points at |
| --- | --- |
| `latest` | The most recent commit on the main branch. |
| `main` | The same thing, named after the branch. |
| `1.2.3`, `1.2`, `1` | A released version, once a version tag exists. The bare major tag is only published from 1.0 onwards. |
| `sha-abc1234` | One specific commit, for pinning exactly. |

Pin to a version tag or a commit tag for an event. Running `latest` on
the day means an unattended restart can pick up a different build than
the one that was rehearsed.

## Running the server

The server needs a publish token before any listener can connect, and
somewhere to keep transcripts. Both are supplied to `docker run`.

Start the relay with a named volume for its data:

```bash
docker run -d --name ears \
  -p 8080:8080 \
  -v ears-data:/data \
  -e EARS_PUBLISH_TOKEN="$(openssl rand -hex 24)" \
  -e EARS_EVENT_NAME="Example Conference 2026" \
  -e EARS_BASE_URL="https://ears.example.org" \
  --restart unless-stopped \
  ghcr.io/dpage/slonik-ears:latest
```

Keep a copy of the publish token, because every listener needs the same
value and the server holds no way to print it back.

Check that it started:

```bash
docker logs ears
curl -sf http://127.0.0.1:8080/healthz
```

## Telling attendees the right address

Set `EARS_BASE_URL` to the address the audience will use. Without it the
server falls back to whatever each request arrived on, which works until
a proxy rewrites it, and the address printed at startup is wrong in a
container either way.

The startup banner works the address out from the network interface it
can see. Inside a container that is the container's own private address,
which nobody outside can reach:

```
  Attendees can watch at:
    http://172.17.0.2:8080
```

With `EARS_BASE_URL` set, the address to read out to the room comes
first:

```
  Attendees can watch at:
    https://ears.example.org
    http://172.17.0.2:8080
```

The QR code on the stage display is built from `EARS_BASE_URL` when it
is set, and from the address the stage display itself used otherwise.
The fallback is usually right, which is what makes the failure worth
guarding against: a reverse proxy that rewrites the `Host` header
produces a QR code pointing at the proxy's idea of the address rather
than the public one, and nobody notices until a phone fails to load it.

## Configuring the server

Almost every setting can be given as an environment variable, which
suits a container better than a file. Pre-declared rooms are the
exception, and need a configuration file.

The following table describes the variables that matter most in a
container; the Server Options document lists all of them:

| Variable | Meaning |
| --- | --- |
| `EARS_PUBLISH_TOKEN` | The secret each listener must present. Required. |
| `EARS_BASE_URL` | The public address, used for QR codes. |
| `EARS_EVENT_NAME` | The event name shown above the room list. |
| `EARS_ADMIN_TOKEN` | Switches on the organiser's page at `/admin`. |
| `EARS_VIEWER_PASSCODE` | Required to watch. Leave unset for a public event. |
| `EARS_TRUST_PROXY` | Honour `X-Forwarded-For`. Only behind a proxy you control. |
| `EARS_DATA_DIR` | Where transcripts are written. Already set to `/data`. |
| `EARS_ADDR` | The address to listen on. Already set to `:8080`. |

Pre-declaring rooms needs a configuration file rather than variables.
Mount one read-only and point the server at it:

```bash
docker run -d --name ears \
  -p 8080:8080 \
  -v ears-data:/data \
  -v ./server.yaml:/etc/ears/server.yaml:ro \
  -e EARS_PUBLISH_TOKEN="..." \
  ghcr.io/dpage/slonik-ears:latest --config /etc/ears/server.yaml
```

Keep tokens in the environment rather than in a mounted file, since a
file is easy to commit by accident and the environment is not.

One variable carries no `EARS_` prefix: `PORT`. It exists because that
is the name platform hosts inject to tell an application which port to
bind, and it is applied after everything else, so on such a host the
platform's choice wins over `EARS_ADDR`. That is the right behaviour on
a platform and a trap under plain Docker, because the image's health
check asks port 8080 specifically. Setting `PORT` to anything else
leaves a container that serves perfectly well and is reported unhealthy
for ever. Map the port on the outside with `-p 9000:8080` instead of
changing the port the server binds.

## Storing transcripts

Transcripts are written to `/data`, one file per room, and are read
back when the server restarts. Without a volume they live only in the
container's writable layer and are lost when it is replaced, which
happens on every upgrade.

A named volume, as in the examples above, is the simplest arrangement
and needs nothing else. To use a directory on the host instead, make it
writable by the user the server runs as:

```bash
mkdir -p /srv/ears-data
chown 10001:10001 /srv/ears-data
docker run -d --name ears -v /srv/ears-data:/data ...
```

Getting that ownership wrong fails quietly, which is worth knowing
before it happens. The server starts normally, because the directory
already exists and nothing writes to it until the first line of
transcript arrives. It then logs one error and carries on:

```
level=ERROR msg="transcript storage is failing; the live transcript
continues but is not being saved" operation="open transcript file"
```

The transcript still reaches the audience, and is still served from
memory, so nothing looks wrong from the outside. It is simply never
written, and the whole event is lost at the next restart. Check for that
line after the first talk starts, rather than after the event.

Docker Desktop on macOS and Windows rewrites the ownership of a bind
mount, so this bites on a Linux server and not on the laptop it was
rehearsed on.

Archived transcripts, created when a room is reset or removed, are
written to an `archive` subdirectory of the same volume. Back the volume
up after the event; nothing in Slonik Ears deletes a transcript, but
nothing stops `docker volume rm` either.

## Using Docker Compose

The repository carries a compose file at `deploy/docker-compose.yml`
which builds the image from source. To run the published image instead,
the following is a complete file:

```yaml
services:
  ears:
    image: ghcr.io/dpage/slonik-ears:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      EARS_EVENT_NAME: "Example Conference 2026"
      EARS_BASE_URL: "https://ears.example.org"
      EARS_PUBLISH_TOKEN: "${EARS_PUBLISH_TOKEN:?set EARS_PUBLISH_TOKEN before starting}"
      EARS_ADMIN_TOKEN: "${EARS_ADMIN_TOKEN:-}"
      EARS_VIEWER_PASSCODE: "${EARS_VIEWER_PASSCODE:-}"
      # Enable when a reverse proxy terminates TLS in front of this.
      EARS_TRUST_PROXY: "${EARS_TRUST_PROXY:-false}"
    volumes:
      - ears-data:/data

volumes:
  ears-data:
```

The token is deliberately not written into the file. Supply it from the
environment when starting the stack:

```bash
EARS_PUBLISH_TOKEN=$(openssl rand -hex 24) docker compose up -d
```

## Putting a reverse proxy in front

Terminating TLS in a proxy is the usual arrangement, and it requires one
setting on the server so that logging and rate limiting see the real
client rather than the proxy.

Set `EARS_TRUST_PROXY=true` only when a proxy you control is in front of
the server, because the header it makes the server trust is otherwise
trivial for a client to forge.

The proxy must forward WebSocket upgrades, since both the listener and
every viewer use one. A proxy that quietly drops the upgrade produces a
page that loads and then never shows a word.

The proxy must also pass the `Host` header through unchanged, or name
the public address in `EARS_ALLOWED_ORIGINS`. A WebSocket is accepted
when its `Origin` matches the `Host` the server sees, so a proxy that
rewrites `Host` to the container's own name makes every browser
connection fail the check whilst the listener, which sends no `Origin`
at all, carries on publishing happily. The symptom is the same page that
loads and never fills, with the transcript arriving perfectly well on
the server.

The server can also terminate TLS itself, with `EARS_TLS_CERT_FILE` and
`EARS_TLS_KEY_FILE` pointing at a certificate mounted into the
container.

## Checking that it is healthy

The image carries a health check that asks the server every thirty
seconds whether it is serving.

Ask Docker what it has concluded:

```bash
docker inspect --format '{{.State.Health.Status}}' ears
```

The same endpoint is useful from outside, and reports the version and
how many rooms exist:

```bash
curl -s http://127.0.0.1:8080/healthz
```

```json
{"rooms":3,"status":"ok","uptime":"2h14m","version":"1.0.0"}
```

## Upgrading

Upgrading replaces the container, which is why the transcripts belong in
a volume rather than in the container itself.

Pull the new image and recreate the container:

```bash
docker pull ghcr.io/dpage/slonik-ears:latest
docker stop ears && docker rm ears
docker run -d --name ears ... ghcr.io/dpage/slonik-ears:latest
```

With Compose, the same thing is two commands:

```bash
docker compose pull
docker compose up -d
```

The server reads the transcripts back from `/data` as it starts, so
rooms and their history survive. Viewers reconnect by themselves and ask
for only the lines they missed, and a listener retries until the relay
answers, so an upgrade between talks costs nothing. Doing it during one
loses whatever was being said at that moment.

## Next Steps

- The Running the Server document covers tokens, storage and
  pre-declared rooms in detail, container or not.
- The Server Options document lists every flag, configuration key and
  environment variable the server accepts.
- The Managing Rooms document describes the organiser's page that
  `EARS_ADMIN_TOKEN` switches on.
- The Troubleshooting document covers what to do when the transcript
  does not reach the audience.
