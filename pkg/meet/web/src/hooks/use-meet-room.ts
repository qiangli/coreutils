import { useCallback, useEffect, useMemo, useRef, useState } from "react"
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
  recall,
  recallDM,
  runActionJob,
  runAction,
  usingMock,
} from "@/lib/api"
import { ALL_SEATS } from "@/lib/contracts"
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

/** HOLD_MS is how long a clicked message waits in this browser before it is
 * dispatched.
 *
 * It exists so that "cancel" can mean SOMETHING TRUE. Aborting an in-flight
 * request does not: it cancels the response, never the effect, and the two
 * synchronous send paths — a plain post and a 1:1 — write their record inside
 * that request, so a post-dispatch cancel there is a race the sender loses
 * almost every time. During the hold nothing has left the page, so cancelling
 * is a local fact rather than a claim about the server.
 *
 * Five seconds is the undo-send window people already know from mail clients.
 * It costs an agent conversation nothing: a turn takes minutes.
 */
export const DEFAULT_HOLD_MS = 5_000

/** holdMs is the hold this page is actually using.
 *
 * Configurable for two different readers. An operator who does not want the
 * pause sets it to 0 and gets the old behaviour — dispatch on click, with a
 * recall afterwards that will usually have to retract. A test sets it on the
 * URL so the suite does not spend five seconds per message proving something
 * else, and so the hold itself can be the subject of a test that asks for it.
 *
 * Storage is per-browser on purpose: how long you want to be able to change
 * your mind is a personal setting, not something to impose on everyone else who
 * opens the same room.
 */
export function holdMs(): number {
  const param = new URLSearchParams(window.location.search).get("hold")
  if (param !== null) {
    const ms = Number(param)
    if (Number.isFinite(ms) && ms >= 0) return ms
  }
  try {
    const saved = window.localStorage.getItem("bashy.meet.holdMs")
    if (saved !== null) {
      const ms = Number(saved)
      if (Number.isFinite(ms) && ms >= 0) return ms
    }
  } catch (_) {
    // A browser with storage blocked still has to be able to send a message.
  }
  return DEFAULT_HOLD_MS
}

/** RecallOutcome mirrors the server's verdict vocabulary exactly, plus the one
 * verdict the client may reach on its own — "canceled" during the hold, which
 * it may claim because nothing was sent. */
export type RecallOutcome = "canceled" | "retracted" | "gone"

/** PendingSend is a message between the click and the delivery.
 *
 * `phase` is the whole state machine: "holding" is ours to withdraw, "sending"
 * and "sent" are the server's to answer for. `job` and `ts` are the handles the
 * server gave us, and which one exists depends on the path — a room's addressed
 * send answers with a job, everything else with a record timestamp.
 */
export type PendingSend = {
  phase: "holding" | "sending" | "sent"
  text: string
  agent?: string
  ref: string
  kind: ConversationKind
  timer: number
  until: number
  job?: string
  ts?: string
}

/** stampOf reads the handle off a record the server just returned. */
function stampOf(event: MeetEvent): string {
  const ts = event.ts
  if (typeof ts === "string") return ts
  if (typeof ts === "number") return new Date(ts).toISOString()
  return ""
}

type ConversationKind = "room" | "dm"

function linkedConversation(): { kind: ConversationKind; ref: string } | null {
  const params = new URLSearchParams(window.location.search)
  const dm = params.get("dm")?.trim()
  if (dm) return { kind: "dm", ref: dm }
  const room = params.get("room")?.trim()
  if (room) return { kind: "room", ref: room }
  return null
}

function rememberConversation(kind: ConversationKind, ref: string) {
  const next = new URL(window.location.href)
  next.searchParams.delete("room")
  next.searchParams.delete("dm")
  next.searchParams.set(kind, ref)
  window.history.replaceState(null, "", next)
}

