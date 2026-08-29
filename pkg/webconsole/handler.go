// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package webconsole

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/qiangli/coreutils/pkg/atlas"
	"github.com/qiangli/coreutils/pkg/coopauth"
	"github.com/qiangli/coreutils/pkg/hostauth"
	"github.com/qiangli/coreutils/pkg/meet"
	"github.com/qiangli/coreutils/pkg/websession"
	"github.com/qiangli/coreutils/pkg/webterm"
)

// otelServiceName is the launcher's own span/service identity. It is a real hop
// on the trace plane (browser -> cloudbox -> outpost -> apps -> panel), so it
// names itself rather than borrowing a panel's name.
const otelServiceName = "bashy-apps"

// sessionCookie is the console's session cookie name.
const sessionCookie = "bashy_console"

// DefaultPort is the console's loopback port. It sits beside meet's 8637 and is
// declared to the atlas so `commands --view web` and the console agree.
const DefaultPort = 8639

// Options configures a console.
type Options struct {
	// Ctx is the SERVER's lifetime, not a request's — background work started by
	// a panel outlives the request that asked for it.
	Ctx context.Context

	// Scope is the filesystem root the Files panel is confined to. Empty means
	// the working directory.
	Scope string
	// AllowWrite enables the Files panel's write operations. Default read-only.
	AllowWrite bool

	// RequireLogin disables the ungated-loopback row of the gate. Set
	// automatically when binding a non-loopback address.
	RequireLogin bool
	// Sessions validates console cookies; nil disables cookie auth.
	Sessions *websession.Store
	// Auth verifies an OS password. nil means hostauth.DefaultAuthenticator().
	Auth hostauth.Authenticator

	// Terminal configures the shell behind the Terminal panel.
	Terminal webterm.Options

	// Panels overrides discovery. Nil means Discover().
	Panels []Panel

	// Disable names panels to leave out entirely — not greyed out, not listed:
	// absent from the tile list AND unrouted.
	//
	// This exists because outpost publishes the console under ONE app name, and
	// HostShare grants are per app name. Without it, sharing the console would
	// mean sharing the Terminal — which spawns a real bashy as this OS user —
	// and that is strictly more authority than sharing outpost's own `shell`
	// app was. A consolidation must not widen a grant by accident.
	Disable []string
}

// disabled reports whether a panel was turned off for this console.
func (o Options) disabled(name string) bool {
	for _, d := range o.Disable {
		if strings.EqualFold(strings.TrimSpace(d), name) {
			return true
		}
	}
	return false
}

type server struct {
	opts         Options
	guard        *coopauth.Guard
	sessions     *websession.Store
	auth         hostauth.Authenticator
	limiter      *websession.Limiter
	requireLogin bool
	panels       []Panel
	probes       probeCache
	boards       boardCache
}

// consoleGuard mirrors meet's: an SSO secret, when configured, makes the vouch
// HMAC-verified rather than merely present.
func consoleGuard() *coopauth.Guard {
	return &coopauth.Guard{Policy: coopauth.Policy{}}
}

// Handler returns the console as an http.Handler, plus a closer for whatever it
// had to start. It is the embedding seam: `apps serve` is a thin caller,
// and outpost or a desktop shell is the same shape.
func Handler(opts Options) (http.Handler, func() error, error) {
	_, h, closer, err := newHandler(opts)
	return h, closer, err
}

