import type { PublicConfig, Room, Segment } from './types'

// The passcode arrives in the link the organisers hand out, as ?k=. It is
// held here in memory for the life of the page and nowhere else.
//
// It used to be copied into localStorage and left in the address bar, which
// undid the point of the server setting an HttpOnly cookie: anything running
// on the origin could read it back, it stayed visible in the URL bar and in
// browser history, and it reached every proxy access log that records query
// strings. Stripping it on arrival and exchanging it for the cookie leaves it
// in one place that a script cannot reach.
let viewerKey: string | null = null

/** The viewer passcode for this page, if one arrived or was typed. */
export function getViewerKey(): string | null {
  return viewerKey
}

export function setViewerKey(key: string): void {
  viewerKey = key
}

export function clearViewerKey(): void {
  viewerKey = null
}

/**
 * Takes the passcode out of the address bar and exchanges it for the session
 * cookie, so a reload does not need it and nothing has to remember it.
 *
 * Returns once the exchange has been attempted. A failure is not reported: an
 * out-of-date link should land on the passcode prompt like any other visitor,
 * not on an error.
 */
export async function adoptPasscodeFromURL(): Promise<void> {
  const url = new URL(window.location.href)
  const fromUrl = url.searchParams.get('k')
  if (!fromUrl) return

  viewerKey = fromUrl
  url.searchParams.delete('k')
  try {
    window.history.replaceState(window.history.state, '', url.toString())
  } catch {
    /* an old browser will just keep it in the bar; the rest still holds */
  }
  try {
    await submitPasscode(fromUrl)
  } catch {
    /* offline, or the code has expired: the gate will ask for it */
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
