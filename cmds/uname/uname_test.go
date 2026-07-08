package unamecmd

import (
	"bytes"
	"context"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func wantSysname() string {
	switch runtime.GOOS {
	case "linux":
		return "Linux"
	case "darwin":
		return "Darwin"
	case "windows":
		return "Windows_NT"
	default:
		return ""
	}
}

func TestUnameDefaultIsKernelName(t *testing.T) {
	out, _, code := runTool(t)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if want := wantSysname(); want != "" && out != want+"\n" {
		t.Errorf("uname = %q, want %q", out, want+"\n")
	}
	s, _, _ := runTool(t, "-s")
	if s != out {
		t.Errorf("-s (%q) differs from default (%q)", s, out)
	}
}

func TestUnameFields(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	out, _, code := runTool(t, "-n")
	if code != 0 || out != host+"\n" {
		t.Errorf("-n = (%q, %d), want %q", out, code, host+"\n")
	}
	for _, flag := range []string{"-r", "-m", "-o"} {
		out, _, code := runTool(t, flag)
		if code != 0 || strings.TrimSpace(out) == "" || strings.Count(out, "\n") != 1 {
			t.Errorf("%s = (%q, %d), want one non-empty line", flag, out, code)
		}
	}
	out, _, _ = runTool(t, "-o")
	switch runtime.GOOS {
	case "linux":
		if out != "GNU/Linux\n" {
			t.Errorf("-o = %q, want GNU/Linux", out)
		}
	case "darwin":
		if out != "Darwin\n" {
			t.Errorf("-o = %q, want Darwin", out)
		}
	case "windows":
		if out != "Windows_NT\n" {
			t.Errorf("-o = %q, want Windows_NT", out)
		}
	}
}

func TestUnameCombinedAndAll(t *testing.T) {
	// Output order is fixed regardless of flag order: s n r m o.
	a, _, _ := runTool(t, "-s", "-n")
	b, _, _ := runTool(t, "-n", "-s")
	if a != b {
		t.Errorf("flag order changed output: %q vs %q", a, b)
	}
	s, _, _ := runTool(t, "-s")
	n, _, _ := runTool(t, "-n")
	want := strings.TrimSuffix(s, "\n") + " " + n
	if a != want {
		t.Errorf("-s -n = %q, want %q", a, want)
	}

	all, _, code := runTool(t, "-a")
	if code != 0 {
		t.Fatalf("-a: code=%d", code)
	}
	host, _ := os.Hostname()
	for _, part := range []string{strings.TrimSpace(s), host} {
		if !strings.Contains(all, part) {
			t.Errorf("-a output %q missing %q", all, part)
		}
	}
	if strings.Count(all, "\n") != 1 {
		t.Errorf("-a output is not a single line: %q", all)
	}
}

func TestUnameErrors(t *testing.T) {
	_, errb, code := runTool(t, "extra")
	if code != 2 || !strings.Contains(errb, "extra operand") {
		t.Errorf("operand: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestUnameHelp(t *testing.T) {
	out, _, code := runTool(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: uname") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
}

func TestNewFlagsAndAliases(t *testing.T) {
	sOut, _, _ := runTool(t, "-s")
	rOut, _, _ := runTool(t, "-r")

	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantCode int
		checkOut func(t *testing.T, out string)
	}{
		{
			name:     "processor short flag",
			args:     []string{"-p"},
			wantOut:  "unknown\n",
			wantCode: 0,
		},
		{
			name:     "processor long flag",
			args:     []string{"--processor"},
			wantOut:  "unknown\n",
			wantCode: 0,
		},
		{
			name:     "hardware-platform short flag",
			args:     []string{"-i"},
			wantOut:  "unknown\n",
			wantCode: 0,
		},
		{
			name:     "hardware-platform long flag",
			args:     []string{"--hardware-platform"},
			wantOut:  "unknown\n",
			wantCode: 0,
		},
		{
			name:     "sysname long alias",
			args:     []string{"--sysname"},
			wantOut:  sOut,
			wantCode: 0,
		},
		{
			name:     "release long alias",
			args:     []string{"--release"},
			wantOut:  rOut,
			wantCode: 0,
		},
		{
			name:     "all flag",
			args:     []string{"-a"},
			wantCode: 0,
			checkOut: func(t *testing.T, out string) {
				if strings.Contains(out, "unknown") {
					t.Errorf("-a output %q should not contain 'unknown'", out)
				}
			},
		},
		{
			name:     "all flag with processor",
			args:     []string{"-a", "-p"},
			wantCode: 0,
			checkOut: func(t *testing.T, out string) {
				// Since -p is explicitly requested, "unknown" should be printed
				if !strings.Contains(out, "unknown") {
					t.Errorf("-a -p output %q should contain 'unknown'", out)
				}
			},
		},
		{
			name:     "help output has -p/--processor and -i/--hardware-platform but not aliases",
			args:     []string{"--help"},
			wantCode: 0,
			checkOut: func(t *testing.T, out string) {
				for _, exp := range []string{"-p", "--processor", "-i", "--hardware-platform"} {
					if !strings.Contains(out, exp) {
						t.Errorf("help output %q missing %q", out, exp)
					}
				}
				for _, unexpected := range []string{"--sysname", "--release"} {
					if strings.Contains(out, unexpected) {
						t.Errorf("help output %q should not contain hidden alias %q", out, unexpected)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _, code := runTool(t, tt.args...)
			if code != tt.wantCode {
				t.Fatalf("args=%v code=%d, want %d", tt.args, code, tt.wantCode)
			}
			if tt.checkOut != nil {
				tt.checkOut(t, out)
			} else {
				if out != tt.wantOut {
					t.Errorf("args=%v output=%q, want %q", tt.args, out, tt.wantOut)
				}
			}
		})
	}
}
