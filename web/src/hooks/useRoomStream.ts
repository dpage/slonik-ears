import { useCallback, useEffect, useRef, useState } from 'react'
import { ApiError, fetchRoom, wsURL } from '../api'
import type { ConnectionState, Message, Partial as PartialText, Room, Segment, Status } from '../types'

/** Keep memory bounded: a full conference day is a lot of lines for a phone. */
const MAX_SEGMENTS = 3000

export interface RoomStream {
  room: Room | null
  segments: Segment[]
  partial: PartialText | null
  status: Status | null
  connection: ConnectionState
  error: string | null
  notFound: boolean
}

/**
 * Subscribes to a room's live transcript, reconnecting on its own and
 * resuming from the last segment it saw rather than replaying the talk.
 */
export function useRoomStream(roomId: string | undefined): RoomStream {
  const [room, setRoom] = useState<Room | null>(null)
  const [segments, setSegments] = useState<Segment[]>([])
  const [partial, setPartial] = useState<PartialText | null>(null)
  const [status, setStatus] = useState<Status | null>(null)
  const [connection, setConnection] = useState<ConnectionState>('connecting')
  const [error, setError] = useState<string | null>(null)
  const [notFound, setNotFound] = useState(false)

  const cursorRef = useRef(0)
  const socketRef = useRef<WebSocket | null>(null)
  const retryRef = useRef<number | null>(null)
  const attemptsRef = useRef(0)
  const closedRef = useRef(false)

  const apply = useCallback((msg: Message) => {
    switch (msg.type) {
      case 'snapshot':
        if (msg.room) setRoom(msg.room)
        if (msg.status !== undefined) setStatus(msg.status ?? null)
        setPartial(msg.partial ?? null)
        if (msg.reset) {
          // The room has been turned around for the next talk. Replace what we
          // hold rather than merging into it, and take the server's cursor as
          // given: keeping the larger of the two, as the ordinary resume path
          // does, would leave the previous speaker on screen and cause every
          // segment of the new talk to be discarded as one already seen.
          setSegments(msg.segments ?? [])
          cursorRef.current = msg.cursor ?? 0
          break
        }
        if (msg.segments?.length) {
          setSegments((prev) => merge(prev, msg.segments ?? []))
        }
        if (typeof msg.cursor === 'number') cursorRef.current = Math.max(cursorRef.current, msg.cursor)
        break
      case 'final':
        if (msg.segment) {
          const segment = msg.segment
          setSegments((prev) => merge(prev, [segment]))
          cursorRef.current = Math.max(cursorRef.current, segment.seq)
          setPartial(null)
        }
        break
      case 'partial':
        setPartial(msg.partial && msg.partial.text ? msg.partial : null)
        break
      case 'room_state':
        if (msg.room) setRoom(msg.room)
        break
      case 'status':
        setStatus(msg.status ?? null)
        break
      case 'error':
        setError(msg.error ?? 'The server reported a problem.')
        break
      default:
        break
    }
  }, [])

  useEffect(() => {
    if (!roomId) return
    closedRef.current = false
    cursorRef.current = 0
    attemptsRef.current = 0
    setSegments([])
    setPartial(null)
    setRoom(null)
    setNotFound(false)
    setError(null)

    const connect = () => {
      if (closedRef.current) return
      setConnection(attemptsRef.current === 0 ? 'connecting' : 'reconnecting')

      const socket = new WebSocket(
        wsURL(`/api/watch?room=${encodeURIComponent(roomId)}&since=${cursorRef.current}`),
      )
      socketRef.current = socket

      socket.onopen = () => {
        attemptsRef.current = 0
        setConnection('live')
        setError(null)
        setNotFound(false)
      }

      socket.onmessage = (event) => {
        try {
          apply(JSON.parse(event.data as string) as Message)
        } catch {
          // A malformed frame is not worth tearing the connection down for.
        }
      }

      socket.onclose = () => {
        // Only if this is still the live socket. A phone waking with a
        // half-closed connection opens a replacement before the old one's
        // close event arrives; without this check that event then nulls the
        // reference to the healthy new socket, flips the badge to
        // "Reconnecting" over a working stream, and schedules yet another
        // connect. The sockets pile up and the cleanup only ever closes
        // whichever one the ref happens to be holding.
        if (socketRef.current !== socket) return
        socketRef.current = null
        if (closedRef.current) return
        setConnection('reconnecting')
        // Find out whether this is a passing squall or something permanent.
        void fetchRoom(roomId)
          .then(() => scheduleRetry())
          .catch((err: unknown) => {
            if (err instanceof ApiError && err.status === 404) {
              // Not terminal. A room removed by mistake and recreated a
              // minute later used to leave a stage display reading "No such
              // room" for the rest of the talk until somebody walked over to
              // it. Keep retrying, slowly, and clear the notice if it returns.
              setNotFound(true)
              setConnection('offline')
              scheduleRetry()
              return
            }
            if (err instanceof ApiError && err.status === 401) {
              setConnection('denied')
              return
            }
            scheduleRetry()
          })
      }

      socket.onerror = () => {
        // onclose always follows; the retry happens there.
      }
    }

    const scheduleRetry = () => {
      if (closedRef.current || retryRef.current !== null) return
      attemptsRef.current += 1
      // Back off, but never so far that somebody sitting in the room has to
      // wonder whether to reload the page.
      const delay = Math.min(1000 * 2 ** Math.min(attemptsRef.current, 4), 10000)
      retryRef.current = window.setTimeout(() => {
        retryRef.current = null
        connect()
      }, delay + Math.random() * 400)
    }

    connect()

    // Phones aggressively suspend background tabs; when one comes back, the
    // socket is usually dead without having told anybody.
    const onVisible = () => {
      if (document.visibilityState !== 'visible') return
      // CLOSING counts as gone: the socket will never carry anything again,
      // and waiting for its close event to arrive is what left a phone
      // looking connected whilst receiving nothing.
      if (!socketRef.current || socketRef.current.readyState >= WebSocket.CLOSING) {
        if (retryRef.current !== null) {
          window.clearTimeout(retryRef.current)
          retryRef.current = null
        }
        attemptsRef.current = 0
        connect()
      }
    }
    document.addEventListener('visibilitychange', onVisible)
    window.addEventListener('online', onVisible)

    return () => {
      closedRef.current = true
      document.removeEventListener('visibilitychange', onVisible)
      window.removeEventListener('online', onVisible)
      if (retryRef.current !== null) window.clearTimeout(retryRef.current)
      retryRef.current = null
      socketRef.current?.close()
      socketRef.current = null
    }
  }, [roomId, apply])

  return { room, segments, partial, status, connection, error, notFound }
}

/** Merges incoming segments by sequence number, keeping the list ordered. */
function merge(existing: Segment[], incoming: Segment[]): Segment[] {
  if (incoming.length === 0) return existing
  const bySeq = new Map(existing.map((s) => [s.seq, s]))
  for (const seg of incoming) bySeq.set(seg.seq, seg)
  const merged = [...bySeq.values()].sort((a, b) => a.seq - b.seq)
  return merged.length > MAX_SEGMENTS ? merged.slice(merged.length - MAX_SEGMENTS) : merged
}
