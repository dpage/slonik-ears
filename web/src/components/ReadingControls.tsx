import { useEffect, useState } from 'react'

interface Props {
  fontSize: number
  setFontSize: (n: number) => void
  theme: string
  setTheme: (t: string) => void
  showTimestamps: boolean
  setShowTimestamps: (v: boolean) => void
}

const MIN = 14
const MAX = 44

/**
 * Reading comfort is the whole point of this screen: somebody is relying on
 * it to follow a talk, possibly from the back of the room, possibly because
 * they cannot hear it at all.
 */
export default function ReadingControls({
  fontSize,
  setFontSize,
  theme,
  setTheme,
  showTimestamps,
  setShowTimestamps,
}: Props) {
  const themes = [
    { id: 'dark', label: 'Dark' },
    { id: 'light', label: 'Light' },
    { id: 'contrast', label: 'High contrast' },
  ]

  // On a phone the controls are worth three lines of transcript, so they
  // start collapsed there and stay open on anything roomier.
  const [open, setOpen] = useState(() =>
    typeof window !== 'undefined' ? window.matchMedia('(min-width: 640px)').matches : true,
  )
  useEffect(() => {
    const mq = window.matchMedia('(min-width: 640px)')
    const onChange = (e: MediaQueryListEvent) => setOpen(e.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  return (
    <div className={`controls ${open ? 'open' : ''}`}>
      <button
        type="button"
        className="button controls-toggle"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        Aa Display
      </button>
      {open && (
        <div className="controls-body">
      <div className="control-group" role="group" aria-label="Text size">
        <button
          type="button"
          className="button"
          onClick={() => setFontSize(Math.max(MIN, fontSize - 2))}
          aria-disabled={fontSize <= MIN}
          aria-label="Smaller text"
        >
          A−
        </button>
        <span className="control-value" aria-live="polite">
          {fontSize}px
        </span>
        <button
          type="button"
          className="button"
          onClick={() => setFontSize(Math.min(MAX, fontSize + 2))}
          aria-disabled={fontSize >= MAX}
          aria-label="Larger text"
        >
          A+
        </button>
      </div>

      <div className="control-group" role="group" aria-label="Theme">
        {themes.map((t) => (
          <button
            key={t.id}
            type="button"
            className={`button ${theme === t.id ? 'selected' : ''}`}
            aria-pressed={theme === t.id}
            onClick={() => setTheme(t.id)}
          >
            {t.label}
          </button>
        ))}
      </div>

          <label className="toggle">
            <input
              type="checkbox"
              checked={showTimestamps}
              onChange={(e) => setShowTimestamps(e.target.checked)}
            />
            <span>Timestamps</span>
          </label>
        </div>
      )}
    </div>
  )
}
