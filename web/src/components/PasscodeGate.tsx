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

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setWrong(false)
    const ok = await submitPasscode(key.trim())
    setBusy(false)
    if (ok) onSuccess()
    else setWrong(true)
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
            aria-describedby={wrong ? 'passcode-error' : undefined}
          />
        </label>
        {wrong && (
          <p id="passcode-error" role="alert" className="error">
            That passcode was not accepted. Do check with the organisers.
          </p>
        )}
        <button type="submit" className="button primary" disabled={busy || key.trim() === ''}>
          {busy ? 'Checking…' : 'Watch the transcript'}
        </button>
      </form>
    </main>
  )
}
