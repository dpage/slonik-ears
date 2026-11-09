#!/bin/sh
# Run by whisper-server.service before starting, so a missing model gives a
# clear message instead of a crash loop. WHISPER_MODEL and WHISPER_VAD_MODEL
# come from /etc/default/whisper-server.
fetch='sudo ears-listener --fetch-model <name> --model-dir /var/lib/whisper-server/models'
status=0

if [ ! -r "${WHISPER_MODEL:-}" ]; then
  echo "whisper-server: no model at ${WHISPER_MODEL:-}; fetch one with: $fetch" >&2
  status=1
fi
if [ ! -r "${WHISPER_VAD_MODEL:-}" ]; then
  echo "whisper-server: no VAD model at ${WHISPER_VAD_MODEL:-}; fetch one with: $fetch" >&2
  status=1
fi

exit $status
