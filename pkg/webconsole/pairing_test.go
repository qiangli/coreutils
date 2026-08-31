package webconsole

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/websession"
)

// pairEnv builds a console with pairing on and a private store, so a test
// never touches the developer's own pairing state.
func pairEnv(t *testing.T) (http.Handler, *pairStore) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "pairing.json")
	store := newPairStore(path)
	h := newTestHandler(t, Options{
		RequireLogin:  true,
		Sessions:      websession.NewStore(12*time.Hour, []byte("test-key-test-key-test-key-32byt")),
		Pairing:       true,
		PairStorePath: path,
	})
	return h, store
}

// redeem drives a scan: mint a ticket, GET the redeem URL, return the session
// cookie the console handed back.
func redeemScan(t *testing.T, h http.Handler, store *pairStore, scope []string, ttl time.Duration) *http.Cookie {
	t.Helper()
	_, secret, err := store.issueTicket(scope, ttl, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	w := do(h, "GET", pairRedeemPath+"?v="+pairQRVersion+"&t="+url.QueryEscape(secret), "192.168.1.44:5555", nil)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("redeem = %d, want 303 (body %q)", w.Code, strings.TrimSpace(w.Body.String()))
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			return c
		}
	}
	t.Fatal("redeem set no session cookie")
	return nil
}

func withCookie(c *http.Cookie) http.Header {
	h := http.Header{}
	h.Set("Cookie", c.Name+"="+c.Value)
	return h
}

// TestScanOneQRReachesTheConsoleWithoutAPassword is the story's first
// acceptance line.
func TestScanOneQRReachesTheConsoleWithoutAPassword(t *testing.T) {
	h, store := pairEnv(t)

	// Before pairing, the LAN peer is refused.
	if got := do(h, "GET", "/board/", "192.168.1.44:5555", nil).Code; got == http.StatusOK {
		t.Fatal("an unpaired LAN peer reached the console")
	}
	cookie := redeemScan(t, h, store, nil, time.Hour)
	if got := do(h, "GET", "/board/", "192.168.1.44:5555", withCookie(cookie)).Code; got != http.StatusOK {
		t.Fatalf("paired device on /board/ = %d, want 200", got)
	}
}

func TestPairedDeviceCanLoadSharedChromeAssets(t *testing.T) {
	h, store := pairEnv(t)
	cookie := redeemScan(t, h, store, nil, time.Hour)
	for _, asset := range []string{
		"/app.css", "/backgrounds.css", "/app.js", "/board.js", "/mb.js",
		"/vendor/xterm.css", "/vendor/xterm.js", "/vendor/xterm-addon-fit.js",
	} {
		w := do(h, "GET", asset, "192.168.1.44:5555", withCookie(cookie))
		if w.Code != http.StatusOK {
			t.Errorf("paired device GET %s = %d, want 200 (body %q)", asset, w.Code, strings.TrimSpace(w.Body.String()))
		}
	}
}

// TestSecondScanOfTheSameTicketFails: single-use in both directions.
func TestSecondScanOfTheSameTicketFails(t *testing.T) {
	h, store := pairEnv(t)
	_, secret, err := store.issueTicket(nil, time.Hour, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	target := pairRedeemPath + "?v=" + pairQRVersion + "&t=" + url.QueryEscape(secret)
	if got := do(h, "GET", target, "192.168.1.44:5555", nil).Code; got != http.StatusSeeOther {
		t.Fatalf("first scan = %d, want 303", got)
	}
	w := do(h, "GET", target, "192.168.1.45:5555", nil)
	if w.Code == http.StatusSeeOther {
		t.Fatal("a pairing code was redeemed twice")
	}
	if !strings.Contains(w.Body.String(), "already been used") {
		t.Fatalf("the second scan does not say why it failed: %q", w.Body.String())
	}
}

// TestRevokingTheOperatorGrantRevokesDerivedDevices is the story's third
// acceptance line: a device inherits the operator authority and dies with it.
func TestRevokingTheOperatorGrantRevokesDerivedDevices(t *testing.T) {
	h, store := pairEnv(t)
	cookie := redeemScan(t, h, store, nil, time.Hour)
	if got := do(h, "GET", "/board/", "192.168.1.44:5555", withCookie(cookie)).Code; got != http.StatusOK {
		t.Fatalf("paired device = %d, want 200", got)
	}
	if _, err := store.revokeAll(); err != nil {
		t.Fatal(err)
	}
	w := do(h, "GET", "/board/", "192.168.1.44:5555", withCookie(cookie))
	if w.Code != http.StatusForbidden {
		t.Fatalf("after revoke --all = %d, want 403", w.Code)
	}
	if !strings.Contains(w.Body.String(), "pairing has ended") {
		t.Fatalf("revoked device is refused without saying why: %q", w.Body.String())
	}
	// The cookie must be cleared, or the phone keeps presenting a dead one.
	cleared := false
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("a dead device's cookie was not cleared")
	}
}

