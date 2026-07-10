// Package live is bashy's "live" browser mode — an MV3 Chrome
// extension paired with a Go WebSocket hub, used to drive the user's
// real, logged-in Chrome (cookies, SSO, fingerprint).
//
// The hub (server.go) binds 127.0.0.1:<port> (default 58082) and waits
// for the extension to connect. Once connected, every wire.Action is
// translated into a JSON request, sent over WebSocket, and the response
// is unmarshaled back into a wire.Result.
//
// The extension source lives under ./extension/ and is bundled into the
// binary via go:embed. `bashy browser setup live` extracts the files so
// the user can load them via chrome://extensions → Developer mode →
// Load unpacked.
//
// Migrated from ycode's internal/runtime/mcpservers/live (Apache-2.0);
// only the action↔wire mapping in this file was adapted.
package live

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qiangli/coreutils/pkg/browser/wire"
)

// DefaultPort is the well-known loopback port the live extension
// connects to.
const DefaultPort = 58082

// LiveExtensionMinVersion is the minimum acceptable extension version.
// The hub refuses to dispatch to any older version with an actionable
// "reload at chrome://extensions" error. Reported by the extension's
// `_hello` frame.
const LiveExtensionMinVersion = "0.5.0"

// LiveHandshakeTimeout caps how long hub.call waits for the extension
// to send its _hello frame before treating the connection as too old.
const LiveHandshakeTimeout = 3 * time.Second

// roleKind selects how a Service routes actions: it either owns the hub
// locally (roleHub) or forwards every call to a hub already running in
// another process (roleClient).
type roleKind int

const (
	roleUnset  roleKind = iota
	roleHub             // this process binds 127.0.0.1:<port> and owns the WS
	roleClient          // another process owns the hub; we POST /dispatch
)

// Service is the live-mode backend. It satisfies browser.Client.
type Service struct {
	port int

	mu   sync.Mutex
	role roleKind
	hub  *hub         // populated when role == roleHub
	http *http.Client // populated when role == roleClient
}

// New returns a live-mode service.
func New(port int) *Service {
	if port == 0 {
		port = DefaultPort
	}
	return &Service{port: port}
}

// NewClient returns a ready live Service as a browser.Client. It picks
// the hub/client role automatically (see EnsureReady).
func NewClient(ctx context.Context, port int) (*Service, error) {
	s := New(port)
	if err := s.EnsureReady(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) Port() int { return s.port }

// EnsureReady picks a role based on whether the live port is in use:
//
//   - port free → bind the hub locally
//   - port in use by a live hub → switch to client role and forward
//     every Execute via HTTP POST /dispatch
func (s *Service) EnsureReady(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.role != roleUnset {
		return nil
	}
	if portInUse(s.port) {
		if probeHealth(s.port) {
			s.role = roleClient
			s.http = &http.Client{Timeout: 35 * time.Second}
			slog.Info("live: hub already owned by another process; using client role", "port", s.port)
			return nil
		}
	}
	h := newHub(s.port)
	if err := h.start(ctx); err != nil {
		return err
	}
	s.role = roleHub
	s.hub = h
	return nil
}

// Close stops the hub (hub role) or drops the client (client role).
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch s.role {
	case roleHub:
		err := s.hub.stop(context.Background())
		s.hub = nil
		s.role = roleUnset
		return err
	case roleClient:
		s.http = nil
		s.role = roleUnset
	}
	return nil
}

// RunHub binds the hub and blocks until ctx is cancelled. Used by the
// long-running `bashy browser hub` command so the extension has a stable
// endpoint to connect to across many client dispatches.
func (s *Service) RunHub(ctx context.Context) error {
	if err := s.EnsureReady(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	return s.Close()
}

// Connected reports whether the extension is currently attached.
func (s *Service) Connected() bool {
	s.mu.Lock()
	role := s.role
	h := s.hub
	s.mu.Unlock()
	switch role {
	case roleHub:
		return h != nil && h.connected()
	case roleClient:
		return probeHealth(s.port)
	}
	return false
}

func portInUse(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return true
	}
	_ = ln.Close()
	return false
}

