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
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "xargs",
	Synopsis: "Build and run command lines from standard input.",
	Usage:    "xargs [-0rtx] [-n MAX] [-L MAX-LINES] [-s SIZE] [-I REPL] [-P N] [-E EOF] [-d DELIM] [command [initial-args...]]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type options struct {
	null       bool
	noRunEmpty bool
	trace      bool
	maxArgs    int // <=0 means unlimited (one batch)
	maxLines   int // <=0 means unlimited
	maxChars   int // <=0 means unlimited
	exactSize  bool
	replace    string
	maxProcs   int
	eof        string
	delim      string // raw -d value (pre-unescape); "" = unset
}

func run(rc *tool.RunContext, args []string) int {
	o := options{maxArgs: -1, maxProcs: 1}
	args = expandShortOptionClusters(args)

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
			o.maxArgs, o.maxLines, o.replace = atoiOr(v), 0, ""
		case strings.HasPrefix(a, "--max-args="):
			o.maxArgs, o.maxLines, o.replace = atoiOr(a[len("--max-args="):]), 0, ""
		case strings.HasPrefix(a, "-n") && len(a) > 2:
			o.maxArgs, o.maxLines, o.replace = atoiOr(a[2:]), 0, ""
		case a == "-L":
			v, ok := val()
			if !ok {
				return tool.UsageError(rc, cmd, "option %s requires an argument", a)
			}
			o.maxLines, o.maxArgs, o.replace = atoiOr(v), -1, ""
		case strings.HasPrefix(a, "-L") && len(a) > 2:
			o.maxLines, o.maxArgs, o.replace = atoiOr(a[2:]), -1, ""
		case a == "-s":
			v, ok := val()
			if !ok {
				return tool.UsageError(rc, cmd, "option %s requires an argument", a)
			}
			o.maxChars = atoiOr(v)
		case strings.HasPrefix(a, "-s") && len(a) > 2:
			o.maxChars = atoiOr(a[2:])
		case a == "-x":
			o.exactSize = true
		case a == "-I" || a == "--replace" || a == "-i":
			if v, ok := val(); ok {
				o.replace, o.maxArgs, o.maxLines = v, -1, 1
			} else {
				o.replace, o.maxArgs, o.maxLines = "{}", -1, 1
			}
		case strings.HasPrefix(a, "-I") && len(a) > 2:
			o.replace, o.maxArgs, o.maxLines = a[2:], -1, 1
		case strings.HasPrefix(a, "--replace="):
			o.replace, o.maxArgs, o.maxLines = a[len("--replace="):], -1, 1
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
	if o.maxLines < 0 {
		return tool.UsageError(rc, cmd, "-L requires a positive number")
	}
	if o.maxChars != 0 && o.maxChars < 1 {
		return tool.UsageError(rc, cmd, "-s requires a positive number")
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

// expandShortOptionClusters gives the hand-written option parser the same
// input shape for `-txn1` as for `-t -x -n1`. Once an option that consumes an
// argument is reached, the rest of that word is its argument rather than more
// flags. Expansion stops at the command operand or an explicit `--`.
func expandShortOptionClusters(args []string) []string {
	var expanded []string
	for i, arg := range args {
		if arg == "--" || arg == "-" || arg == "" || arg[0] != '-' {
			expanded = append(expanded, args[i:]...)
			break
		}
		if len(arg) < 3 || arg[1] == '-' {
			expanded = append(expanded, arg)
			continue
		}
		for pos := 1; pos < len(arg); pos++ {
			flag := arg[pos]
			switch flag {
			case '0', 'r', 't', 'p', 'x':
				expanded = append(expanded, "-"+string(flag))
			case 'n', 'L', 's', 'I', 'P', 'E', 'e', 'd':
				if pos+1 < len(arg) {
					expanded = append(expanded, "-"+string(flag)+arg[pos+1:])
				} else {
					expanded = append(expanded, "-"+string(flag))
				}
				pos = len(arg)
			default:
				expanded = append(expanded, "-"+arg[pos:])
				pos = len(arg)
			}
		}
		if i == len(args)-1 {
			return expanded
		}
	}
	return expanded
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
type inputItem struct {
	value string
	line  int
}

func readItems(r io.Reader, o options) ([]inputItem, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var items []inputItem
	switch {
	case o.replace != "":
		lines := strings.Split(string(data), "\n")
		for line, text := range lines {
			if line == len(lines)-1 && text == "" {
				continue
			}
			value, parseErr := unquoteReplacementLine(strings.TrimSuffix(text, "\r"))
			if parseErr != nil {
				return nil, parseErr
			}
			items = append(items, inputItem{value: value, line: line + 1})
		}
	case o.null:
		items = plainItems(splitOn(string(data), '\x00'))
	case o.delim != "":
		d := unescapeDelim(o.delim)
		items = plainItems(splitOn(string(data), d))
	default:
		items, err = splitQuoted(string(data))
		if err != nil {
			return nil, err
		}
	}
	// Honor a logical EOF string: stop at the first item equal to it.
	if o.eof != "" {
		for k, it := range items {
			if it.value == o.eof {
				items = items[:k]
				break
			}
		}
	}
	return items, nil
}

// unquoteReplacementLine applies xargs quoting and escaping to one logical
// line without treating unquoted blanks as item separators. That distinction
// is required by -I: the entire logical line is one replacement item, while
// quotes and backslashes remain syntax rather than literal data.
func unquoteReplacementLine(text string) (string, error) {
	var out strings.Builder
	var quote byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				out.WriteByte(c)
			}
			continue
		}
		switch c {
		case '\\':
			if i+1 == len(text) {
				return "", fmt.Errorf("unmatched backslash")
			}
			i++
			out.WriteByte(text[i])
		case '\'', '"':
			quote = c
		default:
			out.WriteByte(c)
		}
	}
	if quote == '\'' {
		return "", fmt.Errorf("unmatched single quote")
	}
	if quote == '"' {
		return "", fmt.Errorf("unmatched double quote")
	}
	return out.String(), nil
}

