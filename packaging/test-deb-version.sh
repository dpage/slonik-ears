#!/usr/bin/env bash
# Checks packaging/deb-version.sh against the version mapping the release
# workflow relies on. Exits non-zero if any case mismatches.
set -u

here="$(cd "$(dirname "$0")" && pwd)"
fail=0

check() {
  local want="$1"
  shift
  local got
  got="$("$here/deb-version.sh" "$@" 2>&1)"
  if [ "$got" != "$want" ]; then
    echo "FAIL: deb-version.sh $* printed '$got', want '$want'"
    fail=1
  else
    echo "ok: $* -> $want"
  fi
}

check "1.2.0"             tag v1.2.0 abcdef1234
check "1.2.0~rc1"         tag v1.2.0-rc1 abcdef1234
check "2.0.0~beta2"       tag v2.0.0-beta2 abcdef1234
check "0.0.0~dev+abcdef1" branch main abcdef1234

exit "$fail"
