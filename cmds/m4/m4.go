// Package m4cmd implements the POSIX m4 macro processor in pure Go.
//
// It is written from the POSIX.1 (XCU) description of the utility and
// black-box comparison with an m4 binary; no upstream source is used.
// This file holds the option handling; engine.go is the scanner and the
// expansion loop, builtins.go the built-in macros, eval.go the integer
// expression evaluator behind eval.
//
// Scope: the POSIX macro processor built-ins, including definitions,
// conditionals, arithmetic/string operations, diversions, file inclusion,
// system commands, temporary files, diagnostics, tracing, m4wrap and m4exit,
// plus the -s, -D and -U options.
package m4cmd

import (
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{Name: "m4", Synopsis: "Process macros in text.", Usage: "m4 [-s] [-D name[=val]]... [-U name]... [file...]"}

func init() { cmd.Run = run; tool.Register(cmd) }

// cmdlineOp is one -D or -U option. POSIX makes their relative order
// significant, so both flags append to one list.
type cmdlineOp struct {
	undefine bool
	arg      string
}

type cmdlineOps struct {
	ops      *[]cmdlineOp
	undefine bool
}

func (o cmdlineOps) String() string { return "" }
func (o cmdlineOps) Type() string   { return "name" }
func (o cmdlineOps) Set(s string) error {
	*o.ops = append(*o.ops, cmdlineOp{undefine: o.undefine, arg: s})
	return nil
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	var ops []cmdlineOp
	synclines := fs.BoolP("synclines", "s", false, "generate #line directives for the C preprocessor")
	fs.VarP(cmdlineOps{ops: &ops}, "define", "D", "define `name' as `val' (null if omitted) before reading input")
	fs.VarP(cmdlineOps{ops: &ops, undefine: true}, "undefine", "U", "undefine `name' before reading input")
	files, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if rc.FS == nil {
		rc.FS = tool.NewLocalFS()
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	p := newProcessor(rc, files, *synclines)
	for _, op := range ops {
		if op.undefine {
			delete(p.macros, op.arg)
			continue
		}
		name, val, _ := strings.Cut(op.arg, "=")
		if name != "" {
			p.define(name, &macro{text: val}, false)
		}
	}
	return p.process()
}
