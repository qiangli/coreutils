package browsercmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestStatusWorksWithoutBrowser(t *testing.T) {
	out, errb, code := runTool(t, "--mode", "probe", "--probe-url", "http://127.0.0.1:1", "status")
	if code != 0 || errb != "" {
		t.Fatalf("status code=%d stderr=%q", code, errb)
	}
	if !strings.Contains(out, "reachable=false") || !strings.Contains(out, "start Chrome") {
		t.Fatalf("unexpected status output: %q", out)
	}
}

func TestStatusJSONWorksWithoutBrowser(t *testing.T) {
	out, _, code := runTool(t, "--json", "--mode", "probe", "--probe-url", "http://127.0.0.1:1", "status")
	if code != 0 {
		t.Fatalf("status json code=%d out=%q", code, out)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	if env["reachable"].(bool) {
		t.Fatalf("expected unreachable: %#v", env)
	}
}

func TestFetchWorksWithoutBrowser(t *testing.T) {
	restore := stubHTTP(t, 200, "hello\n")
	defer restore()

	out, errb, code := runTool(t, "fetch", "https://example.test")
	if code != 0 || errb != "" || out != "hello\n" {
		t.Fatalf("fetch = out=%q err=%q code=%d", out, errb, code)
	}
}

func TestFetchJSON(t *testing.T) {
	restore := stubHTTP(t, 200, "json body")
	defer restore()

	out, _, code := runTool(t, "--json", "fetch", "https://example.test")
	if code != 0 {
		t.Fatalf("fetch json code=%d out=%q", code, out)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	if env["body"] != "json body" || env["status_code"].(float64) != 200 {
		t.Fatalf("unexpected envelope: %#v", env)
	}
}

func stubHTTP(t *testing.T, status int, body string) func() {
	t.Helper()
	old := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}
	return func() { http.DefaultClient = old }
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestActionWithoutBrowserPrintsClearMessage(t *testing.T) {
	_, errb, code := runTool(t, "--mode", "probe", "--probe-url", "http://127.0.0.1:1", "navigate", "https://example.com")
	if code == 0 || !strings.Contains(errb, noBrowserMessage) {
		t.Fatalf("action without browser = code=%d stderr=%q", code, errb)
	}
}

// TestDefaultModeIsSolo guards the zero-setup default: with no --mode, status
// reports solo (reachability then depends on whether a Chrome binary exists,
// so we assert only the mode, never launch a browser).
func TestDefaultModeIsSolo(t *testing.T) {
	out, _, code := runTool(t, "--json", "status")
	if code != 0 {
		t.Fatalf("status code=%d out=%q", code, out)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	if env["mode"] != "solo" {
		t.Fatalf("default mode = %v, want solo", env["mode"])
	}
}

func TestActionFromArgs(t *testing.T) {
	a, err := actionFromArgs([]string{"type", "#q", "hello", "world"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Type != "type" || a.Selector != "#q" || a.Text != "hello world" {
		t.Fatalf("unexpected action: %#v", a)
	}
}

func TestLoginCompletionDetector(t *testing.T) {
	tests := []struct {
		name  string
		spec  loginSpec
		state loginState
		want  string
		value string
	}{
		{
			name:  "redirect",
			spec:  loginSpec{SuccessURL: "/done"},
			state: loginState{URL: "https://example.test/oauth/done?code=1"},
			want:  "redirect",
			value: "https://example.test/oauth/done?code=1",
		},
		{
			name:  "token",
			spec:  loginSpec{TokenSelector: "#token"},
			state: loginState{Token: " abc "},
			want:  "token",
			value: "abc",
		},
		{
			name:  "cookie",
			spec:  loginSpec{Cookie: "sid"},
			state: loginState{Cookies: []loginCookie{{Name: "sid", Value: "secret"}}},
			want:  "cookie",
			value: "secret",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectLoginCompletion(tt.spec, tt.state)
			if !got.Done || got.Reason != tt.want {
				t.Fatalf("unexpected completion: %#v", got)
			}
			switch tt.want {
			case "redirect":
				if got.URL != tt.value {
					t.Fatalf("URL=%q want %q", got.URL, tt.value)
				}
			case "token":
				if got.Token != tt.value {
					t.Fatalf("token=%q want %q", got.Token, tt.value)
				}
			case "cookie":
				if got.Cookie != tt.value {
					t.Fatalf("cookie=%q want %q", got.Cookie, tt.value)
				}
			}
		})
	}
}

func TestLoginDryRun(t *testing.T) {
	out, errb, code := runTool(t, "--dry-run", "--success-url", "/ok", "login", "https://example.test/login")
	if code != 0 || errb != "" {
		t.Fatalf("dry run code=%d err=%q", code, errb)
	}
	if !strings.Contains(out, `success_url="/ok"`) || !strings.Contains(out, `domain="example.test"`) {
		t.Fatalf("unexpected dry run: %q", out)
	}
}