// TestRevokingOneDeviceLeavesTheOthers.
func TestRevokingOneDeviceLeavesTheOthers(t *testing.T) {
	h, store := pairEnv(t)
	a := redeemScan(t, h, store, nil, time.Hour)
	b := redeemScan(t, h, store, nil, time.Hour)

	st, err := store.load()
	if err != nil {
		t.Fatal(err)
	}
	live := st.liveDevices(time.Now())
	if len(live) != 2 {
		t.Fatalf("expected 2 live devices, got %d", len(live))
	}
	ok, err := store.revoke(live[0].ID)
	if err != nil || !ok {
		t.Fatalf("revoke: ok=%v err=%v", ok, err)
	}
	// One of the two cookies still works, the other does not.
	codes := []int{
		do(h, "GET", "/board/", "192.168.1.44:5555", withCookie(a)).Code,
		do(h, "GET", "/board/", "192.168.1.44:5555", withCookie(b)).Code,
	}
	if !(codes[0] == http.StatusForbidden && codes[1] == http.StatusOK) &&
		!(codes[1] == http.StatusForbidden && codes[0] == http.StatusOK) {
		t.Fatalf("revoking one device did not leave exactly the other live: %v", codes)
	}
}

// TestExpiredDeviceIsRefused.
func TestExpiredDeviceIsRefused(t *testing.T) {
	h, store := pairEnv(t)
	cookie := redeemScan(t, h, store, nil, 50*time.Millisecond)
	time.Sleep(80 * time.Millisecond)
	if got := do(h, "GET", "/board/", "192.168.1.44:5555", withCookie(cookie)).Code; got != http.StatusForbidden {
		t.Fatalf("expired device = %d, want 403", got)
	}
}

// TestDefaultScopeIsNotAShell is the per-panel-scope story's headline: a
// paired phone reaches board/mb/relay and NOT /term/ or /files/.
func TestDefaultScopeIsNotAShell(t *testing.T) {
	h, store := pairEnv(t)
	cookie := redeemScan(t, h, store, nil, time.Hour)

	allowed := []string{"/board/", "/mb/", "/relay/"}
	for _, p := range allowed {
		if got := do(h, "GET", p, "192.168.1.44:5555", withCookie(cookie)).Code; got == http.StatusForbidden {
			t.Fatalf("default scope refuses %s, which it is supposed to allow", p)
		}
	}
	for _, p := range []string{"/term/", "/files/"} {
		w := do(h, "GET", p, "192.168.1.44:5555", withCookie(cookie))
		if w.Code != http.StatusForbidden {
			t.Fatalf("default scope reaches %s (%d) — a scanned phone must not be a shell", p, w.Code)
		}
		if !strings.Contains(w.Body.String(), "--allow") {
			t.Fatalf("scope denial on %s names no remedy: %q", p, w.Body.String())
		}
	}
}

// TestAllowGrantsExactlyWhatItNames.
func TestAllowGrantsExactlyWhatItNames(t *testing.T) {
	h, store := pairEnv(t)
	cookie := redeemScan(t, h, store, []string{"terminal"}, time.Hour)
	if got := do(h, "GET", "/term/", "192.168.1.44:5555", withCookie(cookie)).Code; got == http.StatusForbidden {
		t.Fatal("--allow terminal did not grant the terminal")
	}
	if got := do(h, "GET", "/files/", "192.168.1.44:5555", withCookie(cookie)).Code; got != http.StatusForbidden {
		t.Fatalf("--allow terminal also granted files (%d) — scope must grant exactly what it names", got)
	}
}

