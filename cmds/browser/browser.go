package browsercmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/browser"
	"github.com/qiangli/coreutils/pkg/browser/live"
	"github.com/qiangli/coreutils/pkg/browser/probe"
	"github.com/qiangli/coreutils/pkg/browser/solo"
	"github.com/qiangli/coreutils/pkg/browser/wire"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "browser",
	Synopsis: "Browser automation: status, fetch, and CDP-backed page actions.",
	Usage: "browser [--json] [--mode solo|probe|live] [--probe-url URL] <subcommand> [args]\n" +
		"\n" +
		"Modes (--mode) — how it gets a browser:\n" +
		"  solo   private headless Chrome, zero setup; `navigate URL` returns title/url/content (best for one-shot scrapes)\n" +
		"  probe  (default) attach to a Chrome you started with --remote-debugging-port=9222 — persistent session\n" +
		"  live   drive your real logged-in Chrome via `browser hub` + the MV3 extension (cookies/SSO intact)\n" +
		"\n" +
		"Subcommands: status navigate extract eval click type wait-for-selector screenshot\n" +
		"  cookies-get scroll keyboard-press back tabs fetch hub setup login\n" +
		"(--json emits {success,title,url,content,error}; `bashy fetch URL` is the non-browser HTTP client.)\n" +
		"Guide: coreutils/docs/browser.md",
}

func init() { cmd.Run = run; tool.Register(cmd) }

const noBrowserMessage = "no browser: start Chrome with --remote-debugging-port=9222 or run `bashy browser login`"

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	asJSON := fs.Bool("json", false, "emit JSON result envelopes")
	mode := fs.String("mode", "probe", "browser mode: probe")
	probeURL := fs.String("probe-url", probe.DefaultURL, "Chrome remote debugging URL for probe mode")
	chromePath := fs.String("chrome-path", "", "Chrome/Chromium executable path for solo mode")
	userDataDir := fs.String("user-data-dir", "", "Chrome user-data-dir for solo mode")
	headed := fs.Bool("headed", false, "run solo Chrome headed instead of headless")
	successURL := fs.String("success-url", "", "login completion URL substring")
	tokenSelector := fs.String("token-selector", "", "CSS selector whose value/text is the login token")
	cookieName := fs.String("cookie", "", "cookie name that indicates login completion")
	dryRun := fs.Bool("dry-run", false, "for login, print what would be polled")
	loginTimeout := fs.Duration("timeout", 2*time.Minute, "login polling timeout")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing subcommand")
	}
	ctx := rc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	switch operands[0] {
	case "status":
		return browserStatus(rc, ctx, *mode, *probeURL, solo.Config{ChromePath: *chromePath, UserDataDir: *userDataDir, Headed: *headed}, *asJSON)
	case "hub":
		return browserHub(rc, ctx, operands[1:], *asJSON)
	case "setup":
		return browserSetup(rc, operands[1:], *asJSON)
	case "fetch":
		return browserFetch(rc, ctx, operands[1:], *asJSON)
	case "login":
		return browserLogin(rc, ctx, operands[1:], loginOptions{
			Mode:          *mode,
			ProbeURL:      *probeURL,
			ChromePath:    *chromePath,
			UserDataDir:   *userDataDir,
			Headed:        *headed,
			SuccessURL:    *successURL,
			TokenSelector: *tokenSelector,
			Cookie:        *cookieName,
			DryRun:        *dryRun,
			Timeout:       *loginTimeout,
			JSON:          *asJSON,
		})
	}

	action, err := actionFromArgs(operands)
	if err != nil {
		return tool.UsageError(rc, cmd, "%s", err)
	}
	client, err := clientForMode(ctx, *mode, *probeURL, solo.Config{ChromePath: *chromePath, UserDataDir: *userDataDir, Headed: *headed})
	if err != nil {
		return printNoBrowser(rc, *asJSON, *mode, err)
	}
	defer client.Close()
	res, err := client.Execute(ctx, action)
	if err != nil {
		return printNoBrowser(rc, *asJSON, *mode, err)
	}
	return printResult(rc, res, *asJSON)
}

type loginOptions struct {
	Mode          string
	ProbeURL      string
	ChromePath    string
	UserDataDir   string
	Headed        bool
	SuccessURL    string
	TokenSelector string
	Cookie        string
	DryRun        bool
	Timeout       time.Duration
	JSON          bool
}

