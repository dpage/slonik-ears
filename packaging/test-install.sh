#!/usr/bin/env bash
# End-to-end test of the Debian packages: builds a signed apt repository from
# them, serves it locally, installs from it as a user would, and checks what
# was installed. Run as root inside a disposable debian:trixie (amd64)
# container from the repository root; it changes the system it runs on.
#
#   packaging/test-install.sh <debs-dir>
#
# <debs-dir> is searched recursively and must hold the listener, server,
# whisper-server and sherpa-onnx-server packages. Needs network access for apt and the model fetch.
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <debs-dir>" >&2
  exit 2
fi

here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/.." && pwd)"
debs="$(cd "$1" && pwd)"
cd "$root"

work="$(mktemp -d)"
server_pid=
whisper_pid=
cleanup() {
  if [ -n "$whisper_pid" ]; then kill "$whisper_pid" 2>/dev/null || true; fi
  if [ -n "$server_pid" ]; then kill "$server_pid" 2>/dev/null || true; fi
  rm -rf "$work"
}
trap cleanup EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

step() {
  echo
  echo "=== $*"
}

export DEBIAN_FRONTEND=noninteractive

step "Installing the test tools"
apt-get update
apt-get install -y --no-install-recommends \
  apt-utils gpg gpg-agent ca-certificates curl python3
"$here/install-nfpm.sh" "$work/bin"

# The repository is built from a copy, so the packages added later in the
# test never land in the caller's directory.
mkdir -p "$work/debs"
find "$debs" -type f -name '*.deb' -exec cp {} "$work/debs/" \;
listener_deb="$(find "$work/debs" -name 'slonik-ears-listener_*_amd64.deb' | head -n 1)"
whisper_deb="$(find "$work/debs" -name 'whisper-server_*_amd64.deb' | head -n 1)"
[ -n "$listener_deb" ] || fail "no slonik-ears-listener amd64 package under $debs"
[ -n "$whisper_deb" ] || fail "no whisper-server amd64 package under $debs"
find "$work/debs" -name 'slonik-ears-server_*_amd64.deb' | grep -q . ||
  fail "no slonik-ears-server amd64 package under $debs"
find "$work/debs" -name 'sherpa-onnx-server_*_amd64.deb' | grep -q . ||
  fail "no sherpa-onnx-server amd64 package under $debs"

step "Generating a throwaway signing key"
GNUPGHOME="$work/gnupg"
export GNUPGHOME
mkdir -m 0700 "$GNUPGHOME"
gpg --batch --quiet --passphrase '' \
  --quick-generate-key 'Slonik Ears test key <test@example.org>' ed25519 sign 1d
APT_SIGNING_KEY="$(gpg --batch --armor --export-secret-keys)"
export APT_SIGNING_KEY

step "Building the repository"
"$here/apt/build-repo.sh" "$work/debs" "$work/repo"

# apt checks InRelease; this checks the detached signature that older clients
# use, against the public key the repository publishes.
gpg --batch --quiet --no-default-keyring --keyring "$work/verify.kbx" \
  --import "$work/repo/slonik-ears.asc"
gpg --batch --no-default-keyring --keyring "$work/verify.kbx" \
  --verify "$work/repo/dists/stable/Release.gpg" "$work/repo/dists/stable/Release" ||
  fail "Release.gpg does not verify against slonik-ears.asc"

step "Serving the repository on 127.0.0.1:8000"
python3 -m http.server --bind 127.0.0.1 --directory "$work/repo" 8000 \
  >"$work/http.log" 2>&1 &
server_pid=$!
for _ in $(seq 1 50); do
  curl -fsS -o /dev/null http://127.0.0.1:8000/slonik-ears.asc && break
  sleep 0.2
done
curl -fsS -o /dev/null http://127.0.0.1:8000/slonik-ears.asc ||
  fail "the repository is not being served"

step "Adding the source and key"
install -d -m 0755 /etc/apt/keyrings
curl -fsS -o /etc/apt/keyrings/slonik-ears.asc http://127.0.0.1:8000/slonik-ears.asc
cat >/etc/apt/sources.list.d/slonik-ears.sources <<'EOF'
Types: deb
URIs: http://127.0.0.1:8000/
Suites: stable
Components: main
Signed-By: /etc/apt/keyrings/slonik-ears.asc
EOF

apt_update() {
  apt-get update 2>&1 | tee "$work/update.log"
  if grep -q '^W:' "$work/update.log"; then
    fail "apt-get update gave warnings"
  fi
}

step "Installing the packages"
apt_update
# stdin is closed so that a configuration file prompt fails rather than hangs.
apt-get install -y slonik-ears-listener slonik-ears-server sherpa-onnx-server </dev/null

step "Checking the installed system"
ears-listener --version
ears-server --version
whisper-server --help >/dev/null 2>&1 || fail "whisper-server --help failed"

[ -f /usr/lib/systemd/system/sherpa-onnx-server.service ] ||
  fail "the sherpa-onnx-server unit is missing"
[ -x /usr/bin/sherpa-onnx-offline-websocket-server ] ||
  fail "/usr/bin/sherpa-onnx-offline-websocket-server does not resolve to an executable"
if ldd /usr/bin/sherpa-onnx-offline-websocket-server 2>&1 | grep -q 'not found'; then
  ldd /usr/bin/sherpa-onnx-offline-websocket-server >&2
  fail "sherpa-onnx-offline-websocket-server has unresolved libraries"