// TestScopeHoldsOnTheAPISurfaceToo is the acceptance line that makes the
// first-segment property real: an out-of-scope panel's DATA must be as
// unreachable as its page. /api/board would otherwise resolve to segment
// "api", which owns nothing, and sail straight through.
func TestScopeHoldsOnTheAPISurfaceToo(t *testing.T) {
	h, store := pairEnv(t)
	cookie := redeemScan(t, h, store, []string{"mb"}, time.Hour)

	if got := do(h, "GET", "/api/mb", "192.168.1.44:5555", withCookie(cookie)).Code; got == http.StatusForbidden {
		t.Fatal("an in-scope panel's API is refused")
	}
	if got := do(h, "GET", "/api/board", "192.168.1.44:5555", withCookie(cookie)).Code; got != http.StatusForbidden {
		t.Fatalf("/api/board reachable (%d) by a device scoped to mb only", got)
	}
	// The console-wide surfaces must stay reachable or the launcher cannot render.
	for _, p := range []string{"/", "/api/apps", "/api/session"} {
		if got := do(h, "GET", p, "192.168.1.44:5555", withCookie(cookie)).Code; got == http.StatusForbidden {
			t.Fatalf("console-wide path %s refused to a scoped device", p)
		}
	}
}

// TestScopeSegmentResolution pins the mapping directly, including the /api
// deferral and the name-vs-mount difference (terminal lives at /term/).
func TestScopeSegmentResolution(t *testing.T) {
	cases := map[string]string{
		"/board/":            "board",
		"/api/board":         "board",
		"/api/board/panel/7": "board",
		"/term/ws":           "term",
		"/files/x/y":         "files",
		"/":                  "",
	}
	for path, want := range cases {
		if got := scopeSegment(path); got != want {
			t.Fatalf("scopeSegment(%q) = %q, want %q", path, got, want)
		}
	}
}

// TestUnknownPanelInAllowIsRejected: silently accepting `--allow term` would
// leave the operator believing they granted something they did not.
func TestUnknownPanelInAllowIsRejected(t *testing.T) {
	panels := Discover()
	if err := ValidateScope([]string{"board"}, panels); err != nil {
		t.Fatalf("a real panel was rejected: %v", err)
	}
	err := ValidateScope([]string{"nosuchpanel"}, panels)
	if err == nil {
		t.Fatal("an unknown panel name was accepted")
	}
	if !strings.Contains(err.Error(), "this console serves") {
		t.Fatalf("the error names no valid set: %v", err)
	}
	// The mount segment is accepted too, since that is what the gate sees.
	if err := ValidateScope([]string{"term"}, panels); err != nil {
		t.Fatalf("the mount segment was rejected: %v", err)
	}
}

// TestDeviceTTLNeverExceedsTheOperatorGrant.
func TestDeviceTTLNeverExceedsTheOperatorGrant(t *testing.T) {
	store := newPairStore(filepath.Join(t.TempDir(), "pairing.json"))
	// Ask for a year. The grant ceiling must win.
	tk, _, err := store.issueTicket(nil, 365*24*time.Hour, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ttl, err := time.ParseDuration(tk.TTL)
	if err != nil {
		t.Fatal(err)
	}
	if ttl > operatorGrantTTL {
		t.Fatalf("device TTL %s exceeds the operator grant %s", ttl, operatorGrantTTL)
	}
}

// TestTicketSecretIsNeverWrittenToDisk. Reading the pairing file must not let
// an attacker perform a pairing.
func TestTicketSecretIsNeverWrittenToDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pairing.json")
	store := newPairStore(path)
	_, secret, err := store.issueTicket(nil, time.Hour, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatal("the pairing file contains the ticket secret in the clear")
	}
	if !strings.Contains(string(raw), hashSecret(secret)) {
		t.Fatal("the pairing file does not contain the ticket hash — redemption cannot work")
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("pairing file mode = %o, want 600", perm)
	}
}

