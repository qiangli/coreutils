// Package who owns bashy's login-accounting database for agent sessions.
//
// The database deliberately uses the text format understood by
// cmds/internal/session.  Register returns a handle whose Close method removes
// only the record owned by that PID, so the usual lifecycle is:
//
//	h, err := who.Register(who.Record{...})
//	if err != nil { ... }
//	defer h.Close()
package who

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qiangli/coreutils/pkg/lockfile"
)

const (
	// FileEnv overrides the agent session database.  It primarily gives tests
	// and embedded bashy instances an isolated database.
	FileEnv             = "BASHY_WHO_FILE"
	generatedPlanMarker = "--- observed by bashy, not self-reported, as of "
)

// Record is one live login. Surfaces become who's COMMENT field (for example
// mb,meet,bus), so a reader knows where the name can actually be reached.
type Record struct {
	Name     string
	TTY      string
	ID       string
	PID      int
	Started  time.Time
	Surfaces []string
}

// Handle owns one registered record. Close is idempotent.
type Handle struct {
	path string
	name string
	pid  int
	once sync.Once
	err  error
}

// File returns the bashy-owned login database for the process environment.
func File() string { return FileForEnv(os.Environ()) }

// FileForEnv returns the bashy-owned login database for env.
func FileForEnv(env []string) string {
	if p, ok := envValue(env, FileEnv); ok && strings.TrimSpace(p) != "" {
		return filepath.Clean(p)
	}
	home, _ := envValue(env, "HOME")
	home = strings.TrimSpace(home)
	if home == "" {
		home, _ = envValue(env, "USERPROFILE")
		home = strings.TrimSpace(home)
	}
	if home == "" {
		drive, _ := envValue(env, "HOMEDRIVE")
		rel, _ := envValue(env, "HOMEPATH")
		home = strings.TrimSpace(drive + rel)
	}
	if home == "" {
		return filepath.Join(os.TempDir(), "bashy-who", "sessions")
	}
	return filepath.Join(home, ".bashy", "who", "sessions")
}

// UserDirForEnv returns the durable directory entry associated with name.
// Invalid names have no directory: a login name may never escape the who root.
func UserDirForEnv(env []string, name string) (string, bool) {
	if !safeName(name) {
		return "", false
	}
	return filepath.Join(filepath.Dir(FileForEnv(env)), name), true
}

// Register writes a live record and creates its durable default page. The
// caller must Close the returned handle when the session ends.
func Register(r Record) (*Handle, error) {
	return RegisterFile(File(), r)
}

// RegisterContext registers r for the lifetime of ctx. Cancellation performs
// the same PID-checked deregistration as Handle.Close; callers may still defer
// Close as an additional safeguard.
func RegisterContext(ctx context.Context, r Record) (*Handle, error) {
	if ctx == nil {
		return nil, errors.New("who: nil session context")
	}
	h, err := Register(r)
	if err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		_ = h.Close()
	}()
	return h, nil
}

// RegisterFile is Register against an explicit database. It is useful to an
// embedding launcher which supplies its own state root.
func RegisterFile(path string, r Record) (*Handle, error) {
	if err := normalizeRecord(&r); err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("who: empty session database path")
	}
	path = filepath.Clean(path)
	if err := update(path, func(lines []string) []string {
		lines = prune(lines)
		out := make([]string, 0, len(lines)+1)
		for _, line := range lines {
			name, pid, ok := lineOwner(line)
			if ok && name == r.Name && pid == r.PID {
				continue
			}
			out = append(out, line)
		}
		return append(out, formatRecord(r))
	}); err != nil {
		return nil, err
	}
	if err := writeGeneratedPlan(path, r, true); err != nil {
		_ = RemoveFile(path, r.Name, r.PID)
		return nil, err
	}
	return &Handle{path: path, name: r.Name, pid: r.PID}, nil
}

// Close deregisters this handle's session without disturbing another process
// using the same login name.
func (h *Handle) Close() error {
	if h == nil {
		return nil
	}
	h.once.Do(func() { h.err = RemoveFile(h.path, h.name, h.pid) })
	return h.err
}

// Remove removes the record for name held by pid from the default database.
func Remove(name string, pid int) error { return RemoveFile(File(), name, pid) }

// RemoveFile removes only the named PID. It also marks a generated default
// page offline; a self-published .plan is never changed.
func RemoveFile(path, name string, pid int) error {
	if !safeName(name) || pid <= 0 {
		return errors.New("who: invalid session owner")
	}
	path = filepath.Clean(path)
	err := update(path, func(lines []string) []string {
		out := make([]string, 0, len(lines))
		for _, line := range prune(lines) {
			lineName, linePID, ok := lineOwner(line)
			if ok && lineName == name && linePID == pid {
				continue
			}
			out = append(out, line)
		}
		return out
	})
	if err != nil {
		return err
	}
	return markOfflineIfNoLive(path, name)
}

