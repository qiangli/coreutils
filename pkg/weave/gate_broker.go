package weave

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	_ "github.com/qiangli/coreutils/cmds/browser"
	"github.com/qiangli/coreutils/tool"
)

type GateKind string

const (
	GateNone         GateKind = "none"
	GateTrust        GateKind = "trust"
	GateBrowserOAuth GateKind = "browser_oauth"
	GateDeviceCode   GateKind = "device_code"
	GateAPIKey       GateKind = "api_key"
	GateHuman        GateKind = "human"
)

type GateVerdict struct {
	Kind      GateKind
	URL       string
	Signature string
}

type routeDeps struct {
	Say          func(payload string) error
	BrowserLogin func(url string) error
	Escalate     func(msg string) error
	State        *gateRouteState
	AutoRouteCap int
}

type gateRouteState struct {
	seen       map[string]bool
	autoRoutes int
}

type gateBroker struct {
	deps        routeDeps
	debounce    time.Duration
	initialized bool
	lastTail    string
	lastChange  time.Time
}

const (
	defaultGateAutoRouteCap = 3
	gateTrustClearPayload   = "1"
	gateBrokerDebounce      = 2 * time.Second
)

var (
	httpsURLRE        = regexp.MustCompile(`https://[^\s"'<>]+`)
	browserOAuthURLRE = regexp.MustCompile(`(?i)https://[^\s"'<>]*(oauth|authorize|login|callback)[^\s"'<>]*`)
	deviceCodeRE      = regexp.MustCompile(`(?i)\b([A-Z0-9]{4}[- ]?[A-Z0-9]{4}|[A-Z0-9]{6,9})\b`)
)

// classifyGate classifies the current live PTY tail into the kind of
// interactive gate the worker appears to be blocked on. It is intentionally
// pure so the broker can be tested without a live tool, browser, or PTY.
func classifyGate(tail string) GateVerdict {
	low := strings.ToLower(tail)
	if strings.TrimSpace(low) == "" {
		return GateVerdict{Kind: GateNone}
	}
	if sig, ok := findSignature(low, []string{
		"do you trust", "trust the contents", "trust this directory",
		"trust this folder", "continue? 1", "1. yes", "1) yes",
		"yes, continue", "yes, proceed",
	}); ok && (strings.Contains(low, "trust") || strings.Contains(low, "continue?")) {
		return GateVerdict{Kind: GateTrust, Signature: sig}
	}
	if sig, ok := findSignature(low, []string{
		"no api key", "api key not set", "api key is not set",
		"missing api key", "api key required",
	}); ok {
		return GateVerdict{Kind: GateAPIKey, Signature: sig}
	}
	if sig, ok := deviceGateSignature(low); ok {
		url := firstDeviceURL(tail)
		return GateVerdict{Kind: GateDeviceCode, URL: url, Signature: sig}
	}
	if sig, ok := authLoginGateSignature(low); ok {
		if url := firstBrowserOAuthURL(tail); url != "" {
			return GateVerdict{Kind: GateBrowserOAuth, URL: url, Signature: sig}
		}
		return GateVerdict{Kind: GateHuman, Signature: sig}
	}
	return GateVerdict{Kind: GateNone}
}

func newGateBroker(deps routeDeps, debounce time.Duration) *gateBroker {
	if deps.State == nil {
		deps.State = &gateRouteState{}
	}
	if debounce <= 0 {
		debounce = gateBrokerDebounce
	}
	return &gateBroker{deps: deps, debounce: debounce}
}

func (b *gateBroker) observeTail(tail string, now time.Time) (GateVerdict, string, error) {
	if b == nil {
		return GateVerdict{Kind: GateNone}, "none", nil
	}
	if !b.initialized {
		b.initialized = true
		b.lastTail = tail
		b.lastChange = now
		return GateVerdict{Kind: GateNone}, "debounce", nil
	}
	if tail != b.lastTail {
		b.lastTail = tail
		b.lastChange = now
		return GateVerdict{Kind: GateNone}, "debounce", nil
	}
	if now.Sub(b.lastChange) < b.debounce {
		return GateVerdict{Kind: GateNone}, "debounce", nil
	}
	verdict := classifyGate(tail)
	if verdict.Kind == GateNone {
		return verdict, "none", nil
	}
	action, err := routeGate(verdict, b.deps)
	return verdict, action, err
}

