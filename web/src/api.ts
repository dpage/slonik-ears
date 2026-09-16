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

export function qrURL(id: string, size = 420): string {
  return `/api/rooms/${encodeURIComponent(id)}/qr.png?size=${size}`
}
