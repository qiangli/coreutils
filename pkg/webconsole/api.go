// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package webconsole

import (
	"encoding/json"
	"net/http"

	"github.com/qiangli/coreutils/pkg/coopauth"
)

// appsSchemaVersion identifies the /api/apps payload. Bump it only for a
// breaking shape change; the SPA reads it and can then say so.
const appsSchemaVersion = "bashy-console-apps-v1"

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": otelServiceName})
}

// handleApps is the tile list: what exists, whether it is up right now, and the
// command that would start it if it is not. It is the same data
// `bashy commands --view web` prints — one source, two renderers.
func (s *server) handleApps(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": appsSchemaVersion,
		"base":           coopauth.BaseHref(r),
		"apps":           s.probes.Probe(r.Context(), s.panels),
	})
}

// handleSession tells the SPA who it is talking as, so it can render an identity
// without guessing from the presence of a cookie.
func (s *server) handleSession(w http.ResponseWriter, r *http.Request) {
	user := ""
	via := "loopback"
	if coopauth.ArrivedViaCloud(r) {
		via = "cloud"
		if id, ok := coopauth.IdentityOf(r); ok {
			user = id.Username
		}
	} else if s.sessions != nil {
		if c, err := r.Cookie(sessionCookie); err == nil {
			if u, ok := s.sessions.Validate(c.Value); ok {
				user, via = u, "session"
			}
		}
	}
	if user == "" {
		user = coopauth.SystemUser()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":          user,
		"via":           via,
		"require_login": s.requireLogin,
	})
}
