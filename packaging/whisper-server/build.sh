#!/bin/sh
# Builds the whisper-server package from the whisper.cpp release pinned in
# VERSION, with the Vulkan backend and runtime-dispatched CPU backends.
# Run as root inside debian:trixie (amd64) from the repository root:
#
#   packaging/whisper-server/build.sh <outdir>
#
# Set WORK to a persistent directory to reuse an earlier checkout and build.
#
# Layout: everything lives in /usr/lib/slonik-ears/whisper/, with
# /usr/bin/whisper-server a symlink to the binary there. whisper-server calls
# ggml_backend_load_all(), and with GGML_BACKEND_DL the backends
# (libggml-cpu-*.so, libggml-vulkan.so) are not linked in but found at run
# time. At the pinned commit, ggml_backend_load_best() in
# ggml/src/ggml-backend-reg.cpp searches GGML_BACKEND_DIR (unset here), then
# the directory of readlink("/proc/self/exe"), then the current directory.
# The kernel resolves symlinks in /proc/self/exe, so starting the binary
# through /usr/bin/whisper-server still searches the private directory. The
# shared libraries the binary links against (libwhisper, libggml,
# libggml-base) are found through its $ORIGIN RUNPATH.
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

NFPM_VERSION=2.48.0
# From checksums.txt in the nfpm v2.48.0 release.
NFPM_SHA256=6d7f9a28b2f397934403950fcbba83f598b08d02edbe79a0815b3b4897341750

WORK=${WORK:-$(mktemp -d)}
src=$WORK/whisper.cpp
build=$WORK/build
stage=$WORK/stage

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends \
  build-essential cmake git ca-certificates curl libvulkan-dev glslc spirv-headers

# nfpm from its release binary, checked against the published checksum.
if [ ! -x "$WORK/nfpm" ]; then
  tarball=nfpm_${NFPM_VERSION}_Linux_x86_64.tar.gz
  curl -fsSL -o "$WORK/$tarball" \
    "https://github.com/goreleaser/nfpm/releases/download/v${NFPM_VERSION}/$tarball"
  echo "$NFPM_SHA256  $WORK/$tarball" | sha256sum -c -
  tar -xzf "$WORK/$tarball" -C "$WORK" nfpm
fi

if [ ! -d "$src" ]; then
  git clone --depth 1 --branch "$WHISPER_CPP_TAG" \
    https://github.com/ggml-org/whisper.cpp.git "$src"
fi
head=$(git -C "$src" rev-parse HEAD)
if [ "$head" != "$WHISPER_CPP_COMMIT" ]; then
  echo "whisper.cpp $WHISPER_CPP_TAG is $head, expected $WHISPER_CPP_COMMIT" >&2
  exit 1
fi

# GGML_NATIVE=OFF with GGML_CPU_ALL_VARIANTS builds one CPU backend per x86-64
# feature level and the loader picks the best one the machine supports.
# CMAKE_BUILD_WITH_INSTALL_RPATH puts $ORIGIN into the build tree's binaries,
# which are the ones staged below, instead of the build directory's path.
# shellcheck disable=SC2016 # $ORIGIN is for the dynamic linker, not the shell
cmake -S "$src" -B "$build" \
  -DCMAKE_BUILD_TYPE=Release \
  -DGGML_VULKAN=ON \
  -DGGML_NATIVE=OFF \
  -DGGML_BACKEND_DL=ON \
  -DGGML_CPU_ALL_VARIANTS=ON \
  -DBUILD_SHARED_LIBS=ON \
  -DWHISPER_BUILD_TESTS=OFF \
  -DCMAKE_INSTALL_RPATH='$ORIGIN' \
  -DCMAKE_BUILD_WITH_INSTALL_RPATH=ON
cmake --build "$build" --target whisper-server -j "$(nproc)"

# Stage the binary, the libraries it links against (under their sonames) and
# the dynamically loaded backends into one directory.
rm -rf "$stage"
mkdir -p "$stage"
install -m 0755 "$build/bin/whisper-server" "$stage/"
find "$build/bin" -maxdepth 1 -type f -name 'lib*.so*' | while read -r lib; do
  soname=$(readelf -d "$lib" | sed -n 's/.*(SONAME).*\[\(.*\)\]/\1/p')
  install -m 0644 "$lib" "$stage/${soname:-$(basename "$lib")}"
done
strip --strip-unneeded "$stage"/*
ls -l "$stage"

# Everything must resolve, and nothing private may come from outside the
# staged directory.
for f in "$stage"/*; do
  if ldd "$f" | grep -q 'not found'; then
    ldd "$f" >&2
    echo "unresolved libraries in $f" >&2
    exit 1
  fi
done
echo "Packages providing external libraries (keep nfpm.yaml depends in step):"
for f in "$stage"/*; do ldd "$f"; done | awk '$3 ~ /^\// { print $3 }' |
  grep -v "^$stage/" | xargs readlink -f | sort -u | xargs dpkg -S |
  sed 's/:.*//' | sort -u

cd "$root"
STAGE=$stage SRC=$src DEB_VERSION=$DEB_VERSION \
  "$WORK/nfpm" package --config packaging/whisper-server/nfpm.yaml \
  --packager deb --target "$outdir/"
ls -l "$outdir"/whisper-server_*.deb
