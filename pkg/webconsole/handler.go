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

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/qiangli/coreutils/pkg/atlas"
	"github.com/qiangli/coreutils/pkg/coopauth"
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

	// Terminal configures the shell behind the Terminal panel.
	Terminal webterm.Options

	// Panels overrides discovery. Nil means Discover().
	Panels []Panel
}

type server struct {
	opts         Options
	guard        *coopauth.Guard
	sessions     *websession.Store
	requireLogin bool
	panels       []Panel
	probes       probeCache
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
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}
	s := &server{
		opts:         opts,
		guard:        consoleGuard(),
		sessions:     opts.Sessions,
		requireLogin: opts.RequireLogin,
		panels:       opts.Panels,
	}
	if s.panels == nil {
		s.panels = Discover()
	}

	mux := http.NewServeMux()

	// Ungated: a liveness probe that needs an identity is a probe that cannot run.
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/apps", s.handleApps)
	mux.HandleFunc("GET /api/session", s.handleSession)

	// Deep links from docs/agent-interaction-surfaces-design.md keep working.
	mux.Handle("GET /shell", redirectTo("/term/"))
	mux.Handle("GET /meet", redirectTo("/relay/"))

	// The terminal is served by the launcher itself rather than mounted, so it
	// gets a full page of its own with the launcher's <base href> — every app
	// opens as a real browser page, none of them framed inside another.
	// One pattern, dispatched inside: net/http's mux rejects "GET /term/"
	// alongside "/term/ws" as conflicting, and the socket must accept the
	// upgrade on any method anyway.
	termSocket := webterm.SocketHandler(s.opts.Terminal)
	mux.HandleFunc("/term/", func(w http.ResponseWriter, r *http.Request) {
		if path.Base(r.URL.Path) == "ws" {
			termSocket.ServeHTTP(w, r)
			return
		}
		s.handleTermPage(w, r)
	})
	mux.Handle("GET /term", redirectTo("/term/"))

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
	return h, closer, nil
}

// panelHandler returns the handler for a panel plus any closer it needs, or nil
// when the console has nothing to serve for it.
func (s *server) panelHandler(p Panel) (http.Handler, func() error) {
	switch p.Name {
	case "terminal":
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