export function useMeetRoom() {
  const [draft] = useState(
    () => new URLSearchParams(window.location.search).get("draft") ?? "",
  )
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
  // THE PENDING SEND — the one piece of state that makes a truthful "not sent"
  // possible. While `phase` is "holding", the message is still in this browser
  // and cancelling it is a local fact needing no server's permission. Once it
  // is "sent", only the server can say what a recall achieved, so the handles
  // it answers with (a job for a room, a record timestamp for a chat) are kept
  // here for exactly that call.
  const [pending, setPending] = useState<PendingSend | null>(null)
  const pendingRef = useRef<PendingSend | null>(null)
  pendingRef.current = pending
  const [recalling, setRecalling] = useState(false)

  useEffect(() => {
    let active = true
    const linked = linkedConversation()
    const openChat =
      new URLSearchParams(window.location.search).get("chat") === "1"
    // Opening a 1:1 link is the same operation as choosing an agent from the
    // Chat picker: ensure its durable DM exists, then load the ordinary lists.
    // A sprint-room link is read-only and needs no corresponding mutation.
    const prepare = linked?.kind === "dm" && !usingMock
      ? createDMRequest(linked.ref)
      : Promise.resolve()
    prepare
      .then(() => Promise.all([listRooms(), listAgents(), listDMs()]))
      .then(([nextRooms, nextAgents, nextDMs]) => {
        if (!active) return
        setRooms(nextRooms)
        setAgents(nextAgents)
        setDMs(nextDMs)
        if (linked) {
          setSelectedKind(linked.kind)
          setViewKind(linked.kind)
          setSelectedRef(linked.ref)
        } else if (openChat) {
          setSelectedKind("dm")
          setViewKind("dm")
          setSelectedRef("")
        } else {
          setSelectedRef((current) => current || nextRooms[0]?.id || "")
        }
      })
      .catch((reason: unknown) => {
        if (active) setError(messageFor(reason))
      })
    return () => {
      active = false
    }
  }, [])

  const selectRoom = useCallback((ref: string) => {
    setViewKind("room")
    setSelectedKind("room")
    setSelectedRef(ref)
    rememberConversation("room", ref)
  }, [])

  const selectDM = useCallback((agent: string) => {
    setViewKind("dm")
    setSelectedKind("dm")
    setSelectedRef(agent)
    rememberConversation("dm", agent)
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

  // dispatch is the send itself, once the hold has expired. It is separate from
  // `send` so the hold has something to call and the tests have something to
  // drive; nothing here decides whether to wait.
  const dispatch = useCallback(
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
          const at = await postDM(selectedRef, text)
          markSent({ ts: at })
          setQueued(`Your message to ${selectedRef} was accepted; the reply will appear here.`)
        } else if (agent === ALL_SEATS) {
          // A broadcast is MAIL, not a floor: it lands in every participant's
          // inbox and waits to be read. Running N turns from one browser
          // message is what `meet round` is for, and it is chaired.
          const event = await postMessage(selectedRef, state.human, text, ALL_SEATS)
          setEvents((current) => addUnique(current, event))
          markSent({ ts: stampOf(event) })
          setQueued("Delivered to everyone in the room. Each participant sees it as their own mail.")
        } else if (agent) {
          const job = await runActionJob(selectedRef, "address", { agent, text })
          markSent({ job })
          setQueued(`Your message to ${agent} was accepted; the reply will appear here.`)
        } else {
          const event = await postMessage(selectedRef, state.human, text)
          setEvents((current) => addUnique(current, event))
          markSent({ ts: stampOf(event) })
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

  // send HOLDS first, then dispatches.
  //
  // The hold is the only mechanism that can promise a message was not sent,
  // because during it nothing has left this browser — an aborted request proves
  // nothing, since the server may have committed a microsecond earlier. It is
  // also the only thing that gives the two synchronous paths (a plain post and
  // a 1:1) a cancel at all: both write their record inside the request.
  const send = useCallback(
    async (text: string, agent?: string) => {
      if (!selectedRef || !state) return
      if (pendingRef.current) return
      setError(null)
      setQueued(null)
      const hold = holdMs()
      if (hold <= 0) {
        await dispatch(text, agent)
        return
      }
      const timer = window.setTimeout(() => {
        setPending((current) =>
          current && current.phase === "holding"
            ? { ...current, phase: "sending" }
            : current,
        )
        void dispatch(text, agent)
      }, hold)
      setPending({
        phase: "holding",
        text,
        agent,
        ref: selectedRef,
        kind: selectedKind,
        timer,
        until: Date.now() + hold,
      })
    },
    [dispatch, selectedRef, selectedKind, state],
  )

  // markSent records the handles the server just gave us. Called from inside
  // dispatch, at the moment the message stops being ours to withhold.
  function markSent(handle: { job?: string; ts?: string }) {
    setPending((current) =>
      current
        ? { ...current, phase: "sent", job: handle.job, ts: handle.ts }
        : current,
    )
  }

  // cancelSend is the one control, and it does one of two things depending on
  // where the message actually is — never on how long ago it was clicked.
  const cancelSend = useCallback(async (): Promise<RecallOutcome> => {
    const current = pendingRef.current
    if (!current) return "gone"
    if (current.phase === "holding") {
      // Nothing has been sent, and SAYING SO is half the feature: a control
      // that silently stops something leaves the sender wondering whether it
      // went out. This is the only branch allowed to make that claim.
      window.clearTimeout(current.timer)
      setPending(null)
      setError(null)
      setQueued("Canceled — the message was not sent.")
      return "canceled"
    }
    setRecalling(true)
    try {
      const result =
        current.kind === "dm"
          ? await recallDM(current.ref, current.ts ?? "")
          : await recall(current.ref, { job: current.job, ts: current.ts })
      setPending(null)
      if (result.verdict === "retracted") {
        if (result.event) setEvents((rows) => addUnique(rows, result.event!))
        if (current.kind === "dm") setLive(null)
        setQueued(
          "Too late to unsend — the message was already delivered, so a retraction was posted beside it.",
        )
      } else if (result.verdict === "canceled") {
        if (current.kind === "dm") setLive(null)
        setQueued("Canceled — the message was not sent.")
      } else {
        setQueued(
          "Nothing to cancel: that message has already been delivered and answered.",
        )
      }
      return result.verdict
    } catch (reason) {
      setError(messageFor(reason))
      return "gone"
    } finally {
      setRecalling(false)
    }
  }, [])

  // The hold is a live countdown, so the button can show what is left of it.
  // It is a timer rather than a derived value because nothing else re-renders
  // while a message waits.
  const [heldFor, setHeldFor] = useState(0)
  useEffect(() => {
    if (!pending || pending.phase !== "holding") {
      setHeldFor(0)
      return
    }
    const tick = () =>
      setHeldFor(Math.max(0, Math.ceil((pending.until - Date.now()) / 1000)))
    tick()
    const id = window.setInterval(tick, 200)
    return () => window.clearInterval(id)
  }, [pending])

  // A pending send belongs to the conversation it was typed in. Switching away
  // while one is held would otherwise deliver it into a room the sender is no
  // longer looking at — and cancel would then act on the wrong one.
  useEffect(() => {
    const current = pendingRef.current
    if (current && current.phase === "holding" && current.ref !== selectedRef) {
      window.clearTimeout(current.timer)
      void dispatch(current.text, current.agent)
      setPending(null)
    }
  }, [selectedRef, dispatch])

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
        selectRoom(created.id)
        return true
      } catch (reason) {
        setError(messageFor(reason))
        return false
      } finally {
        setCreating(false)
      }
    },
    [selectRoom],
  )

  const createDM = useCallback(async (agent: string) => {
    setCreating(true)
    setError(null)
    try {
      await createDMRequest(agent)
      setDMs(await listDMs())
      selectDM(agent)
      return true
    } catch (reason) {
      setError(messageFor(reason))
      return false
    } finally {
      setCreating(false)
    }
  }, [selectDM])

  const isOrganizer = useMemo(
    () => Boolean(state && state.initiator === state.human),
    [state],
  )

  // Who a message goes to when the reader has not typed "@name".
  //
  // A room's DEFAULT recipient is its owner — the project manager in a
  // sprint's room. That is not a convenience: unaddressed mail in such a room
  // already lands on that seat server-side, so a composer that sent to nobody
  // was disagreeing with the room it was typing into. `""` is a real, choosable
  // value meaning the whole room, which is why this is a string and not a
  // nullable name.
  //
  // The choice is per ROOM and per BROWSER, so it is remembered here rather
  // than on the room: two people in one room may each be talking to a different
  // agent, and writing one reader's pick onto shared state would move the other
  // reader's recipient under them.
  const [recipientByRoom, setRecipientByRoom] = useState<Record<string, string>>(
    () => readStoredRecipients(),
  )
  // The owner when there is one, a real broadcast when there is not. Never the
  // empty string: that posts room history addressed to nobody, which lands in
  // no inbox and wakes no one — indistinguishable, from the outside, from every
  // agent in the room ignoring you.
  const owner = state?.owner || ALL_SEATS
  const recipient =
    selectedKind === "room" && selectedRef
      ? (recipientByRoom[selectedRef] ?? owner)
      : ""
  const setRecipient = useCallback(
    (next: string) => {
      if (!selectedRef) return
      setRecipientByRoom((current) => {
        const updated = { ...current, [selectedRef]: next }
        writeStoredRecipients(updated)
        return updated
      })
    },
    [selectedRef],
  )

  return {
    agents,
    rooms,
    dms,
    selectedRef,
    selectedKind,
    viewKind,
    selectMode: setViewKind,
    selectRoom,
    selectDM,
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
    // The composer needs all three: whether a message is waiting, how long it
    // has left, and the one control that stops it.
    pending,
    heldFor,
    recalling,
    cancelSend,
    act,
    createRoom,
    createDM,
    creating,
    isOrganizer,
    recipient,
    setRecipient,
    usingMock,
    debugRaw,
    setDebugRaw,
    draft,
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
    // A direct message has one recipient and it is the agent named in it.
    // Saying so keeps the composer's recipient logic uniform instead of
    // special-casing the DM view into a second code path.
    owner: dm.agent,
    owner_title: "agent",
    // A chat has no seat to be late-bound to: the recipient IS the agent, and
    // the room-side label ("conductor:99") has no counterpart here.
    default_to: "",
  }
}

