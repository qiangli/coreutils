package tool

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

type commonRun struct {
	out, err string
	code     int
	ops      []string
	zero     bool
	stdin    string
}

func runCommon(t *testing.T, env []string, own func(fs interface {
	Bool(string, bool, string) *bool
}), args ...string) commonRun {
	t.Helper()
	tl := &Tool{Name: "sample", Usage: "sample [OPTION]... [FILE]..."}
	fs := NewFlags("sample")
	fs.BoolP("all", "a", false, "all")
	zero := fs.Bool("zero", false, "zero")
	if own != nil {
		own(fs)
	}
	var out, errb bytes.Buffer
	in := strings.NewReader("piped")
	rc := &RunContext{Env: env, Stdio: Stdio{In: in, Out: &out, Err: &errb}}
	ops, code := Parse(rc, tl, fs, args)
	rest, _ := io.ReadAll(in)
	return commonRun{out.String(), errb.String(), code, ops, *zero, string(rest)}
}

func TestExplainNeverRuns(t *testing.T) {
	r := runCommon(t, nil, nil, "--explain", "-a", "f1", "f2")
	const want = "explain: sample\nusage: sample [OPTION]... [FILE]...\nflags: --all\ncommon: --explain\noperands: f1 f2\n" +
		"effects: not evaluated; the command was not executed and its side effects are not known to the argument parser\n"
	if r.code != 0 || r.out != want || r.ops != nil {
		t.Fatalf("%+v", r)
	}
}

func TestExplainStillValidatesArgv(t *testing.T) {
	r := runCommon(t, nil, nil, "--explain", "--bogus")
	if r.code != 2 || !strings.Contains(r.err, "unknown flag: --bogus") || r.out != "" {
		t.Fatalf("%+v", r)
	}
}

func TestNoInputKeepsStdin(t *testing.T) {
	r := runCommon(t, nil, nil, "--no-input", "f")
	if r.code != -1 || r.stdin != "piped" || len(r.ops) != 1 {
		t.Fatalf("%+v", r)
	}
}

func TestNoInputPromptBlocked(t *testing.T) {
	rc := &RunContext{Stdio: Stdio{In: strings.NewReader("y\n"), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}}
	if PromptBlocked(rc, "x") {
		t.Fatal("blocked without --no-input")
	}
	rc.NoInput = true
	if !PromptBlocked(rc, "x") || !PromptBlocked(rc, "x") {
		t.Fatal("not blocked")
	}
	if got := rc.Err.(*bytes.Buffer).String(); strings.Count(got, "--no-input forbids prompting") != 1 {
		t.Fatalf("%q", got)
	}
}

func TestNoInputPromptExitDistinct(t *testing.T) {
	tl := &Tool{Name: "prompty", Run: func(rc *RunContext, args []string) int {
		PromptBlocked(rc, "prompty")
		return 0
	}}
	Register(tl)
	rc := &RunContext{Stdio: Stdio{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}, NoInput: true}
	if code := tl.Run(rc, nil); code != 2 {
		t.Fatalf("code=%d", code)
	}
}

func TestExplainWithJSONFieldsStillExplains(t *testing.T) {
	r := runCommon(t, nil, nil, "--explain", "--json", "--fields=a", "-a", "f")
	if r.code != 0 || !strings.HasPrefix(r.out, "explain: sample\n") || !strings.Contains(r.out, "common: --explain --json --fields=a\n") ||
		strings.Contains(r.out, "effects: none") || r.err != "" || r.stdin != "piped" {
		t.Fatalf("%+v", r)
	}
}

func TestCommonOptionsInHelpOnlyOutsideCert(t *testing.T) {
	help := func(env []string) string { return runCommon(t, env, nil, "--help").out }
	if h := help(nil); !strings.Contains(h, "--no-input") || !strings.Contains(h, "--explain") || !strings.Contains(h, "--fields") {
		t.Fatalf("missing from help: %s", h)
	}
	if h := help([]string{"VSC_PROFILE=cert"}); strings.Contains(h, "--no-input") || strings.Contains(h, "--json") {
		t.Fatalf("leaked into cert help: %s", h)
	}
}

func TestCommonOptionsDoNotAbbreviate(t *testing.T) {
	tl := &Tool{Name: "sample", Usage: "sample"}
	fs := NewFlags("sample")
	d := fs.Bool("no-dereference", false, "")
	_, code := Parse(&RunContext{Stdio: Stdio{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}}, tl, fs, []string{"--no-d"})
	if code != -1 || !*d {
		t.Fatalf("code=%d", code)
	}
}

func TestJSONAndFieldsUnsupported(t *testing.T) {
	for _, args := range [][]string{{"--json"}, {"--fields=a,b"}, {"--fields", "a"}, {"--json", "--fields=a"}} {
		r := runCommon(t, nil, nil, args...)
		if r.code != 2 || !strings.Contains(r.err, "not supported by pure-Go coreutils") || r.out != "" {
			t.Fatalf("%v: %+v", args, r)
		}
	}
}

func TestNullMapsToNativeZero(t *testing.T) {
	r := runCommon(t, nil, nil, "--null", "f")
	if r.code != -1 || !r.zero || len(r.ops) != 1 {
		t.Fatalf("%+v", r)
	}
}

func TestNullUnsupportedWithoutNative(t *testing.T) {
	tl := &Tool{Name: "sample", Usage: "sample"}
	fs := NewFlags("sample")
	var errb bytes.Buffer
	_, code := Parse(&RunContext{Stdio: Stdio{Out: &bytes.Buffer{}, Err: &errb}}, tl, fs, []string{"--null"})
	if code != 2 || !strings.Contains(errb.String(), "--null") {
		t.Fatalf("code=%d %q", code, errb.String())
	}
}

func TestOwnFlagWins(t *testing.T) {
	tl := &Tool{Name: "sample", Usage: "sample"}
	fs := NewFlags("sample")
	j := fs.Bool("json", false, "")
	f := fs.String("fields", "", "")
	n := fs.Bool("null", false, "")
	e := fs.Bool("explain", false, "")
	ops, code := Parse(&RunContext{Stdio: Stdio{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}}, tl, fs,
		[]string{"--json", "--fields", "x", "--null", "--explain", "op"})
	if code != -1 || !*j || *f != "x" || !*n || !*e || len(ops) != 1 {
		t.Fatalf("code=%d j=%v f=%q n=%v e=%v ops=%v", code, *j, *f, *n, *e, ops)
	}
}

func TestOptionValueNotMisreadAsCommon(t *testing.T) {
	tl := &Tool{Name: "sample", Usage: "sample"}
	fs := NewFlags("sample")
	s := fs.String("label", "", "")
	_, code := Parse(&RunContext{Stdio: Stdio{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}}, tl, fs, []string{"--label", "--json"})
	if code != -1 || *s != "--json" {
		t.Fatalf("code=%d s=%q", code, *s)
	}
}

// Cert-profile bytes must equal the pre-feature baseline: every common
// option is an ordinary unknown flag.
func TestCertProfileCommonOptionsGolden(t *testing.T) {
	for _, o := range []string{"--explain", "--no-input", "--json", "--fields=a", "--null"} {
		r := runCommon(t, []string{"VSC_PROFILE=cert"}, nil, o)
		name, _, _ := strings.Cut(o, "=")
		want := "sample: unknown flag: " + name + "\nsample: not every GNU flag is implemented in pure-Go coreutils — see 'sample --help' for the supported subset\n"
		if r.code != 2 || r.err != want || r.out != "" || r.zero {
			t.Fatalf("%s: %+v", o, r)
		}
	}
}
