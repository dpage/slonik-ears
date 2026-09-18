// Fills a throwaway server with an invented transcript, so the screenshots
// show something readable rather than a loop of the sample recording. None
// of the text here was said by anybody; see README.md in this directory for
// how to start the server it publishes to.
// Deliberately not EARS_PUBLISH_TOKEN and friends: a shell that already has
// those exported for a real event would otherwise hand this script the real
// event's secrets, and the connection would fail in a thoroughly confusing
// way when the throwaway server rejected them.
const BASE = process.env.EARS_SHOTS_BASE ?? 'http://127.0.0.1:8131'
const ADMIN = process.env.EARS_SHOTS_ADMIN_TOKEN ?? 'shots-admin'
const PUBLISH = process.env.EARS_SHOTS_PUBLISH_TOKEN ?? 'shots'

const ROOMS = {
  'main-hall': {
    meta: { title: 'Main Hall', track: 'Track A', speaker: 'Alex Roe' },
    segments: [
      'Good morning, and thank you all for turning up this early.',
      'I want to talk about the thing nobody puts on a slide, which is what happens to your replication setup at three in the morning.',
      'Postgres handles replication lag rather well these days,',
      'provided you remember to monitor the replication slot and not let it grow unbounded over a long weekend.',
      'That is, as far as I can tell, how most of us learned about replication slots in the first place.',
      'So the first thing I would check is pg_stat_replication, and specifically the write, flush and replay positions.',
      'If replay is drifting behind flush, the standby is receiving the data but cannot apply it fast enough.',
    ],
    partial: 'and that usually points at a long running query on the standby',
  },
  lightning: {
    meta: { title: 'Lightning Talks', track: 'Track A', speaker: 'Jo Okafor' },
    segments: [
      'Five minutes, one idea, and I have already spent fifteen seconds of it.',
      'Every index you add makes reads faster and writes slower, and most of us only ever measure the first half of that trade.',
      'So here is the smallest useful habit I can offer you.',
      'Before you create an index, write down the query it is for, and put that query in the index comment.',
      'Six months later, when somebody asks whether the index is still needed, the answer is right there.',
      'You can find the unused ones with pg_stat_user_indexes, and the ones nobody can explain with a bit of honesty.',
    ],
    partial: 'which brings me neatly to the end of my time',
  },
  workshop: {
    meta: { title: 'Workshop Room', track: 'Track B', speaker: 'Sam Patel' },
    segments: [
      'Welcome to the workshop; please grab a seat near a power socket while you still can.',
      'We are going to spend the next two hours breaking a database and then putting it back together.',
      'The first exercise is a restore from a base backup, because that is the one everybody skips.',
      'If your notes say the restore takes twenty minutes, and you have never timed it, then your notes say nothing at all.',
    ],
    partial: '',
  },
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

async function publish(id, room) {
  const url = new URL('/api/publish', BASE)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  url.searchParams.set('room', id)
  url.searchParams.set('token', PUBLISH)

  const ws = new WebSocket(url.toString())
  await new Promise((resolve, reject) => {
    ws.addEventListener('open', resolve, { once: true })
    ws.addEventListener(
      'error',
      () => reject(new Error(`could not publish to ${id}; is the server up, and is the token right?`)),
      { once: true },
    )
  })
  const send = (m) => ws.send(JSON.stringify(m))
  send({ type: 'hello', version: 1, room: { id, ...room.meta, language: 'en' } })
  await sleep(200)

  // Backdate the timestamps so the talk looks as though it has been running
  // for a few minutes rather than starting the instant the script did.
  let at = Date.now() - room.segments.length * 9000
  let ms = 0
  room.segments.forEach((text, i) => {
    const dur = 4000 + text.length * 40
    send({
      type: 'final',
      segment: { seq: 0, id: `${id}-${i}`, text, startMs: ms, endMs: ms + dur, at, language: 'en' },
    })
    ms += dur + 1200
    at += 9000
  })
  if (room.partial) {
    await sleep(200)
    send({ type: 'partial', partial: { text: room.partial, afterSeq: 0, at: Date.now() } })
  }
  send({ type: 'status', status: { level: 0.42, listening: true, backend: 'ready' } })
  await sleep(600)
  return ws
}

for (const id of Object.keys(ROOMS)) {
  const res = await fetch(`${BASE}/api/admin/rooms/${id}/reset`, {
    method: 'POST',
    headers: { authorization: `Bearer ${ADMIN}` },
  })
  // A room that does not exist yet is not an error; publishing creates it.
  if (!res.ok && res.status !== 404) {
    console.error('reset', id, res.status, await res.text())
  }
}
await sleep(400)

const open = []
for (const [id, room] of Object.entries(ROOMS)) {
  open.push(await publish(id, room))
}

// The workshop is meant to look finished rather than live, so its publisher
// disconnects whilst the other two stay open and keep their rooms live.
open[2].close()

console.log('seeded; leaving the publishers connected. Ctrl-C when done.')
await new Promise(() => {})
