import {
  agentOptionSchema,
  dmDetailSchema,
  dmEventSchema,
  dmObserveFrameSchema,
  dmSummarySchema,
  errorSchema,
  eventSchema,
  jobRefSchema,
  observeFrameSchema,
  roomDetailSchema,
  roomSummarySchema,
  stateSchema,
  type MeetEvent,
  type AgentOption,
  type ObserveFrame,
  type RoomDetail,
  type RoomSummary,
  type State,
  type DMDetail,
  type DMEvent,
  type DMSummary,
} from "./contracts"
import {
  MockHttpError,
  mockAction,
  mockListAgents,
  mockCreateRoom,
  mockGetRoom,
  mockListRooms,
  mockPost,
  observeMock,
} from "./mock"

const params = new URLSearchParams(window.location.search)
export const usingMock =
  params.get("mock") === "1" ||
  (import.meta.env.DEV && params.get("mock") !== "0")

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    message: string,
  ) {
    super(message)
  }
}

export async function listDMs(): Promise<DMSummary[]> {
  if (usingMock) return []
  return dmSummarySchema.array().parse(await request("api/dms"))
}

export async function createDM(agent: string): Promise<DMSummary> {
  return dmSummarySchema.parse(await request("api/dms", {
    method: "POST",
    body: JSON.stringify({ agent }),
  }))
}

export async function getDM(agent: string): Promise<DMDetail> {
  return dmDetailSchema.parse(await request(`api/dms/${encodeURIComponent(agent)}`))
}

export async function postDM(agent: string, text: string): Promise<void> {
  await request(`api/dms/${encodeURIComponent(agent)}/messages`, {
    method: "POST",
    body: JSON.stringify({ text }),
  })
}

export function observeDM(
  agent: string,
  onEvent: (event: DMEvent) => void,
  onStatus: (status: "connecting" | "open" | "closed") => void,
): () => void {
  const url = new URL("observe-dm", document.baseURI)
  url.protocol = window.location.protocol === "https:" ? "wss:" : "ws:"
  url.searchParams.set("agent", agent)
  let stopped = false
  let socket: WebSocket | null = null
  let retry: number | null = null
  const connect = () => {
    if (stopped) return
    onStatus("connecting")
    const next = new WebSocket(url)
    socket = next
    next.addEventListener("open", () => onStatus("open"))
    next.addEventListener("close", () => {
      if (stopped) return
      onStatus("closed")
      retry = window.setTimeout(connect, 500)
    })
    next.addEventListener("error", () => next.close())
    next.addEventListener("message", (message) => {
      const result = dmObserveFrameSchema.safeParse(JSON.parse(String(message.data)))
      if (result.success) onEvent(dmEventSchema.parse(result.data.data))
    })
  }
  connect()
  return () => {
    stopped = true
    if (retry !== null) window.clearTimeout(retry)
    socket?.close()
    onStatus("closed")
  }
}

function endpoint(path: string): URL {
  return new URL(path.replace(/^\/+/, ""), document.baseURI)
}

async function request(path: string, init?: RequestInit): Promise<unknown> {
  const response = await fetch(endpoint(path), {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...init?.headers,
    },
  })
  if (!response.ok) {
    const body = errorSchema.safeParse(await response.json().catch(() => ({})))
    throw new ApiError(
      response.status,
      body.success ? body.data.error : `Request failed (${response.status})`,
    )
  }
  return response.status === 204 ? undefined : response.json()
}

function normalizeError(error: unknown): never {
  if (error instanceof MockHttpError) {
    throw new ApiError(error.status, error.message)
  }
  throw error
}

export async function listRooms(): Promise<RoomSummary[]> {
  if (usingMock) return roomSummarySchema.array().parse(await mockListRooms())
  return roomSummarySchema.array().parse(await request("api/rooms"))
}

export async function listAgents(): Promise<AgentOption[]> {
  if (usingMock) return agentOptionSchema.array().parse(await mockListAgents())
  return agentOptionSchema.array().parse(await request("api/agents"))
}

