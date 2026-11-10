# Installing on Debian

Debian 13 (trixie) machines can install Slonik Ears from a signed apt
repository instead of building it. There are four packages:
`slonik-ears-listener` for a room machine, which pulls in
`whisper-server` (whisper.cpp built with Vulkan support) or
`sherpa-onnx-server` (for the Parakeet backend), and
`slonik-ears-server` for the relay on a cloud instance. The listener,
`whisper-server` and `sherpa-onnx-server` are built for amd64; the
server is built for amd64 and arm64.

No package enables or starts a service, and none downloads a model. You
edit the configuration first and then turn things on, which keeps an
upgrade from surprising a room in the middle of a talk.

## Adding the repository

Download the signing key and tell apt to trust it for this repository
only:

```bash
sudo curl -fsSLo /etc/apt/keyrings/slonik-ears.asc \
  https://dpage.github.io/slonik-ears/apt/slonik-ears.asc
```

Then create `/etc/apt/sources.list.d/slonik-ears.sources` containing:

```text
Types: deb
URIs: https://dpage.github.io/slonik-ears/apt/
Suites: stable
Components: main
Signed-By: /etc/apt/keyrings/slonik-ears.asc
```

Only stable releases are published here; a release candidate is never
offered by apt.

## A room machine

Install the listener, which brings `whisper-server` with it:

```bash
sudo apt update
sudo apt install slonik-ears-listener
```

Fetch a model into the directory `whisper-server` reads from. The
Choosing a Model page explains which one suits the machine:

```bash
sudo ears-listener --fetch-model medium.en-q5_0 \
  --model-dir /var/lib/whisper-server/models
```

The default in `/etc/default/whisper-server` is `medium.en-q5_0` with
`--audio-ctx 768`, which keeps the room machines about three seconds
behind the speaker; if you fetched a different model, change `WHISPER_MODEL` to
name it. Then edit `/etc/slonik-ears/listener.yaml`
for the room name and the address of the relay, and put the publish
token in a file only you can read:

```bash
mkdir -p ~/.config/slonik-ears
echo 'EARS_PUBLISH_TOKEN=replace-with-the-relay-token' \
  > ~/.config/slonik-ears/listener.env
chmod 600 ~/.config/slonik-ears/listener.env
```

`whisper-server` is a system service and the listener is a service of
your own user, so they are enabled differently:

```bash
sudo systemctl enable --now whisper-server
systemctl --user enable --now slonik-ears-listener
```

The listener is a user service, so it starts when the room's desktop
session does and needs that session to be active to reach the sound
devices; the room machines are expected to log in automatically. Choosing the
capture device is covered in Capturing Audio, including the PulseAudio
monitor sources that `--list-devices` marks and that you do not want for
a microphone.
Automatic login, power settings and the browser on the room display are
the operating system's business and not something the package touches.

## Using Parakeet instead of Whisper

The `sherpa-onnx-server` package runs sherpa-onnx's WebSocket server for
the Parakeet backend, which answers much faster than Whisper on the room
machines (see Choosing a Model). The listener package depends on
`whisper-server | sherpa-onnx-server`, so installing the listener alone
still brings `whisper-server`; install `sherpa-onnx-server` first, or as
well, to use Parakeet:

```bash
sudo apt install sherpa-onnx-server slonik-ears-listener
```

Fetch a model into the directory the service reads from:

```bash
sudo ears-listener --fetch-model parakeet-tdt-0.6b-v2-int8 \
  --model-dir /var/lib/sherpa-onnx-server/models
```

`/etc/default/sherpa-onnx-server` names that model in `SHERPA_MODEL_DIR`
and sets the port (8082) and `SHERPA_THREADS`; change `SHERPA_MODEL_DIR`
if you fetched `parakeet-tdt-0.6b-v3-int8` instead. The package does not
enable the service. Enable it, change `backend` from `whisper` to
`parakeet` in `/etc/slonik-ears/listener.yaml`, and restart the
listener:

```bash
sudo systemctl enable --now sherpa-onnx-server
```

```yaml
backend: parakeet
parakeet_url: "ws://127.0.0.1:8082"
```

```bash
systemctl --user restart slonik-ears-listener
```

Once the listener is working with Parakeet, you can stop whisper-server
so that it does not keep its model in memory or start at boot:

```bash
sudo systemctl disable --now whisper-server
```

The sherpa-onnx server listens on every network interface, so the
service unit restricts it, through systemd's `IPAddressAllow`, to
connections from the machine itself. Leave the listener pointing at
`127.0.0.1`. Parakeet cannot translate, so `translate: true` is
refused at start-up.

## The cloud server

On the relay instance install the server package instead:

```bash
sudo apt update
sudo apt install slonik-ears-server
```

Edit `/etc/slonik-ears/server.yaml`: set `auth.publish_token` (generate
one with `openssl rand -hex 24`), `server.base_url` and the event
details. The file is readable only by root and the `slonik-ears` service
account, which is why the token may live in it. Then enable the service:

```bash
sudo systemctl enable --now slonik-ears-server
```

The server runs as the `slonik-ears` user and keeps its data in
`/var/lib/slonik-ears`. For TLS, either put a reverse proxy in front or
set `tls_cert_file` and `tls_key_file`, as the Running the Server page
describes.

## GPU or CPU

`whisper-server` uses Vulkan, which on an integrated Radeon is usually
faster than the processor, though that depends on the model and the
machine. The service is in the `render` and `video` groups so that it can
open the graphics device. To force the processor, add `--no-gpu` to
`WHISPER_ARGS` in `/etc/default/whisper-server` and restart the
service. Do not guess which is faster: record the room with
`ears-listener --config /etc/slonik-ears/listener.yaml --dry-run --record /tmp/room.wav`,
then replay it with
`ears-listener --config /etc/slonik-ears/listener.yaml --dry-run --file /tmp/room.wav --fast --no-partials`,
once with and once without `--no-gpu`, and compare the `speed` figure the
listener logs for each segment, as the Tuning Transcription page
describes. The one that keeps well above 1 on the machine that will be in
the room is the one to use.

## Upgrading

```bash
sudo apt update && sudo apt upgrade
```

Configuration files you have edited are kept. Services are not
restarted by the upgrade, so restart them yourself when the room is
idle:

```bash
sudo systemctl restart whisper-server
sudo systemctl restart sherpa-onnx-server
systemctl --user restart slonik-ears-listener
sudo systemctl restart slonik-ears-server
```