// TestQRPayloadIsVersioned: a v2 payload (one carrying a certificate
// fingerprint) must be REFUSED by a v1 console rather than half-understood.
func TestQRPayloadIsVersioned(t *testing.T) {
	h, store := pairEnv(t)
	_, secret, err := store.issueTicket(nil, time.Hour, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	w := do(h, "GET", pairRedeemPath+"?v=2&t="+url.QueryEscape(secret), "192.168.1.44:5555", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("a v2 payload was accepted by a v1 console (%d)", w.Code)
	}
	if !strings.Contains(w.Body.String(), "newer bashy") {
		t.Fatalf("version refusal names no remedy: %q", w.Body.String())
	}
	// The ticket must still be unspent after a version refusal.
	st, _ := store.load()
	if st.openTickets(time.Now()) != 1 {
		t.Fatal("a refused version consumed the ticket")
	}
}

// TestRedeemURLCarriesTheVersion.
func TestRedeemURLCarriesTheVersion(t *testing.T) {
	got := redeemURL("workshop.local", 8639, "sec-ret")
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != pairRedeemPath {
		t.Fatalf("path = %q", u.Path)
	}
	if u.Query().Get("v") != pairQRVersion || u.Query().Get("t") != "sec-ret" {
		t.Fatalf("payload = %q", got)
	}
	if u.Host != "workshop.local:8639" {
		t.Fatalf("host = %q", u.Host)
	}
}

// TestPairingIsOffByDefault: no --pair means no redemption route at all.
func TestPairingIsOffByDefault(t *testing.T) {
	h := newTestHandler(t, Options{
		RequireLogin: true,
		Sessions:     websession.NewStore(time.Hour, []byte("test-key-test-key-test-key-32byt")),
	})
	w := do(h, "GET", pairRedeemPath+"?t=anything", "192.168.1.44:5555", nil)
	if w.Code == http.StatusSeeOther {
		t.Fatal("a console without --pair redeemed a pairing ticket")
	}
}

// TestDevicesJSONIsTypedData, not a rendered table.
func TestDevicesJSONIsTypedData(t *testing.T) {
	h, store := pairEnv(t)
	redeemScan(t, h, store, []string{"board"}, time.Hour)
	st, err := store.load()
	if err != nil {
		t.Fatal(err)
	}
	live := st.liveDevices(time.Now())
	if len(live) != 1 {
		t.Fatalf("want 1 device, got %d", len(live))
	}
	raw, err := json.Marshal(live[0])
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	scope, ok := back["scope"].([]any)
	if !ok || len(scope) != 1 || scope[0] != "board" {
		t.Fatalf("scope is not typed data: %#v", back["scope"])
	}
}

// TestTerminalQRRenders: the code has to be scannable, and a QR that fails to
// render must say so rather than print an empty block.
func TestTerminalQRRenders(t *testing.T) {
	block, err := terminalQR(redeemURL("192.168.1.20", 8639, strings.Repeat("a", 43)))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(block, "\n"), "\n")
	if len(lines) < 12 {
		t.Fatalf("QR block is only %d lines; that is not a scannable code", len(lines))
	}
	// Square-ish: a QR that renders as one long line is a rendering bug.
	if w := len([]rune(lines[len(lines)/2])); w < 20 {
		t.Fatalf("QR block is %d columns wide", w)
	}
}

// TestPairAuditRecordsTheGrant: a grant nobody can audit is a grant nobody can
// supervise. These records are NOT gated on $BASHY_AUDIT.
func TestPairAuditRecordsTheGrant(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Setenv("BASHY_AUDIT", "") // explicitly off for commands
	auditPairEvent("pair.issued", map[string]string{"ticket": "abc", "scope": "board"})

	logPath := filepath.Join(home, "audit", "audit.jsonl")
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("pairing left no audit record: %v", err)
	}
	if !strings.Contains(string(raw), "pair.issued") || !strings.Contains(string(raw), "ticket=abc") {
		t.Fatalf("audit record does not describe the pairing: %s", raw)
	}
}

// TestPairGatedListenerFollowsTheDeviceSet is the structural claim: exposure
// is a FUNCTION of the paired device set, not a switch left on.
func TestPairGatedListenerFollowsTheDeviceSet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pairing.json")
	store := newPairStore(path)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}
	var out strings.Builder
	stop, err := runPairGatedListener(ctx, &out, srv, addr, path)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	// Nothing paired: the port must NOT be open.
	time.Sleep(3 * time.Second)
	if dialable(addr) {
		t.Fatalf("the LAN port is open with nothing paired:\n%s", out.String())
	}

	// An outstanding ticket opens it (somebody is about to scan).
	if _, _, err := store.issueTicket(nil, time.Hour, time.Minute); err != nil {
		t.Fatal(err)
	}
	if !eventually(6*time.Second, func() bool { return dialable(addr) }) {
		t.Fatalf("the LAN port did not open for a pending pairing:\n%s", out.String())
	}

	// Revoking everything closes it again, with no operator action.
	if _, err := store.revokeAll(); err != nil {
		t.Fatal(err)
	}
	if !eventually(8*time.Second, func() bool { return !dialable(addr) }) {
		t.Fatalf("the LAN port stayed open after the last pairing ended:\n%s", out.String())
	}
}

