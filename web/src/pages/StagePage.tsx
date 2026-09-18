import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { qrURL } from '../api'
import { toParagraphs } from '../sentences'
import ConnectionBadge from '../components/ConnectionBadge'
import { useRoomStream } from '../hooks/useRoomStream'

/**
 * The view for a screen on stage or beside it: a handful of lines at a size
 * readable from the back row, no chrome, and a QR code so the audience can
 * pick it up on their own devices.
 *
 * ?lines=N sets how many lines are shown, ?qr=0 hides the code, and moving
 * the mouse brings the controls back for the person driving the laptop.
 */
export default function StagePage() {
  const { roomId } = useParams<{ roomId: string }>()
  const [params] = useSearchParams()
  const { room, segments, partial, connection } = useRoomStream(roomId)
  const [idle, setIdle] = useState(false)
  const [qrAttempt, setQrAttempt] = useState(0)
  const qrRetry = useRef<number | undefined>(undefined)

  const lineCount = clamp(Number(params.get('lines') ?? 5), 2, 12)
  const showQR = params.get('qr') !== '0'
  const size = clamp(Number(params.get('size') ?? 100), 50, 200)

  // Hide the controls when nobody has touched the laptop for a while.
  useEffect(() => {
    let timer = window.setTimeout(() => setIdle(true), 5000)
    const wake = () => {
      setIdle(false)
      window.clearTimeout(timer)
      timer = window.setTimeout(() => setIdle(true), 5000)
    }
    window.addEventListener('mousemove', wake)
    window.addEventListener('keydown', wake)
    return () => {
      window.clearTimeout(timer)
      window.removeEventListener('mousemove', wake)
      window.removeEventListener('keydown', wake)
    }
  }, [])

  // Re-request the code when the stream reconnects, since a drop and recovery
  // is the likeliest reason it failed in the first place.
  useEffect(() => {
    if (connection === 'live') setQrAttempt((n) => n + 1)
  }, [connection])

  useEffect(() => () => window.clearTimeout(qrRetry.current), [])

  const retryQR = () => {
    // Back off to a few seconds and stay there: a stage screen is left running
    // for hours, and a tight retry loop against a server that is down is both
    // useless and noisy.
    window.clearTimeout(qrRetry.current)
    qrRetry.current = window.setTimeout(() => setQrAttempt((n) => n + 1), 5000)
  }

  const qrLoaded = () => window.clearTimeout(qrRetry.current)

  // Sentences, not committed segments: on a screen showing only a handful of
  // lines it matters even more that each is a whole thought.
  const paragraphs = useMemo(() => toParagraphs(segments), [segments])
  const visible = paragraphs.slice(-lineCount)

  return (
    <div className="stage" data-theme="contrast" data-idle={idle} data-qr={showQR}>
      <header className="stage-header">
        <h1>{room?.title || roomId}</h1>
        <div className="stage-meta">
          {room?.speaker && <span>{room.speaker}</span>}
          <ConnectionBadge state={connection} roomLive={room?.live} />
          {/*
            In the header row rather than floated over the top right corner,
            which is where the speaker's name and the live badge already are:
            the two were drawn on top of one another until the idle timer faded
            this one out. Hiding it still only changes its opacity, so the row
            does not reflow every time somebody nudges the laptop.
          */}
          <Link to={`/r/${roomId}`} className="stage-exit">
            Exit stage view
          </Link>
        </div>
      </header>

      <main
        className="stage-transcript"
        style={{ fontSize: `calc(${size / 100} * clamp(28px, 4.2vw, 76px))` }}
        role="log"
        aria-live="polite"
        aria-label="Live transcript"
      >
        {visible.length === 0 && !partial && (
          <p className="muted">Waiting for the talk to begin…</p>
        )}
        {visible.map((p) => (
          <p key={p.key} className="stage-line">
            {p.text}
          </p>
        ))}
        {partial?.text && <p className="stage-line partial">{partial.text}</p>}
      </main>

      {showQR && roomId && (
        <aside className="stage-qr">
          {/*
            A broken image is never retried by the browser, so a stage display
            that happened to load whilst the server was restarting kept an
            empty box where the code should be for the rest of the talk, and
            nobody in the room could scan anything. Ask again, backing off, and
            once more whenever the stream reconnects: that is the moment the
            server is known to be answering again.
          */}
          <img
            key={qrAttempt}
            src={qrURL(roomId, 260, qrAttempt)}
            alt=""
            onError={retryQR}
            onLoad={qrLoaded}
          />
          <p>Follow along on your phone</p>
        </aside>
      )}
    </div>
  )
}

function clamp(n: number, min: number, max: number): number {
  if (Number.isNaN(n)) return min
  return Math.min(max, Math.max(min, n))
}
