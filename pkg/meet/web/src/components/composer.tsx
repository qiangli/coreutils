import { useMemo, useRef, useState } from "react"
import {
  AtSign,
  BookmarkCheck,
  Check,
  ChevronDown,
  CircleDot,
  ListChecks,
  ListTodo,
  LoaderCircle,
  MessageCircleQuestion,
  MessagesSquare,
  NotebookPen,
  Send,
  Sparkles,
  UsersRound,
  Vote,
  X,
} from "lucide-react"

import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Textarea } from "@/components/ui/textarea"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import {
  memberName,
  memberRole,
  type Member,
  type State,
} from "@/lib/contracts"
import { cn } from "@/lib/utils"

interface ComposerProps {
  state: State | null
  sending: boolean
  queued: string | null
  error: string | null
  isOrganizer: boolean
  onDismissQueued: () => void
  onSend: (text: string, agent?: string) => Promise<void>
  onAction: (action: string, label: string, body?: unknown) => Promise<boolean>
  onManageParticipants: () => void
}

export function Composer({
  state,
  sending,
  queued,
  error,
  isOrganizer,
  onDismissQueued,
  onSend,
  onAction,
  onManageParticipants,
}: ComposerProps) {
  const [text, setText] = useState("")
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const agents = useMemo(
    () => {
      const participants = (state?.participants ?? []).filter(
        (member) =>
          memberName(member) !== state?.human && memberRole(member) !== "human",
      )
      const aliases = Object.keys(state?.role_holders ?? {}).map((name) => ({
        name,
        role: "role",
        live: true,
      }))
      // A permanent role name is routable before it has a holder: addressing
      // it is the operation that lazily assigns the holder.  Keeping it out of
      // this list made @steward impossible to start from the browser.
      const lazyAliases =
        state?.permanent && state.name && !(state.name in (state.role_holders ?? {}))
          ? [{ name: state.name, role: "role", live: false }]
          : []
      return [...participants, ...aliases, ...lazyAliases]
    },
    [state],
  )
  const mentionQuery = text.startsWith("@")
    ? text.slice(1).split(/\s/, 1)[0].toLocaleLowerCase()
    : null
  const showMentions =
    mentionQuery !== null && !text.slice(1).includes(" ") && agents.length > 0
  const matchingAgents = agents.filter((agent) =>
    memberName(agent).toLocaleLowerCase().startsWith(mentionQuery ?? ""),
  )
  const roomOpen = state?.status === "open"

  async function submit() {
    const value = text.trim()
    if (!value || sending) return
    // Route every syntactically complete @name message to the address API.
    // The server is authoritative about whether the name is a seated agent or
    // a lazy permanent role.  An unknown/typoed name must produce a visible
    // error; silently storing it as ordinary human prose looks exactly like an
    // agent ignored the user.
    const addressed = value.match(/^@([^\s]+)\s+([\s\S]+)$/)
    setText("")
    if (addressed) {
      await onSend(addressed[2].trim(), addressed[1])
    } else {
      await onSend(value)
    }
  }

  function mention(agent: Member) {
    setText(`@${memberName(agent)} `)
    requestAnimationFrame(() => textareaRef.current?.focus())
  }

  return (
    <div className="shrink-0 border-t border-border/70 bg-background/88 px-3 pb-3 pt-2 backdrop-blur-xl sm:px-7 sm:pb-5">
      <div className="mx-auto max-w-[780px]">
        {queued && (
          <div className="mb-2 flex items-center gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-[11px] text-amber-900">
            <CircleDot className="size-3.5 shrink-0 animate-pulse" />
            <span className="flex-1">
              <strong className="font-semibold">Working.</strong> {queued}
            </span>
            <button
              aria-label="Dismiss queued notice"
              className="rounded p-0.5 hover:bg-amber-100"
              onClick={onDismissQueued}
              type="button"
            >
              <X className="size-3" />
            </button>
          </div>
        )}
        {error && (
          <div className="mb-2 rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-[11px] text-rose-800">
            {error}
          </div>
        )}
        <div className="relative rounded-2xl border border-border bg-card shadow-[0_10px_34px_rgb(15_23_42_/_0.08)] transition-shadow focus-within:border-primary/35 focus-within:shadow-[0_12px_40px_rgb(15_118_110_/_0.12)]">
          {showMentions && matchingAgents.length > 0 && (
            <div className="absolute bottom-[calc(100%+8px)] left-0 z-30 w-64 overflow-hidden rounded-xl border bg-popover p-1.5 shadow-xl">
              <div className="px-2 pb-1.5 pt-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                Talk to an agent
              </div>
              {matchingAgents.map((agent) => (
                <button
                  className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-xs hover:bg-accent"
                  key={memberName(agent)}
                  onClick={() => mention(agent)}
                  type="button"
                >
                  <Avatar className="size-6">
                    <AvatarFallback className="bg-teal-100 text-[9px] font-bold text-teal-800">
                      {initials(memberName(agent))}
                    </AvatarFallback>
                  </Avatar>
                  <span className="flex-1 font-medium">{memberName(agent)}</span>
                  <span className="text-[10px] capitalize text-muted-foreground">
                    {memberRole(agent) || "agent"}
                  </span>
                </button>
              ))}
            </div>
          )}
          <Textarea
            aria-label="Message the room"
            className="max-h-40 min-h-[56px] resize-none border-0 bg-transparent px-4 pb-2 pt-3.5 text-[14px] leading-6 shadow-none focus-visible:ring-0"
            disabled={!state || state.status === "closed"}
            onChange={(event) => setText(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter" && !event.shiftKey) {
                event.preventDefault()
                void submit()
              }
            }}
            placeholder={
              state?.status === "closed"
                ? "This room is closed"
                : "Message the room…"
            }
            ref={textareaRef}
            value={text}
          />
          <div className="flex items-center gap-1.5 px-2.5 pb-2.5">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  className="h-8 rounded-lg px-2.5 text-[11px] text-muted-foreground"
                  size="sm"
                  variant="ghost"
                  disabled={!roomOpen || sending}
                >
                  <Sparkles className="size-3.5 text-primary" />
                  Room actions
                  <ChevronDown className="size-3" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent
                align="start"
                className="w-72 rounded-xl p-1.5"
                side="top"
                sideOffset={8}
              >
                <DropdownMenuLabel className="px-2 py-2 text-[10px] uppercase tracking-wider text-muted-foreground">
                  Help the room move forward
                </DropdownMenuLabel>
                <ActionItem
                  description="Give every agent a turn"
                  icon={UsersRound}
                  label="Hear from everyone"
                  onSelect={() => onAction("round", "Round table")}
                />
                <ActionItem
                  description="Ask a quick multiple-choice question"
                  icon={Vote}
                  label="Take a quick pulse"
                  onSelect={() =>
                    onAction("poll", "Quick pulse", {
                      question:
                        text.trim() || "Are we ready to move forward?",
                      choices: ["Yes", "Needs more discussion"],
                    })
                  }
                />
                <ActionItem
                  description="Open one question to the whole room"
                  icon={MessageCircleQuestion}
                  label="Ask the room"
                  onSelect={() =>
                    onAction("ask", "Ask the room", {
                      question: text.trim() || "What are we missing?",
                    })
                  }
                />
                <ActionItem
                  description="Synthesize agreement and open questions"
                  icon={MessagesSquare}
                  label="Find common ground"
                  onSelect={() => onAction("converge", "Convergence")}
                />
                <DropdownMenuSeparator />
                <DropdownMenuLabel className="px-2 py-2 text-[10px] uppercase tracking-wider text-muted-foreground">
                  Record the current draft
                </DropdownMenuLabel>
                <ActionItem
                  description="Save the draft as an agreed outcome"
                  disabled={!text.trim()}
                  icon={BookmarkCheck}
                  label="Record decision"
                  onSelect={() =>
                    onAction("mark", "Record decision", {
                      kind: "decision",
                      text: text.trim(),
                    })
                  }
                />
                <ActionItem
                  description="Save the draft as follow-up work"
                  disabled={!text.trim()}
                  icon={ListTodo}
                  label="Record action item"
                  onSelect={() =>
                    onAction("mark", "Record action item", {
                      kind: "action",
                      text: text.trim(),
                    })
                  }
                />
                <ActionItem
                  description="Add the draft to the agenda"
                  disabled={!text.trim()}
                  icon={NotebookPen}
                  label="Add agenda item"
                  onSelect={() =>
                    onAction("mark", "Add agenda item", {
                      kind: "agenda",
                      text: text.trim(),
                    })
                  }
                />
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  className="gap-3 rounded-lg px-2 py-2"
                  onSelect={() => setText("@")}
                >
                  <AtSign className="size-4 text-violet-600" />
                  <div>
                    <div className="text-xs font-medium">Talk to one agent</div>
                    <div className="text-[10px] text-muted-foreground">
                      Choose someone from the room
                    </div>
                  </div>
                </DropdownMenuItem>
                <DropdownMenuItem
                  className="gap-3 rounded-lg px-2 py-2"
                  disabled={!isOrganizer}
                  onSelect={onManageParticipants}
                >
                  <ListChecks className="size-4 text-slate-500" />
                  <div>
                    <div className="text-xs font-medium">Manage participants</div>
                    <div className="text-[10px] text-muted-foreground">
                      {isOrganizer
                        ? "Invite or remove an agent"
                        : "Organizer only"}
                    </div>
                  </div>
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  aria-label="Mention an agent"
                  className="size-8 rounded-lg text-muted-foreground"
                  onClick={() => {
                    setText("@")
                    textareaRef.current?.focus()
                  }}
                  size="icon-sm"
                  variant="ghost"
                >
                  <AtSign className="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent side="top">Talk to one agent</TooltipContent>
            </Tooltip>
            <div className="ml-auto flex items-center gap-2">
              {text.trim() && (
                <span className="hidden text-[10px] text-muted-foreground sm:block">
                  Enter to send · Shift+Enter for a new line
                </span>
              )}
              <Button
                aria-label="Send message"
                className={cn(
                  "size-8 rounded-lg transition-all",
                  text.trim()
                    ? "bg-primary text-primary-foreground"
                    : "bg-secondary text-muted-foreground",
                )}
                disabled={!text.trim() || sending}
                onClick={() => void submit()}
                size="icon-sm"
              >
                {sending ? (
                  <LoaderCircle className="size-4 animate-spin" />
                ) : (
                  <Send className="size-3.5" />
                )}
              </Button>
            </div>
          </div>
        </div>
        <div className="mt-1.5 flex items-center justify-between px-1">
          <span className="text-[9px] text-muted-foreground/65">
            Agent replies render as sanitized markdown.
          </span>
          {state?.status === "open" && (
            <Badge
              className="h-4 border-0 bg-transparent px-1 text-[9px] font-normal text-emerald-700"
              variant="outline"
            >
              <Check className="mr-1 size-2.5" />
              Room open
            </Badge>
          )}
        </div>
      </div>
    </div>
  )
}

function ActionItem({
  icon: Icon,
  label,
  description,
  onSelect,
  disabled = false,
}: {
  icon: typeof UsersRound
  label: string
  description: string
  onSelect: () => void
  disabled?: boolean
}) {
  return (
    <DropdownMenuItem
      className="gap-3 rounded-lg px-2 py-2.5"
      disabled={disabled}
      onSelect={onSelect}
    >
      <div className="grid size-8 shrink-0 place-items-center rounded-lg bg-primary/8 text-primary">
        <Icon className="size-4" />
      </div>
      <div>
        <div className="text-xs font-medium">{label}</div>
        <div className="text-[10px] text-muted-foreground">{description}</div>
      </div>
    </DropdownMenuItem>
  )
}

function initials(name: string) {
  return name
    .split(/\s+/)
    .map((part) => part[0])
    .join("")
    .slice(0, 2)
    .toUpperCase()
}
