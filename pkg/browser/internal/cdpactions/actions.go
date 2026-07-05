package cdpactions

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"

	"github.com/qiangli/coreutils/pkg/browser/wire"
)

// Run dispatches one wire action to an already-attached chromedp target.
func Run(ctx context.Context, mode string, action wire.Action) (*wire.Result, error) {
	switch action.Type {
	case wire.ActionNavigate:
		return Navigate(ctx, action.URL)
	case wire.ActionClick:
		return Click(ctx, action)
	case wire.ActionType:
		return Type(ctx, action)
	case wire.ActionScroll:
		return Scroll(ctx, action.Direction, action.Amount)
	case wire.ActionScreenshot:
		return Screenshot(ctx, action)
	case wire.ActionExtract:
		return Extract(ctx, action)
	case wire.ActionBack:
		return Back(ctx)
	case wire.ActionTabs:
		return Tabs(ctx, mode, action)
	case wire.ActionEvaluate:
		return Evaluate(ctx, action.Script)
	case wire.ActionWaitForSelector:
		return WaitForSelector(ctx, action)
	case wire.ActionKeyboardPress:
		return KeyboardPress(ctx, action)
	case wire.ActionCookiesGet:
		return CookiesGet(ctx, action)
	case wire.ActionCapabilities:
		return Capabilities(mode)
	}
	return &wire.Result{Error: fmt.Sprintf("%s: action %q not supported", mode, action.Type)}, nil
}

func Navigate(ctx context.Context, url string) (*wire.Result, error) {
	if url == "" {
		return &wire.Result{Error: "navigate: url required"}, nil
	}
	var title, currentURL, body string
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.Title(&title),
		chromedp.Location(&currentURL),
		chromedp.Text("body", &body, chromedp.NodeVisible),
	)
	if err != nil {
		return &wire.Result{Error: err.Error()}, nil
	}
	return &wire.Result{Success: true, Title: title, URL: currentURL, Content: Truncate(body, 16000)}, nil
}

func Click(ctx context.Context, a wire.Action) (*wire.Result, error) {
	if a.Selector != "" {
		if err := chromedp.Run(ctx, chromedp.Click(a.Selector, chromedp.ByQuery)); err != nil {
			return &wire.Result{Error: err.Error()}, nil
		}
		return &wire.Result{Success: true}, nil
	}
	if a.MatchText != "" {
		var out any
		if err := chromedp.Run(ctx, chromedp.Evaluate(ClickByTextScript(a.MatchText, a.Scope), &out)); err != nil {
			return &wire.Result{Error: err.Error()}, nil
		}
		if b, ok := out.(bool); ok && b {
			return &wire.Result{Success: true}, nil
		}
		return &wire.Result{Error: "click: no element matched match_text"}, nil
	}
	return &wire.Result{Error: "click: selector or match_text required"}, nil
}

func ClickByTextScript(matchText, scope string) string {
	return `(function(){
  var want = ` + JSString(matchText) + `.toLowerCase();
  var scope = ` + JSString(scope) + `;
  var root = scope ? document.querySelector(scope) : document;
  if (!root) return false;
  var nodes = root.querySelectorAll("a, button, input[type=button], input[type=submit], [role='button'], [role='link']");
  for (var i=0; i<nodes.length; i++) {
    var n = nodes[i];
    var v = ((n.innerText) || n.value || n.getAttribute("aria-label") || "").trim().toLowerCase();
    if (v.indexOf(want) >= 0) { n.click(); return true; }
  }
  return false;
})()`
}

func Type(ctx context.Context, a wire.Action) (*wire.Result, error) {
	if a.Selector == "" {
		return &wire.Result{Error: "type: selector required"}, nil
	}
	if err := chromedp.Run(ctx, chromedp.SendKeys(a.Selector, a.Text, chromedp.ByQuery)); err != nil {
		return &wire.Result{Error: err.Error()}, nil
	}
	return &wire.Result{Success: true}, nil
}

