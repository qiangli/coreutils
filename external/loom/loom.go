// Package loom runs Gitea as a managed external binary (pkg/binmgr) — the git
// forge for the dhnt mesh. The Gitea binary is downloaded → sha256-verified →
// cached by binmgr, never compiled in; bashy ("the OS of binaries") launches it
// via `bashy loom`, and outpost exposes it over the mesh. "loom" keeps ycode's
// name for the gitea-backed forge. See dhnt/docs/external-binary-builtins.md.
package loom

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/binmgr"
)

const (
	// DefaultVersion pins the Gitea release loom runs. "" / "latest" resolves the
	// newest release dynamically; pin for reproducibility ($LOOM_GITEA_VERSION
	// or --gitea-version override). go-gitea/gitea is MIT.
	DefaultVersion = "latest"
	DefaultAddr    = "127.0.0.1"
	DefaultPort    = 3000
)

// Spec is the binmgr GitHub spec loom resolves the Gitea binary from.
func Spec(version string) binmgr.GitHubSpec {
	if version == "" {
		version = DefaultVersion
	}
	return binmgr.GitHubSpec{Name: "loom", Repo: "go-gitea/gitea", Version: version}
}

// DefaultDataDir is loom's work-path (Gitea data + config + repos).
func DefaultDataDir() string {
	if d := os.Getenv("LOOM_DATA_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "loom")
	}
	return filepath.Join(home, ".agents", "bashy", "loom")
}

// Options configures Serve.
type Options struct {
	Version string // Gitea release (default DefaultVersion)
	DataDir string // work-path (default DefaultDataDir)
	Addr    string // HTTP bind addr (default 127.0.0.1 — loopback for the mesh)
	Port    int    // HTTP port (default 3000)
	RootURL string // Gitea public/UI URL (default http://127.0.0.1:<port>/)
	// Actions toggles Gitea's built-in GitHub-Actions-compatible CI. nil = the
	// default (on, unless $LOOM_ACTIONS is "0"/"false"). This is what makes loom
	// a local CI control plane: act_runner registers against it and dials out
	// over the mesh. See dhnt/docs/local-p2p-cicd.md.
	Actions *bool
	Stdout  io.Writer
	Stderr  io.Writer
}

