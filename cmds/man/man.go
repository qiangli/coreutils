// Package mancmd implements the mandatory POSIX man utility without an
// external formatter or manual-page provider. The registered utility help is
// the documentation database used by this product.
package mancmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"golang.org/x/term"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name: "man", Synopsis: "Display system documentation.",
	Usage: "man [-k] name...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

var manIsTerminal = func(w io.Writer) bool {
	f, ok := w.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(f.Fd()))
}

var shellHelp = map[string]string{
	"alias":   "alias [name[=value] ...] — define or display aliases",
	"cd":      "cd [-L|-P] [dir] — change the working directory",
	"command": "command [-p] command_name [argument ...] — execute a simple command",
	"getopts": "getopts optstring name [arg ...] — parse utility options",
	"hash":    "hash [utility ...] — remember or report utility locations",
	"read":    "read [-r] var... — read a line from standard input",
	"sh":      "sh [-abCefhimnuvx] [-o option] [command_file [argument ...]] — shell command language",
	"umask":   "umask [-S] [mask] — set or report the file mode creation mask",
	"unalias": "unalias [-a] name... — remove alias definitions",
	"wait":    "wait [pid ...] — wait for asynchronous commands",
}

func run(rc *tool.RunContext, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(rc.Err, "man: name operand required")
		return 2
	}
	keyword := false
	if args[0] == "-k" {
		keyword = true
		args = args[1:]
	}
	terminated := false
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
		terminated = true
	}
	if len(args) == 0 {
		fmt.Fprintln(rc.Err, "man: name operand required")
		return 2
	}
	for _, name := range args {
		if !terminated && strings.HasPrefix(name, "-") {
			fmt.Fprintf(rc.Err, "man: unsupported option %s\n", name)
			return 2
		}
	}
	if keyword {
		return search(rc, args)
	}
	status := 0
	var output bytes.Buffer
	for _, name := range args {
		doc := documentation(rc, name)
		if doc == "" {
			fmt.Fprintf(rc.Err, "man: no documentation for %s\n", name)
			status = 1
			continue
		}
		if err := json.NewEncoder(&output).Encode(tool.DocumentAsMCP(name, doc)); err != nil {
			fmt.Fprintf(rc.Err, "man: %v\n", err)
			return 1
		}
	}
	if output.Len() == 0 {
		return status
	}
	if manIsTerminal(rc.Out) {
		pager := rc.Getenv("PAGER")
		if pager == "" {
			pager = "more"
		}
		sh := rc.ResolveCommand("sh")
		if sh == "" {
			fmt.Fprintln(rc.Err, "man: pager: sh not found")
			return 1
		}
		child, err := rc.StartCommand(sh, []string{"-c", pager}, &output, rc.Out, rc.Err)
		if err == nil {
			err = child.Wait()
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "man: pager: %v\n", err)
			return 1
		}
		return status
	}
	if _, err := io.Copy(rc.Out, &output); err != nil {
		fmt.Fprintf(rc.Err, "man: %v\n", err)
		return 1
	}
	return status
}

func documentation(rc *tool.RunContext, name string) string {
	if help, ok := shellHelp[name]; ok {
		return help
	}
	t := tool.Lookup(name)
	if t == nil || name == "posix-providers" || name == "posix-gate" {
		return ""
	}
	var out, errs bytes.Buffer
	copy := *rc
	copy.Out, copy.Err = &out, &errs
	if t.Run(&copy, []string{"--help"}) == 0 && out.Len() > 0 {
		return strings.TrimSpace(out.String())
	}
	return strings.TrimSpace(name + " — " + t.Synopsis + "\nUsage: " + t.Usage)
}

func search(rc *tool.RunContext, terms []string) int {
	patterns := make([]*regexp.Regexp, 0, len(terms))
	for _, term := range terms {
		p, err := regexp.Compile("(?i)" + term)
		if err != nil {
			fmt.Fprintf(rc.Err, "man: invalid keyword %q: %v\n", term, err)
			return 2
		}
		patterns = append(patterns, p)
	}
	names := append(tool.Names(), "alias", "cd", "command", "getopts", "hash", "read", "sh", "umask", "unalias", "wait")
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] || name == "posix-providers" || name == "posix-gate" {
			continue
		}
		seen[name] = true
		purpose := shellHelp[name]
		if purpose == "" {
			purpose = name + " — " + tool.Lookup(name).Synopsis
		}
		for _, p := range patterns {
			if p.MatchString(purpose) {
				if _, err := fmt.Fprintln(rc.Out, purpose); err != nil {
					fmt.Fprintf(rc.Err, "man: %v\n", err)
					return 1
				}
				break
			}
		}
	}
	return 0
}
