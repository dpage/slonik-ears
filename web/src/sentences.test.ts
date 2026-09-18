import { test } from 'node:test'
import assert from 'node:assert/strict'
import { splitSentences, toParagraphs } from './sentences.ts'
import type { Segment } from './types.ts'

function seg(seq: number, text: string, at = seq * 1000): Segment {
  return { id: String(seq), seq, text, at, startMs: at, endMs: at }
}

function texts(segments: Segment[]): string[] {
  return toParagraphs(segments).map((p) => p.text)
}

test('a sentence spread over several segments becomes one paragraph', () => {
  // This is the case that makes the transcript look arbitrary: the chunker cut
  // where the speaker drew breath, not where the sentence ended.
  assert.deepEqual(
    texts([
      seg(1, 'Logical replication between Postgres 18 and Postgres 19'),
      seg(2, 'uses the publication and subscription model.'),
    ]),
    ['Logical replication between Postgres 18 and Postgres 19 uses the publication and subscription model.'],
  )
})

test('several sentences in one segment become several paragraphs', () => {
  assert.deepEqual(
    texts([seg(1, 'Still six. Can you still hear me after the silence?')]),
    ['Still six.', 'Can you still hear me after the silence?'],
  )
})

test('a sentence still being spoken is shown rather than held back', () => {
  // Waiting for the full stop would delay text already transcribed, which for
  // somebody relying on the transcript is the wrong way round.
  const paragraphs = toParagraphs([seg(1, 'This is a deliberately long sentence that I am')])
  assert.equal(paragraphs.length, 1)
  assert.equal(paragraphs[0]?.incomplete, true)
  assert.equal(paragraphs[0]?.text, 'This is a deliberately long sentence that I am')
})

test('an incomplete paragraph is completed by what follows', () => {
  const first = toParagraphs([seg(1, 'We ran it overnight and')])
  assert.equal(first[0]?.incomplete, true)
  const then = toParagraphs([seg(1, 'We ran it overnight and'), seg(2, 'it finished by morning.')])
  assert.deepEqual(then.map((p) => p.text), ['We ran it overnight and it finished by morning.'])
  assert.equal(then[0]?.incomplete, false)
})

test('a paragraph is timestamped from where the sentence began', () => {
  const paragraphs = toParagraphs([seg(1, 'The question is'), seg(2, 'whether it scales.')])
  assert.equal(paragraphs[0]?.startMs, 1000)
})

test('dotted identifiers are not sentence ends', () => {
  // The glossary is full of these, and splitting them would be worse than the
  // problem being solved.
  for (const identifier of ['pg_hba.conf', 'postgresql.conf', 'postgresql.auto.conf', 'recovery.signal']) {
    assert.deepEqual(
      texts([seg(1, `Check ${identifier} before restarting.`)]),
      [`Check ${identifier} before restarting.`],
      identifier,
    )
  }
})

test('abbreviations are not sentence ends', () => {
  for (const line of [
    'Use a pooler, e.g. PgBouncer, in front of it.',
    'That was Dr. Page speaking.',
    'See fig. 3 for the numbers.',
    'Written by J. Smith and others.',
  ]) {
    assert.deepEqual(texts([seg(1, line)]), [line], line)
  }
})

test('a decimal point is not a sentence end', () => {
  assert.deepEqual(
    texts([seg(1, 'It went from 1.5 seconds to 0.2 seconds.')]),
    ['It went from 1.5 seconds to 0.2 seconds.'],
  )
})

test('questions and exclamations end sentences too', () => {
  assert.deepEqual(
    texts([seg(1, 'Does it scale? It does! Mostly.')]),
    ['Does it scale?', 'It does!', 'Mostly.'],
  )
})

test('a quoted sentence keeps its closing quote', () => {
  assert.deepEqual(
    texts([seg(1, 'He said "it works." Then he left.')]),
    ['He said "it works."', 'Then he left.'],
  )
})

test('empty and whitespace-only segments are ignored', () => {
  assert.deepEqual(texts([seg(1, ''), seg(2, '   '), seg(3, 'Right.')]), ['Right.'])
  assert.deepEqual(texts([]), [])
})

test('keys are unique even when one segment holds several sentences', () => {
  const paragraphs = toParagraphs([seg(1, 'One. Two. Three.')])
  const keys = paragraphs.map((p) => p.key)
  assert.equal(new Set(keys).size, keys.length)
})

test('splitSentences returns the unfinished tail separately', () => {
  const { complete, rest } = splitSentences('First one. Second one. And a third that is not')
  assert.deepEqual(complete, ['First one.', 'Second one.'])
  assert.equal(rest, 'And a third that is not')
})
