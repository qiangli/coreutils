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
	"os"
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

	// InvocationName is argv[0] as supplied to the utility. Standalone
	// multicall dispatch sets it to os.Args[0]; embedded callers may leave it
	// empty when the utility has no specified argv[0]-visible behavior.
	InvocationName string

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

	// DedicatedProcess is true only when this invocation owns the lifetime of
	// the current OS process (for example, the standalone multicall binary).
	// Embedded callers must leave it false. Commands whose upstream semantics
	// give special meaning to process ID 0 use this distinction to avoid
	// mutating the long-lived embedding host as though it were a disposable
	// command process.
	DedicatedProcess bool

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

// NativeAbs reports whether operand is an absolute path in the shell's own
// spelling and, if so, returns it in the host's native form — on Windows that
// maps the MSYS drive form (/c/… or \c\…), the WSL mount form (/mnt/c/…),
// the /dev/null and /tmp pseudo-operands, and a drive-relative /foo onto a
// real path. It does no joining with rc.Dir: applets that must keep a
// RELATIVE operand raw for POSIX semantics (rmdir's trailing "." check) use
// it for the absolute case and their own rule for the rest.
func NativeAbs(operand string) (string, bool) {
	if !isAbsPath(operand) {
		return "", false
	}
	return normalizePath(operand), true
}

// IsAbsPath reports whether operand is absolute in the shell's spelling on
// this platform: on Windows that includes a leading slash (/c/x, /tmp/x,
// /usr/bin) which filepath.IsAbs rejects; on Unix it is filepath.IsAbs.
func IsAbsPath(operand string) bool { return isAbsPath(operand) }

// Path resolves operand against the invocation working directory.
// Absolute operands pass through (after conversion to the host's native
// form on Windows). Tools must route every file-system operand through
// this (or equivalent) — never through process cwd.
//
// On Windows an operand in the shell's spelling resolves exactly as
// bashy's interpreter resolves it — the SAME mvdan.cc/sh/v3/pathconv
// converter, with the process mount table: under BASHY_ROOT /tmp is the
// run's %TEMP%, /bin and /usr/bin are root\usr\bin, /etc is root\etc and
// any other /foo is root\foo; the MSYS drive form (/c/…) and the WSL
// mount form (/mnt/c/…) name drives; /dev/null is NUL; without a mount
// table a drive-less /foo lands on the invocation directory's volume. The
// characters NTFS refuses in a filename (: * ? " < > |) are encoded as
// U+F000+c, the Cygwin/MSYS convention, so `touch 'x*x'` creates the file
// the shell's own `>'x*x'` would; DisplayName decodes them on the way
// back out. rc.Dir may itself be in the shell's spelling (an in-process
// applet gets the interpreter's cwd) and is converted before joining.
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
		return normalizePathIn(rc.Dir, operand)
	}
	joined := joinPath(rc.Dir, operand)
	// A terminating separator is semantic, not cosmetic: POSIX pathname
	// resolution requires the preceding component to resolve as a directory.
	// filepath.Join cleans it away, which can turn "symlink/" back into the
	// symlink itself and let an applet's no-follow policy observe the wrong
	// object. Preserve one native separator after joining.
	if hasTrailingPathSeparator(operand) && !hasTrailingPathSeparator(joined) {
		joined += string(filepath.Separator)
	}
	if rc.DirIsProcessCwd && len(joined) > pathLengthLimit {
		return normalizePathIn(rc.Dir, operand)
	}
	return joined
}

// RawPath resolves operand under the invocation directory WITHOUT
// filepath.Join's lexical Clean: every component the caller kept (f/..,
// missing/.., a trailing ".") reaches the kernel, as POSIX pathname
// resolution requires for applets such as rmdir. An absolute operand is
// converted exactly as Path converts it; a relative one is spelled natively
// (on Windows: separators and the NTFS-special encoding, nothing else) and
// appended to NativeDir with one separator. With no directory the operand
// is returned as spelled.
func (rc *RunContext) RawPath(operand string) string {
	if native, ok := NativeAbs(operand); ok {
		return native
	}
	rel := normalizePathIn(rc.Dir, operand)
	if rc.Dir == "" {
		return rel
	}
	dir := rc.NativeDir()
	if hasTrailingPathSeparator(dir) {
		return dir + rel
	}
	return dir + string(filepath.Separator) + rel
}

// NativeDir is the invocation directory in the host's native form. On
// Windows rc.Dir may arrive in the shell's spelling (an in-process applet
// gets the interpreter's /tmp/x or /c/Users/x); an applet that opens or
// stats the directory itself, rather than an operand under it, must resolve
// it through this. Elsewhere it is rc.Dir.
func (rc *RunContext) NativeDir() string {
	return normalizePathIn("", rc.Dir)
}

// DisplayName spells a name read back from the filesystem — a directory
// entry, or an operand echoed in a diagnostic — the way the shell prints
// it. On Windows the U+F000 private-use runes that stand for the
// characters NTFS refuses in a filename (: * ? " < > |, the Cygwin/MSYS
// convention pathconv encodes on the way in) become those characters again,
// so a file the shell created as x*x lists as x*x. Elsewhere it is the
// identity.
func DisplayName(name string) string { return displayName(name) }

func hasTrailingPathSeparator(path string) bool {
	return len(path) > 0 && os.IsPathSeparator(path[len(path)-1])
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