export async function getRoom(ref: string): Promise<RoomDetail> {
  if (usingMock) {
    try {
      return roomDetailSchema.parse(await mockGetRoom(ref))
    } catch (error) {
      normalizeError(error)
    }
  }
  return roomDetailSchema.parse(
    await request(`api/rooms/${encodeURIComponent(ref)}`),
  )
}

/** NewRoom is the smallest room a browser can open. One participant is the 1:1
 * assistant case; several make it a meeting. Everything else on CreateOptions
 * (chair, secretary, agenda, bands) has a working default, and a room can grow
 * into those from the inside — the model has one room type, so the create form
 * does not need to ask which kind you want. */
export interface NewRoom {
  topic: string
  participants: string[]
}

export async function createRoom(input: NewRoom): Promise<State> {
  if (usingMock) {
    try {
      return stateSchema.parse(await mockCreateRoom(input.topic, input.participants))
    } catch (error) {
      normalizeError(error)
    }
  }
  return stateSchema.parse(
    await request("api/rooms", {
      method: "POST",
      body: JSON.stringify({ topic: input.topic, participants: input.participants }),
    }),
  )
}

export async function postMessage(
  ref: string,
  author: string,
  text: string,
): Promise<MeetEvent> {
  if (usingMock) return eventSchema.parse(await mockPost(ref, author, text))
  return eventSchema.parse(
    await request(`api/rooms/${encodeURIComponent(ref)}/post`, {
      method: "POST",
      body: JSON.stringify({ author, text }),
    }),
  )
}

export async function runAction(
  ref: string,
  action: string,
  body?: unknown,
): Promise<void> {
  if (usingMock) {
    try {
      const result = await mockAction(action, body)
      if (result) jobRefSchema.parse(result)
      return
    } catch (error) {
      normalizeError(error)
    }
  }
  const result = await request(
    `api/rooms/${encodeURIComponent(ref)}/${action}`,
    {
      method: "POST",
      body: body === undefined ? undefined : JSON.stringify(body),
    },
  )
  if (result !== undefined) {
    if (action === "mark") eventSchema.parse(result)
    else if (action === "open") stateSchema.parse(result)
    else jobRefSchema.parse(result)
  }
}

export function observeRoom(
  ref: string,
  onFrame: (frame: ObserveFrame) => void,
  onStatus: (status: "connecting" | "open" | "closed") => void,
): () => void {
  if (usingMock) {
    onStatus("open")
    const stop = observeMock((frame) => onFrame(observeFrameSchema.parse(frame)))
    return () => {
      stop()
      onStatus("closed")
    }
  }

  const url = new URL("observe", document.baseURI)
  url.protocol = window.location.protocol === "https:" ? "wss:" : "ws:"
  url.searchParams.set("room", ref)

  let stopped = false
  let socket: WebSocket | null = null
  let retry: number | null = null
  let retryDelay = 250

  const connect = () => {
    if (stopped) return
    onStatus("connecting")
    const next = new WebSocket(url)
    socket = next
    next.addEventListener("open", () => {
      if (stopped || socket !== next) return
      retryDelay = 250
      onStatus("open")
    })
    next.addEventListener("close", (event) => {
      if (socket === next) socket = null
      if (stopped) return
      onStatus("closed")
      // A normal close is how the server says this room ended. Reopening the
      // room changes the hook's observe revision and creates a fresh socket.
      // Every other close (service restart, network loss, tunnel flap) retries
      // automatically, so a browser tab never needs a manual refresh merely
      // because its backend restarted.
      if (event.code === 1000) return
      retry = window.setTimeout(connect, retryDelay)
      retryDelay = Math.min(retryDelay * 2, 5_000)
    })
    next.addEventListener("error", () => next.close())
    next.addEventListener("message", (message) => {
      try {
        const result = observeFrameSchema.safeParse(
          JSON.parse(String(message.data)),
        )
        if (result.success) onFrame(result.data)
        else console.warn("Ignored invalid meet frame", result.error)
      } catch (error) {
        console.warn("Ignored unreadable meet frame", error)
      }
    })
  }

  connect()
  return () => {
    stopped = true
    if (retry !== null) window.clearTimeout(retry)
    socket?.close()
    socket = null
    onStatus("closed")
  }
}
