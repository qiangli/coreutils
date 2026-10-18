package tool

import (
	"bytes"
	"testing"
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
