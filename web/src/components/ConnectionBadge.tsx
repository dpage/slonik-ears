import type { ConnectionState } from '../types'

const LABELS: Record<ConnectionState, string> = {
  connecting: 'Connecting',
  live: 'Live',
  reconnecting: 'Reconnecting',
  offline: 'Offline',
  denied: 'Not authorised',
}

interface Props {
  state: ConnectionState
  /** Whether a listener is actually publishing to the room. */
  roomLive?: boolean
}

/**
 * Two different things can be "not live": the browser's connection to the
 * server, and whether anybody is speaking into the room's microphone. They
 * are worth distinguishing, because only one of them is the attendee's
 * problem.
 */
export default function ConnectionBadge({ state, roomLive }: Props) {
  const connected = state === 'live'
  const label = connected && roomLive === false ? 'Waiting for the room' : LABELS[state]
  const tone = !connected ? state : roomLive === false ? 'waiting' : 'live'

  return (
    <span className={`badge badge-${tone}`} role="status">
      <span className="badge-dot" aria-hidden="true" />
      {label}
    </span>
  )
}
