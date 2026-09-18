import { toParagraphs } from './sentences.ts'
import type { Room, Segment } from './types'

/**
 * Renders the plain text transcript in the browser, from the same segments the
 * page is displaying and through the same sentence regrouping.
 *
 * The server can produce this file too, and did until the transcript on screen
 * started being grouped by sentence rather than by committed chunk. Keeping
 * both in step would mean the same sentence-splitting rules written twice, in
 * two languages, with the abbreviations and the dotted identifiers and the
 * decimal points enumerated in each; they would agree on the day they were
 * written and drift thereafter. Since the page already holds every segment it
 * is showing, it may as well write the file.
 *
 * Subtitles are a different matter and stay on the server: SRT and VTT cues
 * are per segment with their own timings, so they have nothing to gain from
 * being regrouped and a good deal to lose.
 */
export function transcriptText(room: Room | null, segments: Segment[], timestamps: boolean): string {
  const lines: string[] = []
  lines.push(room?.title || room?.id || 'Transcript')
  if (room?.speaker) lines.push(`Speaker: ${room.speaker}`)
  if (room?.track) lines.push(`Track: ${room.track}`)
  if (room?.startedAt) lines.push(`Started: ${new Date(room.startedAt).toUTCString()}`)
  lines.push('-'.repeat(60), '')

  for (const paragraph of toParagraphs(segments)) {
    lines.push(timestamps ? `[${clock(paragraph.startMs)}] ${paragraph.text}` : paragraph.text)
  }
  return lines.join('\n') + '\n'
}

/** A name that sorts sensibly in a downloads folder. */
export function transcriptFilename(room: Room | null, roomId: string): string {
  const date = new Date().toISOString().slice(0, 10)
  return `${room?.id || roomId}-${date}.txt`
}

/** mm:ss, or h:mm:ss once a talk has run over the hour. */
export function clock(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000))
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  const pad = (n: number) => String(n).padStart(2, '0')
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${pad(m)}:${pad(s)}`
}

/** Hands the text to the browser as a download. */
export function downloadTranscript(room: Room | null, roomId: string, segments: Segment[], timestamps: boolean): void {
  const blob = new Blob([transcriptText(room, segments, timestamps)], {
    type: 'text/plain;charset=utf-8',
  })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = transcriptFilename(room, roomId)
  document.body.appendChild(a)
  a.click()
  a.remove()
  // Revoking immediately can cancel the download in some browsers; a moment
  // later is safe and the object is small.
  window.setTimeout(() => URL.revokeObjectURL(url), 10_000)
}
