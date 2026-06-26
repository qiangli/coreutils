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
	Stdio
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
func (rc *RunContext) Path(operand string) string {
	if isAbsPath(operand) || rc.Dir == "" {
		return normalizePath(operand)
	}
	return normalizePath(filepath.Join(rc.Dir, operand))
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
