// Package xargscmd implements a pure-Go GNU-subset xargs: read items from
// standard input and run a command with them as arguments.
//
// Structure adapted from https://github.com/u-root/u-root cmds/core/xargs
// (BSD-3-Clause); extended here for GNU compatibility with -I (replace-str),
// -P (parallel), -r (no-run-if-empty), -E (eof-str), -d (delimiter), and GNU
// default quote/backslash word splitting. Unsupported options fail loudly
// rather than silently mis-behave.
//
// Default input splitting is on blanks/newlines with single/double quotes and
// backslash escapes honored (GNU default); -0 reads NUL-delimited items and -d
// splits on a literal delimiter, both disabling quote processing. The child's
// stdin is the null device (it does not inherit xargs's consumed input).
package xargscmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "xargs",
	Synopsis: "Build and run command lines from standard input (GNU-subset xargs).",
	Usage:    "xargs [-0rt] [-n MAX] [-I REPL] [-P N] [-E EOF] [-d DELIM] [command [initial-args...]]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type options struct {
	null       bool
	noRunEmpty bool
	trace      bool
	maxArgs    int // <=0 means unlimited (one batch)
	replace    string
	maxProcs   int
	eof        string
	delim      string // raw -d value (pre-unescape); "" = unset
}

func run(rc *tool.RunContext, args []string) int {
	o := options{maxArgs: -1, maxProcs: 1}

	// Hand-parse xargs options up to the first non-flag (the command), so the
	// command's own flags are never consumed as ours (the wrapper rule).
	i := 0
	val := func() (string, bool) {
		if i+1 >= len(args) {
			return "", false
		}
		i++
		return args[i], true
	}
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			i++
			break
		}
		if a == "" || a[0] != '-' || a == "-" {
			break
		}
		switch {
		case a == "-0" || a == "--null":
			o.null = true
		case a == "-r" || a == "--no-run-if-empty":
			o.noRunEmpty = true
		case a == "-t" || a == "--verbose":
			o.trace = true
		case a == "-p" || a == "--interactive":
			return tool.UsageError(rc, cmd, "-p/--interactive is not supported (no controlling terminal)")
		case a == "-n" || a == "--max-args":
			v, ok := val()
			if !ok {
				return tool.UsageError(rc, cmd, "option %s requires an argument", a)
			}
			o.maxArgs = atoiOr(v)
		case strings.HasPrefix(a, "--max-args="):
			o.maxArgs = atoiOr(a[len("--max-args="):])
		case strings.HasPrefix(a, "-n") && len(a) > 2:
			o.maxArgs = atoiOr(a[2:])
		case a == "-I" || a == "--replace" || a == "-i":
			if v, ok := val(); ok {
				o.replace = v
			} else {
				o.replace = "{}"
			}
		case strings.HasPrefix(a, "-I") && len(a) > 2:
			o.replace = a[2:]
		case strings.HasPrefix(a, "--replace="):
			o.replace = a[len("--replace="):]
		case a == "-P" || a == "--max-procs":
			v, ok := val()
			if !ok {
				return tool.UsageError(rc, cmd, "option %s requires an argument", a)
			}
			o.maxProcs = atoiOr(v)
		case strings.HasPrefix(a, "-P") && len(a) > 2:
			o.maxProcs = atoiOr(a[2:])
		case a == "-E" || a == "--eof":
			if v, ok := val(); ok {
				o.eof = v
			}
		case strings.HasPrefix(a, "-E") && len(a) > 2:
			o.eof = a[2:]
		case strings.HasPrefix(a, "--eof="):
			o.eof = a[len("--eof="):]
		case strings.HasPrefix(a, "-e"): // GNU deprecated alias for -E[str]
			o.eof = a[2:]
		case a == "-d" || a == "--delimiter":
			v, ok := val()
			if !ok {
				return tool.UsageError(rc, cmd, "option %s requires an argument", a)
			}
			o.delim = v
		case strings.HasPrefix(a, "-d") && len(a) > 2:
			o.delim = a[2:]
		case strings.HasPrefix(a, "--delimiter="):
			o.delim = a[len("--delimiter="):]
		default:
			return tool.UsageError(rc, cmd, "unknown option %q", a)
		}
	}
	if o.maxArgs != -1 && o.maxArgs < 1 { // -1 = unlimited; otherwise positive
		return tool.UsageError(rc, cmd, "-n requires a positive number")
	}
	if o.maxProcs == -2 {
		return tool.UsageError(rc, cmd, "-P requires a non-negative number")
	}

	command := args[i:]
	if len(command) == 0 {
		command = []string{"echo"}
	}

	items, err := readItems(rc.In, o)
	if err != nil {
		fmt.Fprintf(rc.Err, "xargs: %v\n", err)
		return 1
	}

	// Build the batches of invocations.
	batches, err := plan(command, items, o)
	if err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
	}
	if len(batches) == 0 {
		return 0 // empty input + (-r or -I): nothing to run
	}

	return execBatches(rc, batches, o)
}

// atoiOr returns the parsed int, or the sentinel -2 on a malformed value (which
// the post-parse validation rejects loudly).
func atoiOr(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return -2
	}
	return n
}

