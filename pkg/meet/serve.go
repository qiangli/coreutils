package meet

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

// The WebSocket surface — the room, reachable off the local filesystem.
//
// `observe` (see observe.go) lets a human on the SAME HOST watch a meeting by tailing its two
// files (transcript.jsonl = the record, live.jsonl = the view). That is one participation
// surface: local, file-based. It is not the only one a room should have — a browser, a phone,
// or an operator on another machine must be able to attach too, and none of them can tail a
// file on this host.
//
// This is the second surface, and the design rule is that it must be a surface over the SAME
// source of truth, not a parallel one. So `meet serve` streams exactly the events `observe`
// reads — the same transcript backlog, then the same tailed record+view — over a WebSocket.
// The JSONL event schema IS the wire protocol; the socket is just a second transport for it.
// A frame a WS client receives is identical to a line `observe` would print. Add a transport,
// never a second truth.
//
// Read-only, like observe: attaching casts no vote and writes nothing, so any number of
// clients (local TUI, browser, mobile) can attach to one room at once without perturbing it.
// (Bidirectional participation — tell/say from a socket client — is the next step; the socket
// is already duplex, and a read pump is here to receive it, but this prototype only streams.)
//
// This is S2 of docs/plan-observable-steward-sessions.md: the network surface that unlocks
// web and mobile. It deliberately reuses resolveMeeting / loadState / storeDir / lineTail /
// readEvents / readLive — the exact machinery observe uses — so the two surfaces cannot drift.

func newServeCmd() *cobra.Command {
	var (
		port int
		bind string
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "serve the room over WebSocket, so a browser / phone / remote client can attach",
		Long: "Expose meetings over a WebSocket so participation surfaces beyond the local TUI\n" +
			"(a browser, a mobile app, an operator on another machine) can attach and watch\n" +
			"live. It streams the SAME transcript + live events that `bashy meet observe`\n" +
			"tails — one source of truth, a second transport.\n\n" +
			"  ws://<host>:<port>/observe?room=<ROOM|id>   read-only live stream\n" +
			"  GET /healthz                                liveness\n\n" +
			"Read-only for now: any number of clients may attach without perturbing the room.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMeetServe(cmd.Context(), cmd.OutOrStdout(), bind, port)
		},
	}
	cmd.Flags().IntVar(&port, "port", 8637, "listen port")
	cmd.Flags().StringVar(&bind, "bind", "127.0.0.1", "bind address")
	return cmd
}

func runMeetServe(ctx context.Context, out interface{ Write([]byte) (int, error) }, bind string, port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/observe", handleObserveWS)

	addr := fmt.Sprintf("%s:%d", bind, port)
	srv := &http.Server{Addr: addr, Handler: mux}
	fmt.Fprintf(out, "meet serve → ws://%s/observe?room=<ROOM>  (the room over WebSocket; a second surface, same events)\n", addr)

	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return srv.Shutdown(sctx)
	case err := <-errc:
		return err
	}
}

// wsFrame is the envelope every socket frame carries. `kind` distinguishes the RECORD
// (a whole-turn transcript Event) from the VIEW (a line-granular LiveEvent) — the same two
// channels observe keeps distinct, kept distinct here so a client parser is never handed two
// schemas with no tag to tell them apart.
type wsFrame struct {
	Kind string      `json:"kind"` // "event" (record) | "live" (view) | "history-end" | "info"
	Data interface{} `json:"data,omitempty"`
	Note string      `json:"note,omitempty"`
}

var wsUpgrader = websocket.Upgrader{
	// Same-truth read-only stream; a browser on any origin may observe. Auth/origin policy
	// is a portal concern (the room rides the portal for cross-machine reach), not the
	// socket's — the socket serves whoever the surface in front of it let through.
	CheckOrigin: func(r *http.Request) bool { return true },
}

func handleObserveWS(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("room")
	if ref == "" {
		http.Error(w, "missing ?room=<ROOM|id>", http.StatusBadRequest)
		return
	}
	id, err := resolveMeeting(ref)
	if err != nil {
		http.Error(w, "no such room: "+err.Error(), http.StatusNotFound)
		return
	}
	if _, err := loadState(id); err != nil {
		http.Error(w, "load room: "+err.Error(), http.StatusInternalServerError)
		return
	}
	dir, err := storeDir(id)
	if err != nil {
		http.Error(w, "room dir: "+err.Error(), http.StatusInternalServerError)
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade already wrote the error
	}
	defer conn.Close()

	// A read pump: gorilla only processes control frames (close/ping) while a reader runs,
	// and it is where client→room messages (tell/say) will arrive next. For now it just
	// drains, so a client disconnect is noticed promptly.
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	rec := &lineTail{path: filepath.Join(dir, "transcript.jsonl")}
	live := &lineTail{path: filepath.Join(dir, "live.jsonl")}
	// The view is followed forward only; its history is already in the record as whole turns.
	live.skipToEnd()

	writeFrame := func(f wsFrame) error {
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteJSON(f)
	}

	// 1. Replay the record — complete and authoritative — so the client sees everything that
	//    happened before it attached, exactly as observe does.
	backlog, err := readEvents(rec)
	if err == nil {
		for _, e := range backlog {
			if writeFrame(wsFrame{Kind: "event", Data: e}) != nil {
				return
			}
		}
	}
	if writeFrame(wsFrame{Kind: "history-end", Note: fmt.Sprintf("%d event(s) of history", len(backlog))}) != nil {
		return
	}

	// 2. Tail forward: new view lines and new record events, the same loop observe runs.
	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-closed:
			return
		case <-ping.C:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if conn.WriteMessage(websocket.PingMessage, nil) != nil {
				return
			}
		case <-time.After(observePoll):
		}

		if lines, err := readLive(live); err == nil {
			for _, l := range lines {
				if writeFrame(wsFrame{Kind: "live", Data: l}) != nil {
					return
				}
			}
		}
		if events, err := readEvents(rec); err == nil {
			for _, e := range events {
				if writeFrame(wsFrame{Kind: "event", Data: e}) != nil {
					return
				}
			}
		}
	}
}
