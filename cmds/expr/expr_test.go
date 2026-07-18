package exprcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func checkExpr(t *testing.T, wantOut string, wantCode int, args ...string) {
	t.Helper()
	var out, err bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}
	if code := run(rc, args); code != wantCode || out.String() != wantOut {
		t.Errorf("expr %q = (code=%d, out=%q, err=%q), want (code=%d, out=%q)", args, code, out.String(), err.String(), wantCode, wantOut)
	}
}

func TestExprArithmetic(t *testing.T) {
	var out, err bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, []string{"1", "+", "2", "*", "3"})
	if code != 0 || out.String() != "7\n" {
		t.Fatalf("code=%d out=%q err=%s", code, out.String(), err.String())
	}
}

func TestExprMatch(t *testing.T) {
	var out, err bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, []string{"abc123", ":", "[a-z]*\\([0-9]*\\)"})
	if code != 0 || out.String() != "123\n" {
		t.Fatalf("code=%d out=%q err=%s", code, out.String(), err.String())
	}
}

func TestExprHelpVersionAliases(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"--version"}, {"-V"}} {
		var out, err bytes.Buffer
		code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, args)
		if code != 0 || err.String() != "" || out.String() == "" {
			t.Fatalf("expr %v: code=%d out=%q err=%q", args, code, out.String(), err.String())
		}
		if args[0] == "--help" && (!strings.Contains(out.String(), "--help") || !strings.Contains(out.String(), "--version")) {
			t.Fatalf("expr help missing long options: %q", out.String())
		}
	}
}

func TestExprPOSIXArithmeticAndComparison(t *testing.T) {
	checkExpr(t, "7\n", 0, "1", "+", "2", "*", "3")
	checkExpr(t, "9223372036854775808\n", 0, "9223372036854775807", "+", "1")
	checkExpr(t, "18446744073709551616\n", 0, "4294967296", "*", "4294967296")
	checkExpr(t, "-2\n", 0, "-7", "/", "3")
	checkExpr(t, "-1\n", 0, "-7", "%", "3")
	checkExpr(t, "1\n", 0, "0000000000000000000002", ">", "1")
	checkExpr(t, "1\n", 0, "01", "=", "1")
	checkExpr(t, "", 2, "1", "/", "0")
}

func TestExprPOSIXBooleanAndExitStatus(t *testing.T) {
	checkExpr(t, "2\n", 0, "2", "&", "3")
	checkExpr(t, "0\n", 1, "0", "&", "3")
	checkExpr(t, "1\n", 0, "1", "|", "2")
	checkExpr(t, "2\n", 0, "0", "|", "2")
	checkExpr(t, "0\n", 1, "", "|", "")
	checkExpr(t, "0\n", 1, "0")
	checkExpr(t, "-0\n", 1, "-0")
	checkExpr(t, "value\n", 0, "value")
}

func TestExprLeadingPlusQuotesKeyword(t *testing.T) {
	checkExpr(t, "length\n", 0, "+", "length")
	checkExpr(t, "match\n", 0, "+", "match")
	checkExpr(t, "0\n", 1, "+", "0")
	checkExpr(t, "+\n", 0, "+", "+")
}

func TestExprPOSIXMatchAndStringFunctions(t *testing.T) {
	checkExpr(t, "ab\n", 0, "abab", ":", `\(ab\)\1`)
	checkExpr(t, "3\n", 0, "abc123", ":", "[[:alpha:]]*")
	checkExpr(t, "\n", 1, "abc", ":", `a\(z\)`)
	checkExpr(t, "2\n", 0, "length", "éx")
	checkExpr(t, "bc\n", 0, "substr", "abc", "2", "5")
	checkExpr(t, "\n", 1, "substr", "abc", "0", "2")
	checkExpr(t, "2\n", 0, "index", "abc", "xcb")
	checkExpr(t, "b\n", 0, "match", "abc", `a\(b\)`)
}
