package tool

import (
	"bytes"
	"testing"

	"github.com/spf13/pflag"
)

func runNormalize(t *testing.T, env []string, args ...string) (bool, string, int) {
	t.Helper()
	tl := &Tool{Name: "sample", Usage: "sample [OPTION]..."}
	fs := NewFlags("sample")
	fs.Bool("no-dereference", false, "")
	fs.String("out_file", "", "")
	var errb bytes.Buffer
	rc := &RunContext{Env: env, Out: &bytes.Buffer{}, Err: &errb}
	_, code := Parse(rc, tl, fs, args)
	got, _ := fs.GetBool("no-dereference")
	return got, errb.String(), code
}

func TestUnderscoreNormalizedOutsideCert(t *testing.T) {
	got, err, code := runNormalize(t, nil, "--no_dereference")
	if code != -1 || !got || err != "" {
		t.Fatalf("got=%v code=%d err=%q", got, code, err)
	}
}

func TestUnderscoreExactDefinedNameWins(t *testing.T) {
	_, err, code := runNormalize(t, nil, "--out_file=x")
	if code != -1 || err != "" {
		t.Fatalf("code=%d err=%q", code, err)
	}
}

func TestUnderscoreNoOpInCertProfile(t *testing.T) {
	_, err, code := runNormalize(t, []string{"VSC_PROFILE=cert"}, "--no_dereference")
	const want = "sample: unknown flag: --no_dereference\nsample: not every GNU flag is implemented in pure-Go coreutils — see 'sample --help' for the supported subset\n"
	if code != 2 || err != want {
		t.Fatalf("code=%d err=%q", code, err)
	}
}

func TestCertProfileHelpGoldenAndNoRegistration(t *testing.T) {
	run := func(env []string) (string, []string) {
		tl := &Tool{Name: "sample", Usage: "sample [OPTION]..."}
		fs := NewFlags("sample")
		fs.Bool("no-dereference", false, "do not dereference")
		var out bytes.Buffer
		rc := &RunContext{Env: env, Out: &out, Err: &bytes.Buffer{}}
		if _, code := Parse(rc, tl, fs, []string{"--help"}); code != 0 {
			t.Fatalf("code=%d", code)
		}
		var names []string
		fs.VisitAll(func(f *pflag.Flag) { names = append(names, f.Name) })
		return out.String(), names
	}
	const want = "Usage: sample [OPTION]...\n\nOptions:\n" +
		"  -h, --help             display this help and exit\n" +
		"  -V, --version          output version information and exit\n" +
		"      --no-dereference   do not dereference\n" +
		"\nImplements the documented GNU semantics for the flags above;\nanything else fails with a clear error (exit 2), never a silent guess.\n"
	cert, names := run([]string{"VSC_PROFILE=cert"})
	if len(names) != 5 {
		t.Fatalf("cert profile registered extra flags: %v", names)
	}
	if cert != want {
		t.Fatalf("cert help drifted:\n%q", cert)
	}
}

func TestUnderscoreIgnoredAfterDoubleDash(t *testing.T) {
	tl := &Tool{Name: "sample", Usage: "sample"}
	fs := NewFlags("sample")
	fs.Bool("no-dereference", false, "")
	rc := &RunContext{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	ops, code := Parse(rc, tl, fs, []string{"--", "--no_dereference"})
	if code != -1 || len(ops) != 1 || ops[0] != "--no_dereference" {
		t.Fatalf("ops=%v code=%d", ops, code)
	}
}
