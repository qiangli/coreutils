import { useMemo, useRef, useState } from "react"
import {
  AtSign,
  Check,
  ChevronDown,
  CircleDot,
  Hash,
  LoaderCircle,
  Send,
  Undo2,
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
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Textarea } from "@/components/ui/textarea"
import {
  ALL_SEATS,
  memberName,
  memberRole,
  type Member,
  type State,
} from "@/lib/contracts"
import type { PendingSend, RecallOutcome } from "@/hooks/use-meet-room"
import { isGenericTitle, seatOf } from "@/lib/seats"
import { cn } from "@/lib/utils"

interface ComposerProps {
  initialDraft?: string
  state: State | null
  sending: boolean
  queued: string | null
  error: string | null
  /** Who an unaddressed message goes to; "" is the whole room. */
  recipient: string
  onRecipientChange: (name: string) => void
  onDismissQueued: () => void
  onSend: (text: string, agent?: string) => Promise<void>
  kind?: "room" | "dm"
  /** The message between the click and the delivery, or null. */
  pending: PendingSend | null
  /** Seconds left of the hold, for the countdown on the button. */
  heldFor: number
  /** A recall is in flight; the server has not answered yet. */
  recalling: boolean
  /** Stop the pending message. Resolves with what that achieved. */
  onCancel: () => Promise<RecallOutcome>
}

