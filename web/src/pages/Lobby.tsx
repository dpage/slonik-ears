import { Link } from 'react-router-dom'
import { useConfig } from '../App'
import ConnectionBadge from '../components/ConnectionBadge'
import { useLobby } from '../hooks/useLobby'
import type { Room } from '../types'

export default function Lobby() {
  const config = useConfig()
  const { rooms, connection } = useLobby()

  const live = rooms.filter((r) => r.live)
  const idle = rooms.filter((r) => !r.live)

  return (
    <div className="page" data-theme="dark">
      <header className="masthead">
        <div>
          <h1>{config?.event.name ?? 'Live transcripts'}</h1>
          {config?.event.tagline && <p className="muted">{config.event.tagline}</p>}
        </div>
        <ConnectionBadge state={connection} />
      </header>

      <main className="lobby">
        {rooms.length === 0 && (
          <div className="card">
            <h2>No rooms yet</h2>
            <p className="muted">
              Rooms appear here as soon as a listener starts in one. If a talk has already
              begun, give it a moment.
            </p>
          </div>
        )}

        {live.length > 0 && (
          <section>
            <h2 className="section-heading">Happening now</h2>
            <ul className="room-list">
              {live.map((room) => (
                <RoomCard key={room.id} room={room} />
              ))}
            </ul>
          </section>
        )}

        {idle.length > 0 && (
          <section>
            <h2 className="section-heading">Not currently live</h2>
            <ul className="room-list">
              {idle.map((room) => (
                <RoomCard key={room.id} room={room} />
              ))}
            </ul>
          </section>
        )}
      </main>

      {config?.event.notice && <footer className="notice">{config.event.notice}</footer>}
    </div>
  )
}

function RoomCard({ room }: { room: Room }) {
  return (
    <li className="room-card">
      <Link to={`/r/${room.id}`}>
        <div className="room-card-head">
          <h3>{room.title || room.id}</h3>
          <span className={`badge badge-${room.live ? 'live' : 'waiting'}`}>
            <span className="badge-dot" aria-hidden="true" />
            {room.live ? 'Live' : 'Idle'}
          </span>
        </div>
        {room.track && <p className="room-track">{room.track}</p>}
        {room.speaker && <p className="room-speaker">{room.speaker}</p>}
        <p className="muted small">
          {room.cursor > 0 ? `${room.cursor} lines transcribed` : 'No transcript yet'}
          {room.viewers > 0 && ` · ${room.viewers} watching`}
        </p>
      </Link>
    </li>
  )
}
