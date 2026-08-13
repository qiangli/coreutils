import { useState } from "react"
import {
  Bot,
  ChevronRight,
  Hash,
  MessageSquareText,
  Plus,
  Radio,
  Sparkles,
  UserRound,
} from "lucide-react"

import { NewRoomDialog } from "@/components/new-room-dialog"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { ConnectionStatus } from "@/hooks/use-meet-room"
import {
  memberIsLive,
  memberName,
  memberRole,
  type RoomSummary,
  type AgentOption,
  type DMSummary,
  type State,
} from "@/lib/contracts"
import { cn } from "@/lib/utils"

interface RoomSidebarProps {
  creating: boolean
  agents: AgentOption[]
  onCreate: (topic: string, participants: string[]) => Promise<boolean>
  rooms: RoomSummary[]
  dms: DMSummary[]
  selectedRef: string
  selectedKind: "room" | "dm"
  state: State | null
  connection: ConnectionStatus
  usingMock: boolean
  onSelect: (ref: string) => void
  onSelectDM: (agent: string) => void
  onCreateDM: (agent: string) => Promise<boolean>
  className?: string
}

export function RoomSidebar({
  agents,
  creating,
  onCreate,
  rooms,
  dms,
  selectedRef,
  selectedKind,
  state,
  connection,
  usingMock,
  onSelect,
  onSelectDM,
  onCreateDM,
  className,
}: RoomSidebarProps) {
  const [newRoomOpen, setNewRoomOpen] = useState(false)
  const [dmMenuOpen, setDMMenuOpen] = useState(false)

  function openChannels() {
    if (rooms[0]) onSelect(rooms[0].id)
    else window.setTimeout(() => setNewRoomOpen(true), 0)
  }

  function openDirectMessages() {
    if (dms[0]) onSelectDM(dms[0].agent)
    else window.setTimeout(() => setDMMenuOpen(true), 0)
  }

  return (
    <aside
      className={cn(
        "flex h-full min-h-0 w-[280px] shrink-0 flex-col bg-sidebar text-sidebar-foreground",
        className,
      )}
    >
      <div className="flex h-[68px] items-center gap-3 px-5">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button aria-label="Open Bashy Relay menu" className="size-9 rounded-full bg-sidebar-primary text-sidebar-primary-foreground shadow-[0_8px_30px_rgb(13_148_136_/_0.24)] hover:bg-sidebar-primary/90" size="icon">
              <Sparkles className="size-[18px]" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuItem onSelect={openChannels}>
              <Hash className="size-3.5" /> Channels · Meet
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={openDirectMessages}>
              <Bot className="size-3.5" /> Direct messages · Chat
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <div className="min-w-0">
          <div className="font-display text-[17px] font-semibold tracking-tight">
            Bashy Relay
          </div>
          <div className="flex items-center gap-1.5 text-[11px] text-sidebar-foreground/55">
            <span
              className={cn(
                "size-1.5 rounded-full",
                connection === "open" ? "bg-emerald-400" : "bg-amber-400",
              )}
            />
            {usingMock ? "Demo workspace" : connection}
          </div>
        </div>
      </div>
      <Separator className="bg-sidebar-border" />

      <ScrollArea className="min-h-0 flex-1">
        <div className="px-3 py-5">
          <div className="mb-2 flex items-center justify-between px-2 text-[11px] font-semibold uppercase tracking-[0.14em] text-sidebar-foreground/45">
            <span>Channels</span>
            <div className="flex items-center gap-1">
              <MessageSquareText className="size-3.5" />
              <NewRoomDialog agents={agents} creating={creating} onCreate={onCreate} open={newRoomOpen} onOpenChange={setNewRoomOpen} />
            </div>
          </div>
          <div className="space-y-1">
            {rooms.map((room) => {
              const selected = selectedKind === "room" && room.id === selectedRef
              return (
                <button
                  className={cn(
                    "group flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2.5 text-left transition-colors",
                    selected
                      ? "bg-sidebar-accent text-sidebar-accent-foreground"
                      : "text-sidebar-foreground/70 hover:bg-sidebar-accent/55 hover:text-sidebar-foreground",
                  )}
                  key={room.id}
                  onClick={() => onSelect(room.id)}
                  type="button"
                >
                  <Hash
                    className={cn(
                      "size-4 shrink-0",
                      selected
                        ? "text-sidebar-primary"
                        : "text-sidebar-foreground/40",
                    )}
                  />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-[13px] font-medium">
                      {room.name || room.room || room.topic}
                    </span>
                    <span className="block truncate text-[11px] text-sidebar-foreground/45">
                      {room.permanent ? `Permanent · ${room.topic}` : room.topic}
                    </span>
                  </span>
                  {selected && (
                    <ChevronRight className="size-3.5 text-sidebar-foreground/45" />
                  )}
                </button>
              )
            })}
          </div>

          <div className="mb-2 mt-7 flex items-center justify-between px-2 text-[11px] font-semibold uppercase tracking-[0.14em] text-sidebar-foreground/45">
            <span>Direct messages</span>
            <DropdownMenu onOpenChange={setDMMenuOpen} open={dmMenuOpen}>
              <DropdownMenuTrigger asChild>
                <Button aria-label="Start a direct message" className="size-6 text-sidebar-foreground/55" size="icon" variant="ghost">
                  <Plus className="size-3.5" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="max-h-72 w-64 overflow-y-auto">
                {agents.map((agent) => (
                  <DropdownMenuItem key={agent.name} onSelect={() => void onCreateDM(agent.name)}>
                    <Bot className="size-3.5" /> {agent.nick || agent.name}
                    {agent.nick && <span className="ml-auto text-[10px] text-muted-foreground">{agent.name}</span>}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
          <div className="space-y-1">
            {dms.map((dm) => {
              const selected = selectedKind === "dm" && dm.agent === selectedRef
              return (
                <button className={cn("flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left", selected ? "bg-sidebar-accent text-sidebar-accent-foreground" : "text-sidebar-foreground/70 hover:bg-sidebar-accent/55")} key={dm.agent} onClick={() => onSelectDM(dm.agent)} type="button">
                  <Bot className="size-4 text-teal-300" />
                  <span className="truncate text-[13px] font-medium">{dm.agent}</span>
                </button>
              )
            })}
          </div>

          {selectedKind === "room" && <><div className="mb-2 mt-7 flex items-center justify-between px-2 text-[11px] font-semibold uppercase tracking-[0.14em] text-sidebar-foreground/45">
            <span>In this room</span>
            <Radio className="size-3.5" />
          </div>
          <div className="space-y-1">
            {state?.participants.map((member) => {
              const name = memberName(member)
              const role = memberRole(member)
              const human = role === "human" || name === state.human
              return (
                <div
                  className="flex items-center gap-2.5 rounded-lg px-2.5 py-2"
                  key={name}
                >
                  <div className="relative">
                    <Avatar className="size-7 border border-white/10">
                      <AvatarFallback
                        className={cn(
                          "text-[10px] font-bold",
                          human
                            ? "bg-violet-400/20 text-violet-200"
                            : "bg-teal-400/20 text-teal-100",
                        )}
                      >
                        {human ? (
                          <UserRound className="size-3.5" />
                        ) : (
                          <Bot className="size-3.5" />
                        )}
                      </AvatarFallback>
                    </Avatar>
                    {memberIsLive(member) && (
                      <span className="absolute -bottom-px -right-px size-2.5 rounded-full border-2 border-sidebar bg-emerald-400" />
                    )}
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-[12px] font-medium">{name}</div>
                    <div className="truncate text-[10px] capitalize text-sidebar-foreground/42">
                      {human ? "You" : role || "Agent"}
                    </div>
                  </div>
                  {name === state.initiator && (
                    <Badge
                      className="border-white/10 bg-white/5 px-1.5 py-0 text-[9px] text-sidebar-foreground/55"
                      variant="outline"
                    >
                      host
                    </Badge>
                  )}
                </div>
              )
            })}
          </div></>}
        </div>
      </ScrollArea>
      <div className="border-t border-sidebar-border px-5 py-3.5 text-[10px] leading-relaxed text-sidebar-foreground/38">
        Channels for groups. DMs for one-to-one work.
      </div>
    </aside>
  )
}
