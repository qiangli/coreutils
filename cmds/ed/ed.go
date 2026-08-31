// Package edcmd implements a clean-room, pure-Go POSIX.1 Issue 7
// ed. The command adapter owns flags, RunContext paths, and filesystem I/O;
// cmds/internal/editor owns the reusable line buffer and command interpreter.
package edcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/qiangli/coreutils/cmds/internal/editor"
	"github.com/qiangli/coreutils/pkg/bre"
	"github.com/qiangli/coreutils/pkg/collate"
	"github.com/qiangli/coreutils/pkg/ctype"
	corelocale "github.com/qiangli/coreutils/pkg/locale"
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
	out := &stickyWriter{writer: rc.Out}
	errOut := &stickyWriter{writer: rc.Err}
	child := *rc
	child.Out, child.Err = out, errOut
	code := runCore(&child, args)
	if out.err != nil {
		fmt.Fprintf(rc.Err, "ed: write error: %v\n", out.err)
		return 1
	}
	if errOut.err != nil {
		if code != 0 {
			return code
		}
		return 1
	}
	return code
}

type stickyWriter struct {
	writer io.Writer
	err    error
}

func (w *stickyWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.writer.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.err = err
	}
	return n, err
}

func writeBytes(w io.Writer, data []byte) error {
	n, err := w.Write(data)
	if err == nil && n != len(data) {
		return io.ErrShortWrite
	}
	return err
}

func runCore(rc *tool.RunContext, args []string) int {
	// Install ed's POSIX signal actions before option parsing, locale setup, or
	// an initial file read.  A signal delivered during that startup work is no
	// less part of the utility invocation than one delivered in command mode.
	signals := startEdSignals()
	defer signals.stop()

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
	lcCType := corelocale.Resolve(rc.Env, corelocale.CType)
	lcCollate := corelocale.Resolve(rc.Env, corelocale.Collate)
	byteLocale := !isUTF8Locale(lcCType)
	tables, code := localeTables(rc, lcCType, lcCollate, byteLocale)
	if code != 0 {
		return code
	}

	eng := &editor.Engine{
		Out:        rc.Out,
		Err:        rc.Err,
		Silent:     *silent,
		Prompt:     *prompt,
		ByteLocale: byteLocale,
		Tables:     tables,
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
				writeErr := writeBytes(f, data)
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
				writeErr := writeBytes(f, data)
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
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && ctx.Err() == nil {
			// ed submits the line to the command interpreter; the utility's
			// exit status is not a command-language failure. Preserve any
			// produced output for e/r !command and bare !command alike.
			err = nil
		}
		if err != nil {
			return out.Bytes(), fmt.Errorf("shell command: %v", err)
		}
		return out.Bytes(), nil
	}
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
			return 1
		}
		if !*silent {
			fmt.Fprintln(rc.Out, n)
		}
	}
	return eng.Run(rc.In)
}

func localeTables(rc *tool.RunContext, lcCType, lcCollate string, byteLocale bool) (*bre.LocaleByteTables, int) {
	if !byteLocale && (lcCollate == "C" || lcCollate == "POSIX" || isUTF8Locale(lcCollate)) {
		return nil, 0
	}
	var tables *bre.LocaleByteTables
	if byteLocale && lcCType != "C" && lcCType != "POSIX" {
		provider, err := ctype.Open(lcCType)
		if err != nil {
			fmt.Fprintf(rc.Err, "ed: LC_CTYPE %q: %v\n", lcCType, err)
			return nil, 2
		}
		var snapshotErr error
		tables, snapshotErr = bre.SnapshotLocaleByteCtypeTables(provider)
		closeErr := provider.Close()
		if snapshotErr != nil {
			fmt.Fprintf(rc.Err, "ed: LC_CTYPE %q: %v\n", lcCType, snapshotErr)
			return nil, 2
		}
		if closeErr != nil {
			fmt.Fprintf(rc.Err, "ed: LC_CTYPE %q: %v\n", lcCType, closeErr)
			return nil, 2
		}
	} else {
		var err error
		tables, err = bre.SnapshotLocaleByteCtypeTables(nil)
		if err != nil {
			fmt.Fprintf(rc.Err, "ed: LC_CTYPE %q: %v\n", lcCType, err)
			return nil, 2
		}
	}
	if lcCollate != "C" && lcCollate != "POSIX" {
		provider, err := collate.Open(lcCollate)
		if err != nil {
			fmt.Fprintf(rc.Err, "ed: LC_COLLATE %q: %v\n", lcCollate, err)
			return nil, 2
		}
		var snapshotErr error
		tables, snapshotErr = tables.WithCollation(provider)
		closeErr := provider.Close()
		if snapshotErr != nil {
			fmt.Fprintf(rc.Err, "ed: LC_COLLATE %q: %v\n", lcCollate, snapshotErr)
			return nil, 2
		}
		if closeErr != nil {
			fmt.Fprintf(rc.Err, "ed: LC_COLLATE %q: %v\n", lcCollate, closeErr)
			return nil, 2
		}
	}
	return tables, 0
}

func isUTF8Locale(name string) bool {
	compact := strings.ToUpper(strings.NewReplacer("-", "", "_", "").Replace(name))
	return strings.Contains(compact, "UTF8")
}
