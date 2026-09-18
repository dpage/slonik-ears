import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { qrURL, transcriptURL } from '../api'
import { downloadTranscript } from '../transcriptFile'
import { useConfig } from '../App'
import ConnectionBadge from '../components/ConnectionBadge'
import ReadingControls from '../components/ReadingControls'
import TranscriptView from '../components/TranscriptView'
import { useRoomStream } from '../hooks/useRoomStream'
import { useSetting } from '../hooks/useSetting'

export default function RoomPage() {
  const { roomId } = useParams<{ roomId: string }>()
  const config = useConfig()
  const { room, segments, partial, status, connection, notFound, error } = useRoomStream(roomId)

  const [fontSize, setFontSize] = useSetting<number>('fontSize', 20)
  const [theme, setTheme] = useSetting<string>('theme', 'dark')
  const [showTimestamps, setShowTimestamps] = useSetting<boolean>('timestamps', false)
  const [showShare, setShowShare] = useState(false)

  if (notFound) {
    return (
      <main className="centred" data-theme={theme}>
        <div className="card">
          <h1>No such room</h1>
          <p className="muted">
            Nothing is being transcribed under the name <code>{roomId}</code>.
          </p>
          <Link className="button" to="/">
            Back to the room list
          </Link>
        </div>
      </main>
    )
  }

  const title = room?.title || roomId || 'Transcript'

  return (
    <div className="page room-page" data-theme={theme}>
      <header className="masthead">
        <div className="room-heading">
          <Link to="/" className="back" aria-label="Back to the room list">
            ←
          </Link>
          <div>
            <h1>{title}</h1>
            <p className="muted small">
              {room?.speaker && <span>{room.speaker}</span>}
              {room?.speaker && room?.track && <span aria-hidden="true"> · </span>}
              {room?.track && <span>{room.track}</span>}
            </p>
          </div>
        </div>
        <ConnectionBadge state={connection} roomLive={room?.live} />
      </header>

      <ReadingControls
        fontSize={fontSize}
        setFontSize={setFontSize}
        theme={theme}
        setTheme={setTheme}
        showTimestamps={showTimestamps}
        setShowTimestamps={setShowTimestamps}
      />

      <main className="transcript-area" style={{ fontSize: `${fontSize}px` }}>
        <TranscriptView
          segments={segments}
          partial={partial}
          showTimestamps={showTimestamps}
          emptyMessage={
            room?.live
              ? 'Listening… the transcript will appear as soon as somebody speaks.'
              : 'This room is not live yet. The transcript will start by itself.'
          }
        />
      </main>

      <footer className="room-footer">
        <div className="footer-actions">
          <button type="button" className="button" onClick={() => setShowShare((v) => !v)}>
            {showShare ? 'Hide share code' : 'Share'}
          </button>
          {/*
            Written here rather than fetched from the server, so the file a
            speaker is given reads the same as the page they were reading:
            grouped into sentences by the one implementation of the rules,
            instead of two that agree until somebody edits one of them.
          */}
          <button
            type="button"
            className="button"
            onClick={() => downloadTranscript(room, roomId ?? '', segments, showTimestamps)}
            disabled={segments.length === 0}
          >
            Download text
          </button>
          <a className="button" href={transcriptURL(roomId ?? '', 'srt')}>
            Subtitles
          </a>
          <Link className="button" to={`/r/${roomId}/stage`}>
            Stage display
          </Link>
        </div>

        {showShare && roomId && (
          <div className="share">
            <img src={qrURL(roomId, 300)} alt={`QR code linking to the ${title} transcript`} />
            <p className="muted small">Point a phone camera at this to open the transcript.</p>
          </div>
        )}

        {/*
          The server's own error frames were being collected by the hook and
          rendered nowhere, so a viewer turned away for any reason saw a page
          that looked live and simply stopped updating.
        */}
        {error && (
          <p className="warning small" role="alert">
            {error}
          </p>
        )}
        {status?.detail && (
          <p className="warning small" role="status">
            Room reports: {status.detail}
          </p>
        )}
        {config?.event.notice && <p className="notice small">{config.event.notice}</p>}
      </footer>
    </div>
  )
}
