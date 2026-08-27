// Package edcmd implements a clean-room, pure-Go subset of POSIX.1 Issue 7
// ed. The command adapter owns flags, RunContext paths, and filesystem I/O;
// cmds/internal/editor owns the reusable line buffer and command interpreter.
package edcmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/qiangli/coreutils/cmds/internal/editor"
	"github.com/qiangli/coreutils/tool"
	"golang.org/x/term"
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
			Append: func(name string, data []byte) error {
				f, err := rc.FS.OpenFile(rc.Path(name), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
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
	if f, ok := rc.In.(*os.File); ok {
		eng.ExitOnError = !term.IsTerminal(int(f.Fd()))
	}
	eng.Shell = func(command string, input []byte) ([]byte, error) {
		path := rc.ResolveCommand("sh")
		if path == "" {
			return nil, fmt.Errorf("shell not found in invocation PATH")
		}
		ctx := rc.Ctx
		if ctx == nil {
			ctx = context.Background()
		}
		c := exec.CommandContext(ctx, path, "-c", command)
		c.Dir, c.Env, c.Stderr = rc.Dir, append([]string(nil), rc.Env...), rc.Err
		if input != nil {
			c.Stdin = bytes.NewReader(input)
		} else {
			c.Stdin = rc.In
		}
		var out bytes.Buffer
		c.Stdout = &out
		err := c.Run()
		if err != nil {
			return out.Bytes(), fmt.Errorf("shell command: %v", err)
		}
		return out.Bytes(), nil
	}
	signals := startEdSignals()
	defer signals.stop()
	eng.PollSignal = signals.poll
	eng.Signals = signals.channel()
	eng.Hangup = func(data []byte) error {
		if err := eng.Files.Write("ed.hup", data); err == nil {
			return nil
		}
		home := rc.Getenv("HOME")
		if home == "" {
			return fmt.Errorf("cannot save ed.hup")
		}
		return eng.Files.Write(home+string(os.PathSeparator)+"ed.hup", data)
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