// actionsOn resolves the effective Actions toggle: an explicit Options.Actions
// wins; otherwise on unless $LOOM_ACTIONS is "0"/"false"/"no"/"off".
func (o *Options) actionsOn() bool {
	if o.Actions != nil {
		return *o.Actions
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOOM_ACTIONS"))) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func (o *Options) defaults() {
	if o.Version == "" {
		o.Version = DefaultVersion
	}
	if o.DataDir == "" {
		o.DataDir = DefaultDataDir()
	}
	if o.Addr == "" {
		o.Addr = DefaultAddr
	}
	if o.Port == 0 {
		o.Port = DefaultPort
	}
	if o.RootURL == "" {
		o.RootURL = os.Getenv("LOOM_ROOT_URL")
	}
	if o.RootURL == "" {
		o.RootURL = fmt.Sprintf("http://127.0.0.1:%d/", o.Port)
	}
	if !strings.HasSuffix(o.RootURL, "/") {
		o.RootURL += "/"
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
}

// Instance is a running loom (Gitea) server.
type Instance struct {
	URL     string // http://addr:port it serves on
	Addr    string // addr:port (for `mesh service add git <addr>`)
	Version string // resolved Gitea version
	proc    *binmgr.Process
}

type State struct {
	PID       int       `json:"pid"`
	URL       string    `json:"url"`
	RootURL   string    `json:"root_url"`
	Addr      string    `json:"addr"`
	Version   string    `json:"version"`
	DataDir   string    `json:"data_dir"`
	LogPath   string    `json:"log_path"`
	StartedAt time.Time `json:"started_at"`
}

// Stop terminates the Gitea process gracefully.
func (i *Instance) Stop() error {
	if i == nil || i.proc == nil {
		return nil
	}
	return i.proc.Stop(10 * time.Second)
}

// Start resolves + launches `gitea web` bound to a loopback port (NON-blocking):
// it returns once the server answers (or errors). The config is seeded on first
// run (INSTALL_LOCK, SQLite, a generated SECRET_KEY) so it comes up ready, not on
// the /install screen. The caller owns the returned Instance's lifecycle — this
// is what outpost's wrap-harness builtin supervises and exposes over the mesh.
func Start(ctx context.Context, o Options) (*Instance, error) {
	o.defaults()
	tool, err := binmgr.ResolveGitHub(ctx, Spec(o.Version))
	if err != nil {
		return nil, fmt.Errorf("loom: resolve gitea: %w", err)
	}
	bin, err := binmgr.Ensure(ctx, tool)
	if err != nil {
		return nil, fmt.Errorf("loom: fetch gitea: %w", err)
	}
	cfg, err := ensureConfig(o.DataDir, o.Addr, o.Port, o.RootURL, o.actionsOn())
	if err != nil {
		return nil, err
	}
	addr := fmt.Sprintf("%s:%d", o.Addr, o.Port)
	url := "http://" + addr
	proc, err := binmgr.Launch(ctx, bin, binmgr.RunSpec{
		Args:       []string{"web", "--config", cfg},
		Env:        []string{"GITEA_WORK_DIR=" + o.DataDir},
		Dir:        o.DataDir,
		Stdout:     o.Stdout,
		Stderr:     o.Stderr,
		HealthURL:  url,
		HealthWait: 60 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("loom: start gitea: %w", err)
	}
	return &Instance{URL: url, Addr: addr, Version: tool.Version, proc: proc}, nil
}

// Serve runs loom in the foreground, blocking until the context is cancelled or
// SIGINT/SIGTERM arrives (the `bashy loom serve` path).
func Serve(ctx context.Context, o Options) error {
	o.defaults()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	inst, err := Start(ctx, o)
	if err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "loom (gitea %s) serving on %s — work dir %s\n", inst.Version, inst.URL, o.DataDir)
	fmt.Fprintln(o.Stdout, "expose it over the mesh:  outpost mesh service add git "+inst.Addr)

	<-ctx.Done()
	fmt.Fprintln(o.Stdout, "loom: shutting down…")
	return inst.Stop()
}

func statePath(dataDir string) string { return filepath.Join(dataDir, "loom-state.json") }

func logPath(dataDir string) string { return filepath.Join(dataDir, "loom.log") }

func readState(dataDir string) (State, error) {
	var st State
	data, err := os.ReadFile(statePath(dataDir))
	if err != nil {
		return st, err
	}
	err = json.Unmarshal(data, &st)
	return st, err
}

func writeState(st State) error {
	if err := os.MkdirAll(st.DataDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(st.DataDir), append(data, '\n'), 0o600)
}

func removeState(dataDir string) error {
	err := os.Remove(statePath(dataDir))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func StartDaemon(ctx context.Context, o Options) (State, error) {
	o.defaults()
	if st, err := readState(o.DataDir); err == nil && healthy(ctx, st.URL, 2*time.Second) {
		if st.RootURL == o.RootURL && st.Addr == fmt.Sprintf("%s:%d", o.Addr, o.Port) {
			return st, nil
		}
		_, _ = StopDaemon(o.DataDir, 10*time.Second)
	}
	tool, err := binmgr.ResolveGitHub(ctx, Spec(o.Version))
	if err != nil {
		return State{}, fmt.Errorf("loom: resolve gitea: %w", err)
	}
	bin, err := binmgr.Ensure(ctx, tool)
	if err != nil {
		return State{}, fmt.Errorf("loom: fetch gitea: %w", err)
	}
	cfg, err := ensureConfig(o.DataDir, o.Addr, o.Port, o.RootURL, o.actionsOn())
	if err != nil {
		return State{}, err
	}
	if err := os.MkdirAll(o.DataDir, 0o755); err != nil {
		return State{}, err
	}
	logFile := logPath(o.DataDir)
	log, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return State{}, err
	}
	cmd := exec.Command(bin, "web", "--config", cfg)
	cmd.Dir = o.DataDir
	cmd.Env = append(os.Environ(), "GITEA_WORK_DIR="+o.DataDir)
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = log.Close()
		return State{}, fmt.Errorf("loom: start gitea: %w", err)
	}
	go func() {
		_ = cmd.Wait()
		_ = log.Close()
	}()
	addr := fmt.Sprintf("%s:%d", o.Addr, o.Port)
	st := State{
		PID:       cmd.Process.Pid,
		URL:       "http://" + addr,
		RootURL:   o.RootURL,
		Addr:      addr,
		Version:   tool.Version,
		DataDir:   o.DataDir,
		LogPath:   logFile,
		StartedAt: time.Now().UTC(),
	}
	if err := waitHTTP(ctx, st.URL, 60*time.Second); err != nil {
		_ = cmd.Process.Kill()
		_ = removeState(o.DataDir)
		return State{}, err
	}
	if err := writeState(st); err != nil {
		return State{}, err
	}
	return st, nil
}

func StopDaemon(dataDir string, timeout time.Duration) (State, error) {
	st, err := readState(dataDir)
	if err != nil {
		return st, err
	}
	proc, err := os.FindProcess(st.PID)
	if err != nil {
		return st, err
	}
	_ = proc.Signal(os.Interrupt)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !healthy(context.Background(), st.URL, 500*time.Millisecond) {
			_ = removeState(dataDir)
			return st, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	_ = proc.Kill()
	_ = removeState(dataDir)
	return st, nil
}

func ExposeService(ctx context.Context, service, addr string) error {
	service = strings.TrimSpace(service)
	addr = strings.TrimSpace(addr)
	if service == "" {
		service = "git"
	}
	if addr == "" {
		return fmt.Errorf("loom: no address to expose")
	}
	cmd := exec.CommandContext(ctx, "outpost", "mesh", "service", "add", service, addr)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("loom: expose via outpost: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func waitHTTP(ctx context.Context, url string, max time.Duration) error {
	deadline := time.Now().Add(max)
	for {
		if healthy(ctx, url, 2*time.Second) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("loom: %s not healthy after %s", url, max)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func healthy(ctx context.Context, url string, timeout time.Duration) bool {
	client := http.Client{Timeout: timeout}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode < 500
}

// ensureConfig writes a minimal Gitea app.ini on first run and returns its path.
// On an existing app.ini it reuses it, but reconciles the server URL/listener and
// actions toggle so an already-initialized data dir can be exposed over the mesh
// without manual app.ini surgery.
func ensureConfig(dataDir, addr string, port int, rootURL string, actions bool) (string, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", err
	}
	if err := ensureCustomUI(dataDir); err != nil {
		return "", err
	}
	cfgPath := filepath.Join(dataDir, "app.ini")
	if existing, err := os.ReadFile(cfgPath); err == nil {
		merged := ensureServerSection(string(existing), addr, port, rootURL)
		merged = ensureServiceSection(merged)
		merged = ensureActionsSection(merged, actions)
		if merged != string(existing) {
			if err := os.WriteFile(cfgPath, []byte(merged), 0o600); err != nil {
				return "", err
			}
		}
		return cfgPath, nil // already configured — reuse (with actions reconciled)
	}
	secret, err := randomHex(32)
	if err != nil {
		return "", err
	}
	ini := renderConfig(dataDir, addr, port, rootURL, secret, actions)
	if err := os.WriteFile(cfgPath, []byte(ini), 0o600); err != nil {
		return "", err
	}
	return cfgPath, nil
}

func ensureCustomUI(dataDir string) error {
	headerPath := filepath.Join(dataDir, "custom", "templates", "custom", "header.tmpl")
	if err := os.MkdirAll(filepath.Dir(headerPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(headerPath, []byte(loomHeaderTemplate), 0o644)
}

const loomHeaderTemplate = `<style>
#navbar .navbar-left > a.item[target="_blank"][href="https://docs.gitea.com"] {
	display: none !important;
}

.page-footer {
	display: none !important;
}

.page-content.home .ui.stackable.middle.very.relaxed.page.grid .column > p.large {
	display: none !important;
}
</style>
<script>
document.addEventListener('DOMContentLoaded', () => {
	const logo = document.getElementById('navbar-logo');
	if (!logo) return;
	const appSubUrl = window.config && window.config.appSubUrl;
	if (appSubUrl) {
		logo.href = appSubUrl + '/';
		return;
	}
	const path = window.location.pathname;
	const appMount = '/app/loom/';
	const appAt = path.indexOf(appMount);
	if (appAt >= 0) {
		logo.href = path.slice(0, appAt + appMount.length);
		return;
	}
	if (path === '/loom' || path.startsWith('/loom/')) {
		logo.href = '/loom/';
		return;
	}
	logo.href = '/';
});
</script>
`

func ensureServerSection(ini, addr string, port int, rootURL string) string {
	updates := map[string]string{
		"HTTP_ADDR":   addr,
		"HTTP_PORT":   fmt.Sprintf("%d", port),
		"ROOT_URL":    rootURL,
		"DISABLE_SSH": "true",
	}
	return ensureSectionKeys(ini, "server", updates)
}

func ensureServiceSection(ini string) string {
	return ensureSectionKeys(ini, "service", map[string]string{
		"DISABLE_REGISTRATION":                   "true",
		"ENABLE_REVERSE_PROXY_AUTHENTICATION":    "true",
		"ENABLE_REVERSE_PROXY_AUTO_REGISTRATION": "true",
		"ENABLE_REVERSE_PROXY_EMAIL":             "true",
		"ENABLE_REVERSE_PROXY_FULL_NAME":         "true",
		"REVERSE_PROXY_AUTHENTICATION_USER":      "X-WEBAUTH-USER",
		"REVERSE_PROXY_AUTHENTICATION_EMAIL":     "X-WEBAUTH-EMAIL",
		"REVERSE_PROXY_AUTHENTICATION_FULL_NAME": "X-WEBAUTH-FULLNAME",
		"REVERSE_PROXY_TRUSTED_PROXIES":          "127.0.0.0/8,::1/128",
	})
}

// ensureActionsSection reconciles the [actions] ENABLED key in an existing
// app.ini to want. It rewrites an existing key, or appends the section if absent.
// Minimal INI surgery — Gitea owns the rest of the file, we touch only this key.
func ensureActionsSection(ini string, want bool) string {
	val := "false"
	if want {
		val = "true"
	}
	return ensureSectionKeys(ini, "actions", map[string]string{"ENABLED": val})
}

func ensureSectionKeys(ini, section string, updates map[string]string) string {
	lines := strings.Split(ini, "\n")
	sectionHeader := "[" + section + "]"
	inSection := false
	seenSection := false
	seenKeys := map[string]bool{}
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			if inSection {
				lines = insertMissingSectionKeys(lines, i, updates, seenKeys)
				return strings.Join(lines, "\n")
			}
			inSection = strings.EqualFold(t, sectionHeader)
			if inSection {
				seenSection = true
			}
			continue
		}
		if inSection {
			k := strings.TrimSpace(strings.SplitN(t, "=", 2)[0])
			for wantKey, wantValue := range updates {
				if strings.EqualFold(k, wantKey) {
					lines[i] = wantKey + " = " + wantValue
					seenKeys[wantKey] = true
					break
				}
			}
		}
	}
	if inSection {
		lines = insertMissingSectionKeys(lines, len(lines), updates, seenKeys)
		return strings.Join(lines, "\n")
	}
	var b strings.Builder
	if strings.HasSuffix(ini, "\n") {
		b.WriteString(ini)
	} else {
		b.WriteString(ini)
		b.WriteString("\n")
	}
	if seenSection {
		return b.String()
	}
	fmt.Fprintf(&b, "[%s]\n", section)
	for key, value := range updates {
		fmt.Fprintf(&b, "%s = %s\n", key, value)
	}
	return b.String()
}

func insertMissingSectionKeys(lines []string, at int, updates map[string]string, seen map[string]bool) []string {
	var missing []string
	for key, value := range updates {
		if !seen[key] {
			missing = append(missing, key+" = "+value)
		}
	}
	if len(missing) == 0 {
		return lines
	}
	sort.Strings(missing)
	out := append([]string{}, lines[:at]...)
	out = append(out, missing...)
	out = append(out, lines[at:]...)
	return out
}

// renderConfig is the minimal app.ini: INSTALL_LOCK so it boots ready, SQLite so
// there's no external DB, loopback bind so only the mesh reaches it. Gitea
// auto-generates + persists INTERNAL_TOKEN on first run when it's absent.
func renderConfig(dataDir, addr string, port int, rootURL string, secret string, actions bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "APP_NAME = loom\nRUN_MODE = prod\n\n")
	fmt.Fprintf(&b, "[server]\nHTTP_ADDR = %s\nHTTP_PORT = %d\nROOT_URL = %s\nDISABLE_SSH = true\n\n", addr, port, rootURL)
	fmt.Fprintf(&b, "[database]\nDB_TYPE = sqlite3\nPATH = %s\n\n", filepath.Join(dataDir, "gitea.db"))
	fmt.Fprintf(&b, "[repository]\nROOT = %s\n\n", filepath.Join(dataDir, "repositories"))
	fmt.Fprintf(&b, "[security]\nINSTALL_LOCK = true\nSECRET_KEY = %s\n\n", secret)
	fmt.Fprintf(&b, "[service]\n")
	for _, key := range []string{
		"DISABLE_REGISTRATION",
		"ENABLE_REVERSE_PROXY_AUTHENTICATION",
		"ENABLE_REVERSE_PROXY_AUTO_REGISTRATION",
		"ENABLE_REVERSE_PROXY_EMAIL",
		"ENABLE_REVERSE_PROXY_FULL_NAME",
		"REVERSE_PROXY_AUTHENTICATION_USER",
		"REVERSE_PROXY_AUTHENTICATION_EMAIL",
		"REVERSE_PROXY_AUTHENTICATION_FULL_NAME",
		"REVERSE_PROXY_TRUSTED_PROXIES",
	} {
		fmt.Fprintf(&b, "%s = %s\n", key, serviceDefaults()[key])
	}
	fmt.Fprintln(&b)
	// [actions] turns loom into a local CI control plane: act_runner registers
	// against it and dials out over the mesh. DEFAULT_ACTIONS_URL = github lets
	// workflows reference `uses: actions/checkout` at setup; a mesh-local mirror
	// is a follow-up (docs/local-p2p-cicd.md).
	enabled := "false"
	if actions {
		enabled = "true"
	}
	fmt.Fprintf(&b, "[actions]\nENABLED = %s\nDEFAULT_ACTIONS_URL = github\n", enabled)
	return b.String()
}

func serviceDefaults() map[string]string {
	return map[string]string{
		"DISABLE_REGISTRATION":                   "true",
		"ENABLE_REVERSE_PROXY_AUTHENTICATION":    "true",
		"ENABLE_REVERSE_PROXY_AUTO_REGISTRATION": "true",
		"ENABLE_REVERSE_PROXY_EMAIL":             "true",
		"ENABLE_REVERSE_PROXY_FULL_NAME":         "true",
		"REVERSE_PROXY_AUTHENTICATION_USER":      "X-WEBAUTH-USER",
		"REVERSE_PROXY_AUTHENTICATION_EMAIL":     "X-WEBAUTH-EMAIL",
		"REVERSE_PROXY_AUTHENTICATION_FULL_NAME": "X-WEBAUTH-FULLNAME",
		"REVERSE_PROXY_TRUSTED_PROXIES":          "127.0.0.0/8,::1/128",
	}
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// NewLoomCmd builds the `loom` command tree (bashy front-door + any cobra host).
func NewLoomCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "loom",
		Short: "Run Gitea (the mesh git forge) as a managed external binary",
		Long: `loom runs Gitea — downloaded, sha256-verified, and cached by binmgr (not
compiled in). Expose it over the mesh with: outpost mesh service add git <addr>.`,
	}
	var o Options
	var actions bool
	var expose bool
	var service string
	serve := &cobra.Command{
		Use:   "serve",
		Short: "Download (if needed) + run gitea web on a loopback port",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("actions") {
				o.Actions = &actions
			}
			return Serve(cmd.Context(), o)
		},
	}
	serve.Flags().StringVar(&o.Version, "gitea-version", "", "Gitea release tag (default: latest)")
	serve.Flags().StringVar(&o.DataDir, "data", "", "work dir (default ~/.agents/bashy/loom)")
	serve.Flags().StringVar(&o.Addr, "addr", DefaultAddr, "HTTP bind address")
	serve.Flags().IntVar(&o.Port, "port", DefaultPort, "HTTP port")
	serve.Flags().StringVar(&o.RootURL, "root-url", "", "Gitea ROOT_URL for UI links and clone URLs; use the stable cloudbox HTTPS URL for internet access")
	serve.Flags().BoolVar(&actions, "actions", true, "enable Gitea Actions (local CI; $LOOM_ACTIONS=0 to disable)")

	start := &cobra.Command{
		Use:   "start",
		Short: "Start loom as a detached local service",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("actions") {
				o.Actions = &actions
			}
			st, err := StartDaemon(cmd.Context(), o)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "loom started: %s pid=%d log=%s\n", st.URL, st.PID, st.LogPath)
			if expose {
				if err := ExposeService(cmd.Context(), service, st.Addr); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "loom exposed over outpost mesh: service=%s addr=%s\n", serviceName(service), st.Addr)
				fmt.Fprintf(cmd.OutOrStdout(), "remote peers can run: outpost mesh dial --local %s %s\n", st.Addr, serviceName(service))
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "share over outpost mesh: outpost mesh service add git %s\n", st.Addr)
			}
			return nil
		},
	}
	start.Flags().StringVar(&o.Version, "gitea-version", "", "Gitea release tag (default: latest)")
	start.Flags().StringVar(&o.DataDir, "data", "", "work dir (default ~/.agents/bashy/loom)")
	start.Flags().StringVar(&o.Addr, "addr", DefaultAddr, "HTTP bind address")
	start.Flags().IntVar(&o.Port, "port", DefaultPort, "HTTP port")
	start.Flags().StringVar(&o.RootURL, "root-url", "", "Gitea ROOT_URL for UI links and clone URLs; use the stable cloudbox HTTPS URL for internet access")
	start.Flags().BoolVar(&actions, "actions", true, "enable Gitea Actions (local CI; $LOOM_ACTIONS=0 to disable)")
	start.Flags().BoolVar(&expose, "expose", false, "also publish loom through outpost mesh")
	start.Flags().StringVar(&service, "service", "git", "outpost mesh service name for --expose")

	status := &cobra.Command{
		Use:   "status",
		Short: "Show loom service status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.defaults()
			st, err := readState(o.DataDir)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Fprintln(cmd.OutOrStdout(), "loom stopped")
					return nil
				}
				return err
			}
			state := "stopped"
			if healthy(cmd.Context(), st.URL, 2*time.Second) {
				state = "running"
			}
			rootURL := st.RootURL
			if rootURL == "" {
				rootURL = st.URL + "/"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "loom %s: %s root=%s pid=%d data=%s log=%s\n", state, st.URL, rootURL, st.PID, st.DataDir, st.LogPath)
			if state == "running" {
				fmt.Fprintf(cmd.OutOrStdout(), "share over outpost mesh: outpost mesh service add git %s\n", st.Addr)
				fmt.Fprintf(cmd.OutOrStdout(), "remote peers can run: outpost mesh dial --local %s git\n", st.Addr)
			}
			return nil
		},
	}
	status.Flags().StringVar(&o.DataDir, "data", "", "work dir (default ~/.agents/bashy/loom)")

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the detached loom service",
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.defaults()
			st, err := StopDaemon(o.DataDir, 10*time.Second)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Fprintln(cmd.OutOrStdout(), "loom already stopped")
					return nil
				}
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "loom stopped: %s pid=%d\n", st.URL, st.PID)
			return nil
		},
	}
	stopCmd.Flags().StringVar(&o.DataDir, "data", "", "work dir (default ~/.agents/bashy/loom)")

	logs := &cobra.Command{
		Use:   "logs",
		Short: "Print the loom service log",
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.defaults()
			data, err := os.ReadFile(logPath(o.DataDir))
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(data)
			return err
		},
	}
	logs.Flags().StringVar(&o.DataDir, "data", "", "work dir (default ~/.agents/bashy/loom)")

	exposeCmd := &cobra.Command{
		Use:   "expose",
		Short: "Publish a running loom service through outpost mesh",
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.defaults()
			st, err := readState(o.DataDir)
			if err != nil {
				return err
			}
			if !healthy(cmd.Context(), st.URL, 2*time.Second) {
				return fmt.Errorf("loom: service is not running at %s", st.URL)
			}
			if err := ExposeService(cmd.Context(), service, st.Addr); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "loom exposed over outpost mesh: service=%s addr=%s\n", serviceName(service), st.Addr)
			fmt.Fprintf(cmd.OutOrStdout(), "remote peers can run: outpost mesh dial --local %s %s\n", st.Addr, serviceName(service))
			return nil
		},
	}
	exposeCmd.Flags().StringVar(&o.DataDir, "data", "", "work dir (default ~/.agents/bashy/loom)")
	exposeCmd.Flags().StringVar(&service, "service", "git", "outpost mesh service name")

	pathCmd := &cobra.Command{
		Use:   "path",
		Short: "Print the cached gitea binary path (fetching it if needed)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			tool, err := binmgr.ResolveGitHub(cmd.Context(), Spec(o.Version))
			if err != nil {
				return err
			}
			p, err := binmgr.Ensure(cmd.Context(), tool)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t(gitea %s)\n", p, tool.Version)
			return nil
		},
	}
	pathCmd.Flags().StringVar(&o.Version, "gitea-version", "", "Gitea release tag (default: latest)")

	root.AddCommand(serve, start, status, stopCmd, logs, exposeCmd, pathCmd)
	return root
}

func serviceName(service string) string {
	if strings.TrimSpace(service) == "" {
		return "git"
	}
	return strings.TrimSpace(service)
}
