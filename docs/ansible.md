# Deploying with Ansible

The `ansible/` directory in the repository holds playbooks that build a
relay server and any number of room machines from fresh Debian 13
installs. They do everything the Installing on Debian page describes,
and also prepare the room machines' desktops to run unattended. Run them
again at any time to apply a changed setting.

The playbooks need [Ansible](https://docs.ansible.com/) 2.15 or later on
the machine you run them from, and nothing beyond `ansible-core`. Every
target machine needs SSH access for an account with `sudo`.

## What the Playbooks Do

The repository includes three playbooks: `server.yml` for the relay,
`listeners.yml` for the room machines, and `site.yml`, which runs both
with the server first.

On the relay server, the playbooks:

- add the Slonik Ears apt repository and install `slonik-ears-server`.
- write `/etc/slonik-ears/server.yaml` with the event details, the tokens
  and one room for each listener in the inventory.
- enable and start the `slonik-ears-server` service.
- install Caddy in front of the server with a Let's Encrypt certificate,
  when `ears_domain` is set.

On each room machine, the playbooks:

- replace the apt sources with the Debian archive, including
  `trixie-backports`, and add the Slonik Ears repository.
- install GNOME and PipeWire if the machine does not already have them.
- create an `ears` account with a locked password and no `sudo` rights,
  and log the desktop in as that account automatically.
- stop the machine from sleeping and the screen from blanking or locking.
- install the listener with `sherpa-onnx-server` (or `whisper-server`),
  fetch the model and start the transcription service.
- stop and disable whichever transcription server the room does not use.
- write `/etc/slonik-ears/listener.yaml` and the account's token file,
  and enable the listener as a service of the `ears` account.
- reboot the machine when the kernel, login or power settings change, so
  that the desktop session and the listener start.

The playbooks overwrite `/etc/apt/sources.list.d/debian.sources` and
empty `/etc/apt/sources.list` on the room machines, so a local mirror
configured there does not survive the first run. The relay server's apt
sources are left alone and must already work.

The listener runs as a service of the `ears` account because it needs
that account's desktop session to reach the sound devices; it starts
whenever the session does. The listener keeps a local copy of each
room's transcript in `~ears/.local/state/slonik-ears-listener/`.

## Tokens

The playbooks generate the publish token and the organiser's admin
token on the first run, so you never type either. Ansible writes them to
a `credentials/` directory beside the inventory on the machine you run
the playbooks from, and reads them back on later runs, so the server
and every listener share the same tokens. Git ignores that directory.

The following table describes the generated files:

| File | Purpose |
| --- | --- |
| `credentials/publish_token` | The token listeners present to the server; written into `server.yaml` and each room account's `listener.env`. |
| `credentials/admin_token` | The token for the organiser's page at `/admin`; written into `server.yaml`. |

Keep the `credentials/` directory safe, and copy the admin token from it
when you need to sign in to the organiser's page. Delete a file and run
the playbooks again to replace that token everywhere at once. Set
`ears_admin_enabled: false` to switch the organiser's page off.

## Writing an Inventory

The repository includes an example inventory in
`ansible/inventory/example/`, which uses documentation addresses that
need replacing. Git ignores every other directory under
`ansible/inventory/`, so copy the example to a directory of your own and
edit the copy:

```bash
cd ansible
cp -R inventory/example inventory/myevent
```

The inventory needs exactly one host in the `ears_server` group and
one host for each room in the `ears_listeners` group. The following
`hosts.yml` describes a relay and two rooms:

```yaml
all:
  children:
    ears_server:
      hosts:
        relay:
          ansible_host: 192.0.2.10
    ears_listeners:
      hosts:
        room-a:
          ansible_host: 192.0.2.21
          ears_room: main-hall
          ears_room_title: Main Hall
          ears_track: Track A
        room-b:
          ansible_host: 192.0.2.22
          ears_room: seminar-1
          ears_room_title: Seminar Room 1
          ears_track: Track B
          ears_device: USB Audio
```

Set `ansible_user` in `group_vars/all.yml` to the account Ansible logs
in as, along with the event details described below.

## Variables

The following table describes the variables you are most likely to set;
the defaults live in each role's `defaults/main.yml`:

| Variable | Where | Default | Meaning |
| --- | --- | --- | --- |
| `ears_room` | Each listener | None; required | The room's id: up to 64 lowercase letters, digits, hyphens and underscores, starting with a letter or digit. |
| `ears_room_title` | Each listener | The room id | The room's name in the lobby. |
| `ears_track` | Each listener | Empty | The track shown beside the room. |
| `ears_language` | Each listener | `en` | The room's language, as an ISO 639-1 code. |
| `ears_device` | Each listener | Empty, the default input | The capture device, as `ears-listener --list-devices` names it. |
| `ears_channel` | Each listener | Empty, the loudest | The input channel carrying the microphone. |
| `ears_backend` | Listeners | `parakeet` | `parakeet` or `whisper`. |
| `ears_parakeet_model` | Listeners | `parakeet-tdt-0.6b-v2-int8` | The Parakeet model; the default is English only, and `parakeet-tdt-0.6b-v3-int8` covers 25 European languages. |
| `ears_sherpa_threads` | Listeners | `6` | Threads for `sherpa-onnx-server`. |
| `ears_whisper_model` | Listeners | `medium.en-q5_0` | The Whisper model, when `ears_backend` is `whisper`. |
| `ears_listener_extra` | Listeners | Empty | Any other `listener.yaml` settings, as a mapping; it must not repeat a setting the playbooks already write. |
| `ears_mt7902_wifi` | Listeners | `false` | Install the backports kernel and `firmware-mediatek` for MediaTek MT7902 Wi-Fi. |
| `ears_reboot` | Listeners | `true` | Reboot when the kernel, login or power settings change. |
| `ears_user` | All | `ears` | The room machines' desktop account. |
| `ears_event_name` | All | `Example Conference` | The event's name, shown in the lobby. |
| `ears_event_tagline` | All | Empty | A line shown under the event's name. |
| `ears_event_notice` | All | A warning about errors | The notice shown under the transcript. |
| `ears_viewer_passcode` | All | Empty, an open event | A passcode attendees must enter. |
| `ears_admin_enabled` | All | `true` | Generate an admin token and switch the organiser's page on. |
| `ears_allow_auto_rooms` | All | `true` | Let a listener create a room the server was not told about. |
| `ears_history` | All | `2000` | The number of lines the server keeps for each room. |
| `ears_extra_rooms` | All | Empty | Rooms with no listener of their own, each with an `id` and optionally a `title`, `track` and `language`. |
| `ears_domain` | All | Empty | A domain name for the server; setting it adds Caddy and HTTPS. |
| `ears_acme_email` | All | Empty | The address Let's Encrypt writes to about the certificate. |
| `ears_server_url` | All | Derived | The address listeners and attendees use; override it when the rooms reach the server by another name. |
| `ears_credentials_dir` | All | `credentials/` beside the inventory | Where the generated tokens are kept on the machine running Ansible. |
| `ears_apt_url` | All | The project's repository | The Slonik Ears apt repository, for a mirror of your own. |

Without `ears_domain`, the server answers plain HTTP on port 8080 and
the listeners use the server's `ansible_host` address. Set
`ears_server_url` when the rooms reach the server by a different
address, such as a public name for a server on a private network.

## Serving over HTTPS

Setting `ears_domain` installs Debian's `caddy` package on the server,
which obtains a certificate from Let's Encrypt and forwards requests to
the server on `127.0.0.1:8080`. The server then listens only on the
loopback address and trusts Caddy's forwarded addresses. The domain
name must resolve to the server, and ports 80 and 443 must be open to
the internet before the first run, or Let's Encrypt cannot issue the
certificate.

In `group_vars/all.yml`, set:

```yaml
ears_domain: ears.example.org
ears_acme_email: organisers@example.org
```

Removing `ears_domain` later returns the server to plain HTTP on port
8080 but leaves Caddy installed and running; stop or remove Caddy by
hand.

## Running the Playbooks

Run the playbooks from the `ansible/` directory so that Ansible finds its
configuration and roles.

The following command sets up the server and every room machine:

```bash
ansible-playbook -i inventory/myevent/hosts.yml site.yml --ask-become-pass
```

Run `server.yml` or `listeners.yml` instead to set up one side only, and
add `--limit room-a` to work on a single room machine. Drop
`--ask-become-pass` if the account has passwordless `sudo`.

The first run on a room machine downloads about 660 MB of Parakeet
model, and installs GNOME if the machine has no desktop yet, so expect
it to take a while. The machine reboots at the end of that first run
and comes back logged in as `ears` with the listener running. Later runs
change only what the inventory changes, restart only the services those
changes affect, and reboot only when the kernel, login or power settings
change.

### Checking the Result

The server's room list shows each room and whether its listener is
connected. The following command fetches the list from the server:

```bash
curl -s http://192.0.2.10:8080/api/rooms
```

On a room machine, the following command shows the listener's recent
log:

```bash
sudo journalctl _UID=$(id -u ears) --since -10min -o cat
```

A good start logs `transcription backend ready` and
`connected to server`. A warning that every input channel was near silent means a
quiet room or too little gain on the audio interface.

### Machines Set Up by Hand

The playbooks can take over machines installed by hand, but they
replace the existing configuration files rather than merging with them.
On the first run they generate a new publish token, so the server and
every listener in the inventory change to it together; a listener
outside the inventory keeps the old token and can no longer publish.

### Changing the Rooms

Changing a room's `ears_room` or removing a listener from the inventory
updates the server's configuration, but the server keeps any room that
already has a saved transcript. Remove such a room from the organiser's
page, which the Managing Rooms document describes.

## What the Playbooks Leave to You

A few things depend on the venue or the hardware, so the playbooks leave
them alone:

- The playbooks do not configure networking. A room machine on Wi-Fi
  needs a connection saved system-wide with NetworkManager, such as one
  made with `sudo nmcli device wifi connect <SSID> --ask`, so that it
  connects at boot before anybody logs in.
- An MT7902 machine has no Wi-Fi until the backports kernel is running,
  so its first run needs wired Ethernet.
- The playbooks do not open the stage display; open the room's stage
  page in a full-screen browser on the room machine's desktop.
- The audio interface's gain is a physical control; set it using the
  input level the listener logs at startup.

## Upgrading

The playbooks install the Slonik Ears packages but do not upgrade them,
so that a run during an event never changes the software under a talk.
The one exception is `ears_mt7902_wifi`, which keeps the backports
kernel and firmware current and reboots the machine when they change;
avoid running the playbooks against those rooms during an event.
Upgrade with apt when the rooms are idle, as the Installing on Debian
page describes, or with an ad hoc command:

```bash
ansible -i inventory/myevent/hosts.yml all -b -m apt -a "upgrade=safe update_cache=true"
```

Then restart the services, because a playbook run restarts a service
only when its settings change, not after an upgrade. The following
commands restart the server, then the transcription server and listener
on every room machine:

```bash
ansible -i inventory/myevent/hosts.yml ears_server -b -m systemd -a "name=slonik-ears-server state=restarted"
ansible -i inventory/myevent/hosts.yml ears_listeners -b -m shell -a "systemctl try-restart sherpa-onnx-server whisper-server && systemctl --user -M ears@ daemon-reload && systemctl --user -M ears@ restart slonik-ears-listener"
```

## Next Steps

- The Installing on Debian document describes the same steps by hand.
- The Capturing Audio document explains how to find the device and
  channel to set for each room.
- The Running an Event document provides a checklist for the day.
