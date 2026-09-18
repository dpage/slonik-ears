import type { Segment } from './types'

/**
 * A paragraph as the reader sees it: one sentence, which may have been
 * assembled from several committed segments.
 */
export interface Paragraph {
  /** Stable across re-renders: the sequence number the sentence started in. */
  key: number
  /** Offset into the talk at which the sentence started, for the gutter. */
  startMs: number
  text: string
  /**
   * True when no sentence-ending punctuation has arrived yet, so the speaker
   * is still in the middle of saying it.
   */
  incomplete: boolean
}

/**
 * Regroups committed segments into one paragraph per sentence.
 *
 * Segments are cut where the speaker paused or where the chunker hit its
 * maximum utterance length, neither of which has anything to do with where a
 * sentence ends. The result on screen is paragraph breaks in arbitrary places,
 * sometimes mid-clause. The committed segments themselves are left alone —
 * their sequence numbers and text are what let a viewer resume from a cursor,
 * and a segment whose text changed after publication would break that — so the
 * regrouping happens here, on the way to being displayed.
 *
 * A sentence still being spoken is emitted rather than held back. Waiting for
 * the full stop would delay text that has already been transcribed, which for
 * somebody relying on the transcript to follow the talk is the wrong trade:
 * better a paragraph that grows as the speaker finishes their thought.
 */
export function toParagraphs(segments: Segment[]): Paragraph[] {
  const out: Paragraph[] = []
  let buffer = ''
  let startedAt = 0
  let startedKey = 0

  for (const seg of segments) {
    const text = seg.text.trim()
    if (text === '') continue
    if (buffer === '') {
      startedAt = seg.startMs
      startedKey = seg.seq
    }
    buffer = buffer === '' ? text : `${buffer} ${text}`

    const { complete, rest } = splitSentences(buffer)
    for (const sentence of complete) {
      out.push({ key: startedKey, startMs: startedAt, text: sentence, incomplete: false })
      // Anything after the first sentence in this batch began here.
      startedAt = seg.startMs
      startedKey = seg.seq
    }
    buffer = rest
  }

  if (buffer !== '') {
    out.push({ key: startedKey, startMs: startedAt, text: buffer, incomplete: true })
  }
  // Two sentences finishing inside one segment would otherwise share a key.
  return dedupeKeys(out)
}

/**
 * Terminators that end a sentence, allowing for a closing quote or bracket
 * after the full stop.
 */
const sentenceEnd = /([.!?]+["'”’)\]]?)(\s+)/g

/**
 * Words that take a full stop without ending a sentence. Without these,
 * "e.g. Postgres" and "Dr. Smith" become two paragraphs each.
 *
 * Note what is NOT needed here: pg_hba.conf and postgresql.conf survive
 * untouched, because a sentence only ends when the full stop is followed by
 * whitespace, and there is none inside an identifier.
 */
const abbreviations = new Set([
  'mr', 'mrs', 'ms', 'dr', 'prof', 'sr', 'jr', 'st',
  'e.g', 'eg', 'i.e', 'ie', 'etc', 'vs', 'cf', 'approx', 'al',
  'fig', 'no', 'vol', 'ch', 'pp', 'inc', 'ltd', 'co',
  'jan', 'feb', 'mar', 'apr', 'jun', 'jul', 'aug', 'sep', 'sept', 'oct', 'nov', 'dec',
])

/**
 * splitSentences divides text into completed sentences plus whatever is left
 * over, which is presumed to be a sentence still in progress.
 */
export function splitSentences(text: string): { complete: string[]; rest: string } {
  const complete: string[] = []
  let start = 0
  sentenceEnd.lastIndex = 0

  for (let m = sentenceEnd.exec(text); m !== null; m = sentenceEnd.exec(text)) {
    const terminator = m[1] ?? ''
    const gap = m[2] ?? ''
    const endOfSentence = m.index + terminator.length
    const following = text.slice(endOfSentence + gap.length)
    if (!startsNewSentence(following) || isAbbreviation(text.slice(start, m.index + 1))) {
      continue
    }
    complete.push(text.slice(start, endOfSentence).trim())
    start = endOfSentence + gap.length
  }

  // The pattern needs whitespace after the full stop, which the end of the
  // string does not have. Without this, a segment ending in a full stop —
  // which is most of them — would be left in the tail and shown as though the
  // speaker were still mid-sentence for ever.
  const tail = text.slice(start).trim()
  if (/[.!?]+["'”’)\]]?$/.test(tail) && !isAbbreviation(tail)) {
    complete.push(tail)
    return { complete, rest: '' }
  }
  return { complete, rest: tail }
}

/**
 * A sentence starts with a capital, a digit or an opening quote. Lower case
 * after a full stop is far more likely to be an abbreviation or a decimal than
 * a new sentence, and the transcript is full of both.
 */
function startsNewSentence(following: string): boolean {
  const first = following.trimStart()[0]
  if (first === undefined) return false
  return /[A-Z0-9"'“‘([]/.test(first)
}

function isAbbreviation(upToDot: string): boolean {
  const words = upToDot.trim().split(/\s+/)
  const last = words[words.length - 1] ?? ''
  const word = last.replace(/[.]+$/, '').toLowerCase()
  if (word === '') return false
  // A lone capital is an initial: "J. Smith" is one name, not two sentences.
  if (/^[a-z]$/.test(word)) return true
  return abbreviations.has(word)
}

/**
 * React needs distinct keys. Two sentences completed by the same segment share
 * the sequence number they started from, so later ones are suffixed.
 */
function dedupeKeys(paragraphs: Paragraph[]): Paragraph[] {
  const seen = new Map<number, number>()
  return paragraphs.map((p) => {
    const n = seen.get(p.key) ?? 0
    seen.set(p.key, n + 1)
    return n === 0 ? p : { ...p, key: p.key + n / 1000 }
  })
}
