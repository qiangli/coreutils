// Package tool is the framework every command in this repository is
// built on: a registry of named tools, a process-free invocation
// context (stdio + working directory + environment, no os globals),
// and a strict GNU-style flag layer (flags.go).
//
// Tools are embeddable: consumers (the multicall binary, the
// mvdan.cc/sh/v3 ExecHandler adapter, outpost, ycode) construct a
// RunContext and call Tool.Run — no process is spawned, no global
// state is read. That is what makes one identical toolset possible on
// every platform.
package tool

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Stdio is the three standard streams for one invocation.
type Stdio struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// RunContext carries everything a tool invocation may consult. Tools
// MUST NOT read os.Stdin/os.Stdout/os.Getwd/os.Environ directly — the
// embedding shell owns those, and its cwd/env routinely differ from
// the process's.
type RunContext struct {
	Ctx context.Context
	Dir string   // working directory; resolve every relative operand against it
	Env []string // os.Environ() shape ("KEY=VALUE"); nil = empty environment
	FS  *LocalFS // local OS filesystem with path translation; never nil in practice
	Stdio

	// Umask is the embedding shell's virtual file-creation mask. UmaskSet is
	// false for standalone tools, where the child process's inherited OS umask
	// remains authoritative. In-process adapters set both fields so creation
	// helpers can honor the shell without changing process-global state.
	Umask    fs.FileMode
	UmaskSet bool

	// SIGPIPEIgnored is true if the calling shell ignores SIGPIPE (e.g. trap '' PIPE).
	SIGPIPEIgnored bool

	// DirIsProcessCwd is the host's guarantee that Dir IS this process's
	// working directory for the whole invocation (true for the standalone
	// multicall binary, which inherits its cwd exactly as a GNU tool
	// does; false for an embedding shell, whose virtual cwd routinely
	// differs from the process's). Under that guarantee Path may hand a
	// relative operand to the OS unjoined when materializing Dir+operand
	// would overrun the platform's path-length limit: the kernel then
	// resolves it against the process cwd — the same lookup an execve'd
	// GNU tool performs, which never builds that string at all.
	DirIsProcessCwd bool

	// ExitSignal is the process-boundary channel for a command wrapper
	// (env, timeout, …) that ran a COMMAND which was terminated by a
	// signal. When non-zero after Run returns, it is that signal's number.
	//
	// It exists because those wrappers fork-and-wait rather than
	// execve-replacing the caller (self-replace would kill the embedding
	// host — the whole point of this repo), so a signal that killed COMMAND
	// cannot reach whatever waits on the wrapper the way it would for GNU's
	// execve-based env. Run still returns the safe 128+N exit code for every
	// caller. A *standalone* process boundary (multicall.Main) may
	// additionally restore that signal's default disposition and re-raise it
	// on itself so its own wait status matches COMMAND's (WIFSIGNALED with
	// the same signal, and a core dump for the core-producing signals),
	// exactly as an execve-replacing env would. An EMBEDDED host must ignore
	// this field and use the returned exit code: it must never signal the
	// host process.
	ExitSignal int
}

// Getenv looks up key in rc.Env (last assignment wins, matching how a
// real environment behaves when built by appending).
func (rc *RunContext) Getenv(key string) string {
	prefix := key + "="
	for i := len(rc.Env) - 1; i >= 0; i-- {
		if strings.HasPrefix(rc.Env[i], prefix) {
			return rc.Env[i][len(prefix):]
		}
	}
	return ""
}

// Path resolves operand against the invocation working directory.
// Absolute operands pass through (after separator normalization on
// Windows). Tools must route every file-system operand through this
// (or equivalent) — never through process cwd.
//
// On Windows, both / and \ separators are accepted, and /foo (no drive
// letter) is recognised as drive-relative absolute (matching the
// behaviour of every Windows API, which treats it as root on the
// current drive).
//
// A valid working directory and a valid relative operand can join into
// a single string longer than the platform's path-length limit (each
// was built and is resolvable one component at a time — a shell's cd
// and a near-PATH_MAX relative pathname are both legitimate), and the
// OS would reject that materialized string with ENAMETOOLONG even
// though the file is plainly reachable. When the host has declared
// DirIsProcessCwd, Path keeps such an operand relative instead, so the
// kernel resolves it against the process cwd exactly as it would for
// the GNU binary. Below the limit the joined absolute form is returned
// as always.
func (rc *RunContext) Path(operand string) string {
	if isAbsPath(operand) || rc.Dir == "" {
		return normalizePath(operand)
	}
	joined := normalizePath(filepath.Join(rc.Dir, operand))
	if rc.DirIsProcessCwd && len(joined) > pathLengthLimit {
		return normalizePath(operand)
	}
	return joined
}

// ResolveExecutable resolves name as an executable file against the
// working directory. On Windows this tries PATHEXT suffixes (.exe,
// .com, .bat, .cmd) in order; on other platforms it returns
// rc.Path(name).
func (rc *RunContext) ResolveExecutable(name string) string {
	return resolveExecutable(rc, name)
}

// Tool is one command.
type Tool struct {
	// Name is the command name exactly as upstream spells it.
	Name string
	// Synopsis is the one-line description shown in tool listings.
	Synopsis string
	// Usage is the "Usage: …" block printed by --help, above the flag
	// list. Multi-line allowed; no trailing newline required.
	Usage string
	// Run executes the tool and returns its exit code (GNU
	// conventions: 0 success, 1 failure, 2 usage error).
	Run func(rc *RunContext, args []string) int
}

var (
	mu       sync.RWMutex
	registry = map[string]*Tool{}
)

// Register adds t to the registry. Panics on duplicates or empty
// names — both are programmer errors caught at init time.
func Register(t *Tool) {
	if t == nil || t.Name == "" || t.Run == nil {
		panic("tool: Register with empty Name or nil Run")
	}
	mu.Lock()
	defer mu.Unlock()
	if _, dup := registry[t.Name]; dup {
		panic(fmt.Sprintf("tool: duplicate registration of %q", t.Name))
	}
	registry[t.Name] = t
}

// Lookup returns the named tool, or nil.
func Lookup(name string) *Tool {
	mu.RLock()
	defer mu.RUnlock()
	return registry[name]
}

// Names returns all registered tool names, sorted.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
