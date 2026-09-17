import { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { fetchConfig } from './api'
import PasscodeGate from './components/PasscodeGate'
import AdminPage from './pages/AdminPage'
import Lobby from './pages/Lobby'
import RoomPage from './pages/RoomPage'
import StagePage from './pages/StagePage'
import type { PublicConfig } from './types'

const ConfigContext = createContext<PublicConfig | null>(null)

/** The event's public configuration, loaded once at startup. */
export function useConfig(): PublicConfig | null {
  return useContext(ConfigContext)
}

export default function App() {
  const [config, setConfig] = useState<PublicConfig | null>(null)
  const [failed, setFailed] = useState(false)

  const load = useCallback(() => {
    setFailed(false)
    fetchConfig()
      .then(setConfig)
      .catch(() => setFailed(true))
  }, [])

  useEffect(load, [load])

  if (failed) {
    return (
      <main className="centred">
        <div className="card">
          <h1>Cannot reach the server</h1>
          <p>
            The transcript server is not answering. If you are on the venue's Wi-Fi, it may
            have dropped out for a moment.
          </p>
          <button type="button" className="button" onClick={load}>
            Try again
          </button>
        </div>
      </main>
    )
  }

  if (!config) {
    return (
      <main className="centred">
        <p className="muted">Connecting…</p>
      </main>
    )
  }

  if (config.requiresKey && !config.authenticated) {
    return <PasscodeGate event={config.event} onSuccess={load} />
  }

  return (
    <ConfigContext.Provider value={config}>
      <Routes>
        <Route path="/" element={<Lobby />} />
        <Route path="/r/:roomId" element={<RoomPage />} />
        <Route path="/r/:roomId/stage" element={<StagePage />} />
        {/* Not linked from anywhere. It has its own token, and a link in the
            lobby would only invite people to try the door. */}
        <Route path="/admin" element={<AdminPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </ConfigContext.Provider>
  )
}
