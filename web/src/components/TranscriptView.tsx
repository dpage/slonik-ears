import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { Partial as PartialText, Segment } from '../types'

interface Props {
  segments: Segment[]
  partial: PartialText | null
  showTimestamps: boolean
  emptyMessage: string
}

/**
 * The transcript itself: committed lines, plus the in-progress hypothesis
 * shown in a lighter style so nobody mistakes a guess for a quote.
 *
 * Scrolling follows the speaker unless the reader has scrolled back to look
 * at something, in which case it stays put and offers a way back to live.
 */
export default function TranscriptView({ segments, partial, showTimestamps, emptyMessage }: Props) {
  const scrollerRef = useRef<HTMLDivElement>(null)
  const [pinned, setPinned] = useState(true)

  const onScroll = () => {
    const el = scrollerRef.current
    if (!el) return
    const distance = el.scrollHeight - el.scrollTop - el.clientHeight
    setPinned(distance < 80)
  }

  useLayoutEffect(() => {
    if (!pinned) return
    const el = scrollerRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [segments, partial, pinned])

  // Keep following the transcript when the phone is rotated or the on-screen
  // keyboard changes the viewport.
  useEffect(() => {
    const onResize = () => {
      if (!pinned) return
      const el = scrollerRef.current
      if (el) el.scrollTop = el.scrollHeight
    }
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [pinned])

  const jumpToLive = () => {
    const el = scrollerRef.current
    if (el) el.scrollTop = el.scrollHeight
    setPinned(true)
  }

  return (
    <div className="transcript-wrapper">
      <div
        className="transcript"
        ref={scrollerRef}
        onScroll={onScroll}
        role="log"
        aria-live="polite"
        aria-relevant="additions text"
        aria-label="Live transcript"
        tabIndex={0}
      >
        {segments.length === 0 && !partial && <p className="muted empty">{emptyMessage}</p>}

        {segments.map((seg) => (
          <p className="line" key={seg.seq}>
            {showTimestamps && <span className="stamp">{clock(seg.startMs)}</span>}
            <span className="text">{seg.text}</span>
          </p>
        ))}

        {partial && partial.text && (
          <p className="line partial">
            {showTimestamps && <span className="stamp" aria-hidden="true">···</span>}
            <span className="text">{partial.text}</span>
          </p>
        )}
      </div>

      {!pinned && (
        <button type="button" className="jump" onClick={jumpToLive}>
          Jump to live ↓
        </button>
      )}
    </div>
  )
}

function clock(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000))
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  const pad = (n: number) => n.toString().padStart(2, '0')
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${pad(m)}:${pad(s)}`
}