export function Composer({
  initialDraft = "",
  state,
  sending,
  queued,
  error,
  recipient,
  onRecipientChange,
  onDismissQueued,
  onSend,
  kind = "room",
  pending,
  heldFor,
  recalling,
  onCancel,
}: ComposerProps) {
  // A cross-app shortcut may supply an editable starting point. It is a draft,
  // never an instruction: opening a link must not spend tokens or start work.
  const [text, setText] = useState(initialDraft)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  // The retraction confirm. It is local to the composer because it is a UI
  // question, not a state of the message: the message is already delivered
  // whether or not this is open.
  const [confirming, setConfirming] = useState(false)
  const holding = pending?.phase === "holding"
  // A delivered message is recallable only while the box is EMPTY. Writing the
  // next one is itself the decision not to recall the last, and the send button
  // is the same control — so as soon as there is something to send, it goes
  // back to being a send button. Without this the recall offer sat on top of
  // the one control the sender needs next.
  const inFlight = Boolean(pending) && !holding && !text.trim()
  // Whether the control is currently the pending-message control at all.
  const pendingControl = holding || inFlight

  // cancel is one call for both branches — the HOOK decides which one applies,
  // and for a dispatched message the SERVER does. The composer only reports the
  // answer, and it puts a cancelled message back in the box: a send that was
  // stopped in time should leave the sender exactly where they were, not make
  // them retype what they wrote.
  async function cancel() {
    const held = pending?.text ?? ""
    const outcome = await onCancel()
    setConfirming(false)
    if (outcome === "canceled" && held && !text.trim()) {
      setText(held)
      requestAnimationFrame(() => textareaRef.current?.focus())
    }
  }
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
      const all = [...participants, ...aliases, ...lazyAliases]
      // The owner is not necessarily SEATED: a meeting's facilitator runs the
      // floor without being on the roster, and a sprint room's seat holder may
      // not have spoken yet. It is still the default recipient, so leaving it
      // out made the one name the composer preselects the one name the reader
      // could not choose again after switching away.
      const owner = state?.owner ?? ""
      if (owner && !all.some((m) => memberName(m).toLocaleLowerCase() === owner.toLocaleLowerCase())) {
        all.unshift({ name: owner, role: state?.owner_title || "owner", live: false })
      }
      return all
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
  // The SERVER resolves the owner: the room stores a late-bound seat label
  // ("conductor:99") and only the host holds the table that says who sits in
  // it today. ownerTitle is the domain's own word — "project manager" for a
  // sprint's room, "facilitator" for an ordinary meeting.
  const ownerName = state?.owner ?? ""
  const ownerTitle = state?.owner_title || "owner"
  const isOwner = (name: string) =>
    Boolean(ownerName) && name.toLocaleLowerCase() === ownerName.toLocaleLowerCase()
  // What one name is CALLED in this room — the same resolution the transcript
  // and the roster use, so a reader who learns who the project manager is in
  // one list recognises them in the other two. A generic title ("agent",
  // "participant") is dropped rather than printed: it is the word that was on
  // every row before, and it distinguished nobody.
  const titleOf = (name: string, member?: Member) => {
    const title = seatOf(name, state, member ? memberRole(member) : undefined).title
    return isGenericTitle(title) ? "" : title
  }
  // The DM's counterpart, named the way the composer would address it. A 1:1
  // has one recipient and no way to change it, so this is a LABEL where a room
  // has a control — the recipient is stated rather than chosen.
  const dmAgent = state?.name || state?.owner || ""
  // Two labels, deliberately. The BUTTON shows the name only — "codex-gpt5.6-sol
  // · project manager" is wider than the control and truncates to "codex-gpt5.6-
  // sol ·…", which spends the space on an ellipsis. The accessible name carries
  // the title, so a screen reader still hears who this is, and the title itself
  // is on the badge in the list where there is room for it.
  const broadcast = recipient === ALL_SEATS
  const recipientName = broadcast || !recipient ? "Everyone" : recipient
  const recipientTitle = recipient && !broadcast ? titleOf(recipient) : ""
  const recipientLabel = recipientTitle
    ? `${recipient} · ${recipientTitle}`
    : recipientName

  async function submit() {
    const value = text.trim()
    if (!value || sending) return
    // Route every syntactically complete @name message to the address API.
    // The server is authoritative about whether the name is a seated agent or
    // a lazy permanent role.  An unknown/typoed name must produce a visible
    // error; silently storing it as ordinary human prose looks exactly like an
    // agent ignored the user.
    const addressed = kind === "room" ? value.match(/^@([^\s]+)\s+([\s\S]+)$/) : null
    setText("")
    if (addressed) {
      await onSend(addressed[2].trim(), addressed[1])
    } else if (kind === "room" && recipient) {
      // The selected recipient, which defaults to the room's owner. A typed
      // "@name" above overrides it for that one message without changing the
      // selection — the same relationship a To: field has with a reply.
      await onSend(value, recipient)
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
        {/* THE CONFIRM, and only on the irreversible branch. The message is out;
            the sender cannot have it back, and the choice is whether to say so
            in the room. That is a new permanent record everyone sees — worth one
            question — where cancelling a held message is worth none. */}
        {confirming && inFlight && (
          <div className="mb-2 flex flex-wrap items-center gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-[11px] text-amber-900">
            <Undo2 className="size-3.5 shrink-0" />
            <span className="flex-1">
              <strong className="font-semibold">Already delivered.</strong> It
              cannot be unsent
              {pending?.agent ? ` — ${pending.agent} may be reading it` : ""}.
              Post a retraction beside it?
            </span>
            <Button
              className="h-6 rounded-md px-2 text-[11px]"
              disabled={recalling}
              onClick={() => void cancel()}
              size="sm"
              variant="outline"
            >
              {recalling ? "Retracting…" : "Post retraction"}
            </Button>
            <Button
              className="h-6 rounded-md px-2 text-[11px]"
              onClick={() => setConfirming(false)}
              size="sm"
              variant="ghost"
            >
              Leave it
            </Button>
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
                  <span className="min-w-0 flex-1 truncate font-medium">
                    {memberName(agent)}
                  </span>
                  {/* Same owner mark as the recipient list and the roster. A
                      reader should not have to learn who is accountable in one
                      place and then fail to recognise them in another. */}
                  <span className="shrink-0 text-[10px] capitalize text-muted-foreground">
                    {titleOf(memberName(agent), agent) || "agent"}
                  </span>
                </button>
              ))}
            </div>
          )}
          <Textarea
            aria-label={kind === "dm" ? `Message ${dmAgent || "agent"}` : "Message the room"}
            className="max-h-40 min-h-[56px] resize-none border-0 bg-transparent px-4 pb-2 pt-3.5 text-[14px] leading-6 shadow-none focus-visible:ring-0"
            disabled={!state || state.status === "closed"}
            onChange={(event) => {
              setText(event.target.value)
              // Typing withdraws the retraction question along with the recall
              // offer it belongs to — otherwise it would come back the moment
              // the box was emptied again, asking about a message the sender
              // has already moved on from.
              if (confirming) setConfirming(false)
            }}
            onKeyDown={(event) => {
              if (event.key === "Enter" && !event.shiftKey) {
                event.preventDefault()
                void submit()
              }
            }}
            placeholder={
              state?.status === "closed"
                ? "This room is closed"
                : kind === "dm"
                  ? `Message @${dmAgent || "agent"}…`
                  : "Message the room…"
            }
            ref={textareaRef}
            value={text}
          />
          <div className="flex items-center gap-1.5 px-2.5 pb-2.5">
            {/* WHO THIS GOES TO, in the slot a room keeps its recipient control
                in — a 1:1 states it, a room chooses it. The name is spelled
                "@agent" because that is how the same message is addressed one
                panel over, and because a chat whose recipient is only in the
                window title makes the reader hold it in their head. It is
                deliberately NOT a control: a direct message has exactly one
                recipient, and offering a menu of one is offering a decision
                that does not exist. */}
            {kind === "dm" && dmAgent && (
              <span
                className="flex h-8 max-w-[13rem] items-center gap-1.5 rounded-lg border border-input bg-background px-2.5 text-[11px] font-medium text-foreground"
                title={`This conversation goes to ${dmAgent}`}
              >
                <span className="truncate">@{dmAgent}</span>
              </span>
            )}
            {/* WHO THIS GOES TO. The room's owner is preselected — in a
                sprint's room that is its project manager, and unaddressed mail
                there already lands on that seat server-side, so a composer
                that defaulted to nobody was disagreeing with its own room.
                "Everyone" is a real choice, not the absence of one. */}
            {kind === "room" && <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  aria-label={`Recipient: ${recipientLabel}`}
                  className="h-8 max-w-[13rem] gap-1.5 rounded-lg px-2.5 text-[11px]"
                  disabled={!roomOpen || sending}
                  size="sm"
                  variant="outline"
                >
                  {broadcast || !recipient ? (
                    <Hash className="size-3.5 text-muted-foreground" />
                  ) : (
                    <AtSign className="size-3.5 text-primary" />
                  )}
                  <span className="truncate">{recipientName}</span>
                  <ChevronDown className="size-3 shrink-0 opacity-60" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent
                align="start"
                className="max-h-80 w-72 overflow-y-auto rounded-xl p-1.5"
                side="top"
                sideOffset={8}
              >
                <DropdownMenuLabel className="px-2 py-2 text-[10px] uppercase tracking-wider text-muted-foreground">
                  Send to
                </DropdownMenuLabel>
                {/* A REAL broadcast, not the absence of an addressee. Posting
                    with no addressee reaches the transcript and no inbox, which
                    from outside is indistinguishable from every agent in the
                    room ignoring you. */}
                <DropdownMenuItem
                  className="gap-3 rounded-lg px-2 py-2"
                  onSelect={() => onRecipientChange(ALL_SEATS)}
                >
                  <Hash className="size-4 text-muted-foreground" />
                  <div className="min-w-0 flex-1">
                    <div className="text-xs font-medium">Everyone</div>
                    <div className="text-[10px] text-muted-foreground">
                      Delivered to every participant&apos;s inbox
                    </div>
                  </div>
                  {(broadcast || !recipient) && <Check className="size-3.5 text-primary" />}
                </DropdownMenuItem>
                {agents.map((agent) => {
                  const name = memberName(agent)
                  return (
                    <DropdownMenuItem
                      className="gap-3 rounded-lg px-2 py-2"
                      key={name}
                      onSelect={() => onRecipientChange(name)}
                    >
                      <Avatar className="size-6">
                        <AvatarFallback className="bg-teal-100 text-[9px] font-bold text-teal-800">
                          {initials(name)}
                        </AvatarFallback>
                      </Avatar>
                      <div className="min-w-0 flex-1">
                        <div className="truncate text-xs font-medium">{name}</div>
                        <div className="truncate text-[10px] capitalize text-muted-foreground">
                          {titleOf(name, agent) || "agent"}
                        </div>
                      </div>
                      {isOwner(name) && (
                        <Badge
                          className="h-4 shrink-0 border-primary/30 px-1.5 text-[9px] font-medium capitalize text-primary"
                          variant="outline"
                        >
                          {ownerTitle}
                        </Badge>
                      )}
                      {recipient === name && <Check className="size-3.5 shrink-0 text-primary" />}
                    </DropdownMenuItem>
                  )
                })}
              </DropdownMenuContent>
            </DropdownMenu>}
            {/* The recipient control above IS the addressing surface: it names
                who an unaddressed message goes to and changes it in one click.
                A second control that only typed "@" into the box was a slower
                way to reach the same decision, so it is gone. Typing "@name"
                still works and still overrides the selection for one message —
                the mention list above the box completes it. */}
            <div className="ml-auto flex items-center gap-2">
              {!pendingControl && text.trim() && (
                <span className="hidden text-[10px] text-muted-foreground sm:block">
                  Enter to send · Shift+Enter for a new line
                </span>
              )}
              {holding && (
                <span className="text-[10px] font-medium text-muted-foreground">
                  Sending in {heldFor}s · click to cancel
                </span>
              )}
              {inFlight && (
                <span className="text-[10px] font-medium text-muted-foreground">
                  {confirming ? "Already delivered" : "Sent · click to recall"}
                </span>
              )}
              {/* THE ONE CONTROL. While a message is pending the send button IS
                  the cancel button, so there is nothing extra to find at the
                  moment a sender wants it — and it says which of the two things
                  a click will do, because those are not the same act.

                  No confirmation while it is still held: that click is instant,
                  reverses nothing, and hands the text back for editing. Asking
                  "are you sure" there would add a step to the recovery path and
                  risk the hold expiring mid-dialog — sending the very message
                  the dialog was about. The confirm is on the OTHER branch, where
                  the message is already delivered and the only remaining move
                  posts a permanent record everyone can see. */}
              <Button
                aria-label={
                  holding
                    ? `Cancel send (${heldFor}s)`
                    : inFlight
                      ? "Recall message"
                      : "Send message"
                }
                className={cn(
                  "relative size-8 rounded-lg transition-all",
                  pendingControl
                    ? "bg-secondary text-foreground hover:bg-amber-100 hover:text-amber-900"
                    : text.trim()
                      ? "bg-primary text-primary-foreground"
                      : "bg-secondary text-muted-foreground",
                )}
                disabled={pendingControl ? recalling : !text.trim() || sending}
                onClick={() => {
                  if (holding) {
                    void cancel()
                    return
                  }
                  if (inFlight) {
                    setConfirming(true)
                    return
                  }
                  void submit()
                }}
                size="icon-sm"
              >
                {pendingControl ? (
                  <>
                    <LoaderCircle className="size-4 animate-spin opacity-60" />
                    <X className="absolute size-3" />
                  </>
                ) : sending ? (
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
            {holding
              ? "Nothing has been sent yet. Cancelling now leaves no trace."
              : inFlight
                ? "Delivered. It can be retracted, which posts a record — not erased."
                : kind === "dm"
                  ? `Goes to ${dmAgent ? `@${dmAgent}` : "this agent"}. Enter to send.`
                  : broadcast || !recipient
                    ? "Goes to everyone in the room. Start with @name to send one message elsewhere."
                    : `Goes to @${recipient}. Start with @name to send one message elsewhere.`}
          </span>
          {state?.status === "open" && (
            <Badge
              className="h-4 border-0 bg-transparent px-1 text-[9px] font-normal text-emerald-700"
              variant="outline"
            >
              <Check className="mr-1 size-2.5" />
              {kind === "dm" ? "Chat open" : "Room open"}
            </Badge>
          )}
        </div>
      </div>
    </div>
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