func plainItems(values []string) []inputItem {
	items := make([]inputItem, len(values))
	for i, value := range values {
		items[i] = inputItem{value: value, line: i + 1}
	}
	return items
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
func splitQuoted(s string) ([]inputItem, error) {
	var items []inputItem
	var cur strings.Builder
	inItem := false
	line, itemLine := 1, 1
	flush := func() {
		if inItem {
			items = append(items, inputItem{value: cur.String(), line: itemLine})
			cur.Reset()
			inItem = false
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case ' ', '\t', '\r', '\v', '\f':
			flush()
		case '\n':
			flush()
			line++
			itemLine = line
		case '\\':
			if i+1 == len(s) {
				return nil, fmt.Errorf("unmatched backslash")
			}
			i++
			cur.WriteByte(s[i])
			inItem = true
		case '\'':
			inItem = true
			for i++; i < len(s) && s[i] != '\''; i++ {
				cur.WriteByte(s[i])
			}
			if i == len(s) {
				return nil, fmt.Errorf("unmatched single quote")
			}
		case '"':
			inItem = true
			for i++; i < len(s) && s[i] != '"'; i++ {
				cur.WriteByte(s[i])
			}
			if i == len(s) {
				return nil, fmt.Errorf("unmatched double quote")
			}
		default:
			cur.WriteByte(c)
			inItem = true
		}
	}
	flush()
	return items, nil
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
func plan(command []string, items []inputItem, o options) ([][]string, error) {
	// Replace mode: one invocation per item, substituting the replace-str.
	if o.replace != "" {
		var batches [][]string
		for _, it := range items {
			argv := make([]string, len(command))
			for k, a := range command {
				argv[k] = strings.ReplaceAll(a, o.replace, it.value)
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

	var batches [][]string
	baseSize := argvSize(command)
	if o.maxChars > 0 && baseSize > o.maxChars {
		return nil, fmt.Errorf("command exceeds -s size limit")
	}
	for start := 0; start < len(items); {
		argv := append([]string(nil), command...)
		size, lines, end := baseSize, 0, start
		lastLine := -1
		for end < len(items) {
			it := items[end]
			newLine := it.line != lastLine
			if newLine && o.maxLines > 0 && lines == o.maxLines {
				break
			}
			if o.maxArgs > 0 && end-start == o.maxArgs {
				break
			}
			itemSize := len(it.value) + 1
			if o.maxChars > 0 && size+itemSize > o.maxChars && end > start {
				break
			}
			if o.maxChars > 0 && size+itemSize > o.maxChars && o.exactSize {
				return nil, fmt.Errorf("input item exceeds -s size limit")
			}
			argv = append(argv, it.value)
			size += itemSize
			if newLine {
				lines++
				lastLine = it.line
			}
			end++
		}
		if end == start { // A single large input item is permitted unless -x was set.
			argv = append(argv, items[end].value)
			end++
		}
		batches = append(batches, argv)
		start = end
	}
	return batches, nil
}

// argvSize is the byte budget occupied by a command argument vector.  The
// terminating NUL for each argument is included so -s batches cannot exceed
// the requested command size.
func argvSize(argv []string) int {
	n := 0
	for _, arg := range argv {
		n += len(arg) + 1
	}
	return n
}

// execBatches runs the planned invocations (parallel when -P>1) and returns the
// xargs exit status.
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
	runOne := func(argv []string, stdout, stderr io.Writer) (stop bool) {
		if o.trace {
			fmt.Fprintln(stderr, strings.Join(argv, " "))
		}
		path := rc.ResolveCommand(argv[0])
		if path == "" {
			fmt.Fprintf(stderr, "xargs: %s: command not found\n", argv[0])
			note(127)
			return false
		}
		// The child reads from the null device, not xargs's consumed input.
		c, startErr := rc.StartCommand(path, argv[1:], nil, stdout, stderr)
		if startErr != nil {
			note(126)
			return false
		}
		switch err := c.Wait().(type) {
		case nil:
		case *exec.ExitError:
			switch ec := err.ExitCode(); {
			case ec == 255:
				note(124)
				return true
			case ec < 0:
				note(125) // killed by signal
			default:
				note(123) // any 1..125
			}
		default:
			note(126) // could not run
		}
		return false
	}

	procs := o.maxProcs
	if procs <= 0 { // -P0 = run as many as possible
		procs = len(batches)
	}
	if procs <= 1 {
		for _, argv := range batches {
			if runOne(argv, rc.Out, rc.Err) { // exit 255 stops further input
				break
			}
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
