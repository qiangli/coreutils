package nohupcmd

import (
	"fmt"
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
	path := lookCommand(rc, argv[0])
	if path == "" {
		fmt.Fprintf(rc.Err, "nohup: %s: command not found\n", argv[0])
		return 127
	}
	c := exec.CommandContext(rc.Ctx, path, argv[1:]...)
	c.Dir = rc.Dir
	c.Env = rc.Env
	c.Stdin = rc.In
	c.Stdout = rc.Out
	c.Stderr = rc.Err
	if rc.Out == nil {
		outPath := filepath.Join(rc.Dir, "nohup.out")
		f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			fmt.Fprintf(rc.Err, "nohup: failed to open 'nohup.out': %v\n", err)
			return 125
		}
		defer f.Close()
		c.Stdout = f
	}
	err := c.Run()
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	if os.IsNotExist(err) {
		fmt.Fprintf(rc.Err, "nohup: failed to run command %q: %v\n", argv[0], err)
		return 127
	}
	fmt.Fprintf(rc.Err, "nohup: failed to run command %q: %v\n", argv[0], err)
	return 126
}

func lookCommand(rc *tool.RunContext, name string) string {
	if strings.ContainsAny(name, `/\`) {
		p := rc.Path(name)
		if isExecFile(p) {
			return p
		}
		return ""
	}
	for _, dir := range filepath.SplitList(rc.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		cand := filepath.Join(dir, name)
		if isExecFile(cand) {
			return cand
		}
		if runtime.GOOS == "windows" {
			for _, ext := range []string{".exe", ".bat", ".cmd", ".com"} {
				if isExecFile(cand + ext) {
					return cand + ext
				}
			}
		}
	}
	return ""
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
