import type { PublicConfig, Room, Segment } from './types'

const KEY_STORAGE = 'ears.viewerKey'

/** The viewer passcode, remembered between visits so nobody types it twice. */
export function getViewerKey(): string | null {
  const fromUrl = new URLSearchParams(window.location.search).get('k')
  if (fromUrl) {
    setViewerKey(fromUrl)
    return fromUrl
  }
  try {
    return window.localStorage.getItem(KEY_STORAGE)
  } catch {
    return null
  }
}

export function setViewerKey(key: string): void {
  try {
    window.localStorage.setItem(KEY_STORAGE, key)
  } catch {
    /* private browsing; the key still travels in the query string */
  }
}

export function clearViewerKey(): void {
  try {
    window.localStorage.removeItem(KEY_STORAGE)
  } catch {
    /* nothing we can do, and nothing that matters */
  }
}

/** Adds the viewer passcode to a URL when one is in play. */
export function withKey(path: string): string {
  const key = getViewerKey()
  if (!key) return path
  const joiner = path.includes('?') ? '&' : '?'
  return `${path}${joiner}k=${encodeURIComponent(key)}`
}

/** Builds an absolute WebSocket URL for a same-origin API path. */
export function wsURL(path: string): string {
  const url = new URL(withKey(path), window.location.href)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return url.toString()
}

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

async function getJSON<T>(path: string): Promise<T> {
  const resp = await fetch(withKey(path), { headers: { Accept: 'application/json' } })
  if (!resp.ok) {
    let message = resp.statusText
    try {
      const body = (await resp.json()) as { error?: string }
      if (body.error) message = body.error
    } catch {
      /* the status line will have to do */
    }
    throw new ApiError(message, resp.status)
  }
  return (await resp.json()) as T
}

export function fetchConfig(): Promise<PublicConfig> {
  return getJSON<PublicConfig>('/api/config')
}

export function fetchRooms(): Promise<Room[]> {
  return getJSON<{ rooms: Room[] }>('/api/rooms').then((r) => r.rooms ?? [])
}

export function fetchRoom(id: string): Promise<{ room: Room }> {
  return getJSON<{ room: Room }>(`/api/rooms/${encodeURIComponent(id)}`)
}

export function fetchTranscript(id: string): Promise<{ room: Room; segments: Segment[] }> {
  return getJSON(`/api/rooms/${encodeURIComponent(id)}/transcript?format=json`)
}

/** Exchanges a passcode for a session cookie; returns true if it was right. */
export async function submitPasscode(key: string): Promise<boolean> {
  const resp = await fetch('/api/session', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ key }),
  })
  if (resp.ok) {
    setViewerKey(key)
    return true
  }
  return false
}

export function transcriptURL(id: string, format: 'txt' | 'json' | 'srt' | 'vtt'): string {
  return withKey(`/api/rooms/${encodeURIComponent(id)}/transcript?format=${format}`)
}

/**
 * attempt busts the browser's cache of a failed request. Without it a retry
 * can be answered from cache with the same failure, which is no retry at all.
 */
export function qrURL(id: string, size = 420, attempt = 0): string {
  const base = `/api/rooms/${encodeURIComponent(id)}/qr.png?size=${size}`
  return attempt > 0 ? `${base}&retry=${attempt}` : base
}

// ------------------------------------------------------------------- admin
//
// These sit behind the admin token rather than the attendee passcode, and it
// is exchanged for a cookie once so that turning a room around between talks
// does not involve pasting a secret into anything.

/** Exchanges the admin token for a session cookie. */
export function submitAdminToken(token: string): Promise<void> {
  return sendJSON('/api/admin/session', 'POST', { token })
}

/** Lists rooms as the admin. A 401 here means the session has lapsed. */
export function fetchAdminRooms(): Promise<Room[]> {
  return getJSON<{ rooms: Room[] }>('/api/admin/rooms').then((r) => r.rooms ?? [])
}

/** Changes a room's title, speaker or track. Its id and URL do not move. */
export function updateRoom(id: string, meta: Partial<Room>): Promise<void> {
  return sendJSON(`/api/admin/rooms/${encodeURIComponent(id)}`, 'PUT', meta)
}

/** Empties a room for the next talk. The old transcript is archived, not lost. */
export function resetRoom(id: string): Promise<void> {
  return sendJSON(`/api/admin/rooms/${encodeURIComponent(id)}/reset`, 'POST')
}

/** Takes a room out of the lobby entirely. Its transcript is archived too. */
export function deleteRoom(id: string): Promise<void> {
  return sendJSON(`/api/admin/rooms/${encodeURIComponent(id)}`, 'DELETE')
}

async function sendJSON(path: string, method: string, body?: unknown): Promise<void> {
  const resp = await fetch(path, {
    method,
    headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!resp.ok) {
    let message = resp.statusText
    try {
      const parsed = (await resp.json()) as { error?: string }
      if (parsed.error) message = parsed.error
    } catch {
      /* the status line will have to do */
    }
    throw new ApiError(message, resp.status)
  }
}
