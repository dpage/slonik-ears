// Mirrors internal/protocol in the Go server. Keep the two in step: the
// server is the source of truth.

export const PROTOCOL_VERSION = 1

export interface Room {
  id: string
  title?: string
  track?: string
  speaker?: string
  language?: string
  live: boolean
  viewers: number
  cursor: number
  startedAt?: number
  lastActivity?: number
}

export interface Segment {
  seq: number
  id: string
  text: string
  startMs: number
  endMs: number
  at: number
  language?: string
}

export interface Partial {
  text: string
  afterSeq: number
  at: number
}

export interface Status {
  level: number
  listening: boolean
  backend?: string
  detail?: string
  queuedSegments?: number
}

export type MessageType =
  | 'snapshot'
  | 'final'
  | 'partial'
  | 'room_state'
  | 'rooms'
  | 'status'
  | 'error'
  | 'ack'

export interface Message {
  type: MessageType
  version?: number
  room?: Room
  rooms?: Room[]
  segment?: Segment
  segments?: Segment[]
  partial?: Partial
  status?: Status
  cursor?: number
  error?: string
  serverTime?: number
}

export interface EventConfig {
  name: string
  tagline?: string
  notice?: string
}

export interface PublicConfig {
  event: EventConfig
  version: string
  requiresKey: boolean
  authenticated: boolean
  baseUrl?: string
  protocolVersion: number
}

/** How the live connection is getting on. */
export type ConnectionState = 'connecting' | 'live' | 'reconnecting' | 'offline' | 'denied'