func Scroll(ctx context.Context, direction string, amount int) (*wire.Result, error) {
	if amount == 0 {
		amount = 500
	}
	if direction == "up" {
		amount = -amount
	}
	script := fmt.Sprintf("window.scrollBy(0, %d); window.scrollY", amount)
	var y float64
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &y)); err != nil {
		return &wire.Result{Error: err.Error()}, nil
	}
	return &wire.Result{Success: true, Data: fmt.Sprintf("scrollY=%g", y)}, nil
}

func Screenshot(ctx context.Context, a wire.Action) (*wire.Result, error) {
	var buf []byte
	if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&buf)); err != nil {
		return &wire.Result{Error: err.Error()}, nil
	}
	if a.SavePath != "" {
		path, err := writeScreenshot(buf, a.SavePath)
		if err != nil {
			return &wire.Result{Error: err.Error()}, nil
		}
		return &wire.Result{Success: true, Path: path}, nil
	}
	img := base64.StdEncoding.EncodeToString(buf)
	if a.MaxBytes > 0 && len(img) > a.MaxBytes {
		path, err := writeScreenshot(buf, "")
		if err != nil {
			return &wire.Result{Error: err.Error()}, nil
		}
		return &wire.Result{Success: true, Path: path}, nil
	}
	return &wire.Result{Success: true, Image: img}, nil
}