func probeHealth(port int) bool {
	c := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Execute satisfies browser.Client.
func (s *Service) Execute(ctx context.Context, action wire.Action) (*wire.Result, error) {
	s.mu.Lock()
	role := s.role
	h := s.hub
	client := s.http
	s.mu.Unlock()

	method, params, err := actionToParams(action)
	if err != nil {
		return &wire.Result{Error: err.Error()}, nil
	}

	// Hard staleness + per-method gate, only meaningful in roleHub (in
	// roleClient the hub-owning process runs the same checks). Allow
	// `capabilities` through so doctor can introspect a stale extension.
	if role == roleHub && h != nil && action.Type != wire.ActionCapabilities {
		if ver := h.ExtVersion(); ver == "" {
			// Conn up but no _hello yet — awaitHello surfaces the timeout.
		} else if versionLess(ver, LiveExtensionMinVersion) {
			return &wire.Result{Error: staleExtensionError(ver)}, nil
		} else if methods := h.ExtMethods(); len(methods) > 0 && !slices.Contains(methods, method) {
			return &wire.Result{Error: methodNotAdvertisedError(method, ver)}, nil
		}
	}

	// wait-for-selector callers pass timeout_ms; respect it as the outer
	// deadline (plus a buffer) so the call doesn't time out early.
	timeout := 30 * time.Second
	if action.TimeoutMs > 0 {
		t := time.Duration(action.TimeoutMs)*time.Millisecond + 5*time.Second
		if t > timeout {
			timeout = t
		}
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var res *wire.Result
	switch role {
	case roleHub:
		res, err = s.executeHub(callCtx, h, method, params)
	case roleClient:
		res, err = s.executeClient(callCtx, client, method, params)
	default:
		return nil, errors.New("live: not ready (call EnsureReady first)")
	}
	if res != nil && h != nil && res.URL != "" {
		h.RecordLastTab(res.URL)
	}
	return res, err
}

func staleExtensionError(ver string) string {
	return fmt.Sprintf("live: extension stale (v%s < required v%s). "+
		"Reload it at chrome://extensions, or run: bashy browser install-extension",
		ver, LiveExtensionMinVersion)
}

func methodNotAdvertisedError(method, ver string) string {
	return fmt.Sprintf("live: method %q not advertised by extension v%s. "+
		"Reload it at chrome://extensions, or run: bashy browser install-extension",
		method, ver)
}

func (s *Service) executeHub(ctx context.Context, h *hub, method string, params map[string]any) (*wire.Result, error) {
	resp, err := h.call(ctx, method, params)
	if err != nil {
		return &wire.Result{Error: err.Error()}, nil
	}
	if resp.Error != "" {
		return &wire.Result{Error: resp.Error}, nil
	}
	return unmarshalExt(resp.Result)
}

func (s *Service) executeClient(ctx context.Context, c *http.Client, method string, params map[string]any) (*wire.Result, error) {
	body, err := json.Marshal(map[string]any{"method": method, "params": params})
	if err != nil {
		return &wire.Result{Error: err.Error()}, nil
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/dispatch", s.port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return &wire.Result{Error: err.Error()}, nil
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return &wire.Result{Error: fmt.Sprintf("live: dispatch to hub: %v", err)}, nil
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return &wire.Result{Error: fmt.Sprintf("live: hub returned %d: %s", resp.StatusCode, string(rawBody))}, nil
	}
	var dispatchResp struct {
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(rawBody, &dispatchResp); err != nil {
		return &wire.Result{Error: fmt.Sprintf("live: bad dispatch payload: %v", err)}, nil
	}
	if dispatchResp.Error != "" {
		return &wire.Result{Error: dispatchResp.Error}, nil
	}
	return unmarshalExt(dispatchResp.Result)
}

func unmarshalExt(raw json.RawMessage) (*wire.Result, error) {
	var inner extResult
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &inner); err != nil {
			return &wire.Result{Error: fmt.Sprintf("live: bad result payload: %v", err)}, nil
		}
	}
	return &wire.Result{
		Success:   true,
		Title:     inner.Title,
		URL:       inner.URL,
		Content:   inner.Content,
		Elements:  inner.Elements,
		Data:      inner.Data,
		Image:     inner.Image,
		Path:      inner.Path,
		Total:     inner.Total,
		Truncated: inner.Truncated,
	}, nil
}

// versionLess returns true when a < b using a dotted-numeric compare.
func versionLess(a, b string) bool {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var an, bn int
		if i < len(as) {
			an, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bn, _ = strconv.Atoi(bs[i])
		}
		if an < bn {
			return true
		}
		if an > bn {
			return false
		}
	}
	return false
}

