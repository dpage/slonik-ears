import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useConfig } from '../App'
import { ApiError, deleteRoom, fetchAdminRooms, resetRoom, submitAdminToken, updateRoom } from '../api'
import type { Room } from '../types'

/**
 * The page for whoever is running the event, at /admin. Deliberately not
 * linked from anywhere: it is behind its own token, and a link in the lobby
 * would only be an invitation to try the door.
 *
 * It does the three things that come up between talks. Retitle a room, because
 * the next speaker is not the last one. Reset it, so the new talk starts on a
 * clean screen whilst the previous transcript is kept. Remove it, for a track
 * that has finished for the day.
 *
 * What it deliberately cannot do is change a room's id. That is what is printed
 * on the QR code, sitting in the URL bar of every phone in the room, and
 * configured on the listener at the back of it; changing it would silently
 * strand all three.
 */
export default function AdminPage() {
  const config = useConfig()
  const [rooms, setRooms] = useState<Room[] | null>(null)
  // Taken from the config the app already loads rather than by firing a request
  // at the admin API and reading the 401. That works, but it leaves a failed
  // request in the console of a page that is otherwise quiet, and a console
  // with routine errors in it is one nobody reads.
  const [signedIn, setSignedIn] = useState(config?.adminAuthenticated ?? false)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      setRooms(await fetchAdminRooms())
      setError(null)
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setSignedIn(false)
        setRooms(null)
        return
      }
      setError(err instanceof Error ? err.message : 'could not load the rooms')
    }
  }, [])

  useEffect(() => {
    if (!signedIn) return
    void load()
    // The lobby changes under you during an event, and a stale list is how the
    // wrong room gets reset.
    const timer = window.setInterval(() => void load(), 5000)
    return () => window.clearInterval(timer)
  }, [load, signedIn])

  const run = async (id: string, what: string, action: () => Promise<void>) => {
    setBusy(`${id}:${what}`)
    setError(null)
    try {
      await action()
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : `could not ${what} ${id}`)
    } finally {
      setBusy(null)
    }
  }

  if (!signedIn) {
    return (
      <AdminLogin
        enabled={config?.adminEnabled ?? true}
        onDone={() => {
          setSignedIn(true)
          void load()
        }}
      />
    )
  }

  return (
    <div className="page">
      <header className="masthead">
        <div className="room-heading">
          <Link to="/" className="back" aria-label="Back to the lobby">
            ‹
          </Link>
          <h1>Rooms</h1>
        </div>
        <span className="small muted">Organiser controls</span>
      </header>

      <main className="lobby">
        {error && <p className="error">{error}</p>}
        {rooms === null && <p className="muted">Loading…</p>}
        {rooms?.length === 0 && (
          <p className="muted">No rooms yet. One appears as soon as a listener connects to it.</p>
        )}
        {rooms?.map((room) => (
          <AdminRoom
            key={room.id}
            room={room}
            busy={busy}
            onSave={(meta) => run(room.id, 'save', () => updateRoom(room.id, meta))}
            onReset={() => run(room.id, 'reset', () => resetRoom(room.id))}
            onDelete={() => run(room.id, 'remove', () => deleteRoom(room.id))}
          />
        ))}
      </main>
    </div>
  )
}