type loginSpec struct {
	SuccessURL    string `json:"success_url,omitempty"`
	TokenSelector string `json:"token_selector,omitempty"`
	Cookie        string `json:"cookie,omitempty"`
	Domain        string `json:"domain,omitempty"`
}

type loginState struct {
	URL     string
	Token   string
	Cookies []loginCookie
}

type loginCookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
}

type loginCompletion struct {
	Done   bool   `json:"done"`
	Reason string `json:"reason,omitempty"`
	Token  string `json:"token,omitempty"`
	Cookie string `json:"cookie,omitempty"`
	URL    string `json:"url,omitempty"`
}

func browserLogin(rc *tool.RunContext, ctx context.Context, args []string, opt loginOptions) int {
	if len(args) != 1 {
		return tool.UsageError(rc, cmd, "login requires URL")
	}
	loginURL := args[0]
	if opt.SuccessURL == "" && opt.TokenSelector == "" && opt.Cookie == "" {
		return tool.UsageError(rc, cmd, "login requires --success-url, --token-selector, or --cookie")
	}
	spec := loginSpec{
		SuccessURL:    opt.SuccessURL,
		TokenSelector: opt.TokenSelector,
		Cookie:        opt.Cookie,
		Domain:        domainForURL(loginURL),
	}
	if opt.DryRun {
		if opt.JSON {
			return writeJSON(rc, map[string]any{"url": loginURL, "poll": spec, "dry_run": true})
		}
		fmt.Fprintf(rc.Out, "url=%s success_url=%q token_selector=%q cookie=%q domain=%q\n", loginURL, spec.SuccessURL, spec.TokenSelector, spec.Cookie, spec.Domain)
		return 0
	}

	client, err := clientForMode(ctx, opt.Mode, opt.ProbeURL, solo.Config{ChromePath: opt.ChromePath, UserDataDir: opt.UserDataDir, Headed: opt.Headed})
	if err != nil {
		return printNoBrowser(rc, opt.JSON, opt.Mode, err)
	}
	defer client.Close()
	if res, err := client.Execute(ctx, wire.Action{Type: wire.ActionNavigate, URL: loginURL}); err != nil {
		return printNoBrowser(rc, opt.JSON, opt.Mode, err)
	} else if !res.Success {
		return printResult(rc, res, opt.JSON)
	}

	deadline := time.Now().Add(opt.Timeout)
	for {
		state := pollLoginState(ctx, client, spec)
		done := DetectLoginCompletion(spec, state)
		if done.Done {
			if opt.JSON {
				return writeJSON(rc, done)
			}
			switch {
			case done.Token != "":
				fmt.Fprintln(rc.Out, done.Token)
			case done.Cookie != "":
				fmt.Fprintln(rc.Out, done.Cookie)
			default:
				fmt.Fprintln(rc.Out, done.URL)
			}
			return 0
		}
		if time.Now().After(deadline) {
			if opt.JSON {
				return writeJSON(rc, map[string]any{"done": false, "error": "login timed out"})
			}
			fmt.Fprintln(rc.Err, "browser login: timed out waiting for completion")
			return 1
		}
		select {
		case <-ctx.Done():
			fmt.Fprintf(rc.Err, "browser login: %v\n", ctx.Err())
			return 1
		case <-time.After(time.Second):
		}
	}
}

func pollLoginState(ctx context.Context, client browser.Client, spec loginSpec) loginState {
	var state loginState
	if res, err := client.Execute(ctx, wire.Action{Type: wire.ActionEvaluate, Script: "location.href"}); err == nil && res != nil && res.Success {
		state.URL = res.Data
	}
	if spec.TokenSelector != "" {
		script := fmt.Sprintf(`(function(){var el=document.querySelector(%q); return el ? (el.value || el.textContent || "") : "";})()`, spec.TokenSelector)
		if res, err := client.Execute(ctx, wire.Action{Type: wire.ActionEvaluate, Script: script}); err == nil && res != nil && res.Success {
			state.Token = res.Data
		}
	}
	if spec.Cookie != "" {
		res, err := client.Execute(ctx, wire.Action{Type: wire.ActionCookiesGet, Name: spec.Cookie, Domain: spec.Domain})
		if err == nil && res != nil && res.Success && res.Data != "" {
			_ = json.Unmarshal([]byte(res.Data), &state.Cookies)
		}
	}
	return state
}