// newHandler is Handler plus the server it built. Tests use it to reach state
// that has no HTTP surface — the board cache, which must be seedable so a test
// never runs the real collector and its subprocess fan-out.
func newHandler(opts Options) (*server, http.Handler, func() error, error) {
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}
	s := &server{
		opts:         opts,
		guard:        consoleGuard(),
		sessions:     opts.Sessions,
		auth:         opts.Auth,
		requireLogin: opts.RequireLogin,
		panels:       opts.Panels,
	}
	if s.requireLogin {
		if s.sessions == nil {
			// Ephemeral key: sessions do not survive a restart, which is the
			// right default for a process started by hand. runServe persists one.
			s.sessions = websession.NewStore(12*time.Hour, nil)
		}
		// Burst 5, then one attempt per 12s. PAM/dscl is already slow on the
		// success path; this bounds a LAN brute-forcer on the failure path.
		s.limiter = websession.NewLimiter(5, 12*time.Second)
	}
	if s.panels == nil {
		s.panels = Discover()
	}
	if len(opts.Disable) > 0 {
		kept := s.panels[:0]
		for _, p := range s.panels {
			if !opts.disabled(p.Name) {
				kept = append(kept, p)
			}
		}
		s.panels = kept
	}
	// on reports whether a panel survived, so a disabled one is never ROUTED
	// either. Dropping it from the tile list alone would leave the surface
	// reachable to anyone who typed the path.
	on := map[string]bool{}
	for _, p := range s.panels {
		on[p.Name] = true
	}

	mux := http.NewServeMux()

	// Ungated: a liveness probe that needs an identity is a probe that cannot run.
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/apps", s.handleApps)
	mux.HandleFunc("GET /api/session", s.handleSession)
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)

	// Deep links from docs/agent-interaction-surfaces-design.md keep working,
	// each only while its target panel is enabled.
	if on["relay"] {
		mux.Handle("GET /meet", redirectTo("/relay/"))
	}

	// The terminal is served by the launcher itself rather than mounted, so it
	// gets a full page of its own with the launcher's <base href> — every app
	// opens as a real browser page, none of them framed inside another.
	// One pattern, dispatched inside: net/http's mux rejects "GET /term/"
	// alongside "/term/ws" as conflicting, and the socket must accept the
	// upgrade on any method anyway.
	if on["terminal"] {
		termSocket := webterm.SocketHandler(s.opts.Terminal)
		mux.HandleFunc("/term/", func(w http.ResponseWriter, r *http.Request) {
			if path.Base(r.URL.Path) == "ws" {
				termSocket.ServeHTTP(w, r)
				return
			}
			s.handleTermPage(w, r)
		})
		mux.Handle("GET /term", redirectTo("/term/"))
		mux.Handle("GET /shell", redirectTo("/term/"))
	}

	// The board, served the same way and for the same reason: its own page at
	// the launcher's root, so it inherits app.css and the header chrome. The
	// atlas declares the TILE (Discover picks it up); panelHandler returns nil
	// for it, exactly as it does for the terminal, so the panel loop below does
	// not also mount it under coopauth.Mount and take the <base href> with it.
	if on["mb"] {
		mux.HandleFunc("GET /mb/", s.handleMBPage)
		mux.Handle("GET /mb", redirectTo("/mb/"))
		mux.HandleFunc("GET /api/mb", s.handleMBList)
		mux.HandleFunc("POST /api/mb", s.handleMBSend)
		mux.HandleFunc("GET /api/mb/{seq}/viewers", s.handleMBViewers)
	}

	// The steward board, same shape again: the tile is an atlas declaration,
	// the page is served at the launcher's root, and panelHandler claims
	// nothing. Read-only by construction — `board` is the one work verb the
	// atlas marks CapReadOnly, and the panel must not erode that.
	if on["board"] {
		mux.HandleFunc("GET /board/", s.handleBoardPage)
		mux.Handle("GET /board", redirectTo("/board/"))
		mux.HandleFunc("GET /api/board", s.handleBoardOverview)
		mux.HandleFunc("GET /api/board/panel/{id}", s.handleBoardPanel)
	}

	closers := []func() error{}
	for _, p := range s.panels {
		if !p.Available {
			continue
		}
		h, closer := s.panelHandler(p)
		if closer != nil {
			closers = append(closers, closer)
		}
		if h == nil {
			continue
		}
		// Mount from Path, not Name: they are allowed to differ (the terminal is
		// "terminal" at /term/), and deriving the route from the name instead
		// silently serves a 404 at the very link the tile renders.
		mount := strings.TrimSuffix(p.Path, "/")
		if mount == "" {
			continue
		}
		mux.Handle(mount+"/", coopauth.Mount(mount, h))
		mux.Handle(mount, coopauth.Mount(mount, h))
	}

	// Everything else is the start page, including its client-side routes.
	mux.HandleFunc("/", s.handleSPA)

	// otelhttp costs nothing without an exporter: the global provider is a no-op,
	// so this is span-shaped bookkeeping that never leaves the process.
	h := otelhttp.NewHandler(
		coopauth.StripSpoofedTrustHeaders(s.consoleGate(mux)),
		otelServiceName,
	)
	closer := func() error {
		var err error
		for _, c := range closers {
			if cerr := c(); cerr != nil && err == nil {
				err = cerr
			}
		}
		return err
	}
	return s, h, closer, nil
}

// panelHandler returns the handler for a panel plus any closer it needs, or nil
// when the console has nothing to serve for it.
func (s *server) panelHandler(p Panel) (http.Handler, func() error) {
	switch p.Name {
	case "terminal", "mb", "board":
		// Served above, at the launcher's own root, not mounted here.
		return nil, nil
	case "files":
		h, closer, err := filesPanel(s.opts.Scope, s.opts.AllowWrite)
		if err != nil {
			// An unmountable panel must not take the whole launcher down: the
			// tile says why and everything else still works.
			slog.Error("files panel", "err", err)
			return nil, nil
		}
		return h, closer
	case "relay":
		// The room is MOUNTED, not proxied to a separate `relay serve`: one
		// engine, one lease, one transcript. The pass-through gate is required —
		// see meet.MountOptions.
		return meet.HandlerWith(s.opts.Ctx, meet.MountOptions{Gate: passthrough}), nil
	}
	if p.Mode == atlas.WebProxy && p.Port > 0 {
		return s.proxyTo(p), nil
	}
	return nil, nil
}

// proxyTo reverse-proxies a separately-supervised service, and says so plainly
// when it is not running — a stopped tile that renders a connection-refused
// stack trace teaches nothing.
func (s *server) proxyTo(p Panel) http.Handler {
	target := &url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(p.Port)}
	rp := httputil.NewSingleHostReverseProxy(target)
	// FlushInterval -1 streams: these panels carry SSE and WebSockets, and any
	// buffering turns a live view into a hang.
	rp.FlushInterval = -1
	rp.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		msg := p.Label + " is not running on 127.0.0.1:" + strconv.Itoa(p.Port) + ".\n"
		if hint := p.StartHint(); hint != "" {
			msg += "Start it with:  " + hint + "\n"
		}
		_, _ = w.Write([]byte(msg))
	}
	return rp
}

func redirectTo(path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, coopauth.PrefixPath(r, path), http.StatusFound)
	})
}