func dialable(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func eventually(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// TestOperatorSessionIsUnaffectedByScope: an ordinary host-OS login keeps the
// full panel set. Narrowing the phone must not narrow the laptop.
func TestOperatorSessionIsUnaffectedByScope(t *testing.T) {
	dir := t.TempDir()
	sessions := websession.NewStore(time.Hour, []byte("test-key-test-key-test-key-32byt"))
	h := newTestHandler(t, Options{
		RequireLogin:  true,
		Sessions:      sessions,
		Pairing:       true,
		PairStorePath: filepath.Join(dir, "pairing.json"),
	})
	cookieVal, err := sessions.Mint("someuser")
	if err != nil {
		t.Fatal(err)
	}
	c := &http.Cookie{Name: sessionCookie, Value: cookieVal}
	for _, p := range []string{"/", "/board/", "/files/", "/term/"} {
		if got := do(h, "GET", p, "192.168.1.44:5555", withCookie(c)).Code; got == http.StatusForbidden {
			t.Fatalf("an operator session was refused %s", p)
		}
	}
}

// TestSplitDeviceSubject pins the cookie subject encoding, including the fact
// that an ordinary operator subject is NOT read as a device.
func TestSplitDeviceSubject(t *testing.T) {
	u, d, ok := splitDeviceSubject(deviceSubject("alice", "dev123"))
	if !ok || u != "alice" || d != "dev123" {
		t.Fatalf("round trip = %q %q %v", u, d, ok)
	}
	if _, _, ok := splitDeviceSubject("alice"); ok {
		t.Fatal("a bare operator subject was read as a device")
	}
	if _, err := websession.NewStore(time.Hour, nil).Mint(deviceSubject("alice", "dev123")); err != nil {
		t.Fatalf("the device subject is not mintable: %v", err)
	}
}

var _ = httptest.NewRequest

// TestUnreadableTTLIsRefusedNotDefaulted pins the direction this must fail in.
// An earlier draft rounded the stored TTL to the second, so a sub-second TTL
// became "0s", which redeem read as unparseable and replaced with the 4-hour
// default — silently WIDENING a grant the operator had narrowed. A ticket this
// code cannot read is a refusal.
func TestUnreadableTTLIsRefusedNotDefaulted(t *testing.T) {
	store := newPairStore(filepath.Join(t.TempDir(), "pairing.json"))
	_, secret, err := store.issueTicket(nil, time.Hour, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.mutate(func(st *pairState) error {
		st.Tickets[0].TTL = "not-a-duration"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.redeem(secret, "phone", ""); err == nil {
		t.Fatal("a ticket with an unreadable lifetime was redeemed anyway")
	}

	// And a sub-second TTL survives the round trip as itself.
	store2 := newPairStore(filepath.Join(t.TempDir(), "pairing.json"))
	_, secret2, err := store2.issueTicket(nil, 200*time.Millisecond, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	d, err := store2.redeem(secret2, "phone", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := time.Until(d.Expires); got > time.Second {
		t.Fatalf("a 200ms TTL became %s — the grant was widened", got)
	}
}

// TestScopeDenialNamesThePanelTheOperatorTypes: the gate sees the MOUNT
// segment ("term") while --allow takes the panel NAME ("terminal"). Echoing
// the segment back would hand the reader a remedy they have to translate.
func TestScopeDenialNamesThePanelTheOperatorTypes(t *testing.T) {
	h, store := pairEnv(t)
	cookie := redeemScan(t, h, store, []string{"board"}, time.Hour)
	body := do(h, "GET", "/term/", "192.168.1.44:5555", withCookie(cookie)).Body.String()
	if !strings.Contains(body, "--allow terminal") {
		t.Fatalf("denial suggests a name --allow does not take: %q", body)
	}
}

// TestAuditRecordsAreTimestamped. audit.Append fills Seq/PrevHash/Hash but not
// Time; a pairing record whose timestamp is empty cannot answer "when was this
// device let in?", which is most of the point of recording it.
func TestAuditRecordsAreTimestamped(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	auditPairEvent("pair.issued", map[string]string{"ticket": "t1"})
	raw, err := os.ReadFile(filepath.Join(home, "audit", "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var rec struct {
		Time string `json:"time"`
	}
	if err := json.Unmarshal([]byte(strings.SplitN(string(raw), "\n", 2)[0]), &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Time == "" {
		t.Fatal("pairing audit record carries no timestamp")
	}
	if _, err := time.Parse(time.RFC3339Nano, rec.Time); err != nil {
		t.Fatalf("timestamp %q is not RFC3339: %v", rec.Time, err)
	}
}
