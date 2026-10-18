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
	const want = "explain: sample\nusage: sample [OPTION]... [FILE]...\nflags: --all\noperands: f1 f2\n" +
		"effects: none; arguments were parsed and validated only, the command was not executed\n"
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

func TestNoInputRejectsPromptingFlag(t *testing.T) {
	r := runCommon(t, nil, func(fs interface {
		Bool(string, bool, string) *bool
	}) { fs.Bool("interactive", false, "") }, "--no-input", "--interactive")
	if r.code != 2 || !strings.Contains(r.err, "would prompt") || r.stdin != "piped" {
		t.Fatalf("%+v", r)
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
