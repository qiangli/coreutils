import { useCallback, useEffect, useMemo, useState } from "react"
import { ZodError } from "zod"

import {
  ApiError,
  createRoom as createRoomRequest,
  createDM as createDMRequest,
  getDM,
  getRoom,
  listAgents,
  listDMs,
  listRooms,
  observeRoom,
  observeDM,
  postMessage,
  postDM,
  startDMWork,
  runAction,
  usingMock,
} from "@/lib/api"
import type {
  AgentOption,
  DMEvent,
  DMSummary,
  LiveEvent,
  MeetEvent,
  RoomDetail,
  RoomSummary,
  State,
} from "@/lib/contracts"

export type ConnectionStatus = "connecting" | "open" | "closed"

export function useMeetRoom() {
  const [rooms, setRooms] = useState<RoomSummary[]>([])
  const [dms, setDMs] = useState<DMSummary[]>([])
  const [agents, setAgents] = useState<AgentOption[]>([])
  const [selectedRef, setSelectedRef] = useState("")
  const [selectedKind, setSelectedKind] = useState<"room" | "dm">("room")
  const [viewKind, setViewKind] = useState<"room" | "dm">("room")
  const [detail, setDetail] = useState<RoomDetail | null>(null)
  const [state, setState] = useState<State | null>(null)
  const [events, setEvents] = useState<MeetEvent[]>([])
  const [live, setLive] = useState<LiveEvent | null>(null)
  const [connection, setConnection] =
    useState<ConnectionStatus>("connecting")
  const [observeRevision, setObserveRevision] = useState(0)
  // The raw-transport view, off by default. It lives here rather than in the
  // message list because the server decides what to send: turning it on
  // reconnects the socket with ?raw=1, so an ordinary session never carries the
  // extra bytes.
  const [debugRaw, setDebugRaw] = useState(false)
  const [queued, setQueued] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [sending, setSending] = useState(false)
  const [creating, setCreating] = useState(false)

  useEffect(() => {
    let active = true
    Promise.all([listRooms(), listAgents(), listDMs()])
      .then(([nextRooms, nextAgents, nextDMs]) => {
        if (!active) return
        setRooms(nextRooms)
        setAgents(nextAgents)
        setDMs(nextDMs)
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
    if (selectedKind === "dm") {
      // Fetch and observe the SAME projection. If the HTTP snapshot omitted
      // raw while the socket included it, their identical event keys made the
      // winner timing-dependent when the reader toggled the debug view.
      getDM(selectedRef, debugRaw)
        .then((nextDetail) => {
          if (!active) return
          setDetail(null)
          setState(dmState(nextDetail.state))
          setEvents(nextDetail.events.map(dmMeetEvent))
        })
        .catch((reason: unknown) => {
          if (active) setError(messageFor(reason))
        })
      const stop = observeDM(
        selectedRef,
        (event) => {
          if (active) {
            setEvents((current) => addUnique(current, dmMeetEvent(event)))
            if (event.role === "assistant") setLive(null)
          }
        },
        setConnection,
        debugRaw,
      )
      return () => {
        active = false
        stop()
      }
    }
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
      debugRaw,
    )
    return () => {
      active = false
      stop()
    }
  }, [selectedRef, selectedKind, observeRevision, debugRaw])

  const send = useCallback(
    async (text: string, agent?: string) => {
      if (!selectedRef || !state) return
      setSending(true)
      setError(null)
      setQueued(null)
      try {
        if (selectedKind === "dm") {
          setLive({
            kind: "speaking",
            round: 0,
            speaker: selectedRef,
            role: "assistant",
            text: "",
            status: "working",
          })
          await postDM(selectedRef, text)
          setQueued(`Your message to ${selectedRef} was accepted; the reply will appear here.`)
        } else if (agent) {
          await runAction(selectedRef, "address", { agent, text })
          setQueued(`Your message to ${agent} was accepted; the reply will appear here.`)
        } else {
          const event = await postMessage(selectedRef, state.human, text)
          setEvents((current) => addUnique(current, event))
        }
      } catch (reason) {
        if (selectedKind === "dm") setLive(null)
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
    [selectedRef, selectedKind, state],
  )

  const act = useCallback(
    async (action: string, label: string, body?: unknown) => {
      if (!selectedRef) return false
      setSending(true)
      setError(null)
      setQueued(null)
      try {
        await runAction(selectedRef, action, body)
        // The roster lives on State, and State reaches the browser exactly once
        // — in the info frame sent when /observe connects. A verb that changes
        // membership therefore leaves the member list stale until a reload, so
        // re-read the room after one. The turn verbs need no such thing: their
        // output arrives as transcript events, which the socket does stream.
        if (["invite", "kick", "mark", "open", "close"].includes(action)) {
          const refreshed = await getRoom(selectedRef)
          setDetail(refreshed)
          setState(refreshed.state)
          if (action === "open") {
            setEvents([])
            setLive(null)
            setObserveRevision((current) => current + 1)
          }
          setRooms(await listRooms())
        }
        return true
      } catch (reason) {
        if (reason instanceof ApiError && reason.status === 409) {
          setQueued(`${label} is queued while the current turn finishes.`)
        } else if (reason instanceof ApiError && reason.status === 403) {
          setError("This control is available to the room organizer.")
        } else {
          setError(messageFor(reason))
        }
        return false
      } finally {
        setSending(false)
      }
    },
    [selectedRef],
  )

  /** createRoom opens a room and switches to it. The room list is re-fetched
   * rather than patched: the server assigns the room number and the seat the
   * caller took, and guessing either would put a number on screen that a reload
   * corrects. Returns whether it worked, so the dialog knows to close. */
  const createRoom = useCallback(
    async (topic: string, owner: string, participants: string[]) => {
      setCreating(true)
      setError(null)
      try {
        const created = await createRoomRequest({ topic, owner, participants })
        const nextRooms = await listRooms()
        setRooms(nextRooms)
        setSelectedKind("room")
        setViewKind("room")
        setSelectedRef(created.id)
        return true
      } catch (reason) {
        setError(messageFor(reason))
        return false
      } finally {
        setCreating(false)
      }
    },
    [],
  )

  const createDM = useCallback(async (agent: string) => {
    setCreating(true)
    setError(null)
    try {
      await createDMRequest(agent)
      setDMs(await listDMs())
      setSelectedKind("dm")
      setViewKind("dm")
      setSelectedRef(agent)
      return true
    } catch (reason) {
      setError(messageFor(reason))
      return false
    } finally {
      setCreating(false)
    }
  }, [])

  const startWork = useCallback(async (text: string) => {
    if (!selectedRef || selectedKind !== "dm") return false
    setSending(true)
    setError(null)
    setQueued(null)
    try {
      await startDMWork(selectedRef, text)
      setQueued(`A managed work session for ${selectedRef} started. Later messages are delivered through the agent inbox.`)
      return true
    } catch (reason) {
      setError(messageFor(reason))
      return false
    } finally {
      setSending(false)
    }
  }, [selectedRef, selectedKind])

  const isOrganizer = useMemo(
    () => Boolean(state && state.initiator === state.human),
    [state],
  )

  return {
    agents,
    rooms,
    dms,
    selectedRef,
    selectedKind,
    viewKind,
    selectMode: setViewKind,
    selectRoom: (ref: string) => { setViewKind("room"); setSelectedKind("room"); setSelectedRef(ref) },
    selectDM: (agent: string) => { setViewKind("dm"); setSelectedKind("dm"); setSelectedRef(agent) },
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
    startWork,
    act,
    createRoom,
    createDM,
    creating,
    isOrganizer,
    usingMock,
    debugRaw,
    setDebugRaw,
  }
}

function dmState(dm: DMSummary): State {
  return {
    id: `dm:${dm.agent}`,
    board: false,
    room: dm.agent,
    name: dm.agent,
    topic: "Direct message",
    agenda: [],
    participants: [
      { name: dm.human, role: "human", live: true },
      { name: dm.agent, role: "agent", live: true },
    ],
    secretary: "",
    chair: "",
    human: dm.human,
    status: "open",
    cwd: "",
    out: "",
    round: 0,
    initiator: dm.human,
    decision_mode: "",
  }
}

function dmMeetEvent(event: DMEvent): MeetEvent {
  return {
    kind: event.role === "human" ? "human" : "turn",
    speaker: event.speaker,
    role: event.role,
    text: event.text,
    ts: event.ts,
    status: event.status,
    raw: event.raw,
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
  if (reason instanceof ZodError) {
    return "Meet received an incompatible response. Refresh the page; if it persists, restart `bashy meet serve`."
  }
  if (reason instanceof Error) return reason.message
  return "Something went wrong. Please try again."
}
