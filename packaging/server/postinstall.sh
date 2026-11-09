#!/bin/sh
# Deliberately does nothing beyond reloading systemd: the service is neither
# enabled nor started, so the operator edits the configuration first.
set -e

if [ -d /run/systemd/system ]; then
  systemctl daemon-reload || true
fi