// ReadLive reads path under the writer lock and removes records whose PIDs no
// longer exist. Reading is reconciliation; no sweeper is required.
func ReadLive(path string) ([]byte, error) {
	path = filepath.Clean(path)
	var data []byte
	staleNames := make(map[string]bool)
	err := withLock(path, func() error {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := splitLines(b)
		for _, line := range lines {
			name, pid, ok := lineOwner(line)
			if ok && !pidAlive(pid) {
				staleNames[name] = true
			}
		}
		live := prune(lines)
		if len(live) != len(lines) {
			if err := writeLines(path, live); err != nil {
				return err
			}
			b = encodeLines(live)
		}
		data = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, line := range splitLines(data) {
		name, _, ok := lineOwner(line)
		if ok {
			delete(staleNames, name)
		}
	}
	for name := range staleNames {
		if err := markOfflineIfNoLive(path, name); err != nil {
			return nil, err
		}
	}
	return data, nil
}

func normalizeRecord(r *Record) error {
	r.Name = strings.TrimSpace(r.Name)
	r.TTY = strings.TrimSpace(r.TTY)
	r.ID = strings.TrimSpace(r.ID)
	if !safeName(r.Name) {
		return errors.New("who: invalid login name")
	}
	if r.TTY == "" {
		r.TTY = "pty/" + r.Name
	}
	if !safeToken(r.TTY) {
		return errors.New("who: invalid terminal name")
	}
	if r.ID == "" {
		r.ID = r.Name
	}
	if !safeToken(r.ID) {
		return errors.New("who: invalid session id")
	}
	if r.PID <= 0 {
		return errors.New("who: pid must be positive")
	}
	if r.Started.IsZero() {
		r.Started = time.Now()
	}
	seen := make(map[string]bool)
	var surfaces []string
	for _, surface := range r.Surfaces {
		for _, part := range strings.Split(surface, ",") {
			part = strings.TrimSpace(part)
			if part == "" || seen[part] || !safeToken(part) {
				continue
			}
			seen[part] = true
			surfaces = append(surfaces, part)
		}
	}
	sort.SliceStable(surfaces, func(i, j int) bool {
		ri, rj := surfaceRank(surfaces[i]), surfaceRank(surfaces[j])
		if ri != rj {
			return ri < rj
		}
		return surfaces[i] < surfaces[j]
	})
	if len(surfaces) == 0 {
		return errors.New("who: at least one live surface is required")
	}
	r.Surfaces = surfaces
	return nil
}

func surfaceRank(surface string) int {
	switch surface {
	case "mb":
		return 0
	case "meet":
		return 1
	case "bus":
		return 2
	default:
		return 3
	}
}

func formatRecord(r Record) string {
	return fmt.Sprintf("%s %s %d %s user id=%s pid=%d", r.Name, r.TTY,
		r.Started.Unix(), strings.Join(r.Surfaces, ","), r.ID, r.PID)
}

func safeToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

func safeName(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func lineOwner(line string) (string, int, bool) {
	f := strings.Fields(line)
	if len(f) < 6 || strings.HasPrefix(f[0], "#") {
		return "", 0, false
	}
	for _, field := range f[5:] {
		if value, ok := strings.CutPrefix(field, "pid="); ok {
			pid, err := strconv.Atoi(value)
			return f[0], pid, err == nil && pid > 0
		}
	}
	return "", 0, false
}

func prune(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			out = append(out, line)
			continue
		}
		_, pid, ok := lineOwner(line)
		if !ok || !pidAlive(pid) {
			continue
		}
		out = append(out, line)
	}
	return out
}

func update(path string, mutate func([]string) []string) error {
	return withLock(path, func() error {
		b, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		return writeLines(path, mutate(splitLines(b)))
	})
}

func withLock(path string, fn func() error) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	l, err := lockfile.Acquire(path+".lock", lockfile.Holder{Name: "who", Intent: "update live sessions"})
	if err != nil {
		return err
	}
	defer l.Release()
	return fn()
}

func splitLines(data []byte) []string {
	text := strings.TrimRight(string(data), "\r\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func encodeLines(lines []string) []byte {
	if len(lines) == 0 {
		return nil
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func writeLines(path string, lines []string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(encodeLines(lines)); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func writeGeneratedPlan(path string, r Record, online bool) error {
	dir, ok := userDir(path, r.Name)
	if !ok {
		return errors.New("who: invalid page directory")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	plan := filepath.Join(dir, ".plan")
	prior, err := os.ReadFile(plan)
	if err == nil && !bytes.HasPrefix(prior, []byte(generatedPlanMarker)) {
		return nil // self-published: never rewrite or merge it
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	status := "OFFLINE"
	if online {
		status = "ONLINE"
	}
	stamp := time.Now().UTC().Format(time.RFC3339)
	body := fmt.Sprintf("%s%s ---\nstatus: %s\nsurfaces: %s\n", generatedPlanMarker,
		stamp, status, strings.Join(r.Surfaces, ","))
	if contains(r.Surfaces, "mb") {
		body += fmt.Sprintf("reach: bashy mb send %s \"...\"\n", r.Name)
	}
	return os.WriteFile(plan, []byte(body), 0o600)
}

func markGeneratedPlanOffline(path, name string) error {
	dir, ok := userDir(path, name)
	if !ok {
		return nil
	}
	plan := filepath.Join(dir, ".plan")
	b, err := os.ReadFile(plan)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !bytes.HasPrefix(b, []byte(generatedPlanMarker)) {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) == 0 {
		return nil
	}
	lines[0] = generatedPlanMarker + time.Now().UTC().Format(time.RFC3339) + " ---"
	for i, line := range lines {
		if strings.HasPrefix(line, "status:") {
			lines[i] = "status: OFFLINE"
		}
	}
	return os.WriteFile(plan, encodeLines(lines), 0o600)
}

func markOfflineIfNoLive(path, name string) error {
	return withLock(path, func() error {
		b, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		for _, line := range prune(splitLines(b)) {
			lineName, _, ok := lineOwner(line)
			if ok && lineName == name {
				return nil
			}
		}
		return markGeneratedPlanOffline(path, name)
	})
}

func userDir(path, name string) (string, bool) {
	if !safeName(name) {
		return "", false
	}
	return filepath.Join(filepath.Dir(path), name), true
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func envValue(env []string, key string) (string, bool) {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return env[i][len(prefix):], true
		}
	}
	return "", false
}