func writeScreenshot(buf []byte, savePath string) (string, error) {
	path := savePath
	if path == "" {
		f, err := os.CreateTemp("", "bashy-browser-*.png")
		if err != nil {
			return "", err
		}
		path = f.Name()
		if _, err := f.Write(buf); err != nil {
			_ = f.Close()
			return "", err
		}
		return path, f.Close()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	return abs, os.WriteFile(abs, buf, 0o644)
}

func Extract(ctx context.Context, a wire.Action) (*wire.Result, error) {
	var title, url string
	if err := chromedp.Run(ctx, chromedp.Title(&title), chromedp.Location(&url)); err != nil {
		return &wire.Result{Error: err.Error()}, nil
	}
	var raw string
	if err := chromedp.Run(ctx, chromedp.Evaluate(ExtractScript(a), &raw)); err != nil {
		return &wire.Result{Error: err.Error()}, nil
	}
	out := ParseExtractPayload(raw)
	out.Title = title
	out.URL = url
	return out, nil
}

func ExtractScript(a wire.Action) string {
	scope := JSString(a.Scope)
	match := JSString(a.MatchText)
	if match == `""` && a.Goal != "" {
		match = JSString(a.Goal)
	}
	limit := a.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := a.Offset
	if offset < 0 {
		offset = 0
	}
	return `(function(){
  var SCOPE_SEL=` + scope + `, MATCH=` + match + `, LIMIT=` + fmt.Sprintf("%d", limit) + `, OFFSET=` + fmt.Sprintf("%d", offset) + `;
  var root = SCOPE_SEL ? document.querySelector(SCOPE_SEL) : document;
  if (!root) return JSON.stringify({error:"extract: scope "+SCOPE_SEL+" not found"});
  var navFilter = !SCOPE_SEL;
  var all = root.querySelectorAll("a, button, input, select, textarea, [role='button'], [role='link']");
  var matches = [];
  for (var i=0; i<all.length; i++) {
    var el = all[i];
    if (navFilter && el.closest && el.closest("nav, aside, [role='navigation'], [role='complementary']")) continue;
    var text = (el.innerText || el.value || el.getAttribute("aria-label") || "").trim();
    if (MATCH && text.toLowerCase().indexOf(MATCH.toLowerCase()) < 0) {
      var ph = el.getAttribute && el.getAttribute("placeholder");
      var ar = el.getAttribute && el.getAttribute("aria-label");
      if (!(ph && ph.toLowerCase().indexOf(MATCH.toLowerCase())>=0) && !(ar && ar.toLowerCase().indexOf(MATCH.toLowerCase())>=0)) continue;
    }
    matches.push(el);
  }
  var total = matches.length;
  var slice = matches.slice(OFFSET, OFFSET+LIMIT);
  var lines = [];
  for (var j=0; j<slice.length; j++) {
    var el = slice[j];
    var tag = el.tagName.toLowerCase();
    var text = (el.innerText || el.value || el.getAttribute("aria-label") || "").trim().slice(0,80);
    var attrs = [];
    var keys = ["type","placeholder","href","name","value","role","aria-label"];
    for (var k=0; k<keys.length; k++) {
      var v = el.getAttribute(keys[k]);
      if (v) attrs.push(keys[k]+"=\""+String(v).slice(0,60)+"\"");
    }
    lines.push("["+(OFFSET+j+1)+"] <"+tag+" "+attrs.join(" ")+">"+text+"</"+tag+">");
  }
  var body = (root === document ? (document.body && document.body.innerText) : root.innerText) || "";
  return JSON.stringify({
    content: body.length>16000 ? body.slice(0,16000)+"\n... (truncated)" : body,
    elements: lines.join("\n"),
    total: total,
    truncated: total > (OFFSET+LIMIT),
  });
})()`
}

func ParseExtractPayload(raw string) *wire.Result {
	var inner struct {
		Content   string `json:"content"`
		Elements  string `json:"elements"`
		Total     int    `json:"total"`
		Truncated bool   `json:"truncated"`
		Error     string `json:"error"`
	}
	_ = json.Unmarshal([]byte(raw), &inner)
	if inner.Error != "" {
		return &wire.Result{Error: inner.Error}
	}
	return &wire.Result{Success: true, Content: inner.Content, Elements: inner.Elements, Total: inner.Total, Truncated: inner.Truncated}
}

func Back(ctx context.Context) (*wire.Result, error) {
	if err := chromedp.Run(ctx, chromedp.NavigateBack()); err != nil {
		return &wire.Result{Error: err.Error()}, nil
	}
	return &wire.Result{Success: true}, nil
}

func Tabs(ctx context.Context, mode string, a wire.Action) (*wire.Result, error) {
	switch a.TabAction {
	case "list", "":
		targets, err := chromedp.Targets(ctx)
		if err != nil {
			return &wire.Result{Error: err.Error()}, nil
		}
		var b strings.Builder
		for i, t := range targets {
			fmt.Fprintf(&b, "[%d] %s\n    %s\n", i+1, t.Title, t.URL)
		}
		return &wire.Result{Success: true, Content: b.String()}, nil
	}
	return &wire.Result{Error: fmt.Sprintf("%s: tab action %q not yet supported", mode, a.TabAction)}, nil
}

func Evaluate(ctx context.Context, script string) (*wire.Result, error) {
	if script == "" {
		return &wire.Result{Error: "evaluate: argument 'script' is required (alias 'expression' also accepted)"}, nil
	}
	var out any
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &out)); err != nil {
		return &wire.Result{Error: err.Error()}, nil
	}
	return &wire.Result{Success: true, Data: fmt.Sprintf("%v", out)}, nil
}