func routeGate(verdict GateVerdict, deps routeDeps) (action string, err error) {
	if verdict.Kind == GateNone {
		return "none", nil
	}
	state := deps.State
	if state == nil {
		state = &gateRouteState{}
	}
	if state.seen == nil {
		state.seen = map[string]bool{}
	}
	key := routeDedupeKey(verdict)
	if state.seen[key] {
		return "dedupe", nil
	}
	state.seen[key] = true

	switch verdict.Kind {
	case GateTrust:
		if exceededAutoRouteCap(state, deps.AutoRouteCap) {
			return "escalate", callEscalate(deps, "trust gate auto-route cap reached; human should inspect the worker log before clearing another trust prompt")
		}
		state.autoRoutes++
		return "say_trust", callSay(deps, gateTrustClearPayload)
	case GateBrowserOAuth:
		if exceededAutoRouteCap(state, deps.AutoRouteCap) {
			return "escalate", callEscalate(deps, fmt.Sprintf("browser OAuth gate auto-route cap reached; open %s manually and inspect the worker log", verdict.URL))
		}
		state.autoRoutes++
		if err := callBrowserLogin(deps, verdict.URL); err != nil {
			msg := fmt.Sprintf("browser OAuth login failed for %s: %v; human should complete login or inspect browser availability", verdict.URL, err)
			return "escalate", callEscalate(deps, msg)
		}
		return "browser_login", nil
	case GateDeviceCode:
		msg := "device-code gate detected; open the verification URL"
		if verdict.URL != "" {
			msg += " " + verdict.URL
		}
		msg += " and enter the code shown in the worker log"
		return "escalate", callEscalate(deps, msg)
	case GateAPIKey:
		return "escalate", callEscalate(deps, "API key gate detected; set the missing provider API key and resume or restart the worker")
	case GateHuman:
		msg := "interactive auth gate detected"
		if verdict.Signature != "" {
			msg += " (" + verdict.Signature + ")"
		}
		msg += "; no browser URL or safe keystroke route was found, so a human must inspect the worker log"
		return "escalate", callEscalate(deps, msg)
	default:
		return "none", nil
	}
}

func exceededAutoRouteCap(state *gateRouteState, cap int) bool {
	if cap <= 0 {
		cap = defaultGateAutoRouteCap
	}
	return state.autoRoutes >= cap
}

func routeDedupeKey(v GateVerdict) string {
	return string(v.Kind) + "\x00" + v.Signature + "\x00" + v.URL
}

func callSay(deps routeDeps, payload string) error {
	if deps.Say == nil {
		return fmt.Errorf("say route unavailable")
	}
	return deps.Say(payload)
}

func callBrowserLogin(deps routeDeps, url string) error {
	if deps.BrowserLogin == nil {
		return fmt.Errorf("browser login route unavailable")
	}
	return deps.BrowserLogin(url)
}

func callEscalate(deps routeDeps, msg string) error {
	if deps.Escalate == nil {
		return fmt.Errorf("escalation route unavailable: %s", msg)
	}
	return deps.Escalate(msg)
}

func newLiveGateRouteDeps(cmdErr io.Writer, dir string, issueID int64, ctlSock string) routeDeps {
	return routeDeps{
		State: &gateRouteState{},
		Say: func(payload string) error {
			return gateBrokerSay(ctlSock, payload)
		},
		BrowserLogin: func(rawURL string) error {
			return gateBrokerBrowserLogin(context.Background(), rawURL)
		},
		Escalate: func(msg string) error {
			if cmdErr != nil {
				fmt.Fprintf(cmdErr, "weave wait --broker: run #%d blocked: %s\n", issueID, msg)
			}
			return gateBrokerEscalate(dir, issueID, msg)
		},
	}
}

func runGateBrokerForItem(brokers map[int64]*gateBroker, errw io.Writer, dir string, it *weaveItem, now time.Time) {
	if it == nil || it.State != "working" || it.LogPath == "" {
		return
	}
	b := brokers[it.ID]
	if b == nil {
		b = newGateBroker(newLiveGateRouteDeps(errw, dir, it.ID, it.CtlSock), gateBrokerDebounce)
		brokers[it.ID] = b
	}
	tail := weaveReadThrottleLogTail(it.LogPath)
	verdict, action, err := b.observeTail(tail, now)
	if verdict.Kind == GateNone || action == "none" || action == "debounce" || action == "dedupe" {
		return
	}
	if err != nil && errw != nil {
		fmt.Fprintf(errw, "weave wait --broker: run #%d route %s failed: %v\n", it.ID, action, err)
	}
}

