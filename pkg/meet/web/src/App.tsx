import { useState } from "react"
import {
  ChevronRight,
  Menu,
  PanelRightClose,
  PanelRightOpen,
  Users,
} from "lucide-react"

import { Composer } from "@/components/composer"
import { MessageList } from "@/components/message-list"
import { RoomDetails } from "@/components/room-details"
import { RoomSidebar } from "@/components/room-sidebar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetContent,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet"
import { TooltipProvider } from "@/components/ui/tooltip"
import { useMeetRoom } from "@/hooks/use-meet-room"

export function App() {
  const meet = useMeetRoom()
  const [detailsOpen, setDetailsOpen] = useState(true)
  const [mobileDetailsOpen, setMobileDetailsOpen] = useState(false)
  const memberCount = meet.state?.participants.length ?? 0

  return (
    <TooltipProvider>
      <div className="flex h-dvh min-h-[540px] overflow-hidden bg-background text-foreground">
        <RoomSidebar
          agents={meet.agents}
          className="hidden lg:flex"
          connection={meet.connection}
          creating={meet.creating}
          dms={meet.dms}
          onCreateDM={meet.createDM}
          onCreate={meet.createRoom}
          onSelect={meet.selectRoom}
          onSelectDM={meet.selectDM}
          onModeChange={meet.selectMode}
          rooms={meet.rooms}
          selectedRef={meet.selectedRef}
          selectedKind={meet.selectedKind}
          viewKind={meet.viewKind}
          state={meet.state}
          usingMock={meet.usingMock}
        />

        <main className="flex min-w-0 flex-1 flex-col">
          <header className="console-bar shrink-0">
            {/* The brand block every console app carries: the panel's own mark
                in a rounded tile, then bashy + the panel name. board, messages
                and terminal state it in markup; relay is a mounted SPA, so it
                draws the same thing itself.

                The wordmark appears from sm: up. On a phone this header also
                holds the rooms button and the conversation title, and the title
                is the thing someone needs there — the mark alone still says
                which app this is, and still returns to its own root. */}
            <a className="console-brand" href="./" title="bashy relay">
              <svg className="console-logo" viewBox="0 0 40 40" aria-hidden="true">
                <rect width="40" height="40" rx="10" fill="var(--logo-bg, currentColor)" opacity="0.08" />
                <g
                  transform="translate(8 8)"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.9"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                >
                  <path d="M4 7.5a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v4a2 2 0 0 1-2 2H8.5L4.5 16.5zM18.5 10.5H19a2 2 0 0 1 2 2v5l-2.8-2H13" />
                </g>
              </svg>
              {/* Hidden on a phone: this header also carries the rooms button
                  and the conversation title, and the title is what matters
                  there. The mark alone still identifies the app. */}
              <span className="console-wordmark hidden sm:flex">
                bashy<b>relay</b>
              </span>
            </a>

            <Sheet>
              <SheetTrigger asChild>
                <Button
                  aria-label="Open rooms and members"
                  className="lg:hidden"
                  size="icon-sm"
                  variant="ghost"
                >
                  <Menu />
                </Button>
              </SheetTrigger>
              <SheetContent
                className="w-[300px] border-0 p-0"
                side="left"
                showCloseButton={false}
              >
                <SheetTitle className="sr-only">Rooms and members</SheetTitle>
                <RoomSidebar
                  agents={meet.agents}
                  className="w-full"
                  connection={meet.connection}
                  creating={meet.creating}
                  dms={meet.dms}
                  onCreateDM={meet.createDM}
                  onCreate={meet.createRoom}
                  onSelect={meet.selectRoom}
                  onSelectDM={meet.selectDM}
                  onModeChange={meet.selectMode}
                  rooms={meet.rooms}
                  selectedRef={meet.selectedRef}
                  selectedKind={meet.selectedKind}
                  viewKind={meet.viewKind}
                  state={meet.state}
                  usingMock={meet.usingMock}
                />
              </SheetContent>
            </Sheet>

            <div className="min-w-0 flex-1">
              <div className="flex min-w-0 items-center gap-2">
                <h1 className="font-display truncate text-[15px] font-semibold tracking-tight sm:text-[17px]">
                  {meet.state?.name || meet.state?.room || "Opening conversation…"}
                </h1>
                {meet.usingMock && (
                  <Badge
                    className="hidden h-[18px] rounded-full border-violet-200 bg-violet-50 px-1.5 text-[9px] uppercase tracking-wide text-violet-700 sm:inline-flex"
                    variant="outline"
                  >
                    demo
                  </Badge>
                )}
              </div>
              <div className="flex items-center gap-1 text-[10px] text-muted-foreground sm:text-[11px]">
                <span className="max-w-[52vw] truncate">
                  {meet.state?.topic || "Connecting to the conversation"}
                </span>
                <ChevronRight className="size-3" />
                <Users className="size-3" />
                <span>{memberCount}</span>
              </div>
            </div>

            {meet.selectedKind === "room" && <Button
              aria-label={detailsOpen ? "Hide room details" : "Show room details"}
              className="hidden xl:inline-flex"
              onClick={() => setDetailsOpen((open) => !open)}
              size="icon-sm"
              variant="ghost"
            >
              {detailsOpen ? <PanelRightClose /> : <PanelRightOpen />}
            </Button>}

            {meet.selectedKind === "room" && <Sheet onOpenChange={setMobileDetailsOpen} open={mobileDetailsOpen}>
              <SheetTrigger asChild>
                <Button
                  aria-label="Open room details"
                  className="xl:hidden"
                  size="icon-sm"
                  variant="outline"
                >
                  <PanelRightOpen />
                </Button>
              </SheetTrigger>
              <SheetContent
                className="w-[340px] max-w-[92vw] p-0"
                side="right"
              >
                <SheetTitle className="sr-only">Room details</SheetTitle>
                <RoomDetails
                  agents={meet.agents}
                  className="w-full border-0"
                  detail={meet.detail}
                  isOrganizer={meet.isOrganizer}
                  onAction={meet.act}
                  sending={meet.sending}
                  state={meet.state}
                />
              </SheetContent>
            </Sheet>}

            {/* The console's all-apps mark — four rounded squares, matching
                #all-apps-btn that injectChrome gives every embedded app. relay
                is MOUNTED, so it never receives that button and draws the same
                mark itself, exactly as the Files app does. */}
            <a className="console-iconbtn" href="../" title="All apps" aria-label="Back to all apps">
                <svg
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.9"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  aria-hidden="true"
                >
                  <rect x="3" y="3" width="7" height="7" rx="1.5" />
                  <rect x="14" y="3" width="7" height="7" rx="1.5" />
                  <rect x="3" y="14" width="7" height="7" rx="1.5" />
                  <rect x="14" y="14" width="7" height="7" rx="1.5" />
                </svg>
            </a>
          </header>

          <MessageList
            events={meet.events}
            human={meet.state?.human ?? ""}
            kind={meet.selectedKind}
            live={meet.live}
          />
          <Composer
            error={meet.error}
            isOrganizer={meet.isOrganizer}
            onAction={meet.act}
            onDismissQueued={meet.dismissQueued}
            onManageParticipants={() => {
              if (window.matchMedia("(min-width: 1280px)").matches) {
                setDetailsOpen(true)
              } else {
                setMobileDetailsOpen(true)
              }
            }}
            onSend={meet.send}
            queued={meet.queued}
            sending={meet.sending}
            state={meet.state}
            kind={meet.selectedKind}
          />
        </main>

        {detailsOpen && meet.selectedKind === "room" && (
          <RoomDetails
            agents={meet.agents}
            className="hidden xl:flex"
            detail={meet.detail}
            isOrganizer={meet.isOrganizer}
            onAction={meet.act}
            sending={meet.sending}
            state={meet.state}
          />
        )}
      </div>
    </TooltipProvider>
  )
}
