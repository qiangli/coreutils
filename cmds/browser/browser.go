package browsercmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/browser"
	"github.com/qiangli/coreutils/pkg/browser/probe"
	"github.com/qiangli/coreutils/pkg/browser/wire"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "browser",
	Synopsis: "Browser automation: status, fetch, and CDP-backed page actions.",
	Usage:    "browser [--json] [--mode probe] [--probe-url URL] status|fetch|navigate|extract|click|type|eval|screenshot|cookies-get|wait-for-selector|tabs|scroll|keyboard-press|back ...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

const noBrowserMessage = "no browser: start Chrome with --remote-debugging-port=9222 or run `bashy browser login`"

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	asJSON := fs.Bool("json", false, "emit JSON result envelopes")
	mode := fs.String("mode", "probe", "browser mode: probe")
	probeURL := fs.String("probe-url", probe.DefaultURL, "Chrome remote debugging URL for probe mode")
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
		return browserStatus(rc, ctx, *mode, *probeURL, *asJSON)
	case "fetch":
		return browserFetch(rc, ctx, operands[1:], *asJSON)
	}

	action, err := actionFromArgs(operands)
	if err != nil {
		return tool.UsageError(rc, cmd, "%s", err)
	}
	client, err := clientForMode(ctx, *mode, *probeURL)
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

func clientForMode(ctx context.Context, mode, probeURL string) (browser.Client, error) {
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
	default:
		return nil, fmt.Errorf("mode %q is not supported", mode)
	}
}

func browserStatus(rc *tool.RunContext, ctx context.Context, mode, probeURL string, asJSON bool) int {
	reachable := false
	message := ""
	if mode == "" || mode == "probe" {
		c := probe.New(probeURL)
		reachable = c.Available(ctx)
		if !reachable {
			message = noBrowserMessage
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
