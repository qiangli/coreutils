// Package edcmd implements a clean-room, pure-Go subset of POSIX.1 Issue 7
// ed. The command adapter owns flags, RunContext paths, and filesystem I/O;
// cmds/internal/editor owns the reusable line buffer and command interpreter.
package edcmd

import (
	"fmt"
	"io"
	"os"

	"github.com/qiangli/coreutils/cmds/internal/editor"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "ed",
	Synopsis: "Edit text with a pure-Go POSIX line editor.",
	Usage:    "ed [-p string] [-s] [file]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	fs.SetInterspersed(false)
	prompt := fs.StringP("prompt", "p", "", "use STRING as the command prompt")
	silent := fs.BoolP("silent", "s", false, "suppress byte counts")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) > 1 {
		return tool.UsageError(rc, cmd, "extra operand %q", operands[1])
	}
	if rc.FS == nil {
		rc.FS = tool.NewLocalFS()
	}

	eng := &editor.Engine{
		Out:    rc.Out,
		Silent: *silent,
		Prompt: *prompt,
		Files: editor.Files{
			Read: func(name string) ([]byte, error) {
				f, err := rc.FS.Open(rc.Path(name))
				if err != nil {
					return nil, err
				}
				defer f.Close()
				return io.ReadAll(f)
			},
			Write: func(name string, data []byte) error {
				f, err := rc.FS.OpenFile(rc.Path(name), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
				if err != nil {
					return err
				}
				_, writeErr := f.Write(data)
				closeErr := f.Close()
				if writeErr != nil {
					return writeErr
				}
				return closeErr
			},
		},
	}
	if len(operands) == 1 {
		n, err := eng.Load(operands[0])
		if err != nil {
			fmt.Fprintf(rc.Err, "ed: %v\n", err)
			fmt.Fprintln(rc.Out, "?")
			return 1
		}
		if !*silent {
			fmt.Fprintln(rc.Out, n)
		}
	}
	return eng.Run(rc.In)
}
