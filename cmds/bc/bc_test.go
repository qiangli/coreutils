package bccmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runBC(t *testing.T, input string, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &err}, FS: tool.NewLocalFS()}
	code := run(rc, args)
	return out.String(), err.String(), code
}

func TestPOSIXArithmeticScaleAndBases(t *testing.T) {
	in := `2+3*4
scale=3
1/8
scale(1.230)
length(123.40)
ibase=16
FF
ibase=A
obase=16
255
`
	out, errOut, code := runBC(t, in)
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	want := "14\n.125\n3\n5\n255\nFF\n"
	if out != want {
		t.Fatalf("stdout=%q, want %q", out, want)
	}
}

func TestPOSIXFunctionsArraysAndControlFlow(t *testing.T) {
	in := `define f(n) {
auto i,r
r=1
for(i=2;i<=n;i++) { r*=i }
return(r)
}
f(10)
a[2]=7
while(a[2]>4) { a[2]-- }
a[2]
if(a[2]==4) { "ok" }
`
	out, errOut, code := runBC(t, in)
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	if out != "3628800\n7\n6\n5\n4\nok" {
		t.Fatalf("stdout=%q", out)
	}
}

func TestFilesPrecedeStandardInputAndShareState(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/program.bc"
	if err := os.WriteFile(path, []byte("x=41\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := runBC(t, "x+1\n", path)
	if code != 0 || errOut != "" || out != "42\n" {
		t.Fatalf("got (%q,%q,%d)", out, errOut, code)
	}
}

func TestReadConsumesExpressionFromStandardInput(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/program.bc"
	if err := os.WriteFile(path, []byte("read()*2\nquit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := runBC(t, "21\n", path)
	if code != 0 || errOut != "" || out != "42\n" {
		t.Fatalf("got (%q,%q,%d)", out, errOut, code)
	}
}

func TestDiagnosticsAndExitStatus(t *testing.T) {
	_, errOut, code := runBC(t, "1/0\n")
	if code == 0 || !strings.Contains(errOut, "divide by zero") {
		t.Fatalf("got stderr=%q code=%d", errOut, code)
	}
}

func TestPOSIXMathLibrary(t *testing.T) {
	out, errOut, code := runBC(t, "s(0)\nc(0)\ns(1)\nc(1)\na(1)\nl(e(1))\n", "-l")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 6 || lines[0] != "0" || lines[1] != "1.00000000000000000000" || !strings.HasPrefix(lines[2], ".841470984807") || !strings.HasPrefix(lines[3], ".540302305868") || !strings.HasPrefix(lines[4], ".785398163397") || !strings.HasPrefix(lines[5], ".999999999999") {
		t.Fatalf("unexpected math output %q", out)
	}
}

func TestDecimalScaleRulesAndFractionalOutputBases(t *testing.T) {
	in := `scale=0
.5*.5
scale=3
1.20*3.4
5.5%2
2^10
scale=4
2^-3
sqrt(2)
obase=2
1/3
obase=8
1/3
obase=16
1/3
`
	out, errOut, code := runBC(t, in)
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	want := ".2\n4.080\n0\n1024\n.1250\n1.4142\n.01010101010100\n.25251\n.5553\n"
	if out != want {
		t.Fatalf("stdout=%q, want %q", out, want)
	}
}

func TestPOSIXOptionAndOperandContract(t *testing.T) {
	out, errOut, code := runBC(t, "scale\nc(0)\n", "-l")
	if code != 0 || errOut != "" || out != "20\n1.00000000000000000000\n" {
		t.Fatalf("-l: stdout=%q stderr=%q code=%d", out, errOut, code)
	}
	_, errOut, code = runBC(t, "", "-x")
	if code != 2 || !strings.Contains(errOut, "unknown shorthand flag") {
		t.Fatalf("unknown option: stderr=%q code=%d", errOut, code)
	}
}

func TestMissingInputStopsProcessing(t *testing.T) {
	out, errOut, code := runBC(t, "9\n", t.TempDir()+"/missing.bc")
	if code == 0 || out != "" || !strings.Contains(errOut, "missing.bc") {
		t.Fatalf("stdout=%q stderr=%q code=%d", out, errOut, code)
	}
}

func TestPOSIXLocaleIndependentRadix(t *testing.T) {
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Env:   []string{"LANG=de_DE.UTF-8", "LC_NUMERIC=de_DE.UTF-8"},
		Stdio: tool.Stdio{In: strings.NewReader("scale=2;1/4\n"), Out: &out, Err: &errOut},
		FS:    tool.NewLocalFS(),
	}
	code := run(rc, nil)
	if code != 0 || errOut.String() != "" || out.String() != ".25\n" {
		t.Fatalf("stdout=%q stderr=%q code=%d", out.String(), errOut.String(), code)
	}
}

func TestPOSIXInteractiveExecutionRecoveryAndFlush(t *testing.T) {
	old := isTerminalFn
	isTerminalFn = func(any) bool { return true }
	t.Cleanup(func() { isTerminalFn = old })
	input := "1\n@\n2\ndefine f(x) {\nreturn(x+1)\n}\nf(4)\n"
	out, errOut, code := runBC(t, input)
	if code != 0 || out != "1\n2\n5\n" || !strings.Contains(errOut, "invalid character") {
		t.Fatalf("stdout=%q stderr=%q code=%d", out, errOut, code)
	}
}

// Set COREUTILS_BC_REFERENCE to GNU bc 1.07.1 or Gavin Howard bc to run the
// differential corpus. The ordinary suite remains hermetic and requires no
// host utility.
func TestDifferentialPOSIXReference(t *testing.T) {
	ref := os.Getenv("COREUTILS_BC_REFERENCE")
	if ref == "" {
		t.Skip("COREUTILS_BC_REFERENCE is not set")
	}
	programs := []struct {
		args []string
		src  string
	}{
		{src: "scale=3\n1/8\n1.20*3.4\n5.50%2.0\n2^2.0\n"},
		{src: "i=0\nwhile(i<3) i++\nfor(j=0;j<3;j++) j\n"},
		{src: "define f(a[],x) {\na[0]=9\nreturn(a[1]+x)\n}\nb[0]=1\nb[1]=4\nf(b[],3)\nb[0]\n"},
		{src: "ibase=8\nibase=A\nobase=20\n25\n"},
		{args: []string{"-l"}, src: "scale=12\ns(-3.25)\nc(8.5)\na(-2)\ne(-1.5)\nl(.125)\nl(e(1))\nj(0,1)\nj(1,2.5)\nj(-2,3)\n"},
	}
	for _, tc := range programs {
		got, gotErr, code := runBC(t, tc.src, tc.args...)
		if code != 0 || gotErr != "" {
			t.Fatalf("Go bc failed for %q: stdout=%q stderr=%q code=%d", tc.src, got, gotErr, code)
		}
		c := exec.Command(ref, tc.args...)
		c.Stdin = strings.NewReader(tc.src)
		want, err := c.Output()
		if err != nil {
			t.Fatalf("reference failed for %q: %v", tc.src, err)
		}
		if got != string(want) {
			t.Errorf("program %q:\nGo=%q\nreference=%q", tc.src, got, string(want))
		}
	}
}

// COREUTILS_BC_CORPUS_DIR may point at tests/bc from Gavin Howard's
// BSD-2-Clause bc repository. These files are not vendored; the gate is an
// opt-in broad differential check over its POSIX arithmetic corpus.
func TestDifferentialGavinPOSIXCorpus(t *testing.T) {
	dir := os.Getenv("COREUTILS_BC_CORPUS_DIR")
	if dir == "" {
		t.Skip("COREUTILS_BC_CORPUS_DIR is not set")
	}
	for _, name := range []string{"add", "multiply", "divide", "modulus", "power", "sqrt", "scale", "arrays", "functions"} {
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(dir, name+".txt"))
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join(dir, name+"_results.txt"))
			if err != nil {
				t.Fatal(err)
			}
			got, stderr, code := runBC(t, string(src), "-l")
			if code != 0 || stderr != "" {
				t.Fatalf("stderr=%q code=%d", stderr, code)
			}
			if got != string(want) {
				gotLines, wantLines := strings.Split(got, "\n"), strings.Split(string(want), "\n")
				for i := 0; i < len(gotLines) && i < len(wantLines); i++ {
					if gotLines[i] != wantLines[i] {
						t.Fatalf("first difference at output line %d: got %q, want %q", i+1, gotLines[i], wantLines[i])
					}
				}
				t.Fatalf("output line count differs: got %d, want %d", len(gotLines), len(wantLines))
			}
		})
	}
}