// actionToParams translates a wire.Action into a {method, params} pair
// for the WebSocket protocol. Keep in sync with the extension's
// background.js dispatch table.
func actionToParams(a wire.Action) (string, map[string]any, error) {
	switch a.Type {
	case wire.ActionNavigate:
		return "navigate", map[string]any{"url": a.URL}, nil
	case wire.ActionClick:
		return "click", map[string]any{
			"selector":   a.Selector,
			"element_id": a.ElementID,
			"match_text": a.MatchText,
			"scope":      a.Scope,
		}, nil
	case wire.ActionType:
		return "type", map[string]any{"selector": a.Selector, "element_id": a.ElementID, "text": a.Text}, nil
	case wire.ActionScroll:
		return "scroll", map[string]any{"direction": a.Direction, "amount": a.Amount}, nil
	case wire.ActionScreenshot:
		return "screenshot", map[string]any{}, nil
	case wire.ActionExtract:
		return "extract", map[string]any{
			"goal":       a.Goal,
			"match_text": a.MatchText,
			"scope":      a.Scope,
			"limit":      a.Limit,
			"offset":     a.Offset,
		}, nil
	case wire.ActionBack:
		return "back", map[string]any{}, nil
	case wire.ActionTabs:
		return "tabs", map[string]any{"action": a.TabAction, "tab_id": a.TabID}, nil
	case wire.ActionEvaluate:
		return "evaluate", map[string]any{"script": a.Script}, nil
	case wire.ActionWaitForSelector:
		return "wait_for_selector", map[string]any{
			"selector":   a.Selector,
			"timeout_ms": a.TimeoutMs,
			"state":      a.State,
		}, nil
	case wire.ActionKeyboardPress:
		return "keyboard_press", map[string]any{
			"key":       a.Key,
			"modifiers": a.Modifiers,
			"selector":  a.Selector,
		}, nil
	case wire.ActionClipboardRead:
		return "clipboard_read", map[string]any{}, nil
	case wire.ActionClipboardWrite:
		return "clipboard_write", map[string]any{"text": a.Text}, nil
	case wire.ActionCookiesGet:
		return "cookies_get", map[string]any{"name": a.Name, "domain": a.Domain}, nil
	case wire.ActionStorageGet:
		return "storage_get", map[string]any{"storage": a.Storage, "key": a.StorageKey}, nil
	case wire.ActionCapabilities:
		return "capabilities", map[string]any{}, nil
	case wire.ActionNetworkList:
		return "network_list", map[string]any{}, nil
	case wire.ActionConsoleGet:
		return "console_get", map[string]any{}, nil
	case wire.ActionPerfStart:
		return "perf_start", map[string]any{}, nil
	case wire.ActionPerfStop:
		return "perf_stop", map[string]any{}, nil
	case wire.ActionLighthouse:
		return "lighthouse", map[string]any{}, nil
	}
	return "", nil, fmt.Errorf("live: action %q not supported", a.Type)
}