// readItems splits stdin into items per the delimiter rules.
func readItems(r io.Reader, o options) ([]string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var items []string
	switch {
	case o.null:
		items = splitOn(string(data), '\x00')
	case o.delim != "":
		d := unescapeDelim(o.delim)
		items = splitOn(string(data), d)
	default:
		items = splitQuoted(string(data))
	}
	// Honor a logical EOF string: stop at the first item equal to it.
	if o.eof != "" {
		for k, it := range items {
			if it == o.eof {
				items = items[:k]
				break
			}
		}
	}
	return items, nil
}

func splitOn(s string, delim rune) []string {
	var out []string
	for part := range strings.SplitSeq(s, string(delim)) {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// splitQuoted implements GNU xargs default word splitting: blanks/newlines
// separate items; single and double quotes group; backslash escapes the next
// character (outside quotes).
func splitQuoted(s string) []string {
	var items []string
	var cur strings.Builder
	inItem := false
	flush := func() {
		if inItem {
			items = append(items, cur.String())
			cur.Reset()
			inItem = false
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			flush()
		case '\\':
			if i+1 < len(s) {
				i++
				cur.WriteByte(s[i])
				inItem = true
			}
		case '\'':
			inItem = true
			for i++; i < len(s) && s[i] != '\''; i++ {
				cur.WriteByte(s[i])
			}
		case '"':
			inItem = true
			for i++; i < len(s) && s[i] != '"'; i++ {
				cur.WriteByte(s[i])
			}
		default:
			cur.WriteByte(c)
			inItem = true
		}
	}
	flush()
	return items
}

func unescapeDelim(s string) rune {
	if len(s) >= 2 && s[0] == '\\' {
		switch s[1] {
		case 'n':
			return '\n'
		case 't':
			return '\t'
		case 'r':
			return '\r'
		case '0':
			return '\x00'
		case '\\':
			return '\\'
		}
	}
	r := []rune(s)
	if len(r) > 0 {
		return r[0]
	}
	return '\n'
}

// plan turns items into concrete argv batches.
func plan(command, items []string, o options) ([][]string, error) {
	// Replace mode: one invocation per item, substituting the replace-str.
	if o.replace != "" {
		var batches [][]string
		for _, it := range items {
			argv := make([]string, len(command))
			for k, a := range command {
				argv[k] = strings.ReplaceAll(a, o.replace, it)
			}
			batches = append(batches, argv)
		}
		return batches, nil // empty items ⇒ no invocations
	}

	if len(items) == 0 {
		if o.noRunEmpty {
			return nil, nil
		}
		return [][]string{append([]string(nil), command...)}, nil // run once, no extra args
	}

	step := o.maxArgs
	if step <= 0 {
		step = len(items) // unlimited ⇒ one batch
	}
	var batches [][]string
	for s := 0; s < len(items); s += step {
		e := min(s+step, len(items))
		argv := append([]string(nil), command...)
		argv = append(argv, items[s:e]...)
		batches = append(batches, argv)
	}
	return batches, nil
}

// execBatches runs the planned invocations (parallel when -P>1) and returns the
// GNU xargs exit status.
func execBatches(rc *tool.RunContext, batches [][]string, o options) int {
	var mu sync.Mutex
	worst := 0
	note := func(code int) {
		mu.Lock()
		if code > worst {
			worst = code
		}
		mu.Unlock()
	}

	// runOne executes one invocation, writing its output to stdout/stderr.
	runOne := func(argv []string, stdout, stderr io.Writer) {
		if o.trace {
			fmt.Fprintln(stderr, strings.Join(argv, " "))
		}
		path := lookCommand(rc, argv[0])
		if path == "" {
			fmt.Fprintf(stderr, "xargs: %s: command not found\n", argv[0])
			note(127)
			return
		}
		c := exec.Command(path, argv[1:]...)
		c.Dir = rc.Dir
		c.Env = rc.Env
		c.Stdin = nil // child reads from the null device, not xargs's input
		c.Stdout, c.Stderr = stdout, stderr
		switch err := c.Run().(type) {
		case nil:
		case *exec.ExitError:
			switch ec := err.ExitCode(); {
			case ec == 255:
				note(124)
			case ec < 0:
				note(125) // killed by signal
			default:
				note(123) // any 1..125
			}
		default:
			note(126) // could not run
		}
	}

	procs := o.maxProcs
	if procs <= 0 { // -P0 = run as many as possible
		procs = len(batches)
	}
	if procs <= 1 {
		for _, argv := range batches {
			runOne(argv, rc.Out, rc.Err) // stream directly when sequential
		}
		return worst
	}

	// Parallel: capture each invocation's output and flush it atomically under
	// the lock, so concurrent children don't interleave-corrupt the shared
	// writers (and a non-concurrent-safe writer like a buffer is never raced).
	sem := make(chan struct{}, procs)
	var wg sync.WaitGroup
	for _, argv := range batches {
		wg.Add(1)
		sem <- struct{}{}
		go func(a []string) {
			defer wg.Done()
			defer func() { <-sem }()
			var ob, eb bytes.Buffer
			runOne(a, &ob, &eb)
			mu.Lock()
			rc.Out.Write(ob.Bytes())
			rc.Err.Write(eb.Bytes())
			mu.Unlock()
		}(argv)
	}
	wg.Wait()
	return worst
}

// lookCommand resolves a program name against the invocation PATH (rc.Env).
func lookCommand(rc *tool.RunContext, name string) string {
	if strings.ContainsAny(name, `/\`) {
		return rc.Path(name)
	}
	for _, dir := range filepath.SplitList(rc.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		cand := filepath.Join(dir, name)
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return cand
		}
	}
	return ""
}
