#!/bin/sh
# Run by sherpa-onnx-server.service before starting, so a missing model gives
# a clear message instead of a crash loop. SHERPA_MODEL_DIR comes from
# /etc/default/sherpa-onnx-server.
fetch='sudo ears-listener --fetch-model parakeet-tdt-0.6b-v2-int8 --model-dir /var/lib/sherpa-onnx-server/models'
status=0

for f in encoder.int8.onnx decoder.int8.onnx joiner.int8.onnx tokens.txt; do
  if [ ! -r "${SHERPA_MODEL_DIR:-}/$f" ]; then
    echo "sherpa-onnx-server: no $f in ${SHERPA_MODEL_DIR:-}; fetch the model with: $fetch" >&2
    status=1
  fi
done

exit $status
