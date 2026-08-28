// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package webconsole

import (
	"net"
	"net/http"
	"strings"

	"github.com/qiangli/coreutils/pkg/coopauth"
	"github.com/qiangli/coreutils/pkg/hostauth"
)

// sessionTTLSeconds bounds a login: short enough that a forgotten tab is not a
// standing grant, long enough not to interrupt work.
const sessionTTLSeconds = 12 * 60 * 60

// handleLoginPage renders the sign-in form.
//
// Server-rendered HTML rather than part of the SPA, for two reasons: it must
// work in a build whose bundle failed to load, and asking the browser to fetch
// and run a script before it can authenticate is a bootstrap it does not need.
func (s *server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	s.renderLogin(w, r, "")
}

func (s *server) renderLogin(w http.ResponseWriter, r *http.Request, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if errMsg != "" {
		w.WriteHeader(http.StatusUnauthorized)
	}

	owner := currentOSUser()
	errBlock := ""
	if errMsg != "" {
		errBlock = `<p class="err">` + htmlEscape(errMsg) + `</p>`
	}
	_, _ = w.Write([]byte(`<!doctype html>
<html lang="en" data-bashy-console="1"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<base href="` + htmlEscape(coopauth.BaseHref(r)) + `">
<title>Sign in &mdash; bashy apps</title>
<link rel="stylesheet" href="app.css">
</head><body class="standalone">
<main class="login">
  <form method="post" action="api/login">
    <h1>bashy <b>apps</b></h1>
    <p class="sub">This host's shell and files. Sign in as
      <strong>` + htmlEscape(owner) + `</strong> with that account's password.</p>
    ` + errBlock + `
    <label>User<input name="user" autocomplete="username" autofocus value="` + htmlEscape(owner) + `"></label>
    <label>Password<input name="password" type="password" autocomplete="current-password"></label>
    <button class="btn primary" type="submit">Sign in</button>
  </form>
</main>
</body></html>`))
}

// handleLogin verifies an OS password and mints a session.
func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	// Rate limit on the PEER address. Not X-Forwarded-For: that is
	// attacker-supplied on exactly the path this protects.
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if s.limiter != nil && !s.limiter.Allow(host) {
		http.Error(w, "too many attempts", http.StatusTooManyRequests)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderLogin(w, r, "Malformed request.")
		return
	}
	user := strings.TrimSpace(r.FormValue("user"))
	pass := r.FormValue("password")

	// Only the OS user this process runs as may sign in. Admitting anyone else
	// would check the password against one account and grant a shell running as
	// another — the terminal here is this process's user, whoever authenticated.
	owner := currentOSUser()
	if !hostauth.SameUser(user, owner) {
		s.renderLogin(w, r, "Sign in as "+owner+" on this host.")
		return
	}
	auth := s.auth
	if auth == nil {
		auth = hostauth.DefaultAuthenticator()
	}
	if err := auth.Authenticate(user, pass); err != nil {
		s.renderLogin(w, r, "Incorrect password.")
		return
	}

	cookie, err := s.sessions.Mint(hostauth.BareUsername(user))
	if err != nil {
		s.renderLogin(w, r, "Could not start a session.")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    cookie,
		Path:     coopauth.BasePrefix(r) + "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.Header.Get(coopauth.HdrForwardedProto) == "https" || r.TLS != nil,
		MaxAge:   sessionTTLSeconds,
	})
	http.Redirect(w, r, coopauth.PrefixPath(r, "/"), http.StatusSeeOther)
}

func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && s.sessions != nil {
		s.sessions.Revoke(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: coopauth.BasePrefix(r) + "/", MaxAge: -1,
	})
	http.Redirect(w, r, coopauth.PrefixPath(r, "/login"), http.StatusSeeOther)
}

// currentOSUser is the account this process runs as, and therefore the only
// account that may sign in — see handleLogin.
func currentOSUser() string {
	u, err := hostauth.CurrentUser()
	if err != nil {
		return ""
	}
	return u
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;",
		`"`, "&quot;", "'", "&#39;").Replace(s)
}
