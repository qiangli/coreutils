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
	posix := envPresent(rc.Env, "POSIXLY_CORRECT")
	// nohup defines no command-specific options, but POSIX Utility Syntax
	// Guideline 10 still makes `--` the option-argument terminator.  This is
	// also VSC-PCTS GA28.  Retain GNU's standalone help/version extensions
	// outside POSIX mode; `--` disambiguates utilities with those names.
	disambiguated := len(args) > 0 && args[0] == "--"
	if disambiguated {
		args = args[1:]
	}
	if !posix && !disambiguated && len(args) == 1 && (args[0] == "--help" || args[0] == "--version") {
		fs := tool.NewFlags(cmd.Name)
		tool.Parse(rc, cmd, fs, args)
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintf(rc.Err, "%s: missing operand\n", cmd.Name)
		fmt.Fprintf(rc.Err, "Try '%s --help' for more information.\n", cmd.Name)
		return internalFailureCode
	}
	return runNohup(rc, args)
}

func envPresent(env []string, key string) bool {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}

// internalFailureCode is the status nohup returns when it cannot even
// attempt the requested command — no operand was given, or a needed
// redirect could not be set up.
//
// Issue 7 nohup EXIT STATUS lists exactly three values: 126 (utility was
// found but could not be invoked), 127 ("An error occurred in the nohup
// utility or the utility could not be found"), and otherwise the
// utility's own status. An internal nohup error is therefore 127
// unconditionally; it is not conditioned on POSIXLY_CORRECT and does not
// use GNU's 125.
const internalFailureCode = 127

var devNullOpener = func() (*os.File, error) {
	return os.OpenFile(os.DevNull, os.O_WRONLY, 0)
}

func runNohup(rc *tool.RunContext, argv []string) int {
	errOut := rc.Err
	if errOut == nil {
		errOut = io.Discard
	}

	// Redirect terminal stdin before resolving the command: Issue 7 allows
	// nohup to redirect a terminal standard input from an unspecified file,
	// and does so as part of setting the utility up, before it is invoked. A
	// failing redirect is therefore a nohup-internal error (127) reported with
	// the redirect diagnostic, even when the command is also missing.
	inTerminal := rc.In != nil && isTerminal(rc.In)
	outTerminal := rc.Out == nil || isTerminal(rc.Out)
	outClosed := isClosedFile(rc.Out)
	errTerminal := rc.Err == nil || isTerminal(rc.Err)

	var stdin io.Reader = rc.In
	if inTerminal {
		f, err := devNullOpener()
		if err != nil {
			fmt.Fprintf(errOut, "nohup: failed to render standard input unusable: %v\n", err)
			return internalFailureCode
		}
		defer f.Close()
		stdin = f
	}

	path, found := lookCommand(rc, argv[0])
	if !found {
		fmt.Fprintf(errOut, "nohup: %s: command not found\n", argv[0])
		return 127
	}

	c := newNohupCommand(rc.Ctx, path, argv[1:], rc.Env)
	c.Dir = rc.Dir
	c.Stdin = stdin

	var displayPath string
	var nohupOut *os.File
	stdout := rc.Out
	if outClosed {
		// A child executor cannot portably copy a closed *os.File. POSIX permits
		// nohup to replace a closed standard descriptor with an unspecified open
		// file, so use a null sink and still invoke the requested utility.
		stdout = io.Discard
	}
	// POSIX requires terminal stderr to follow nohup.out when stdout is a
	// terminal *or is closed*.  A closed *os.File is not a terminal, so test
	// that condition explicitly rather than silently sending stderr back to a
	// dead descriptor and running the utility anyway.
	if outTerminal || (errTerminal && outClosed) {
		f, disp, err := nohupOutputOpener(rc)
		if err != nil {
			fmt.Fprintf(errOut, "nohup: failed to open 'nohup.out': %v\n", err)
			return internalFailureCode
		}
		defer f.Close()
		nohupOut = f
		if outTerminal {
			stdout = f
		}
		displayPath = fmt.Sprintf("'%s'", disp)
	}

	stderr := rc.Err
	if errTerminal {
		if nohupOut != nil {
			stderr = nohupOut
		} else {
			stderr = stdout
		}
	}

	c.Stdout = stdout
	c.Stderr = stderr

	var msg string
	if inTerminal {
		if outTerminal {
			msg = "ignoring input and appending output to %s"
		} else if errTerminal && outClosed {
			msg = "ignoring input and appending standard error to %s"
		} else if errTerminal {
			msg = "ignoring input and redirecting stderr to stdout"
		} else {
			msg = "ignoring input"
		}
	} else {
		if outTerminal {
			msg = "appending output to %s"
		} else if errTerminal && outClosed {
			msg = "appending standard error to %s"
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

	// Immunity must cover the wait, not just the child: see ignoreHangup.
	restoreHangup := ignoreHangup()
	defer restoreHangup()

	err := nohupCommandRunner(c)
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

var nohupOutputOpener = openNohupOutput

var nohupCommandRunner = func(c *exec.Cmd) error { return c.Run() }

func isClosedFile(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	_, err := f.Stat()
	return err != nil
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
