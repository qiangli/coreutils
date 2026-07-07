package nicecmd

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{Name: "nice", Synopsis: "Run a program with modified scheduling priority.", Usage: "nice [OPTION]... [COMMAND [ARG]...]"}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	adjust, command, code := parseNice(rc, args)
	if code >= 0 {
		return code
	}
	current := currentPriority()
	if len(command) == 0 {
		if adjustSet(args) {
			return tool.UsageError(rc, cmd, "a command must be given with an adjustment")
		}
		fmt.Fprintln(rc.Out, current)
		return 0
	}
	target := current + adjust
	if target > math.MaxInt32 {
		target = math.MaxInt32
	}
	if target < math.MinInt32 {
		target = math.MinInt32
	}
	if err := setPriority(target); err != nil {
		fmt.Fprintf(rc.Err, "nice: cannot set niceness: %v\n", err)
	}
	return runCommand(rc, "nice", command, nil)
}

func parseNice(rc *tool.RunContext, args []string) (int, []string, int) {
	adjust := 10
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help" || a == "--version":
			fs := tool.NewFlags(cmd.Name)
			fs.IntP("adjustment", "n", 10, "add N to niceness")
			tool.Parse(rc, cmd, fs, []string{a})
			return 0, nil, 0
		case a == "-n" || a == "--adjustment":
			if i+1 >= len(args) {
				fmt.Fprintln(rc.Err, "nice: option requires an argument -- n")
				return 0, nil, 125
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil {
				fmt.Fprintf(rc.Err, "nice: invalid adjustment %q\n", args[i+1])
				return 0, nil, 125
			}
			adjust = n
			i++
		case strings.HasPrefix(a, "-n") && len(a) > 2:
			n, err := strconv.Atoi(a[2:])
			if err != nil {
				fmt.Fprintf(rc.Err, "nice: invalid adjustment %q\n", a[2:])
				return 0, nil, 125
			}
			adjust = n
		case isLegacyNice(a):
			n, _ := strconv.Atoi(strings.TrimLeft(a, "-"))
			if strings.HasPrefix(a, "-+") {
				n, _ = strconv.Atoi(a[1:])
			}
			adjust = n
		default:
			return adjust, args[i:], -1
		}
	}
	return adjust, nil, -1
}

func adjustSet(args []string) bool {
	for _, a := range args {
		if a == "-n" || a == "--adjustment" || strings.HasPrefix(a, "-n") || isLegacyNice(a) {
			return true
		}
	}
	return false
}

func isLegacyNice(s string) bool {
	if strings.HasPrefix(s, "--") {
		_, err := strconv.Atoi(s[1:])
		return err == nil
	}
	if strings.HasPrefix(s, "-+") {
		_, err := strconv.Atoi(s[1:])
		return err == nil
	}
	if strings.HasPrefix(s, "-") && len(s) > 1 {
		_, err := strconv.Atoi(s)
		return err == nil
	}
	return false
}

func runCommand(rc *tool.RunContext, name string, argv []string, env []string) int {
	path := lookCommand(rc, argv[0])
	if path == "" {
		fmt.Fprintf(rc.Err, "%s: %s: command not found\n", name, argv[0])
		return 127
	}
	c := exec.CommandContext(rc.Ctx, path, argv[1:]...)
	c.Dir = rc.Dir
	if env != nil {
		c.Env = env
	} else {
		c.Env = rc.Env
	}
	c.Stdin = rc.In
	c.Stdout = rc.Out
	c.Stderr = rc.Err
	err := c.Run()
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	if os.IsNotExist(err) || strings.Contains(err.Error(), "executable file not found") {
		fmt.Fprintf(rc.Err, "%s: failed to run command %q: %v\n", name, argv[0], err)
		return 127
	}
	fmt.Fprintf(rc.Err, "%s: failed to run command %q: %v\n", name, argv[0], err)
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
