// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package webconsole

import (
	"context"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qiangli/coreutils/pkg/atlas"
	"github.com/qiangli/coreutils/pkg/webterm"
)

// Panel is one tile on the start page.
type Panel struct {
	Name  string `json:"name"`  // stable id, also the mount segment
	Label string `json:"label"` // tile title
	Path  string `json:"path"`  // app-relative path, always trailing-slashed
	Mode  string `json:"mode"`  // atlas.WebSelf | WebInProcess | WebProxy
	Port  int    `json:"port,omitempty"`

	// Start is the argv, after `bashy`, that starts a proxied service. It is the
	// answer a stopped tile shows, so the reader never has to go find the
	// command that would fix it.
	Start []string `json:"start,omitempty"`

	// Source records where the declaration came from: builtin | atlas.
	Source string `json:"source"`

	// Available is false when the panel structurally cannot run here (no
	// pseudo-console on Windows, a handler not linked in). Distinct from
	// Status, which is about a service being up right now.
	Available bool   `json:"available"`
	Note      string `json:"note,omitempty"`

	handler http.Handler
}

// StartHint renders Start as the command a human would type.
func (p Panel) StartHint() string {
	if len(p.Start) == 0 {
		return ""
	}
	return "bashy " + strings.Join(p.Start, " ")
}

// builtinPanels are the console's OWN panels. They are not declared in the atlas
// because no verb owns them — there is no `bashy terminal` command whose surface
// this is. Declaring them somewhere else would be indirection with one consumer.
func builtinPanels() []Panel {
	term := Panel{
		Name: "terminal", Label: "Terminal", Path: "/term/",
		Mode: atlas.WebInProcess, Source: "builtin", Available: webterm.Supported(),
	}
	if !term.Available {
		term.Note = "this host has no pseudo-console"
	}
	return []Panel{term}
}

// Discover returns the tile list: the console's own panels, plus every verb that
// declares an atlas.WebSurface. Later sources shadow earlier ones by name, which
// is the assetring precedence order.
func Discover() []Panel {
	out := map[string]Panel{}
	order := []string{}
	add := func(p Panel) {
		if _, seen := out[p.Name]; !seen {
			order = append(order, p.Name)
		}
		out[p.Name] = p
	}

	for _, p := range builtinPanels() {
		add(p)
	}

	for _, verb := range atlas.WebSurfaceNames() {
		w := atlas.WebSurfaces()[verb]
		if w.Mode == atlas.WebSelf {
			// The console does not tile itself.
			continue
		}
		add(Panel{
			Name:      w.Mount,
			Label:     w.Label,
			Path:      "/" + w.Mount + "/",
			Mode:      w.Mode,
			Port:      w.Port,
			Start:     w.Start,
			Source:    "atlas",
			Available: true,
		})
	}

	sort.Strings(order)
	panels := make([]Panel, 0, len(order))
	for _, n := range order {
		panels = append(panels, out[n])
	}
	return panels
}

// Status is a panel plus its liveness at a moment in time.
type Status struct {
	Panel
	// Status is ready | stopped | unavailable.
	Status    string `json:"status"`
	StartHint string `json:"start_hint,omitempty"`
}

const (
	StatusReady       = "ready"
	StatusStopped     = "stopped"
	StatusUnavailable = "unavailable"
)

// probeTTL keeps a start-page repaint from re-dialling every service.
const probeTTL = 3 * time.Second

type probeCache struct {
	mu   sync.Mutex
	at   time.Time
	last []Status
}

// Probe reports each panel's liveness, dialling proxied services in parallel.
//
// A TCP connect, not an HTTP GET: several of these answer 401 or redirect at /,
// and "something is listening on the port it said it would listen on" is the
// honest signal — the same judgement pkg/meet's service status makes.
func (c *probeCache) Probe(ctx context.Context, panels []Panel) []Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.at) < probeTTL && c.last != nil {
		return c.last
	}

	out := make([]Status, len(panels))
	var wg sync.WaitGroup
	for i, p := range panels {
		out[i] = Status{Panel: p, StartHint: p.StartHint()}
		switch {
		case !p.Available:
			out[i].Status = StatusUnavailable
		case p.Mode != atlas.WebProxy:
			out[i].Status = StatusReady
		default:
			wg.Add(1)
			go func(i int, port int) {
				defer wg.Done()
				out[i].Status = StatusStopped
				if port == 0 {
					return
				}
				d := net.Dialer{Timeout: 300 * time.Millisecond}
				conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
				if err == nil {
					_ = conn.Close()
					out[i].Status = StatusReady
				}
			}(i, p.Port)
		}
	}
	wg.Wait()

	c.at, c.last = time.Now(), out
	return out
}
