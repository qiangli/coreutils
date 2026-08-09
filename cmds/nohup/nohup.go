package nohupcmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/term"
)

var cmd = &tool.Tool{Name: "nohup", Synopsis: "Run a command immune to hangups.", Usage: "nohup COMMAND [ARG]..."}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "--version") {
		fs := tool.NewFlags(cmd.Name)
		tool.Parse(rc, cmd, fs, args)
		return 0
	}
	if len(args) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}
	return runNohup(rc, args)
}

var devNullOpener = func() (*os.File, error) {
	return os.OpenFile(os.DevNull, os.O_WRONLY, 0)
}

func runNohup(rc *tool.RunContext, argv []string) int {
	errOut := rc.Err
	if errOut == nil {
		errOut = io.Discard
	}
	path, found := lookCommand(rc, argv[0])
	if !found {
		fmt.Fprintf(errOut, "nohup: %s: command not found\n", argv[0])
		return 127
	}

	inTerminal := rc.In != nil && isTerminal(rc.In)
	outTerminal := rc.Out == nil || isTerminal(rc.Out)
	errTerminal := rc.Err == nil || isTerminal(rc.Err)

	var stdin io.Reader = rc.In
	if inTerminal {
		f, err := devNullOpener()
		if err != nil {
			fmt.Fprintf(errOut, "nohup: failed to redirect stdin from %s: %v\n", os.DevNull, err)
			return 125
		}
		defer f.Close()
		stdin = f
	}

	c := newNohupCommand(rc.Ctx, path, argv[1:], rc.Env)
	c.Dir = rc.Dir
	c.Stdin = stdin

	var displayPath string
	stdout := rc.Out
	if outTerminal {
		f, disp, err := openNohupOutput(rc)
		if err != nil {
			fmt.Fprintf(errOut, "nohup: failed to open 'nohup.out': %v\n", err)
			return 125
		}
		defer f.Close()
		stdout = f
		displayPath = fmt.Sprintf("'%s'", disp)
	}

	stderr := rc.Err
	if errTerminal {
		stderr = stdout
	}

	c.Stdout = stdout
	c.Stderr = stderr

	var msg string
	if inTerminal {
		if outTerminal {
			msg = "ignoring input and appending output to %s"
		} else if errTerminal {
			msg = "ignoring input and redirecting stderr to stdout"
		} else {
			msg = "ignoring input"
		}
	} else {
		if outTerminal {
			msg = "appending output to %s"
		} else if errTerminal {
			msg = "redirecting stderr to stdout"
		}
	}

	if msg != "" {
		if strings.Contains(msg, "%s") {
			fmt.Fprintf(errOut, "nohup: "+msg+"\n", displayPath)
		} else {
			fmt.Fprintln(errOut, "nohup: "+msg)
		}
	}

	err := c.Run()
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	if os.IsNotExist(err) {
		fmt.Fprintf(errOut, "nohup: failed to run command %q: %v\n", argv[0], err)
		return 127
	}
	fmt.Fprintf(errOut, "nohup: failed to run command %q: %v\n", argv[0], err)
	return 126
}

func lookCommand(rc *tool.RunContext, name string) (string, bool) {
	if strings.ContainsAny(name, `/\`) {
		p := rc.Path(name)
		return p, commandExists(p)
	}
	var found string
	for _, dir := range filepath.SplitList(rc.Getenv("PATH")) {
		base := dir
		if base == "" {
			base = "."
		}
		cand := rc.Path(filepath.Join(base, name))
		if isExecFile(cand) {
			return cand, true
		}
		if found == "" && commandExists(cand) {
			found = cand
		}
		if runtime.GOOS == "windows" {
			for _, ext := range []string{".exe", ".bat", ".cmd", ".com"} {
				if isExecFile(cand + ext) {
					return cand + ext, true
				}
				if found == "" && commandExists(cand+ext) {
					found = cand + ext
				}
			}
		}
	}
	return found, found != ""
}

func openNohupOutput(rc *tool.RunContext) (*os.File, string, error) {
	localPath := rc.Path("nohup.out")
	f, err := os.OpenFile(localPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err == nil {
		return f, "nohup.out", nil
	}

	if home := rc.Getenv("HOME"); home != "" {
		homePath := rc.Path(filepath.Join(home, "nohup.out"))
		f, err = os.OpenFile(homePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err == nil {
			return f, homePath, nil
		}
	}
	return nil, "", err
}

func commandExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

func isExecFile(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return fi.Mode()&0o111 != 0
}

func isTerminal(w interface{}) bool {
	if f, ok := w.(interface{ Fd() uintptr }); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}
