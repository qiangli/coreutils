// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package webconsole

import (
	"net/http"
	"strings"

	"github.com/qiangli/coreutils/pkg/coopauth"
)

// openPaths are reachable before any identity is established: the liveness probe
// (a probe that needs an identity cannot run), the login form itself, and the
// /meta self-description.
//
// /meta is open for the same reason /healthz is: it exists to be polled by
// something that cannot present an identity — outpost dials the console over
// loopback, where the gate's ungated-loopback row is switched OFF as soon as the
// console is LAN-bound. It is defensible ONLY because the payload is a
// projection carrying display metadata and nothing else (no port, no start argv,
// no liveness). A field that would be sensitive belongs on /api/apps, behind the
// gate — not here.
func isOpenPath(p string) bool {
	switch p {
	case "/healthz", "/login", "/api/login", "/api/logout", "/meta":
		return true
	}
	if strings.HasPrefix(p, "/login/") {
		return true
	}
	if rest := strings.TrimPrefix(p, "/meta/"); rest != p {
		return !strings.ContainsRune(rest, '/') && validMount(rest) == nil
	}
	return false
}

// firstSegment returns the leading path segment ("/relay/api/x" -> "relay").
func firstSegment(p string) string {
	p = strings.TrimPrefix(p, "/")
	if i := strings.IndexByte(p, '/'); i >= 0 {
		p = p[:i]
	}
	return p
}

// panelTier reports the declared auth tier for the panel owning this request
// path, and whether a panel owns it at all.
//
// Resolution is on the FIRST SEGMENT ONLY, and that is the security property:
// a public app opens its own mount and nothing else. /api/*, /login*, the
// launcher at / and every other panel keep the console-wide ladder, so a public
// tile can never become a hole into the Terminal.
func (s *server) panelTier(path string) (string, bool) {
	seg := firstSegment(path)
	if seg == "" {
		return "", false
	}
	tier, ok := s.panelAuth[seg]
	return tier, ok
}

// consoleGate decides who may reach the console, first match wins:
//
//  0. an open path                          -> admit
//  1. a panel declaring public/custom       -> admit, that mount's subtree only
//  2. arrived through outpost's tunnel      -> RequireAuth (meet's rule, verbatim)
//  3. a valid console session cookie        -> admit as that OS user
//  4. a loopback peer, login not required   -> admit ungated (the dev path)
//  5. anything else                         -> login redirect, or 403
//
// Row 1 is per-panel and never a global downgrade: it is reached only when the
// request path's FIRST SEGMENT names a panel that declared `public` (no identity
// wanted) or `custom` (the app runs its own login, so the console must not
// intercept the redirect). Everything else still walks the original ladder.
//
// Row 3 is why `bashy apps` on 127.0.0.1 never asks for a password: it is
// the same rule pkg/meet already applies, and the machine owner authenticating
// to their own machine to see their own files buys nothing. Row 4 is what makes
// a LAN bind safe to offer at all.
func (s *server) consoleGate(next http.Handler) http.Handler {
	guard := s.guard
	vouched := guard.RequireAuth(next)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isOpenPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if tier, ok := s.panelTier(r.URL.Path); ok && (tier == AuthPublic || tier == AuthCustom) {
			next.ServeHTTP(w, r)
			return
		}
		if coopauth.ArrivedViaCloud(r) {
			vouched.ServeHTTP(w, r)
			return
		}
		if s.sessions != nil {
			if c, err := r.Cookie(sessionCookie); err == nil {
				if _, ok := s.sessions.Validate(c.Value); ok {
					next.ServeHTTP(w, r)
					return
				}
			}
		}
		if !s.requireLogin && coopauth.IsLoopbackAddr(r.RemoteAddr) {
			next.ServeHTTP(w, r)
			return
		}
		s.denied(w, r)
	})
}

// denied sends a browser to the login page and anything else a bare 403 — a
// fetch() that follows a redirect to an HTML form reports a confusing parse
// error rather than the authentication failure that actually happened.
func (s *server) denied(w http.ResponseWriter, r *http.Request) {
	if s.requireLogin && strings.Contains(r.Header.Get("Accept"), "text/html") {
		http.Redirect(w, r, coopauth.PrefixPath(r, "/login"), http.StatusFound)
		return
	}
	http.Error(w, "authentication required", http.StatusForbidden)
}

// passthrough is the gate a mounted panel gets: the console has already decided
// who this caller is, and the panel's own gate would re-read the
// X-Forwarded-Prefix the mount just stamped and 403 a loopback owner. See
// coopauth.Mount and meet.MountOptions.
func passthrough(h http.Handler) http.Handler { return h }
