#!/bin/sh
# Creates the slonik-ears system user and group before the files are
# unpacked, so that the configuration file can be owned by the group.
set -e

if ! getent passwd slonik-ears >/dev/null; then
  adduser --system --group --home /var/lib/slonik-ears --no-create-home slonik-ears
fi
