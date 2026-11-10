#!/bin/sh
# Builds the sherpa-onnx-server package from the prebuilt static release
# pinned in VERSION, which bundles onnxruntime. Nothing is compiled: the
# tarball and the licence are downloaded, checked against the checksums in
# VERSION, and the WebSocket server binary is repackaged.
# Run as root inside debian:trixie (amd64) from the repository root:
#
#   packaging/sherpa-onnx-server/build.sh <outdir>
#
# Set WORK to a persistent directory to reuse the earlier downloads.
#
# Layout: the binary lives in /usr/lib/slonik-ears/sherpa-onnx/, with
# /usr/bin/sherpa-onnx-offline-websocket-server a symlink to it.
set -eu

if [ $# -ne 1 ]; then
  echo "usage: $0 <outdir>" >&2
  exit 2
fi

here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
mkdir -p "$1"
outdir=$(cd "$1" && pwd)

# shellcheck source=/dev/null
. "$here/VERSION"

WORK=${WORK:-$(mktemp -d)}
stage=$WORK/stage
mkdir -p "$WORK"

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends bzip2 ca-certificates curl

# nfpm from its release binary, checked against the published checksum.
"$root/packaging/install-nfpm.sh" "$WORK"

tarball_url=https://github.com/k2-fsa/sherpa-onnx/releases/download/v${SHERPA_ONNX_VERSION}/${SHERPA_ONNX_TARBALL}
license_url=https://raw.githubusercontent.com/k2-fsa/sherpa-onnx/${SHERPA_ONNX_COMMIT}/LICENSE

# A file already in WORK is reused only if it still matches its checksum.
download() {
  url=$1
  file=$2
  sum=$3
  if [ ! -f "$file" ] || ! echo "$sum  $file" | sha256sum -c - >/dev/null 2>&1; then
    curl -fsSL --retry 3 -o "$file" "$url"
  fi
  echo "$sum  $file" | sha256sum -c -
}
download "$tarball_url" "$WORK/$SHERPA_ONNX_TARBALL" "$SHERPA_ONNX_SHA256"
download "$license_url" "$WORK/LICENSE" "$SHERPA_ONNX_LICENSE_SHA256"

# Only the one binary is wanted from the tarball.
binary=sherpa-onnx-offline-websocket-server
rm -rf "$stage"
mkdir -p "$stage"
tar -xjf "$WORK/$SHERPA_ONNX_TARBALL" -C "$stage" --strip-components=2 \
  "${SHERPA_ONNX_TARBALL%.tar.bz2}/bin/$binary"
chmod 0755 "$stage/$binary"
ls -l "$stage"

# The binary must not need anything the package does not depend on.
if ldd "$stage/$binary" | grep -q 'not found'; then
  ldd "$stage/$binary" >&2
  echo "unresolved libraries in $stage/$binary" >&2
  exit 1
fi
echo "Libraries the binary needs (keep nfpm.yaml depends in step):"
ldd "$stage/$binary" || true

cd "$root"
STAGE=$stage LICENSE_FILE=$WORK/LICENSE DEB_VERSION=$DEB_VERSION \
  "$WORK/nfpm" package --config packaging/sherpa-onnx-server/nfpm.yaml \
  --packager deb --target "$outdir/"
ls -l "$outdir"/sherpa-onnx-server_*.deb
