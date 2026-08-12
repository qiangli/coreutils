import { useState, type FormEvent } from "react"
import { Plus } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"

interface NewRoomDialogProps {
  creating: boolean
  onCreate: (topic: string, participants: string[]) => Promise<boolean>
}

/**
 * Opening a room is the one thing the browser could not do: every room had to
 * be started from the CLI, which left the web surface able to join a
 * conversation but never to begin one.
 *
 * There is deliberately no room-type chooser. The model has ONE room type — a
 * room with a single agent is the 1:1 assistant, the same room with three is a
 * meeting, and the roster stays mutable afterwards — so asking up front would
 * invent a distinction the server does not have.
 */
export function NewRoomDialog({ creating, onCreate }: NewRoomDialogProps) {
  const [open, setOpen] = useState(false)
  const [topic, setTopic] = useState("")
  const [agents, setAgents] = useState("")

  const participants = agents
    .split(/[,\s]+/)
    .map((name) => name.trim())
    .filter(Boolean)
  const ready = topic.trim().length > 0 && participants.length > 0

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (!ready || creating) return
    if (await onCreate(topic.trim(), participants)) {
      setTopic("")
      setAgents("")
      setOpen(false)
    }
    // On failure the dialog stays open with the input intact — the error is
    // rendered in the room view, and discarding what was typed would make a
    // transient failure cost the user their message.
  }

  return (
    <Dialog onOpenChange={setOpen} open={open}>
      <DialogTrigger asChild>
        <Button
          aria-label="New room"
          className="size-7 text-sidebar-foreground/55 hover:text-sidebar-foreground"
          size="icon-sm"
          variant="ghost"
        >
          <Plus className="size-4" />
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[440px]">
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>New room</DialogTitle>
            <DialogDescription>
              One agent is a 1:1 conversation; add more for a meeting. You can
              invite others once it is running.
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-4 py-5">
            <div className="grid gap-1.5">
              <label className="text-[13px] font-medium" htmlFor="room-topic">
                Topic
              </label>
              <Input
                autoFocus
                id="room-topic"
                onChange={(event) => setTopic(event.target.value)}
                placeholder="What is this room about?"
                value={topic}
              />
            </div>
            <div className="grid gap-1.5">
              <label className="text-[13px] font-medium" htmlFor="room-agents">
                Agents
              </label>
              <Input
                id="room-agents"
                onChange={(event) => setAgents(event.target.value)}
                placeholder="codex, claude"
                value={agents}
              />
              <p className="text-[11px] text-muted-foreground">
                Separate names with commas or spaces.
              </p>
            </div>
          </div>

          <DialogFooter>
            <Button
              onClick={() => setOpen(false)}
              type="button"
              variant="ghost"
            >
              Cancel
            </Button>
            <Button disabled={!ready || creating} type="submit">
              {creating ? "Opening…" : "Open room"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
