#!/bin/sh
# Creates the slonik-ears system user and group before the files are
# unpacked, so that the configuration file can be owned by the group.
set -e

# A group left behind without its user (the user removed by hand) would make
# --group fail, so only ask for the group when it is not already there.
if ! getent passwd slonik-ears >/dev/null; then
  if getent group slonik-ears >/dev/null; then
    adduser --system --ingroup slonik-ears --home /var/lib/slonik-ears --no-create-home slonik-ears
    exit 0
  fi
  adduser --system --group --home /var/lib/slonik-ears --no-create-home slonik-ears
fi
