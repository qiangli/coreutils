import { useEffect, useRef } from "react"
import {
  Bot,
  CheckCircle2,
  CircleDotDashed,
  ClipboardCheck,
  ListTodo,
  Megaphone,
  UserRound,
  UserRoundPlus,
  Vote,
} from "lucide-react"
import ReactMarkdown from "react-markdown"
import rehypeSanitize from "rehype-sanitize"

import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import type { LiveEvent, MeetEvent } from "@/lib/contracts"
import { cn } from "@/lib/utils"

interface MessageListProps {
  events: MeetEvent[]
  live: LiveEvent | null
  human: string
}

const systemKinds = new Set([
  "agenda",
  "ledger",
  "replan",
  "note",
  "decision",
  "action",
  "confirm",
  "invite",
  "kick",
  "vote",
  "poll",
  "question",
])

export function MessageList({ events, live, human }: MessageListProps) {
  const endRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth", block: "end" })
  }, [events.length, live?.text])

  return (
    <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 py-6 sm:px-8">
      <div className="mx-auto w-full max-w-[780px]">
        <div className="mb-8 rounded-2xl border border-border/70 bg-card/60 px-5 py-4 shadow-sm shadow-slate-900/[0.025]">
          <div className="mb-1 flex items-center gap-2 text-[11px] font-bold uppercase tracking-[0.14em] text-primary">
            <Megaphone className="size-3.5" />
            Room opened
          </div>
          <p className="text-sm leading-relaxed text-muted-foreground">
            This is a shared conversation. Messages, decisions, and actions stay
            together as the room works.
          </p>
        </div>

        <div className="space-y-1">
          {events.map((event, index) =>
            systemKinds.has(event.kind) ? (
              <SystemEvent event={event} key={eventKey(event, index)} />
            ) : (
              <Message
                event={event}
                human={human}
                key={eventKey(event, index)}
              />
            ),
          )}
          {live && <LiveMessage live={live} />}
        </div>
        <div ref={endRef} />
      </div>
    </div>
  )
}

function Message({ event, human }: { event: MeetEvent; human: string }) {
  const isHuman = event.kind === "human" || event.speaker === human
  return (
    <article className="group flex gap-3 rounded-xl px-2 py-4 transition-colors hover:bg-white/55 sm:gap-4 sm:px-3">
      <Avatar className="mt-0.5 size-9 shrink-0 border border-border/70 shadow-sm">
        <AvatarFallback
          className={cn(
            "font-semibold",
            isHuman
              ? "bg-violet-100 text-violet-700"
              : "bg-teal-100 text-teal-800",
          )}
        >
          {isHuman ? (
            <UserRound className="size-4" />
          ) : (
            <Bot className="size-4" />
          )}
        </AvatarFallback>
      </Avatar>
      <div className="min-w-0 flex-1">
        <div className="mb-1 flex flex-wrap items-baseline gap-x-2 gap-y-1">
          <span className="text-[13px] font-semibold text-foreground">
            {event.speaker}
          </span>
          {!isHuman && (
            <Badge
              className="h-[18px] rounded-full border-teal-200/70 bg-teal-50 px-1.5 text-[9px] font-semibold uppercase tracking-wide text-teal-700"
              variant="outline"
            >
              {event.role || "agent"}
            </Badge>
          )}
          <time className="text-[10px] text-muted-foreground/70">
            {formatTime(event.ts)}
          </time>
        </div>
        {isHuman ? (
          <p className="whitespace-pre-wrap text-[14px] leading-6 text-foreground/90">
            {event.text}
          </p>
        ) : (
          <div className="markdown text-[14px] leading-6 text-foreground/90">
            <ReactMarkdown rehypePlugins={[rehypeSanitize]}>
              {event.text}
            </ReactMarkdown>
          </div>
        )}
      </div>
    </article>
  )
}

function LiveMessage({ live }: { live: LiveEvent }) {
  return (
    <article className="flex gap-3 rounded-xl bg-teal-50/45 px-2 py-4 sm:gap-4 sm:px-3">
      <Avatar className="mt-0.5 size-9 shrink-0 border border-teal-200 shadow-sm">
        <AvatarFallback className="bg-teal-100 text-teal-800">
          <Bot className="size-4" />
        </AvatarFallback>
      </Avatar>
      <div className="min-w-0 flex-1">
        <div className="mb-1 flex items-center gap-2">
          <span className="text-[13px] font-semibold">{live.speaker}</span>
          <Badge
            className="h-[18px] animate-pulse rounded-full border-teal-200 bg-teal-50 px-1.5 text-[9px] uppercase tracking-wide text-teal-700"
            variant="outline"
          >
            speaking
          </Badge>
        </div>
        {live.text ? (
          <p className="text-[14px] leading-6 text-foreground/85">{live.text}</p>
        ) : (
          <div
            aria-label={`${live.speaker} is typing`}
            className="flex h-6 items-center gap-1"
          >
            {[0, 1, 2].map((item) => (
              <span
                className="typing-dot size-1.5 rounded-full bg-teal-600/55"
                key={item}
                style={{ animationDelay: `${item * 140}ms` }}
              />
            ))}
          </div>
        )}
      </div>
    </article>
  )
}

function SystemEvent({ event }: { event: MeetEvent }) {
  const Icon = iconFor(event.kind)
  const highlighted = event.kind === "decision" || event.kind === "action"
  return (
    <div className="my-2 flex items-start gap-3 py-2 pl-[3.35rem] pr-3">
      <div
        className={cn(
          "mt-0.5 grid size-6 shrink-0 place-items-center rounded-full",
          highlighted
            ? "bg-amber-100 text-amber-700"
            : "bg-slate-100 text-slate-500",
        )}
      >
        <Icon className="size-3" />
      </div>
      <div className="min-w-0">
        <p
          className={cn(
            "text-[12px] leading-5",
            highlighted
              ? "font-medium text-slate-700"
              : "text-muted-foreground",
          )}
        >
          {event.text || event.question}
        </p>
        <span className="text-[9px] uppercase tracking-wider text-muted-foreground/55">
          {event.kind} · {formatTime(event.ts)}
        </span>
      </div>
    </div>
  )
}

function iconFor(kind: MeetEvent["kind"]) {
  if (kind === "decision" || kind === "confirm") return CheckCircle2
  if (kind === "action") return ClipboardCheck
  if (kind === "agenda" || kind === "replan") return ListTodo
  if (kind === "invite" || kind === "kick") return UserRoundPlus
  if (kind === "poll" || kind === "vote") return Vote
  return CircleDotDashed
}

function formatTime(value?: string | number) {
  if (value === undefined) return ""
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ""
  return new Intl.DateTimeFormat(undefined, {
    hour: "numeric",
    minute: "2-digit",
  }).format(date)
}

function eventKey(event: MeetEvent, index: number) {
  return `${event.kind}-${event.speaker}-${String(event.ts)}-${index}`
}