func WaitForSelector(ctx context.Context, a wire.Action) (*wire.Result, error) {
	if a.Selector == "" {
		return &wire.Result{Error: "wait_for_selector: selector required"}, nil
	}
	timeout := time.Duration(a.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	state := strings.ToLower(a.State)
	var err error
	switch state {
	case "", "visible":
		state = "visible"
		err = chromedp.Run(wctx, chromedp.WaitVisible(a.Selector, chromedp.ByQuery))
	case "attached":
		err = chromedp.Run(wctx, chromedp.WaitReady(a.Selector, chromedp.ByQuery))
	case "detached":
		err = chromedp.Run(wctx, chromedp.WaitNotPresent(a.Selector, chromedp.ByQuery))
	default:
		return &wire.Result{Error: fmt.Sprintf("wait_for_selector: unknown state %q (visible|attached|detached)", a.State)}, nil
	}
	if err != nil {
		return &wire.Result{Error: err.Error()}, nil
	}
	return &wire.Result{Success: true, Data: fmt.Sprintf("state=%s", state)}, nil
}

func KeyboardPress(ctx context.Context, a wire.Action) (*wire.Result, error) {
	if a.Key == "" {
		return &wire.Result{Error: "keyboard_press: key required"}, nil
	}
	actions := []chromedp.Action{}
	if a.Selector != "" {
		actions = append(actions, chromedp.Focus(a.Selector, chromedp.ByQuery))
	}
	actions = append(actions, chromedp.KeyEvent(KeyFor(a.Key)))
	if err := chromedp.Run(ctx, actions...); err != nil {
		return &wire.Result{Error: err.Error()}, nil
	}
	return &wire.Result{Success: true, Data: "pressed=" + a.Key}, nil
}

func KeyFor(k string) string {
	switch strings.ToLower(k) {
	case "enter", "return":
		return kb.Enter
	case "tab":
		return kb.Tab
	case "escape", "esc":
		return kb.Escape
	case "backspace":
		return kb.Backspace
	case "delete":
		return kb.Delete
	case "arrowup", "up":
		return kb.ArrowUp
	case "arrowdown", "down":
		return kb.ArrowDown
	case "arrowleft", "left":
		return kb.ArrowLeft
	case "arrowright", "right":
		return kb.ArrowRight
	case "home":
		return kb.Home
	case "end":
		return kb.End
	case "pageup":
		return kb.PageUp
	case "pagedown":
		return kb.PageDown
	}
	return k
}

func CookiesGet(ctx context.Context, a wire.Action) (*wire.Result, error) {
	var cookies []*network.Cookie
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		_ = network.Enable().Do(c)
		var inner error
		cookies, inner = network.GetCookies().Do(c)
		return inner
	}))
	if err != nil {
		return &wire.Result{Error: err.Error()}, nil
	}
	out := FilterCookies(cookies, a.Name, a.Domain)
	raw, _ := json.Marshal(out)
	return &wire.Result{Success: true, Data: string(raw)}, nil
}

type CookieView struct {
	Name           string  `json:"name"`
	Value          string  `json:"value"`
	Domain         string  `json:"domain"`
	Path           string  `json:"path"`
	Secure         bool    `json:"secure"`
	HTTPOnly       bool    `json:"httpOnly"`
	Session        bool    `json:"session"`
	SameSite       string  `json:"sameSite,omitempty"`
	ExpirationDate float64 `json:"expirationDate,omitempty"`
}

func FilterCookies(cookies []*network.Cookie, wantName, wantDomain string) []CookieView {
	out := make([]CookieView, 0, len(cookies))
	for _, c := range cookies {
		if wantName != "" && c.Name != wantName {
			continue
		}
		if wantDomain != "" {
			cd := strings.TrimPrefix(c.Domain, ".")
			if cd != wantDomain && !strings.HasSuffix(wantDomain, "."+cd) && !strings.HasSuffix(cd, "."+wantDomain) {
				continue
			}
		}
		out = append(out, CookieView{
			Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path,
			Secure: c.Secure, HTTPOnly: c.HTTPOnly, Session: c.Session,
			SameSite: string(c.SameSite), ExpirationDate: c.Expires,
		})
	}
	return out
}

func Capabilities(mode string) (*wire.Result, error) {
	caps := map[string]any{
		"mode": mode,
		"methods": []string{
			wire.ActionNavigate,
			wire.ActionClick,
			wire.ActionType,
			wire.ActionScroll,
			wire.ActionScreenshot,
			wire.ActionExtract,
			wire.ActionBack,
			wire.ActionTabs,
			wire.ActionEvaluate,
			wire.ActionWaitForSelector,
			wire.ActionKeyboardPress,
			wire.ActionCookiesGet,
			wire.ActionCapabilities,
		},
	}
	raw, _ := json.Marshal(caps)
	return &wire.Result{Success: true, Data: string(raw)}, nil
}

func JSString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n... (truncated)"
}
