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
  const memberCount = meet.state?.participants.length ?? 0

  return (
    <TooltipProvider>
      <div className="flex h-dvh min-h-[540px] overflow-hidden bg-background text-foreground">
        <RoomSidebar
          className="hidden lg:flex"
          connection={meet.connection}
          creating={meet.creating}
          onCreate={meet.createRoom}
          onSelect={meet.selectRoom}
          rooms={meet.rooms}
          selectedRef={meet.selectedRef}
          state={meet.state}
          usingMock={meet.usingMock}
        />

        <main className="flex min-w-0 flex-1 flex-col">
          <header className="flex h-[68px] shrink-0 items-center gap-3 border-b border-border/70 bg-background/88 px-3 backdrop-blur-xl sm:px-5">
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
                  className="w-full"
                  connection={meet.connection}
                  creating={meet.creating}
                  onCreate={meet.createRoom}
                  onSelect={meet.selectRoom}
                  rooms={meet.rooms}
                  selectedRef={meet.selectedRef}
                  state={meet.state}
                  usingMock={meet.usingMock}
                />
              </SheetContent>
            </Sheet>

            <div className="min-w-0 flex-1">
              <div className="flex min-w-0 items-center gap-2">
                <h1 className="font-display truncate text-[15px] font-semibold tracking-tight sm:text-[17px]">
                  {meet.state?.room || "Opening room…"}
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

            <Button
              aria-label={detailsOpen ? "Hide room details" : "Show room details"}
              className="hidden xl:inline-flex"
              onClick={() => setDetailsOpen((open) => !open)}
              size="icon-sm"
              variant="ghost"
            >
              {detailsOpen ? <PanelRightClose /> : <PanelRightOpen />}
            </Button>

            <Sheet>
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
                  className="w-full border-0"
                  detail={meet.detail}
                  isOrganizer={meet.isOrganizer}
                  onAction={meet.act}
                  state={meet.state}
                />
              </SheetContent>
            </Sheet>
          </header>

          <MessageList
            events={meet.events}
            human={meet.state?.human ?? ""}
            live={meet.live}
          />
          <Composer
            error={meet.error}
            isOrganizer={meet.isOrganizer}
            onAction={meet.act}
            onDismissQueued={meet.dismissQueued}
            onSend={meet.send}
            queued={meet.queued}
            sending={meet.sending}
            state={meet.state}
          />
        </main>

        {detailsOpen && (
          <RoomDetails
            className="hidden xl:flex"
            detail={meet.detail}
            isOrganizer={meet.isOrganizer}
            onAction={meet.act}
            state={meet.state}
          />
        )}
      </div>
    </TooltipProvider>
  )
}
