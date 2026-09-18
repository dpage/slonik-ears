# Retaking the screenshots

The images under `docs/img/screens/` are captured from a real server with
Playwright, so they can be regenerated whenever the interface changes rather
than quietly going stale.

Nothing here touches your own data or your own transcripts: the server is a
throwaway on port 8131 with its own data directory, and the transcript is
invented by `seed.mjs`. Never point this at a server that has carried a real
talk, because the seed script resets every room it knows about.

From the repository root, in three terminals:

```bash
# 1. a throwaway relay
make build
EARS_PUBLISH_TOKEN=shots EARS_ADMIN_TOKEN=shots-admin \
  EARS_EVENT_NAME="Example Conference 2026" \
  ./bin/ears-server --addr 127.0.0.1:8131 --data-dir /tmp/shotdata

# 2. the invented transcript; leave it running so the rooms stay live
node web/screenshots/seed.mjs

# 3. the capture itself
cd web && node screenshots/capture.mjs
```

The tokens are hard coded to `shots` and `shots-admin`, and are overridden
with `EARS_SHOTS_PUBLISH_TOKEN` and `EARS_SHOTS_ADMIN_TOKEN` rather than the
usual `EARS_` names, so that a shell already holding a real event's secrets
cannot hand them to the throwaway server.

The capture writes straight into `docs/img/screens/`, so `git diff --stat`
afterwards shows which images actually changed.
