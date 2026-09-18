import { useState } from 'react'
import { submitPasscode } from '../api'
import type { EventConfig } from '../types'

interface Props {
  event: EventConfig
  onSuccess: () => void
}

/** Shown when the organisers have put a passcode on the event. */
export default function PasscodeGate({ event, onSuccess }: Props) {
  const [key, setKey] = useState('')
  const [busy, setBusy] = useState(false)
  const [wrong, setWrong] = useState(false)
  const [unreachable, setUnreachable] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setWrong(false)
    setUnreachable(false)
    try {
      const ok = await submitPasscode(key.trim())
      if (ok) onSuccess()
      else setWrong(true)
    } catch {
      // The venue's Wi-Fi dropping for a second used to leave this button
      // reading "Checking…" for ever, with reloading the page as the only
      // way out. Say which of the two went wrong, because "that passcode is
      // not right" sends somebody to ask the organisers about a code that was
      // perfectly correct.
      setUnreachable(true)
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="centred">
      <form className="card" onSubmit={submit}>
        <h1>{event.name}</h1>
        <p className="muted">This live transcript is passcode protected.</p>
        <label className="field">
          <span>Passcode</span>
          <input
            type="text"
            value={key}
            onChange={(e) => setKey(e.target.value)}
            autoFocus
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            aria-invalid={wrong}
            aria-describedby={wrong || unreachable ? 'passcode-error' : undefined}
          />
        </label>
        {wrong && (
          <p id="passcode-error" role="alert" className="error">
            That passcode was not accepted. Do check with the organisers.
          </p>
        )}
        {unreachable && (
          <p id="passcode-error" role="alert" className="error">
            Could not reach the server just then. Try again in a moment.
          </p>
        )}
        <button type="submit" className="button primary" disabled={busy || key.trim() === ''}>
          {busy ? 'Checking…' : 'Watch the transcript'}
        </button>
      </form>
    </main>
  )
}
