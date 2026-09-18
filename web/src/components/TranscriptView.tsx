import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { toParagraphs } from '../sentences'
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

  // Recomputed only when the transcript actually changes: it walks every
  // segment, and a room that has been running all morning has a lot of them.
  const paragraphs = useMemo(() => toParagraphs(segments), [segments])

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
      {/*
        aria-relevant is additions only, deliberately. With "text" as well, a
        screen reader re-reads a paragraph whenever its text node changes, and
        because committed segments are regrouped into sentences the trailing
        paragraph is replaced rather than appended to on every arrival: a
        reader depending on this heard the same growing sentence restarted from
        the beginning several times a second, which is the opposite of what the
        page is for.
      */}
      <div
        className="transcript"
        ref={scrollerRef}
        onScroll={onScroll}
        role="log"
        aria-live="polite"
        aria-relevant="additions"
        aria-label="Live transcript"
        tabIndex={0}
      >
        {segments.length === 0 && !partial && <p className="muted empty">{emptyMessage}</p>}

        {/*
          One paragraph per sentence rather than per committed segment. The
          segments are cut where the speaker paused or where the chunker ran
          out of patience, which puts paragraph breaks in the middle of
          clauses; the sentence is what a reader actually follows.
        */}
        {paragraphs.map((p) => (
          <p className="line" key={p.key}>
            {showTimestamps && <span className="stamp">{clock(p.startMs)}</span>}
            <span className="text">{p.text}</span>
          </p>
        ))}

        {partial && partial.text && (
          <p className="line partial">
            {showTimestamps && <span className="stamp" aria-hidden="true">···</span>}
            <span className="text">{partial.text}</span>
          </p>
        )}
      </div>

      {/*
        Hidden rather than unmounted. Its own onClick pins the view, so
        removing it from the tree threw away the focus of a keyboard reader
        who had just pressed it: the next Tab started again from the top of
        the page. aria-hidden and inert keep it out of the way whilst it is
        not offered.
      */}
      <button
        type="button"
        className={pinned ? 'jump hidden' : 'jump'}
        onClick={jumpToLive}
        aria-hidden={pinned}
        inert={pinned}
      >
        Jump to live ↓
      </button>
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
