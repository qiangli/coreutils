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
