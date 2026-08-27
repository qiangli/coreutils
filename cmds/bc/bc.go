// Package bccmd implements the POSIX bc arbitrary-precision calculator.
package bccmd

import (
	"fmt"
	"io"
	"strings"

	bcengine "github.com/qiangli/coreutils/cmds/internal/bc"
	"github.com/qiangli/coreutils/tool"
	"golang.org/x/term"
)

var cmd = &tool.Tool{Name: "bc", Synopsis: "Evaluate an arbitrary-precision calculator language.", Usage: "bc [-l] [file ...]"}

var isTerminalFn = func(stream any) bool {
	f, ok := stream.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(f.Fd()))
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	fs.SetInterspersed(false)
	mathlib := fs.BoolP("mathlib", "l", false, "load the standard math library")
	files, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if rc.FS == nil {
		rc.FS = tool.NewLocalFS()
	}
	eng := bcengine.New(rc.Out, rc.In)
	if *mathlib {
		eng.Scale = 20
		eng.Mathlib = true
	}
	for _, name := range files {
		f, err := rc.FS.Open(rc.Path(name))
		if err != nil {
			fmt.Fprintf(rc.Err, "bc: %s: %v\n", name, err)
			return 1
		}
		data, err := io.ReadAll(f)
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "bc: %s: %v\n", name, err)
			return 1
		}
		if err = eng.Execute(string(data)); err != nil && err != io.EOF {
			fmt.Fprintf(rc.Err, "bc: %s: %v\n", name, err)
			return 1
		}
		if err == io.EOF {
			return 0
		}
	}
	if isTerminalFn(rc.In) && isTerminalFn(rc.Out) {
		return runInteractive(rc, eng)
	}
	data, err := io.ReadAll(eng.In)
	if err != nil {
		fmt.Fprintf(rc.Err, "bc: %v\n", err)
		return 1
	}
	if err = eng.Execute(string(data)); err != nil && err != io.EOF {
		fmt.Fprintf(rc.Err, "bc: %v\n", err)
		return 1
	}
	return 0
}

func runInteractive(rc *tool.RunContext, eng *bcengine.Interpreter) int {
	var pending strings.Builder
	for {
		line, readErr := eng.In.ReadString('\n')
		pending.WriteString(line)
		if pending.Len() != 0 {
			err := eng.Execute(pending.String())
			switch {
			case err == nil:
				pending.Reset()
				flush(rc.Out)
			case err == io.EOF:
				flush(rc.Out)
				return 0
			case readErr == nil && incompleteInteractiveInput(err):
				// Function definitions and compound statements may span lines.
			case readErr == io.EOF && line == "":
				fmt.Fprintf(rc.Err, "bc: %v\n", err)
				flush(rc.Err)
				return 0
			default:
				fmt.Fprintf(rc.Err, "bc: %v\n", err)
				flush(rc.Err)
				pending.Reset()
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				fmt.Fprintf(rc.Err, "bc: %v\n", readErr)
				return 1
			}
			return 0
		}
	}
}

func incompleteInteractiveInput(err error) bool {
	s := err.Error()
	return strings.Contains(s, "unterminated string") ||
		strings.Contains(s, "unterminated comment") ||
		strings.Contains(s, `got ""`) ||
		strings.Contains(s, `near ""`)
}

func flush(w io.Writer) {
	if f, ok := w.(interface{ Flush() error }); ok {
		_ = f.Flush()
	}
}
