import { z } from "zod"

export const memberSchema = z.union([
  z.string(),
  z.object({
    name: z.string(),
    role: z.string().optional(),
    live: z.boolean().default(false),
  }),
])

export const roomSummarySchema = z.object({
  id: z.string(),
  // The server sends the room NUMBER as an int (RoomSummary.Room is a Go int),
  // so a bare z.string() rejects every real response and the list silently
  // renders empty — which is what it did until a browser test looked. The mock
  // fixtures used strings, so the mismatch was invisible in dev.
  room: z.union([z.string(), z.number()]).optional(),
  name: z.string().optional(),
  permanent: z.boolean().optional(),
  topic: z.string(),
  status: z.string(),
  members: z.array(memberSchema),
  updated: z.union([z.string(), z.number()]),
})

export const eventKindSchema = z.enum([
  "agenda",
  "human",
  "turn",
  "vote",
  "poll",
  "question",
  "ledger",
  "replan",
  "note",
  "decision",
  "action",
  "confirm",
  "invite",
  "kick",
])

export const eventSchema = z
  .object({
    round: z.number().optional(),
    speaker: z.string().default("System"),
    role: z.string().default("system"),
    kind: eventKindSchema,
    text: z.string().default(""),
    file: z.string().optional(),
    ts: z.union([z.string(), z.number()]).optional(),
    status: z.string().optional(),
    exit_code: z.number().optional(),
    chars: z.number().optional(),
    duration_ms: z.number().optional(),
    question: z.string().optional(),
    choice: z.string().optional(),
    choices: z.array(z.string()).optional(),
    ledger: z.unknown().optional(),
  })
  .passthrough()

// LiveEvent (live.go) marks role/text/status `omitempty`, so a `speaking` frame
// carries none of them — only `spoke` sets status, and role is empty for a
// plain agent turn. Requiring them made the SPA drop EVERY live frame as
// invalid: the room painted its history and then never moved, so an addressed
// agent's reply was received and silently discarded.
//
// kind/round/speaker/ts have no omitempty and are always sent, so they stay
// required — that is what makes this a contract rather than a shrug.
export const liveEventSchema = z.object({
  kind: z.enum(["speaking", "line", "spoke"]),
  round: z.number(),
  speaker: z.string(),
  role: z.string().default(""),
  text: z.string().default(""),
  status: z.string().default(""),
  ts: z.union([z.string(), z.number()]).optional(),
  ctl_sock: z.string().optional(),
})

// The server's State marshals most fields with `omitempty`, so a room that has
// no chair, no agenda and no rounds yet simply does not send those keys — and a
// zero round is a MISSING key, not 0. Requiring them rejected every real
// response, which is why the room header sat on "Opening room…" forever while
// the mock fixtures (which spell every field) looked fine.
//
// Rule for this file: model what the SERVER sends, not what the mock does. Give
// every omitempty field a default, and accept both spellings where Go's type and
// the mock's disagree (room is an int; turn_timeout marshals as "20m").
export const stateSchema = z
  .object({
    schema: z.union([z.string(), z.number()]).optional(),
    id: z.string(),
    room: z.union([z.string(), z.number()]).optional(),
    name: z.string().optional(),
    permanent: z.boolean().optional(),
    role_holders: z.record(z.string(), z.string()).optional(),
    topic: z.string().default(""),
    agenda: z.array(z.string()).default([]),
    participants: z.array(memberSchema).default([]),
    secretary: z.string().default(""),
    chair: z.string().default(""),
    human: z.string().default(""),
    status: z.string().default("open"),
    cwd: z.string().default(""),
    out: z.string().default(""),
    turn_timeout: z.union([z.string(), z.number()]).optional(),
    created: z.union([z.string(), z.number()]).optional(),
    round: z.number().default(0),
    initiator: z.string().default(""),
    decision_mode: z.string().default(""),
  })
  .passthrough()

export const synthesisSchema = z
  .object({
    agenda: z.array(z.string()).optional(),
    decisions: z.array(z.string()).optional(),
    minutes: z.array(z.string()).optional(),
  })
  .passthrough()

export const roomDetailSchema = z.object({
  state: stateSchema,
  synthesis: synthesisSchema.nullable(),
})

export const jobRefSchema = z.object({
  id: z.string().optional(),
  job: z.string().optional(),
  ref: z.string().optional(),
})

export const errorSchema = z.object({ error: z.string() })

export const observeFrameSchema = z.discriminatedUnion("kind", [
  z.object({ kind: z.literal("info"), data: stateSchema }),
  z.object({ kind: z.literal("event"), data: eventSchema }),
  z.object({ kind: z.literal("history-end"), note: z.string() }),
  z.object({ kind: z.literal("live"), data: liveEventSchema }),
])

export type Member = z.infer<typeof memberSchema>
export type RoomSummary = z.infer<typeof roomSummarySchema>
export type MeetEvent = z.infer<typeof eventSchema>
export type LiveEvent = z.infer<typeof liveEventSchema>
export type State = z.infer<typeof stateSchema>
export type Synthesis = z.infer<typeof synthesisSchema>
export type RoomDetail = z.infer<typeof roomDetailSchema>
export type ObserveFrame = z.infer<typeof observeFrameSchema>

export function memberName(member: Member): string {
  return typeof member === "string" ? member : member.name
}

export function memberIsLive(member: Member): boolean {
  return typeof member === "string" ? false : member.live
}

export function memberRole(member: Member): string | undefined {
  return typeof member === "string" ? undefined : member.role
}
