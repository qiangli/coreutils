// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package webconsole

import (
	"net/http"
	"strings"

	"github.com/qiangli/coreutils/pkg/coopauth"
)

// openPaths are reachable before any identity is established: the liveness probe
// (a probe that needs an identity cannot run) and the login form itself.
func isOpenPath(p string) bool {
	switch p {
	case "/healthz", "/login", "/api/login", "/api/logout":
		return true
	}
	return strings.HasPrefix(p, "/login/")
}

// consoleGate decides who may reach the console, first match wins:
//
//  0. an open path                          -> admit
//  1. arrived through outpost's tunnel      -> RequireAuth (meet's rule, verbatim)
//  2. a valid console session cookie        -> admit as that OS user
//  3. a loopback peer, login not required   -> admit ungated (the dev path)
//  4. anything else                         -> login redirect, or 403
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