function dmMeetEvent(event: DMEvent): MeetEvent {
  return {
    // The record's OWN kind wins where it has one. A chat projects everything
    // onto user/assistant, and under that projection a retraction would arrive
    // as an ordinary message from the human while the message it withdraws
    // still read as live — the one thing this feature must not do, in the
    // surface a person is most likely to be reading.
    kind:
      event.kind === "retraction"
        ? "retraction"
        : event.role === "human"
          ? "human"
          : "turn",
    speaker: event.speaker,
    role: event.role,
    // A 1:1 has one counterpart, so the addressee is implied and the message
    // list does not print one. Carrying an invented name here would put the
    // same "to" under every message and say nothing.
    to: "",
    text: event.text,
    ts: event.ts,
    status: event.status,
    retracts: event.retracts,
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

// The recipient picks live in localStorage, keyed by room.
//
// Wrapped in try/catch on BOTH sides: a private window, cleared site data, or
// a browser configured to block storage makes the accessor itself throw, and a
// composer that cannot remember a preference must still send messages.
const RECIPIENT_STORE = "bashy.meet.recipient"

function readStoredRecipients(): Record<string, string> {
  try {
    const raw = window.localStorage.getItem(RECIPIENT_STORE)
    if (!raw) return {}
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return {}
    const out: Record<string, string> = {}
    for (const [room, name] of Object.entries(parsed)) {
      if (typeof name === "string") out[room] = name
    }
    return out
  } catch {
    return {}
  }
}

function writeStoredRecipients(value: Record<string, string>) {
  try {
    window.localStorage.setItem(RECIPIENT_STORE, JSON.stringify(value))
  } catch {
    /* a reader who cannot store a preference still gets this session's. */
  }
}
