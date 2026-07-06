package coopauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// sign builds a request carrying a valid (or, via args, invalid) outpost vouch.
func sign(secret, user, groups, prefix string, ts int64) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.5:1234" // non-loopback: exercise the cloud path only
	if prefix != "" {
		r.Header.Set(HdrForwardedPrefix, prefix)
	}
	r.Header.Set(HdrRemoteUser, user)
	r.Header.Set(HdrRemoteEmail, user)
	r.Header.Set(HdrRemoteGroups, groups)
	r.Header.Set(HdrIdentityTs, strconv.FormatInt(ts, 10))
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(user + "\n" + groups + "\n" + prefix + "\n" + strconv.FormatInt(ts, 10)))
		r.Header.Set(HdrIdentitySig, hex.EncodeToString(mac.Sum(nil)))
	}
	return r
}

func TestUsernameCollisionFree(t *testing.T) {
	cases := map[string]string{
		"alice@acme.com":       "alice-acme.com",
		"alice@other.com":      "alice-other.com", // distinct domain => distinct user
		"First.Last@x.io":      "first.last-x.io",
		"a..b@x.com":           "a.b-x.com", // consecutive specials collapsed
		"qiangli@dragon.local": "qiangli-dragon.local",
		"plainuser":            "plainuser",
		"@@@":                  "user",
		"":                     "user",
	}
	for in, want := range cases {
		if got := Username(in); got != want {
			t.Errorf("Username(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVerifyOutpost(t *testing.T) {
	secret, prefix := "s3cr3t", "/matrix/h/dragon/app/loom"
	now := time.Now().Unix()
	cases := []struct {
		name string
		r    *http.Request
		want bool
	}{
		{"valid", sign(secret, "a@x.io", "admin", prefix, now), true},
		{"stale", sign(secret, "a@x.io", "admin", prefix, now-120), false},
		{"future", sign(secret, "a@x.io", "admin", prefix, now+120), false},
		{"wrong secret", sign("other", "a@x.io", "admin", prefix, now), false},
	}
	for _, c := range cases {
		if got := VerifyOutpost(c.r, []byte(secret)); got != c.want {
			t.Errorf("%s: VerifyOutpost = %v, want %v", c.name, got, c.want)
		}
	}
	// Tampering the user (unsigned field) must break verification.
	bad := sign(secret, "a@x.io", "admin", prefix, now)
	bad.Header.Set(HdrRemoteUser, "evil@x.io")
	if VerifyOutpost(bad, []byte(secret)) {
		t.Error("tampered Remote-User verified")
	}
}

func TestResolveAllowlistIsAuthoritative(t *testing.T) {
	secret, prefix := "s3cr3t", "/p"
	pol := Policy{Secret: secret2(secret), RequireHMAC: true, AdminEmails: AdminSet("boss@x.io")}

	// allowlisted email => admin, with a collision-free username
	id, ok := pol.Resolve(sign(secret, "boss@x.io", "user", prefix, time.Now().Unix()))
	if !ok || !id.IsAdmin() || id.Username != "boss-x.io" {
		t.Fatalf("allowlisted admin: ok=%v role=%q user=%q", ok, id.Role, id.Username)
	}
	// a cloud-admin-tier caller NOT on the allowlist is only a user (the classgo
	// lesson: cloudbox stamps admin on every sharee — never trust it for RBAC)
	id, ok = pol.Resolve(sign(secret, "rando@x.io", "admin", prefix, time.Now().Unix()))
	if !ok || id.IsAdmin() {
		t.Fatalf("non-allowlisted cloud-admin must be user: ok=%v role=%q", ok, id.Role)
	}
}

func TestResolveLoopbackOwnerIsAdmin(t *testing.T) {
	pol := Policy{LoopbackAdmin: true, LoopbackUser: "qiangli@dragon.local"}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "127.0.0.1:5555" // loopback, no vouch => on-host owner
	id, ok := pol.Resolve(r)
	if !ok || !id.IsAdmin() || !id.Loopback || id.Username != "qiangli-dragon.local" {
		t.Fatalf("loopback owner: ok=%v admin=%v loopback=%v user=%q", ok, id.IsAdmin(), id.Loopback, id.Username)
	}
}

func TestResolveDirectLANIsAnonymous(t *testing.T) {
	pol := Policy{LoopbackAdmin: true}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.168.1.9:5555" // direct LAN, no vouch, not loopback
	if _, ok := pol.Resolve(r); ok {
		t.Fatal("direct non-loopback LAN with no vouch must be anonymous")
	}
}

func secret2(s string) []byte { return []byte(s) }