func gateBrokerSay(ctlSock, payload string) error {
	if strings.TrimSpace(ctlSock) == "" {
		return fmt.Errorf("control socket unavailable")
	}
	payload = strings.ReplaceAll(strings.ReplaceAll(payload, "\r", " "), "\n", " ")
	frame := payload + "\r\n"
	if strings.ContainsRune(payload, '\x00') {
		frame = "\x00R" + base64.StdEncoding.EncodeToString([]byte(payload)) + "\n"
	}
	return weaveWriteControlFrame(ctlSock, frame)
}

func gateBrokerBrowserLogin(ctx context.Context, rawURL string) error {
	t := tool.Lookup("browser")
	if t == nil {
		return fmt.Errorf("browser tool is not registered")
	}
	successURL := inferBrowserLoginSuccessURL(rawURL)
	args := []string{"login", "--success-url", successURL, rawURL}
	var out, errb bytes.Buffer
	code := t.Run(&tool.RunContext{
		Ctx:   ctx,
		Dir:   ".",
		Env:   os.Environ(),
		FS:    tool.NewLocalFS(),
		Stdio: tool.Stdio{Out: &out, Err: &errb},
	}, args)
	if code != 0 {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		if msg == "" {
			msg = fmt.Sprintf("exit code %d", code)
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func inferBrowserLoginSuccessURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "callback"
	}
	if redirect := u.Query().Get("redirect_uri"); redirect != "" {
		if ru, err := url.Parse(redirect); err == nil {
			if ru.Scheme != "" && ru.Host != "" {
				return ru.Scheme + "://" + ru.Host + ru.Path
			}
			return redirect
		}
	}
	if strings.Contains(strings.ToLower(rawURL), "callback") {
		return "callback"
	}
	return "callback"
}

func gateBrokerEscalate(dir string, issueID int64, msg string) error {
	return withWeaveQueueLock(dir, func(q *weaveQueue) error {
		it := findWeaveItem(q, issueID)
		if it == nil {
			return fmt.Errorf("run #%d not found", issueID)
		}
		weaveAppendComment(it, "broker", "blocker", msg)
		return nil
	})
}

func findSignature(low string, signatures []string) (string, bool) {
	for _, sig := range signatures {
		if strings.Contains(low, sig) {
			return sig, true
		}
	}
	return "", false
}

func authLoginGateSignature(low string) (string, bool) {
	signatures := append([]string{}, authGateSignatures...)
	signatures = append(signatures,
		"login to continue",
		"log in to continue",
		"open the following url",
		"open the following link",
		"visit this url",
		"visit the url",
		"complete authentication",
	)
	return findSignature(low, signatures)
}

func deviceGateSignature(low string) (string, bool) {
	deviceWords := []string{"device code", "device login", "verification url", "verification uri", "verify at", "device"}
	codeWords := []string{"enter the code", "use code", "user code", "one-time code", "activation code"}
	deviceSig, hasDevice := findSignature(low, deviceWords)
	codeSig, hasCode := findSignature(low, codeWords)
	if !(hasDevice && hasCode) {
		return "", false
	}
	if firstDeviceURL(low) == "" && !deviceCodeRE.MatchString(low) {
		return "", false
	}
	if codeSig != "" {
		return codeSig, true
	}
	return deviceSig, true
}

func firstBrowserOAuthURL(s string) string {
	return trimURLPunctuation(browserOAuthURLRE.FindString(s))
}

func firstDeviceURL(s string) string {
	for _, u := range httpsURLRE.FindAllString(s, -1) {
		clean := trimURLPunctuation(u)
		low := strings.ToLower(clean)
		if strings.Contains(low, "verify") || strings.Contains(low, "device") || strings.Contains(low, "login") {
			return clean
		}
	}
	return ""
}

func trimURLPunctuation(u string) string {
	return strings.TrimRight(u, ".,);]")
}
