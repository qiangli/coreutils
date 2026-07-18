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
	c := exec.CommandContext(rc.Ctx, path, argv[1:]...)
	c.Dir = rc.Dir
	c.Env = rc.Env
	c.Stdin = rc.In
	c.Stdout = rc.Out
	c.Stderr = rc.Err
	if rc.Out == nil {
		f, err := openNohupOutput(rc)
		if err != nil {
			fmt.Fprintf(errOut, "nohup: failed to open 'nohup.out': %v\n", err)
			return 125
		}
		defer f.Close()
		c.Stdout = f
		if rc.Err == nil {
			c.Stderr = f
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

func openNohupOutput(rc *tool.RunContext) (*os.File, error) {
	paths := []string{rc.Path("nohup.out")}
	if home := rc.Getenv("HOME"); home != "" {
		paths = append(paths, rc.Path(filepath.Join(home, "nohup.out")))
	}
	var err error
	for _, path := range paths {
		f, openErr := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if openErr == nil {
			return f, nil
		}
		err = openErr
	}
	return nil, err
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