func DetectLoginCompletion(spec loginSpec, state loginState) loginCompletion {
	if spec.SuccessURL != "" && strings.Contains(state.URL, spec.SuccessURL) {
		return loginCompletion{Done: true, Reason: "redirect", URL: state.URL}
	}
	if spec.TokenSelector != "" && strings.TrimSpace(state.Token) != "" {
		return loginCompletion{Done: true, Reason: "token", Token: strings.TrimSpace(state.Token), URL: state.URL}
	}
	if spec.Cookie != "" {
		for _, c := range state.Cookies {
			if c.Name == spec.Cookie && c.Value != "" {
				return loginCompletion{Done: true, Reason: "cookie", Cookie: c.Value, URL: state.URL}
			}
		}
	}
	return loginCompletion{Done: false}
}

func domainForURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func clientForMode(ctx context.Context, mode, probeURL string, soloCfg solo.Config) (browser.Client, error) {
	switch mode {
	case "", "probe":
		c := probe.New(probeURL)
		if !c.Available(ctx) {
			return nil, fmt.Errorf("probe target %s not reachable", c.URL())
		}
		if err := c.EnsureReady(ctx); err != nil {
			return nil, err
		}
		return c, nil
	case "solo":
		c := solo.New(soloCfg)
		if !c.Available(ctx) {
			return nil, fmt.Errorf("solo Chrome not found")
		}
		if err := c.EnsureReady(ctx); err != nil {
			return nil, err
		}
		return c, nil
	case "live":
		// Attach to the running hub (started by `bashy browser hub`) and
		// forward actions to the connected extension via /dispatch. If no
		// hub is up, EnsureReady binds one in-process — but a one-shot CLI
		// process exits right after, so the durable path is a separate
		// `bashy browser hub`.
		return live.NewClient(ctx, live.DefaultPort)
	default:
		return nil, fmt.Errorf("mode %q is not supported", mode)
	}
}

func browserStatus(rc *tool.RunContext, ctx context.Context, mode, probeURL string, soloCfg solo.Config, asJSON bool) int {
	reachable := false
	message := ""
	if mode == "" || mode == "probe" {
		c := probe.New(probeURL)
		reachable = c.Available(ctx)
		if !reachable {
			message = noBrowserMessage
		}
	} else if mode == "solo" {
		c := solo.New(soloCfg)
		reachable = c.Available(ctx)
		if !reachable {
			message = "no browser: Chrome or Chromium was not found for solo mode"
		}
	} else {
		message = fmt.Sprintf("mode %q is not supported", mode)
	}
	if asJSON {
		return writeJSON(rc, map[string]any{
			"mode":      mode,
			"probe_url": probeURL,
			"reachable": reachable,
			"message":   message,
		})
	}
	if reachable {
		fmt.Fprintf(rc.Out, "mode=%s reachable=true probe_url=%s\n", mode, probeURL)
	} else {
		fmt.Fprintf(rc.Out, "mode=%s reachable=false probe_url=%s\n%s\n", mode, probeURL, message)
	}
	return 0
}

func browserFetch(rc *tool.RunContext, ctx context.Context, args []string, asJSON bool) int {
	if len(args) != 1 {
		return tool.UsageError(rc, cmd, "fetch requires exactly one URL")
	}
	url := args[0]
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return tool.UsageError(rc, cmd, "fetch URL must start with http:// or https://")
	}
	hctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(hctx, http.MethodGet, url, nil)
	if err != nil {
		fmt.Fprintf(rc.Err, "browser fetch: %v\n", err)
		return 1
	}
	req.Header.Set("User-Agent", "bashy-browser/0.1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(rc.Err, "browser fetch: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	const maxBytes = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		fmt.Fprintf(rc.Err, "browser fetch: %v\n", err)
		return 1
	}
	truncated := len(body) > maxBytes
	if truncated {
		body = body[:maxBytes]
	}
	if asJSON {
		headers := map[string]string{}
		for k := range resp.Header {
			headers[k] = resp.Header.Get(k)
		}
		return writeJSON(rc, map[string]any{
			"url":         url,
			"status":      resp.Status,
			"status_code": resp.StatusCode,
			"headers":     headers,
			"body":        string(body),
			"truncated":   truncated,
		})
	}
	fmt.Fprint(rc.Out, string(body))
	if len(body) == 0 || body[len(body)-1] != '\n' {
		fmt.Fprintln(rc.Out)
	}
	if resp.StatusCode >= 400 {
		return 1
	}
	return 0
}