function AdminRoom({
  room,
  busy,
  onSave,
  onReset,
  onDelete,
}: {
  room: Room
  busy: string | null
  onSave: (meta: Partial<Room>) => void
  onReset: () => void
  onDelete: () => void
}) {
  const [title, setTitle] = useState(room.title ?? '')
  const [speaker, setSpeaker] = useState(room.speaker ?? '')
  const [track, setTrack] = useState(room.track ?? '')
  const [edited, setEdited] = useState(false)

  // Take updates from the server whilst the fields are untouched, so the list
  // stays honest, but never overwrite something half typed.
  useEffect(() => {
    if (edited) return
    setTitle(room.title ?? '')
    setSpeaker(room.speaker ?? '')
    setTrack(room.track ?? '')
  }, [room.title, room.speaker, room.track, edited])

  const change = (set: (v: string) => void) => (e: React.ChangeEvent<HTMLInputElement>) => {
    setEdited(true)
    set(e.target.value)
  }

  return (
    <section className="card admin-room">
      <div className="admin-room-heading">
        <h2>
          <code>{room.id}</code>
        </h2>
        <span className={`badge ${room.live ? 'badge-live' : 'badge-offline'}`}>
          <span className="badge-dot" />
          {room.live ? 'Live' : 'Idle'}
        </span>
        <span className="small muted">
          {room.cursor} line{room.cursor === 1 ? '' : 's'} · {room.viewers} watching
        </span>
      </div>

      <label className="field">
        Title
        <input value={title} onChange={change(setTitle)} placeholder="Main Hall" />
      </label>
      <label className="field">
        Speaker
        <input value={speaker} onChange={change(setSpeaker)} placeholder="The next speaker" />
      </label>
      <label className="field">
        Track
        <input value={track} onChange={change(setTrack)} placeholder="Track A" />
      </label>

      <div className="admin-actions">
        <button
          type="button"
          className="button primary"
          disabled={!edited || busy !== null}
          onClick={() => {
            onSave({ title, speaker, track })
            setEdited(false)
          }}
        >
          {busy === `${room.id}:save` ? 'Saving…' : 'Save details'}
        </button>

        <ConfirmButton
          label="Reset for next talk"
          confirm="Clear the transcript?"
          detail="The current transcript is archived on the server, not deleted."
          disabled={busy !== null}
          busy={busy === `${room.id}:reset`}
          onConfirm={onReset}
        />

        <ConfirmButton
          label="Remove room"
          confirm="Remove from the lobby?"
          detail="Viewers are disconnected. The transcript is archived on the server."
          disabled={busy !== null}
          busy={busy === `${room.id}:remove`}
          onConfirm={onDelete}
        />

        <Link className="button" to={`/r/${room.id}`}>
          View
        </Link>
      </div>
    </section>
  )
}

/**
 * Both destructive actions take two clicks. A single button that empties a room
 * is the wrong control to put beside a list which reorders itself every five
 * seconds, operated by somebody standing at the back of a hall.
 */
function ConfirmButton({
  label,
  confirm,
  detail,
  disabled,
  busy,
  onConfirm,
}: {
  label: string
  confirm: string
  detail: string
  disabled: boolean
  busy: boolean
  onConfirm: () => void
}) {
  const [armed, setArmed] = useState(false)

  // Disarm on its own, so a half-pressed button does not lie in wait.
  useEffect(() => {
    if (!armed) return
    const timer = window.setTimeout(() => setArmed(false), 5000)
    return () => window.clearTimeout(timer)
  }, [armed])

  if (busy) {
    return (
      <button type="button" className="button" disabled>
        Working…
      </button>
    )
  }
  if (!armed) {
    return (
      <button type="button" className="button" disabled={disabled} onClick={() => setArmed(true)}>
        {label}
      </button>
    )
  }
  return (
    <span className="confirm">
      <button
        type="button"
        className="button danger"
        onClick={() => {
          setArmed(false)
          onConfirm()
        }}
      >
        {confirm}
      </button>
      <button type="button" className="button" onClick={() => setArmed(false)}>
        Cancel
      </button>
      <span className="small muted">{detail}</span>
    </span>
  )
}

function AdminLogin({ enabled, onDone }: { enabled: boolean; onDone: () => void }) {
  const [token, setToken] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await submitAdminToken(token)
      onDone()
    } catch {
      setError('That token is not right.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="page">
      <div className="centred">
        <form className="card" onSubmit={submit}>
          <h1>Organiser controls</h1>
          {enabled ? (
            <p className="muted small">
              The admin token for this server, from <code>EARS_ADMIN_TOKEN</code>.
            </p>
          ) : (
            <p className="warning small">
              This server has no admin token set, so these controls are switched off. Set{' '}
              <code>EARS_ADMIN_TOKEN</code> and restart it.
            </p>
          )}
          <label className="field">
            Admin token
            <input
              type="password"
              value={token}
              autoFocus
              onChange={(e) => setToken(e.target.value)}
            />
          </label>
          {error && <p className="error small">{error}</p>}
          <button type="submit" className="button primary" disabled={busy || token === '' || !enabled}>
            {busy ? 'Checking…' : 'Sign in'}
          </button>
        </form>
      </div>
    </div>
  )
}
