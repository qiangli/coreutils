package nicecmd

import (
	"fmt"
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

// Default adjustment when none is given, and the range a requested adjustment
// is silently brought within: [1-2*NZERO, 2*NZERO-1] with NZERO == 20. This
// mirrors what setpriority()/nice() do rather than rejecting the value.
const (
	defaultAdjustment = 10
	minAdjustment     = 1 - 2*nzero
	maxAdjustment     = 2*nzero - 1
	nzero             = 20
)

// longOptions are matched by unambiguous prefix, GNU long-option style.
var longOptions = []string{"--adjustment", "--help", "--version"}

func run(rc *tool.RunContext, args []string) int {
	adjust, given, command, code := parseNice(rc, args)
	if code >= 0 {
		return code
	}
	if len(command) == 0 {
		if given {
			fmt.Fprintln(rc.Err, "nice: a command must be given with an adjustment")
			return 125
		}
		fmt.Fprintln(rc.Out, currentPriority())
		return 0
	}
	if err := setPriority(currentPriority() + adjust); err != nil {
		fmt.Fprintf(rc.Err, "nice: cannot set niceness: %v\n", err)
	}
	return runCommand(rc, "nice", command, nil)
}

// parseNice returns the adjustment, whether one was given, the COMMAND operand
// and its arguments, and an exit code (negative when parsing succeeded).
func parseNice(rc *tool.RunContext, args []string) (int, bool, []string, int) {
	adjust := defaultAdjustment
	given := false
	setAdjust := func(s string) bool {
		n, err := strconv.Atoi(s)
		if err != nil {
			fmt.Fprintf(rc.Err, "nice: invalid adjustment %q\n", s)
			return false
		}
		adjust = min(max(n, minAdjustment), maxAdjustment)
		given = true
		return true
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return adjust, given, operands(args[i+1:]), -1

		// The obsolete "-NUM", "--NUM" and "-+NUM" forms; the adjustment is
		// everything after the leading dash, so "--5" means -5.
		case isObsoleteAdjustment(a):
			if !setAdjust(a[1:]) {
				return 0, false, nil, 125
			}

		case a == "-n":
			if i+1 >= len(args) {
				fmt.Fprintln(rc.Err, "nice: option requires an argument -- 'n'")
				return 0, false, nil, 125
			}
			if !setAdjust(args[i+1]) {
				return 0, false, nil, 125
			}
			i++

		case strings.HasPrefix(a, "-n") && len(a) > 2:
			if !setAdjust(a[2:]) {
				return 0, false, nil, 125
			}

		case strings.HasPrefix(a, "--"):
			name, value, hasValue := strings.Cut(a, "=")
			long, err := matchLongOption(name)
			if err != nil {
				fmt.Fprintf(rc.Err, "nice: %v\n", err)
				return 0, false, nil, 125
			}
			if long == "--help" || long == "--version" {
				fs := tool.NewFlags(cmd.Name)
				fs.IntP("adjustment", "n", defaultAdjustment, "add N to niceness")
				tool.Parse(rc, cmd, fs, []string{long})
				return 0, false, nil, 0
			}
			if !hasValue {
				if i+1 >= len(args) {
					fmt.Fprintf(rc.Err, "nice: option '%s' requires an argument\n", long)
					return 0, false, nil, 125
				}
				value = args[i+1]
				i++
			}
			if !setAdjust(value) {
				return 0, false, nil, 125
			}

		// A lone "-" is an operand, not an option.
		case strings.HasPrefix(a, "-") && len(a) > 1:
			fmt.Fprintf(rc.Err, "nice: invalid option -- '%s'\n", strings.TrimPrefix(a, "-"))
			return 0, false, nil, 125

		default:
			return adjust, given, args[i:], -1
		}
	}
	return adjust, given, nil, -1
}

func operands(rest []string) []string {
	if len(rest) == 0 {
		return nil
	}
	return rest
}

// matchLongOption resolves an unambiguous prefix of a long option name.
func matchLongOption(name string) (string, error) {
	var matches []string
	for _, opt := range longOptions {
		if opt == name {
			return opt, nil
		}
		if strings.HasPrefix(opt, name) {
			matches = append(matches, opt)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("unrecognized option '%s'", name)
	default:
		return "", fmt.Errorf("option '%s' is ambiguous; possibilities: %s", name, strings.Join(matches, " "))
	}
}

// isObsoleteAdjustment reports whether s is the obsolete adjustment form: a
// dash followed by a digit, optionally preceded by another sign character.
func isObsoleteAdjustment(s string) bool {
	if len(s) < 2 || s[0] != '-' {
		return false
	}
	i := 1
	if s[i] == '-' || s[i] == '+' {
		i++
	}
	return i < len(s) && s[i] >= '0' && s[i] <= '9'
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
