#!/bin/sh
# Installs the pinned nfpm release binary (Linux x86-64) into <dir>, checked
# against the published checksum, for builds without a Go toolchain. Does
# nothing if <dir>/nfpm already exists. Needs curl, sha256sum and tar.
#
#   packaging/install-nfpm.sh <dir>
set -eu

if [ $# -ne 1 ]; then
  echo "usage: $0 <dir>" >&2
  exit 2
fi

NFPM_VERSION=2.48.0
# From checksums.txt in the nfpm v2.48.0 release.
NFPM_SHA256=6d7f9a28b2f397934403950fcbba83f598b08d02edbe79a0815b3b4897341750

mkdir -p "$1"
if [ -x "$1/nfpm" ]; then
  exit 0
fi

tarball=nfpm_${NFPM_VERSION}_Linux_x86_64.tar.gz
curl -fsSL -o "$1/$tarball" \
  "https://github.com/goreleaser/nfpm/releases/download/v${NFPM_VERSION}/$tarball"
echo "$NFPM_SHA256  $1/$tarball" | sha256sum -c -
tar -xzf "$1/$tarball" -C "$1" nfpm
rm -f "$1/$tarball"
