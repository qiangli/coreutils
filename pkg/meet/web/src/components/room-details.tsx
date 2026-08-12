import { useState } from "react"

import {
  Check,
  ClipboardList,
  Crown,
  FileText,
  Gavel,
  LockKeyhole,
  Timer,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import type { RoomDetail, State } from "@/lib/contracts"
import { cn } from "@/lib/utils"

interface RoomDetailsProps {
  state: State | null
  detail: RoomDetail | null
  isOrganizer: boolean
  onAction: (action: string, label: string, body?: unknown) => void
  className?: string
}

export function RoomDetails({
  state,
  detail,
  isOrganizer,
  onAction,
  className,
}: RoomDetailsProps) {
  const [invitee, setInvitee] = useState("")

  return (
    <aside
      className={cn(
        "flex h-full min-h-0 w-[320px] shrink-0 flex-col border-l border-border/70 bg-card/70",
        className,
      )}
    >
      <div className="flex h-[68px] items-center px-5">
        <div>
          <div className="font-display text-[14px] font-semibold">
            Room details
          </div>
          <div className="text-[11px] text-muted-foreground">
            Shared memory and outcomes
          </div>
        </div>
      </div>
      <Separator />
      <ScrollArea className="min-h-0 flex-1">
        <div className="space-y-7 p-5">
          <section>
            <SectionTitle icon={ClipboardList}>Agenda</SectionTitle>
            <ol className="mt-3 space-y-3">
              {state?.agenda.map((item, index) => (
                <li className="flex gap-2.5 text-[12px] leading-5" key={item}>
                  <span className="grid size-5 shrink-0 place-items-center rounded-full bg-secondary text-[9px] font-bold text-muted-foreground">
                    {index + 1}
                  </span>
                  <span className="text-foreground/75">{item}</span>
                </li>
              ))}
            </ol>
          </section>

          <section>
            <SectionTitle icon={Check}>Decisions</SectionTitle>
            <div className="mt-3 space-y-2">
              {detail?.synthesis?.decisions?.map((item) => (
                <div
                  className="rounded-xl border border-amber-200/70 bg-amber-50/65 p-3 text-[12px] leading-5 text-amber-950/75"
                  key={item}
                >
                  {item}
                </div>
              )) || (
                <EmptyCopy>Decisions will collect here as the room agrees.</EmptyCopy>
              )}
            </div>
          </section>

          <section>
            <SectionTitle icon={FileText}>Minutes</SectionTitle>
            <div className="mt-3 space-y-3 border-l border-border pl-3">
              {detail?.synthesis?.minutes?.map((item) => (
                <p
                  className="text-[11px] leading-[1.65] text-muted-foreground"
                  key={item}
                >
                  {item}
                </p>
              )) || <EmptyCopy>No minutes yet.</EmptyCopy>}
            </div>
          </section>

          <section>
            <SectionTitle icon={Gavel}>Room setup</SectionTitle>
            <div className="mt-3 grid grid-cols-2 gap-2">
              <Meta label="Chair" value={state?.chair || "Not assigned"} />
              <Meta
                label="Secretary"
                value={state?.secretary || "Not assigned"}
              />
              <Meta
                icon={Timer}
                label="Turn limit"
                value={`${state?.turn_timeout ?? 0}s`}
              />
              <Meta
                label="Decisions"
                value={state?.decision_mode || "Standard"}
              />
            </div>
          </section>

          <section className="rounded-xl border border-dashed border-border p-3.5">
            <div className="mb-2 flex items-center gap-2 text-[11px] font-semibold">
              {isOrganizer ? (
                <Crown className="size-3.5 text-amber-600" />
              ) : (
                <LockKeyhole className="size-3.5 text-muted-foreground" />
              )}
              Organizer controls
            </div>
            <p className="mb-3 text-[10px] leading-relaxed text-muted-foreground">
              {isOrganizer
                ? "You can manage membership and close this room."
                : `${state?.initiator || "The organizer"} manages membership and room closure.`}
            </p>
            {isOrganizer && (
              <form
                className="mb-2 flex gap-2"
                onSubmit={(event) => {
                  event.preventDefault()
                  const name = invitee.trim()
                  if (!name) return
                  onAction("invite", "Invite", { agent: name })
                  setInvitee("")
                }}
              >
                <Input
                  aria-label="Agent to invite"
                  className="h-8 text-[12px]"
                  onChange={(event) => setInvitee(event.target.value)}
                  placeholder="Invite an agent…"
                  value={invitee}
                />
                <Button
                  className="h-8 shrink-0 text-[12px]"
                  disabled={!invitee.trim()}
                  size="sm"
                  type="submit"
                  variant="outline"
                >
                  Invite
                </Button>
              </form>
            )}
            <div className="flex gap-2">
              <OrganizerButton
                disabled={!isOrganizer}
                label="Close room"
                onClick={() => onAction("close", "Close room")}
              />
            </div>
          </section>
        </div>
      </ScrollArea>
    </aside>
  )
}

function SectionTitle({
  icon: Icon,
  children,
}: {
  icon: typeof ClipboardList
  children: React.ReactNode
}) {
  return (
    <h2 className="flex items-center gap-2 text-[10px] font-bold uppercase tracking-[0.14em] text-muted-foreground">
      <Icon className="size-3.5" />
      {children}
    </h2>
  )
}

function Meta({
  label,
  value,
  icon: Icon,
}: {
  label: string
  value: string
  icon?: typeof Timer
}) {
  return (
    <div className="rounded-lg bg-secondary/60 p-2.5">
      <div className="mb-1 flex items-center gap-1 text-[9px] uppercase tracking-wider text-muted-foreground">
        {Icon && <Icon className="size-2.5" />}
        {label}
      </div>
      <div className="truncate text-[11px] font-medium capitalize">{value}</div>
    </div>
  )
}

function OrganizerButton({
  disabled,
  label,
  onClick,
}: {
  disabled: boolean
  label: string
  onClick: () => void
}) {
  const button = (
    <span className="inline-flex">
      <Button
        className="h-7 text-[10px]"
        disabled={disabled}
        onClick={onClick}
        size="sm"
        variant="outline"
      >
        {label}
      </Button>
    </span>
  )
  if (!disabled) return button
  return (
    <Tooltip>
      <TooltipTrigger asChild>{button}</TooltipTrigger>
      <TooltipContent>Only the organizer can use this control.</TooltipContent>
    </Tooltip>
  )
}

function EmptyCopy({ children }: { children: React.ReactNode }) {
  return (
    <p className="text-[11px] italic leading-5 text-muted-foreground">
      {children}
    </p>
  )
}
