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
  room: z.string().optional(),
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

export const liveEventSchema = z.object({
  kind: z.enum(["speaking", "line", "spoke"]),
  round: z.number(),
  speaker: z.string(),
  role: z.string(),
  text: z.string().default(""),
  status: z.string(),
  ts: z.union([z.string(), z.number()]).optional(),
  ctl_sock: z.string().optional(),
})

export const stateSchema = z
  .object({
    schema: z.union([z.string(), z.number()]),
    id: z.string(),
    room: z.string(),
    topic: z.string(),
    agenda: z.array(z.string()),
    participants: z.array(memberSchema),
    secretary: z.string(),
    chair: z.string(),
    human: z.string(),
    status: z.string(),
    cwd: z.string(),
    out: z.string(),
    turn_timeout: z.number(),
    created: z.union([z.string(), z.number()]),
    round: z.number(),
    initiator: z.string(),
    decision_mode: z.string(),
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
