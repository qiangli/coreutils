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
	"fmt"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
	"github.com/spf13/pflag"
)

var cmd = &tool.Tool{Name: "m4", Synopsis: "Process macros in text.", Usage: "m4 [-s] [-D name[=val]]... [-U name]... [file...]"}

func init() { cmd.Run = run; tool.Register(cmd) }

// cmdlineOp is one -D or -U option. POSIX makes their relative order
// significant, so both flags append to one list.
type cmdlineOp struct {
	beforeFile int
	undefine   bool
	arg        string
	syncSet    bool
	sync       bool
}

type cmdlineOps struct {
	ops      *[]cmdlineOp
	fs       *pflag.FlagSet
	undefine bool
}

func (o cmdlineOps) String() string { return "" }
func (o cmdlineOps) Type() string   { return "name" }
func (o cmdlineOps) Set(s string) error {
	*o.ops = append(*o.ops, cmdlineOp{beforeFile: len(o.fs.Args()), undefine: o.undefine, arg: s})
	return nil
}

type cmdlineSync struct {
	ops *[]cmdlineOp
	fs  *pflag.FlagSet
}

func (o cmdlineSync) String() string   { return "false" }
func (o cmdlineSync) Type() string     { return "bool" }
func (o cmdlineSync) IsBoolFlag() bool { return true }
func (o cmdlineSync) Set(s string) error {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	*o.ops = append(*o.ops, cmdlineOp{beforeFile: len(o.fs.Args()), syncSet: true, sync: v})
	return nil
}

func validMacroName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	var ops []cmdlineOp
	fs.VarPF(cmdlineSync{ops: &ops, fs: fs}, "synclines", "s", "generate #line directives for the C preprocessor").NoOptDefVal = "true"
	fs.VarP(cmdlineOps{ops: &ops, fs: fs}, "define", "D", "define `name' as `val' (null if omitted) before reading input")
	fs.VarP(cmdlineOps{ops: &ops, fs: fs, undefine: true}, "undefine", "U", "undefine `name' before reading input")
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
	for _, op := range ops {
		if op.syncSet {
			continue
		}
		name := op.arg
		if !op.undefine {
			name, _, _ = strings.Cut(op.arg, "=")
		}
		if !validMacroName(name) {
			fmt.Fprintf(rc.Err, "m4: invalid macro name: %s\n", name)
			return 2
		}
	}
	p := newProcessor(rc, files, ops)
	return p.process()
}
