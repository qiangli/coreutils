import {
  errorSchema,
  eventSchema,
  jobRefSchema,
  observeFrameSchema,
  roomDetailSchema,
  roomSummarySchema,
  type MeetEvent,
  type ObserveFrame,
  type RoomDetail,
  type RoomSummary,
} from "./contracts"
import {
  MockHttpError,
  mockAction,
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
  if (result !== undefined) jobRefSchema.parse(result)
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
  const socket = new WebSocket(url)
  onStatus("connecting")
  socket.addEventListener("open", () => onStatus("open"))
  socket.addEventListener("close", () => onStatus("closed"))
  socket.addEventListener("message", (message) => {
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
  return () => socket.close()
}