func actionFromArgs(args []string) (wire.Action, error) {
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "navigate":
		if len(rest) != 1 {
			return wire.Action{}, fmt.Errorf("navigate requires URL")
		}
		return wire.Action{Type: wire.ActionNavigate, URL: rest[0]}, nil
	case "extract":
		a := wire.Action{Type: wire.ActionExtract}
		if len(rest) > 0 {
			a.Scope = rest[0]
		}
		return a, nil
	case "click":
		if len(rest) != 1 {
			return wire.Action{}, fmt.Errorf("click requires selector")
		}
		return wire.Action{Type: wire.ActionClick, Selector: rest[0]}, nil
	case "type":
		if len(rest) < 2 {
			return wire.Action{}, fmt.Errorf("type requires selector and text")
		}
		return wire.Action{Type: wire.ActionType, Selector: rest[0], Text: strings.Join(rest[1:], " ")}, nil
	case "eval", "evaluate":
		if len(rest) == 0 {
			return wire.Action{}, fmt.Errorf("eval requires script")
		}
		return wire.Action{Type: wire.ActionEvaluate, Script: strings.Join(rest, " ")}, nil
	case "screenshot":
		a := wire.Action{Type: wire.ActionScreenshot}
		if len(rest) > 0 {
			a.SavePath = rest[0]
		}
		return a, nil
	case "cookies-get":
		a := wire.Action{Type: wire.ActionCookiesGet}
		if len(rest) > 0 {
			a.Name = rest[0]
		}
		if len(rest) > 1 {
			a.Domain = rest[1]
		}
		return a, nil
	case "wait-for-selector":
		if len(rest) != 1 {
			return wire.Action{}, fmt.Errorf("wait-for-selector requires selector")
		}
		return wire.Action{Type: wire.ActionWaitForSelector, Selector: rest[0]}, nil
	case "tabs":
		a := wire.Action{Type: wire.ActionTabs, TabAction: "list"}
		if len(rest) > 0 {
			a.TabAction = rest[0]
		}
		return a, nil
	case "scroll":
		a := wire.Action{Type: wire.ActionScroll, Direction: "down"}
		if len(rest) > 0 {
			a.Direction = rest[0]
		}
		return a, nil
	case "keyboard-press":
		if len(rest) != 1 {
			return wire.Action{}, fmt.Errorf("keyboard-press requires key")
		}
		return wire.Action{Type: wire.ActionKeyboardPress, Key: rest[0]}, nil
	case "back":
		return wire.Action{Type: wire.ActionBack}, nil
	}
	return wire.Action{}, fmt.Errorf("unknown subcommand %q", sub)
}

func printNoBrowser(rc *tool.RunContext, asJSON bool, mode string, cause error) int {
	if asJSON {
		return writeJSON(rc, map[string]any{
			"success": false,
			"mode":    mode,
			"error":   noBrowserMessage,
			"cause":   cause.Error(),
		})
	}
	fmt.Fprintf(rc.Err, "browser: %s\n", noBrowserMessage)
	return 1
}

func printResult(rc *tool.RunContext, res *wire.Result, asJSON bool) int {
	if res == nil {
		res = &wire.Result{Error: "no result"}
	}
	if asJSON {
		b, _ := json.Marshal(res)
		fmt.Fprintln(rc.Out, string(b))
		if res.Success {
			return 0
		}
		return 1
	}
	if !res.Success {
		fmt.Fprintf(rc.Err, "browser: %s\n", res.Error)
		return 1
	}
	switch {
	case res.Content != "":
		fmt.Fprintln(rc.Out, res.Content)
	case res.Elements != "":
		fmt.Fprintln(rc.Out, res.Elements)
	case res.Data != "":
		fmt.Fprintln(rc.Out, res.Data)
	case res.Path != "":
		fmt.Fprintln(rc.Out, res.Path)
	case res.Image != "":
		fmt.Fprintln(rc.Out, res.Image)
	default:
		fmt.Fprintln(rc.Out, "ok")
	}
	return 0
}

func writeJSON(rc *tool.RunContext, v any) int {
	b, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(rc.Err, "browser: json: %v\n", err)
		return 1
	}
	fmt.Fprintln(rc.Out, string(b))
	return 0
}
