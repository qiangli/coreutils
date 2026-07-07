package stdbufcmd

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

var cmd = &tool.Tool{Name: "stdbuf", Synopsis: "Run COMMAND with modified stdio buffering.", Usage: "stdbuf OPTION... COMMAND"}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	in, out, errs, operands, configured, code := parseArgs(rc, args)
	if code >= 0 {
		return code
	}
	if !configured {
		fmt.Fprintln(rc.Err, "stdbuf: missing buffering mode option")
		return 125
	}
	if len(operands) == 0 {
		fmt.Fprintln(rc.Err, "stdbuf: missing command")
		return 125
	}
	env := append([]string{}, rc.Env...)
	for _, pair := range []struct{ key, val string }{{"_STDBUF_I", in}, {"_STDBUF_O", out}, {"_STDBUF_E", errs}} {
		if pair.val == "" {
			continue
		}
		normalized, err := normalizeMode(pair.val, pair.key == "_STDBUF_I")
		if err != nil {
			fmt.Fprintf(rc.Err, "stdbuf: %v\n", err)
			return 125
		}
		env = appendEnv(env, pair.key, normalized)
	}
	return runCommand(rc, "stdbuf", operands, env)
}

func parseArgs(rc *tool.RunContext, args []string) (string, string, string, []string, bool, int) {
	var in, out, errs string
	configured := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		need := func() (string, bool) {
			if i+1 >= len(args) {
				fmt.Fprintf(rc.Err, "stdbuf: option %s requires an argument\n", a)
				return "", false
			}
			i++
			return args[i], true
		}
		switch {
		case a == "--help" || a == "--version":
			fs := tool.NewFlags(cmd.Name)
			fs.StringP("input", "i", "", "adjust stdin buffering")
			fs.StringP("output", "o", "", "adjust stdout buffering")
			fs.StringP("error", "e", "", "adjust stderr buffering")
			tool.Parse(rc, cmd, fs, []string{a})
			return "", "", "", nil, false, 0
		case a == "-i" || a == "--input":
			v, ok := need()
			if !ok {
				return "", "", "", nil, false, 125
			}
			configured = true
			in = v
		case strings.HasPrefix(a, "--input="):
			configured = true
			in = strings.TrimPrefix(a, "--input=")
		case strings.HasPrefix(a, "-i") && len(a) > 2:
			configured = true
			in = a[2:]
		case a == "-o" || a == "--output":
			v, ok := need()
			if !ok {
				return "", "", "", nil, false, 125
			}
			configured = true
			out = v
		case strings.HasPrefix(a, "--output="):
			configured = true
			out = strings.TrimPrefix(a, "--output=")
		case strings.HasPrefix(a, "-o") && len(a) > 2:
			configured = true
			out = a[2:]
		case a == "-e" || a == "--error":
			v, ok := need()
			if !ok {
				return "", "", "", nil, false, 125
			}
			configured = true
			errs = v
		case strings.HasPrefix(a, "--error="):
			configured = true
			errs = strings.TrimPrefix(a, "--error=")
		case strings.HasPrefix(a, "-e") && len(a) > 2:
			configured = true
			errs = a[2:]
		default:
			return in, out, errs, args[i:], configured, -1
		}
	}
	return in, out, errs, nil, configured, -1
}

func normalizeMode(s string, input bool) (string, error) {
	if s == "L" {
		if input {
			return "", fmt.Errorf("line buffering stdin is meaningless")
		}
		return "L", nil
	}
	n, err := parseBufferSize(s)
	if err != nil {
		return "", fmt.Errorf("invalid mode %q", s)
	}
	return strconv.FormatUint(n, 10), nil
}

func parseBufferSize(s string) (uint64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, fmt.Errorf("missing digits")
	}
	n, err := strconv.ParseUint(s[:i], 10, 64)
	if err != nil {
		return 0, err
	}
	mult, ok := sizeMultiplier(s[i:])
	if !ok {
		return 0, fmt.Errorf("invalid suffix")
	}
	if mult != 0 && n > math.MaxUint64/mult {
		return 0, fmt.Errorf("value too large")
	}
	return n * mult, nil
}

func sizeMultiplier(suffix string) (uint64, bool) {
	if suffix == "" || suffix == "B" {
		return 1, true
	}
	if suffix == "b" {
		return 512, true
	}
	powers := map[byte]int{'K': 1, 'M': 2, 'G': 3, 'T': 4, 'P': 5, 'E': 6}
	c := suffix[0]
	if c >= 'a' && c <= 'z' {
		c -= 'a' - 'A'
	}
	p, ok := powers[c]
	if !ok {
		return 0, false
	}
	var base uint64
	switch {
	case len(suffix) == 1:
		base = 1024
	case len(suffix) == 2 && suffix[1] == 'B':
		base = 1000
	case len(suffix) == 3 && suffix[1] == 'i' && suffix[2] == 'B':
		base = 1024
	default:
		return 0, false
	}
	m := uint64(1)
	for range p {
		if m > math.MaxUint64/base {
			return 0, false
		}
		m *= base
	}
	return m, true
}

func appendEnv(env []string, key, val string) []string {
	prefix := key + "="
	dst := env[:0]
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			dst = append(dst, e)
		}
	}
	return append(dst, prefix+val)
}

func runCommand(rc *tool.RunContext, name string, argv []string, env []string) int {
	path := lookCommand(rc, argv[0])
	if path == "" {
		fmt.Fprintf(rc.Err, "%s: %s: command not found\n", name, argv[0])
		return 127
	}
	c := exec.CommandContext(rc.Ctx, path, argv[1:]...)
	c.Dir = rc.Dir
	c.Env = env
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
