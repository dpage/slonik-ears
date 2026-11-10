#!/usr/bin/env bash
# Prints the Debian package version for a build.
#
#   deb-version.sh <ref_type> <ref_name> <sha>
#
# A tag v1.2.0 gives 1.2.0, and a prerelease tag v1.2.0-rc1 gives 1.2.0~rc1
# (the tilde makes it sort before the final release). Anything that is not a
# tag gives 0.0.0~dev+<7-char sha>.
set -eu

if [ "$#" -ne 3 ]; then
  echo "usage: $0 <ref_type> <ref_name> <sha>" >&2
  exit 2
fi

ref_type="$1"
ref_name="$2"
sha="$3"

if [ "$ref_type" = "tag" ]; then
  v="${ref_name#v}"
  # Only the first hyphen separates the prerelease suffix.
  echo "${v/-/\~}"
else
  echo "0.0.0~dev+${sha:0:7}"
fi
