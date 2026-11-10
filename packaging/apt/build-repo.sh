#!/usr/bin/env bash
# Builds a signed apt repository from a directory of .debs.
#
#   APT_SIGNING_KEY=<armoured private key> build-repo.sh <debs-dir> <out-dir>
#
# Writes pool/main/*.deb, dists/stable/{Release,InRelease,Release.gpg} and
# Packages{,.gz} for amd64 and arm64, plus slonik-ears.asc (the public key),
# into <out-dir>. Needs only apt-ftparchive (apt-utils), gpg and coreutils.
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: APT_SIGNING_KEY=<armoured key> $0 <debs-dir> <out-dir>" >&2
  exit 2
fi
if [ -z "${APT_SIGNING_KEY:-}" ]; then
  echo "error: APT_SIGNING_KEY is not set" >&2
  exit 2
fi
for tool in apt-ftparchive gpg gzip cmp du find sort awk; do
  command -v "$tool" >/dev/null 2>&1 || {
    echo "error: $tool not found (apt-ftparchive is in the apt-utils package)" >&2
    exit 2
  }
done

here="$(cd "$(dirname "$0")" && pwd)"
debs="$(cd "$1" && pwd)"
mkdir -p "$2"
out="$(cd "$2" && pwd)"

GNUPGHOME="$(mktemp -d)"
export GNUPGHOME
trap 'rm -rf "$GNUPGHOME"' EXIT

# The pool and indexes are rebuilt from scratch on every run.
rm -rf "$out/pool" "$out/dists"
mkdir -p "$out/pool/main" "$out/dists/stable/main/binary-amd64" "$out/dists/stable/main/binary-arm64"

# The caller guarantees each file name appears once in <debs-dir>. The search
# is recursive, so a copy of a name in a subdirectory is caught too: it is an
# error if the contents differ, and is skipped if they are identical.
declare -A origin
while IFS= read -r -d '' deb; do
  name="$(basename "$deb")"
  dest="$out/pool/main/$name"
  if [ -e "$dest" ]; then
    if ! cmp -s "$deb" "$dest"; then
      echo "error: $deb and ${origin[$name]} differ but would both be written to pool/main/$name" >&2
      exit 1
    fi
    continue
  fi
  cp "$deb" "$dest"
  origin[$name]="$deb"
done < <(find "$debs" -type f -name '*.deb' -print0 | sort -z)

if [ "${#origin[@]}" -eq 0 ]; then
  echo "error: no .deb files found under $debs" >&2
  exit 1
fi

printf '%s\n' "$APT_SIGNING_KEY" | gpg --batch --quiet --import
keyid="$(gpg --batch --list-secret-keys --with-colons | awk -F: '$1 == "fpr" { print $10; exit }')"
if [ -z "$keyid" ]; then
  echo "error: no secret key found in APT_SIGNING_KEY" >&2
  exit 1
fi

# apt-ftparchive reads its paths relative to the current directory.
cd "$out"
apt-ftparchive generate "$here/apt-ftparchive.conf"
apt-ftparchive -c "$here/apt-ftparchive.conf" release dists/stable > Release.new
mv Release.new dists/stable/Release

gpg --batch --yes --local-user "$keyid" --clearsign \
  --output dists/stable/InRelease dists/stable/Release
gpg --batch --yes --local-user "$keyid" --detach-sign --armor \
  --output dists/stable/Release.gpg dists/stable/Release
gpg --batch --yes --armor --export "$keyid" > slonik-ears.asc

# Clean up apt-ftparchive's cache databases; they are not part of the repository.
rm -f "$out"/*.db

du -sh "$out"
