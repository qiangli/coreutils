package unamecmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"slices"
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
	av, _, code := runTool(t, "-a", "-v")
	if code != 0 {
		t.Fatalf("-a -v: code=%d", code)
	}
	if av != all {
		t.Errorf("uname -a -v output %q differs from uname -a output %q", av, all)
	}
}

// TestUnameAllIsExactlyMNRSV pins Issue 7's uname -a: "Behave as though all
// of the options -mnrsv were specified." GNU's -o operating-system field (and
// the equally non-POSIX -p/-i) must not be implied by -a; they are printed
// only when explicitly requested.
func TestUnameAllIsExactlyMNRSV(t *testing.T) {
	all, _, code := runTool(t, "-a")
	if code != 0 {
		t.Fatalf("-a: code=%d", code)
	}
	mnrsv, _, code := runTool(t, "-m", "-n", "-r", "-s", "-v")
	if code != 0 {
		t.Fatalf("-mnrsv: code=%d", code)
	}
	if all != mnrsv {
		t.Errorf("uname -a = %q, want the -mnrsv output %q", all, mnrsv)
	}

	// The required field order is s n r v m.
	info, err := probe()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{info.sysname, info.nodename, info.release}
	if info.version != "" {
		want = append(want, info.version)
	}
	want = append(want, info.machine)
	if got := strings.TrimSuffix(all, "\n"); got != strings.Join(want, " ") {
		t.Errorf("uname -a = %q, want %q", got, strings.Join(want, " "))
	}

	// -o is a GNU extension and must not appear in -a unless asked for.
	os, _, _ := runTool(t, "-o")
	osField := strings.TrimSuffix(os, "\n")
	if !slices.Contains(want, osField) && strings.Contains(all, osField) {
		t.Errorf("uname -a = %q must not include the non-POSIX -o field %q", all, osField)
	}
	ao, _, code := runTool(t, "-a", "-o")
	if code != 0 || ao != strings.TrimSuffix(all, "\n")+" "+os {
		t.Errorf("uname -a -o = (%q, %d), want %q", ao, code, strings.TrimSuffix(all, "\n")+" "+os)
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
	processorOut := "unknown\n"
	hardwarePlatformOut := "unknown\n"
	if runtime.GOOS == "darwin" {
		info, err := probe()
		if err != nil {
			t.Fatal(err)
		}
		processorOut = info.processor + "\n"
		hardwarePlatformOut = info.hardwarePlatform + "\n"
	}

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
			wantOut:  processorOut,
			wantCode: 0,
		},
		{
			name:     "processor long flag",
			args:     []string{"--processor"},
			wantOut:  processorOut,
			wantCode: 0,
		},
		{
			name:     "hardware-platform short flag",
			args:     []string{"-i"},
			wantOut:  hardwarePlatformOut,
			wantCode: 0,
		},
		{
			name:     "hardware-platform long flag",
			args:     []string{"--hardware-platform"},
			wantOut:  hardwarePlatformOut,
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
				// -a is -mnrsv only, so the non-POSIX -p/-i fields
				// (and their "unknown" placeholders) never appear.
				if strings.Contains(out, "unknown") {
					t.Errorf("-a output %q should not contain 'unknown'", out)
				}
				if strings.Contains(out, operatingSystem()) && operatingSystem() != wantSysname() {
					t.Errorf("-a output %q should not contain the -o field", out)
				}
			},
		},
		{
			name:     "all flag with processor",
			args:     []string{"-a", "-p"},
			wantCode: 0,
			checkOut: func(t *testing.T, out string) {
				info, err := probe()
				if err != nil {
					t.Fatal(err)
				}
				// -p is explicitly requested, so it is appended to -mnrsv.
				if !strings.HasSuffix(out, " "+info.processor+"\n") {
					t.Errorf("-a -p output %q should end with the processor field %q", out, info.processor)
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
				if runtime.GOOS == "darwin" {
					info, err := probe()
					if err != nil {
						t.Fatal(err)
					}
					switch tt.args[0] {
					case "-p", "--processor":
						if out != info.processor+"\n" {
							t.Errorf("args=%v output=%q, want %q", tt.args, out, info.processor+"\n")
						}
						return
					case "-i", "--hardware-platform":
						if out != info.hardwarePlatform+"\n" {
							t.Errorf("args=%v output=%q, want %q", tt.args, out, info.hardwarePlatform+"\n")
						}
						return
					}
				}
				if out != tt.wantOut {
					t.Errorf("args=%v output=%q, want %q", tt.args, out, tt.wantOut)
				}
			}
		})
	}
}

type unameFailWriter struct {
	err   error
	short bool
}

func (w unameFailWriter) Write(p []byte) (int, error) {
	if w.short {
		return len(p) - 1, nil
	}
	return 0, w.err
}

func runUnameRaw(t *testing.T, env []string, out io.Writer, systemProbe probeFunc, args ...string) (string, int) {
	t.Helper()
	var errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Env: env, Stdio: tool.Stdio{In: strings.NewReader("unused"), Out: out, Err: &errb}}
	code := runWithProbe(rc, args, systemProbe)
	return errb.String(), code
}

func fixedProbe() (sysinfo, error) {
	return sysinfo{
		sysname: "S", nodename: "N", release: "R", version: "V", machine: "M",
		processor: "P", hardwarePlatform: "I",
	}, nil
}

