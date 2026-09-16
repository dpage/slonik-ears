import { useEffect, useRef, useState } from 'react'
import { wsURL } from '../api'
import type { ConnectionState, Message, Room } from '../types'

export interface Lobby {
  rooms: Room[]
  connection: ConnectionState
}

/** Watches the list of rooms, so a track that starts appears by itself. */
export function useLobby(): Lobby {
  const [rooms, setRooms] = useState<Room[]>([])
  const [connection, setConnection] = useState<ConnectionState>('connecting')
  const closedRef = useRef(false)
  const retryRef = useRef<number | null>(null)
  const attemptsRef = useRef(0)

  useEffect(() => {
    closedRef.current = false
    let socket: WebSocket | null = null

    const connect = () => {
      if (closedRef.current) return
      socket = new WebSocket(wsURL('/api/lobby'))
      socket.onopen = () => {
        attemptsRef.current = 0
        setConnection('live')
      }
      socket.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data as string) as Message
          if (msg.type === 'rooms') setRooms(msg.rooms ?? [])
        } catch {
          /* ignore a malformed frame */
        }
      }
      socket.onclose = () => {
        socket = null
        if (closedRef.current) return
        setConnection('reconnecting')
        attemptsRef.current += 1
        const delay = Math.min(1000 * 2 ** Math.min(attemptsRef.current, 4), 10000)
        retryRef.current = window.setTimeout(connect, delay)
      }
    }
    connect()

    return () => {
      closedRef.current = true
      if (retryRef.current !== null) window.clearTimeout(retryRef.current)
      socket?.close()
    }
  }, [])

  return { rooms, connection }
}
