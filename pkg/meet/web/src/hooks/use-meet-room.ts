import { useCallback, useEffect, useMemo, useState } from "react"

import {
  ApiError,
  getRoom,
  listRooms,
  observeRoom,
  postMessage,
  runAction,
  usingMock,
} from "@/lib/api"
import type {
  LiveEvent,
  MeetEvent,
  RoomDetail,
  RoomSummary,
  State,
} from "@/lib/contracts"

export type ConnectionStatus = "connecting" | "open" | "closed"

export function useMeetRoom() {
  const [rooms, setRooms] = useState<RoomSummary[]>([])
  const [selectedRef, setSelectedRef] = useState("")
  const [detail, setDetail] = useState<RoomDetail | null>(null)
  const [state, setState] = useState<State | null>(null)
  const [events, setEvents] = useState<MeetEvent[]>([])
  const [live, setLive] = useState<LiveEvent | null>(null)
  const [connection, setConnection] =
    useState<ConnectionStatus>("connecting")
  const [queued, setQueued] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [sending, setSending] = useState(false)

  useEffect(() => {
    let active = true
    listRooms()
      .then((nextRooms) => {
        if (!active) return
        setRooms(nextRooms)
        setSelectedRef((current) => current || nextRooms[0]?.id || "")
      })
      .catch((reason: unknown) => {
        if (active) setError(messageFor(reason))
      })
    return () => {
      active = false
    }
  }, [])

  useEffect(() => {
    if (!selectedRef) return
    let active = true
    setEvents([])
    setLive(null)
    setError(null)
    getRoom(selectedRef)
      .then((nextDetail) => {
        if (!active) return
        setDetail(nextDetail)
        setState(nextDetail.state)
      })
      .catch((reason: unknown) => {
        if (active) setError(messageFor(reason))
      })

    const stop = observeRoom(
      selectedRef,
      (frame) => {
        if (!active) return
        if (frame.kind === "info") setState(frame.data)
        if (frame.kind === "event") {
          setEvents((current) => addUnique(current, frame.data))
        }
        if (frame.kind === "live") {
          if (frame.data.kind === "spoke") {
            setLive(null)
          } else if (frame.data.kind === "speaking") {
            setLive(frame.data)
          } else {
            setLive((current) => ({
              ...frame.data,
              text:
                current?.speaker === frame.data.speaker &&
                current.round === frame.data.round &&
                current.text
                  ? `${current.text}\n${frame.data.text}`
                  : frame.data.text,
            }))
          }
        }
      },
      setConnection,
    )
    return () => {
      active = false
      stop()
    }
  }, [selectedRef])

  const send = useCallback(
    async (text: string, agent?: string) => {
      if (!selectedRef || !state) return
      setSending(true)
      setError(null)
      setQueued(null)
      try {
        if (agent) {
          await runAction(selectedRef, "address", { agent, text })
        } else {
          const event = await postMessage(selectedRef, state.human, text)
          setEvents((current) => addUnique(current, event))
        }
      } catch (reason) {
        if (reason instanceof ApiError && reason.status === 409) {
          setQueued(
            agent
              ? `Your message to ${agent} is queued for the next turn.`
              : "Your request is queued for the next turn.",
          )
        } else {
          setError(messageFor(reason))
        }
      } finally {
        setSending(false)
      }
    },
    [selectedRef, state],
  )

  const act = useCallback(
    async (action: string, label: string, body?: unknown) => {
      if (!selectedRef) return
      setSending(true)
      setError(null)
      setQueued(null)
      try {
        await runAction(selectedRef, action, body)
      } catch (reason) {
        if (reason instanceof ApiError && reason.status === 409) {
          setQueued(`${label} is queued while the current turn finishes.`)
        } else if (reason instanceof ApiError && reason.status === 403) {
          setError("This control is available to the room organizer.")
        } else {
          setError(messageFor(reason))
        }
      } finally {
        setSending(false)
      }
    },
    [selectedRef],
  )

  const isOrganizer = useMemo(
    () => Boolean(state && state.initiator === state.human),
    [state],
  )

  return {
    rooms,
    selectedRef,
    selectRoom: setSelectedRef,
    detail,
    state,
    events,
    live,
    connection,
    queued,
    dismissQueued: () => setQueued(null),
    error,
    sending,
    send,
    act,
    isOrganizer,
    usingMock,
  }
}

function eventKey(event: MeetEvent) {
  return [event.kind, event.speaker, event.ts, event.text].join("|")
}

function addUnique(current: MeetEvent[], event: MeetEvent) {
  const key = eventKey(event)
  return current.some((item) => eventKey(item) === key)
    ? current
    : [...current, event]
}

function messageFor(reason: unknown) {
  if (reason instanceof Error) return reason.message
  return "Something went wrong. Please try again."
}