func TestUnameIssue7SelectorCompositionAndOrder(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "S\n"},
		{[]string{"-vmnsr"}, "S N R V M\n"},
		{[]string{"-v", "-s", "-v", "-n"}, "S N V\n"},
		{[]string{"-a"}, "S N R V M\n"},
		{[]string{"-a", "-o", "-p", "-i"}, "S N R V M P I " + operatingSystem() + "\n"},
	} {
		var out bytes.Buffer
		errText, code := runUnameRaw(t, nil, &out, fixedProbe, tc.args...)
		if code != 0 || errText != "" || out.String() != tc.want {
			t.Errorf("args=%v code=%d stdout=%q stderr=%q want=%q", tc.args, code, out.String(), errText, tc.want)
		}
	}
}

// TestUnameIssue7OptionTerminatorAndSelectorOrder pins the XBD Utility
// Syntax Guidelines behind the Issue 7 synopsis "uname [-amnrsv]" and the
// STDOUT clause: "--" ends option parsing (later option-shaped tokens are
// operands, and OPERANDS is "None"), and output order never follows flag
// order.
func TestUnameIssue7OptionTerminatorAndSelectorOrder(t *testing.T) {
	// OPERANDS: None; after the terminator "-s" is an operand, rejected.
	var out bytes.Buffer
	errText, code := runUnameRaw(t, nil, &out, fixedProbe, "--", "-s")
	if code != 2 || out.Len() != 0 || !strings.Contains(errText, "extra operand") {
		t.Fatalf("terminator operand: code=%d stdout=%q stderr=%q", code, out.String(), errText)
	}

	// A lone "-" is also an operand, not standard input.
	out.Reset()
	errText, code = runUnameRaw(t, nil, &out, fixedProbe, "-")
	if code != 2 || out.Len() != 0 || !strings.Contains(errText, "extra operand") {
		t.Fatalf("dash operand: code=%d stdout=%q stderr=%q", code, out.String(), errText)
	}

	// A bare terminator leaves no selectors: the default -s applies.
	out.Reset()
	errText, code = runUnameRaw(t, nil, &out, fixedProbe, "--")
	if code != 0 || errText != "" || out.String() != "S\n" {
		t.Fatalf("bare terminator: code=%d stdout=%q stderr=%q", code, out.String(), errText)
	}

	// STDOUT: symbols are written in sysname nodename release version
	// machine order regardless of the order the selectors were typed.
	out.Reset()
	errText, code = runUnameRaw(t, nil, &out, fixedProbe, "-m", "-v", "-r", "-n", "-s")
	if code != 0 || errText != "" || out.String() != "S N R V M\n" {
		t.Fatalf("reverse selector order: code=%d stdout=%q stderr=%q", code, out.String(), errText)
	}
}

func TestUnameIssue7ProviderAndOutputFailures(t *testing.T) {
	t.Run("probe", func(t *testing.T) {
		var out bytes.Buffer
		errText, code := runUnameRaw(t, nil, &out, func() (sysinfo, error) { return sysinfo{}, errors.New("probe failed") })
		if code != 1 || out.String() != "" || errText != "uname: cannot get system name: probe failed\n" {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errText)
		}
	})

	t.Run("missing selected field", func(t *testing.T) {
		var out bytes.Buffer
		errText, code := runUnameRaw(t, nil, &out, func() (sysinfo, error) {
			info, _ := fixedProbe()
			info.version = ""
			return info, nil
		}, "-v")
		if code != 1 || out.String() != "" || errText != "uname: requested version is unavailable\n" {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errText)
		}
	})

	for _, tc := range []struct {
		name string
		out  io.Writer
	}{
		{"write error", unameFailWriter{err: errors.New("write failed")}},
		{"short write", unameFailWriter{short: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errText, code := runUnameRaw(t, nil, tc.out, fixedProbe, "-s")
			if code != 1 || !strings.HasPrefix(errText, "uname: write error: ") {
				t.Fatalf("code=%d stderr=%q", code, errText)
			}
		})
	}
}

func TestUnameExtensionsRemainAvailableWithPOSIXEnvironment(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"-p"}, "P\n"},
		{[]string{"-o"}, operatingSystem() + "\n"},
		{[]string{"--kernel-name"}, "S\n"},
	} {
		var out bytes.Buffer
		errText, code := runUnameRaw(t, []string{"POSIXLY_CORRECT="}, &out, fixedProbe, tc.args...)
		if code != 0 || errText != "" || out.String() != tc.want {
			t.Errorf("args=%v code=%d stdout=%q stderr=%q", tc.args, code, out.String(), errText)
		}
	}

	var out bytes.Buffer
	errText, code := runUnameRaw(t, []string{"POSIXLY_CORRECT="}, &out, fixedProbe, "-vmnsr")
	if code != 0 || errText != "" || out.String() != "S N R V M\n" {
		t.Fatalf("POSIX required selectors: code=%d stdout=%q stderr=%q", code, out.String(), errText)
	}
	out.Reset()
	errText, code = runUnameRaw(t, []string{"POSIXLY_CORRECT="}, &out, fixedProbe, "--help")
	if code != 0 || errText != "" || out.Len() == 0 {
		t.Fatalf("--help: code=%d stdout=%q stderr=%q", code, out.String(), errText)
	}
}