fi

[ -f /usr/lib/systemd/user/slonik-ears-listener.service ] ||
  fail "the listener's user unit is missing"
# systemd is not running in the container, so enabling a unit would show up as
# a symlink in a .wants directory.
for unit in slonik-ears-server.service whisper-server.service sherpa-onnx-server.service slonik-ears-listener.service; do
  if compgen -G "/etc/systemd/system/*.wants/$unit" >/dev/null ||
    compgen -G "/etc/systemd/user/*.wants/$unit" >/dev/null; then
    fail "$unit was enabled by the package"
  fi
done

perms="$(stat -c '%a %G' /etc/slonik-ears/server.yaml)"
[ "$perms" = "640 slonik-ears" ] ||
  fail "/etc/slonik-ears/server.yaml is '$perms', expected '640 slonik-ears'"

step "Checking check-model without a model"
set +e
# shellcheck source=/dev/null
check_out="$(set -a && . /etc/default/whisper-server && /usr/lib/slonik-ears/whisper/check-model 2>&1)"
check_status=$?
set -e
echo "$check_out"
[ "$check_status" -eq 1 ] || fail "check-model exited $check_status, expected 1"
grep -qF 'ears-listener --fetch-model' <<<"$check_out" ||
  fail "check-model did not print the fetch command"

step "Checking sherpa-onnx-server's check-model without a model"
set +e
# shellcheck source=/dev/null
sherpa_out="$(set -a && . /etc/default/sherpa-onnx-server && /usr/lib/slonik-ears/sherpa-onnx/check-model 2>&1)"
sherpa_status=$?
set -e
echo "$sherpa_out"
[ "$sherpa_status" -eq 1 ] || fail "sherpa-onnx-server check-model exited $sherpa_status, expected 1"
grep -qF 'ears-listener --fetch-model parakeet-tdt-0.6b-v2-int8' <<<"$sherpa_out" ||
  fail "sherpa-onnx-server check-model did not print the fetch command"

step "Transcribing testdata/sample.wav with tiny.en"
models=/var/lib/whisper-server/models
ears-listener --fetch-model tiny.en --model-dir "$models"
whisper-server --model "$models/ggml-tiny.en.bin" --host 127.0.0.1 --port 8081 \
  --no-gpu >"$work/whisper.log" 2>&1 &
whisper_pid=$!
# Loading the model is slow under emulation.
for _ in $(seq 1 300); do
  curl -fsS -o /dev/null http://127.0.0.1:8081/ 2>/dev/null && break
  kill -0 "$whisper_pid" 2>/dev/null || {
    cat "$work/whisper.log" >&2
    fail "whisper-server exited"
  }
  sleep 1
done
curl -fsS -o "$work/inference.json" -F file=@testdata/sample.wav \
  http://127.0.0.1:8081/inference || {
  cat "$work/whisper.log" >&2
  fail "the inference request failed"
}
cat "$work/inference.json"
echo
python3 -I -c '
import json, sys
text = json.load(open(sys.argv[1]))["text"].strip()
sys.exit(0 if text else 1)
' "$work/inference.json" || fail "whisper-server returned no text"
kill "$whisper_pid"
wait "$whisper_pid" 2>/dev/null || true
whisper_pid=

step "Checking that an edited configuration survives an upgrade"
echo '# edited' >>/etc/slonik-ears/listener.yaml
version="$(dpkg-deb -f "$listener_deb" Version)"
dpkg-deb -x "$listener_deb" "$work/listener-root"
VERSION="${version}+1" BINARY="$work/listener-root/usr/bin/ears-listener" \
  "$work/bin/nfpm" package --config packaging/listener/nfpm.yaml \
  --packager deb --target "$work/debs/"
"$here/apt/build-repo.sh" "$work/debs" "$work/repo"
apt_update
apt-get upgrade -y </dev/null
installed="$(dpkg-query -W -f '${Version}' slonik-ears-listener)"
[ "$installed" = "${version}+1" ] ||
  fail "slonik-ears-listener is $installed after the upgrade, expected ${version}+1"
grep -qx '# edited' /etc/slonik-ears/listener.yaml ||
  fail "the edit to /etc/slonik-ears/listener.yaml was lost in the upgrade"

step "Checking that build-repo.sh deduplicates identical packages"
whisper_name="$(basename "$whisper_deb")"
mkdir -p "$work/dupe/a" "$work/dupe/b"
cp "$whisper_deb" "$work/dupe/a/"
cp "$whisper_deb" "$work/dupe/b/"
"$here/apt/build-repo.sh" "$work/dupe" "$work/dupe-repo"
count="$(find "$work/dupe-repo/pool" -name "$whisper_name" | wc -l)"
[ "$count" -eq 1 ] || fail "found $count copies of $whisper_name in the pool, expected 1"

step "Checking that build-repo.sh rejects different packages with one name"
echo 'not the same' >>"$work/dupe/b/$whisper_name"
if "$here/apt/build-repo.sh" "$work/dupe" "$work/dupe-repo" 2>"$work/dupe.log"; then
  fail "build-repo.sh accepted two different files named $whisper_name"
fi
cat "$work/dupe.log"
for f in "$work/dupe/a/$whisper_name" "$work/dupe/b/$whisper_name"; do
  grep -qF "$f" "$work/dupe.log" || fail "build-repo.sh did not name $f"
done

echo
echo "All package install tests passed."
