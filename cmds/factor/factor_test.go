package factorcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestFactor(t *testing.T) {
	var out, err bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, []string{"36"})
	if code != 0 || out.String() != "36: 2 2 3 3\n" {
		t.Fatalf("code=%d out=%q err=%s", code, out.String(), err.String())
	}
}

func TestFactorExponents(t *testing.T) {
	var out, err bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, []string{"--exponents", "36"})
	if code != 0 || out.String() != "36: 2^2 3^2\n" {
		t.Fatalf("code=%d out=%q err=%s", code, out.String(), err.String())
	}
}

func TestFactorHelpVersionAndShortH(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"--version"}, {"-V"}} {
		var out, err bytes.Buffer
		code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, args)
		if code != 0 || err.String() != "" || out.String() == "" {
			t.Fatalf("factor %v: code=%d out=%q err=%q", args, code, out.String(), err.String())
		}
		if args[0] == "--help" {
			for _, want := range []string{"--exponents", "--help", "--version"} {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("factor help missing %q in %q", want, out.String())
				}
			}
		}
	}
	var out, err bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, []string{"-h", "36"})
	if code != 0 || out.String() != "36: 2^2 3^2\n" {
		t.Fatalf("factor -h remains exponents: code=%d out=%q err=%q", code, out.String(), err.String())
	}
}
