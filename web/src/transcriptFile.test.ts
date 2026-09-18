import { test } from 'node:test'
import assert from 'node:assert/strict'
import { clock, transcriptFilename, transcriptText } from './transcriptFile.ts'
import type { Room, Segment } from './types.ts'

function seg(seq: number, text: string, startMs: number): Segment {
  return { id: String(seq), seq, text, at: startMs, startMs, endMs: startMs + 1000 }
}

const room: Room = {
  id: 'main-hall',
  title: 'Main Hall',
  speaker: 'Dave Page',
  track: 'Track A',
  live: true,
  viewers: 1,
  cursor: 2,
  startedAt: Date.UTC(2026, 8, 18, 9, 30, 0),
}

test('the file is grouped into the same sentences as the screen', () => {
  // The point of moving this into the browser: one implementation of the
  // splitting, so the download cannot drift from what was being read.
  const text = transcriptText(
    room,
    [
      seg(1, 'Logical replication between Postgres 18 and Postgres 19', 0),
      seg(2, 'uses the publication and subscription model.', 4000),
    ],
    false,
  )
  assert.match(
    text,
    /Logical replication between Postgres 18 and Postgres 19 uses the publication and subscription model\./,
  )
  // And not as two separate lines, which is how it was committed.
  assert.doesNotMatch(text, /Postgres 19\n/)
})

test('the header carries the room details', () => {
  const text = transcriptText(room, [seg(1, 'Hello.', 0)], false)
  const lines = text.split('\n')
  assert.equal(lines[0], 'Main Hall')
  assert.equal(lines[1], 'Speaker: Dave Page')
  assert.equal(lines[2], 'Track: Track A')
  assert.match(lines[3] ?? '', /^Started: /)
  assert.equal(lines[4], '-'.repeat(60))
})

test('a room with nothing but an id still produces a sensible header', () => {
  const bare: Room = { id: 'side-room', live: false, viewers: 0, cursor: 0 }
  const lines = transcriptText(bare, [seg(1, 'Hello.', 0)], false).split('\n')
  assert.equal(lines[0], 'side-room')
  assert.equal(lines[1], '-'.repeat(60))
})

test('timestamps are optional and mark where the sentence began', () => {
  const segments = [seg(1, 'The question is', 65_000), seg(2, 'whether it scales.', 70_000)]
  assert.match(transcriptText(room, segments, true), /\[01:05\] The question is whether it scales\./)
  assert.doesNotMatch(transcriptText(room, segments, false), /\[01:05\]/)
})

test('the clock grows an hours field only when it needs one', () => {
  assert.equal(clock(0), '00:00')
  assert.equal(clock(5_000), '00:05')
  assert.equal(clock(65_000), '01:05')
  assert.equal(clock(3_600_000), '1:00:00')
  assert.equal(clock(3_725_000), '1:02:05')
  assert.equal(clock(-1), '00:00')
})

test('an empty transcript still produces a readable file', () => {
  const text = transcriptText(room, [], false)
  assert.match(text, /^Main Hall\n/)
  assert.ok(text.endsWith('\n'))
})

test('the filename is dated and sorts sensibly', () => {
  assert.match(transcriptFilename(room, 'main-hall'), /^main-hall-\d{4}-\d{2}-\d{2}\.txt$/)
  assert.match(transcriptFilename(null, 'side-room'), /^side-room-\d{4}-\d{2}-\d{2}\.txt$/)
})
