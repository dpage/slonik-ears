import { useEffect, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { qrURL } from '../api'
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

  const visible = segments.slice(-lineCount)

  return (
    <div className="stage" data-theme="contrast" data-idle={idle} data-qr={showQR}>
      <header className="stage-header">
        <h1>{room?.title || roomId}</h1>
        <div className="stage-meta">
          {room?.speaker && <span>{room.speaker}</span>}
          <ConnectionBadge state={connection} roomLive={room?.live} />
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
        {visible.map((seg) => (
          <p key={seg.seq} className="stage-line">
            {seg.text}
          </p>
        ))}
        {partial?.text && <p className="stage-line partial">{partial.text}</p>}
      </main>

      {showQR && roomId && (
        <aside className="stage-qr">
          <img src={qrURL(roomId, 260)} alt="" />
          <p>Follow along on your phone</p>
        </aside>
      )}

      <Link to={`/r/${roomId}`} className="stage-exit">
        Exit stage view
      </Link>
    </div>
  )
}

function clamp(n: number, min: number, max: number): number {
  if (Number.isNaN(n)) return min
  return Math.min(max, Math.max(min, n))
}
