package exprcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// TestExprIssue7DoubleDashDelimiter pins the XBD Utility Syntax Guideline 10
// end-of-options behavior that the expr APPLICATION USAGE requires: a leading
// "--" protects otherwise option-looking operands, including negative
// integers, in POSIX mode as well as in default mode.
func TestExprIssue7DoubleDashDelimiter(t *testing.T) {
	for _, env := range [][]string{nil, {"POSIXLY_CORRECT=1"}, {"POSIXLY_CORRECT="}} {
		t.Run(strings.Join(env, ","), func(t *testing.T) {
			var out, errb bytes.Buffer
			rc := &tool.RunContext{
				Ctx:   context.Background(),
				Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")},
				Env:   env,
			}
			code := run(rc, []string{"--", "-5", "+", "3"})
			if code != 0 || out.String() != "-2\n" || errb.String() != "" {
				t.Fatalf("expr -- -5 + 3 = (%q, %q, %d), want (-2, \"\", 0)", out.String(), errb.String(), code)
			}
		})
	}
}

// TestExprIssue7NonconflictingExtensionsSurvivePosixlyCorrect verifies that
// the GNU keyword extensions keep working under POSIXLY_CORRECT. Issue 7
// leaves results for the string arguments length, substr, index, and match
// unspecified, so the extension cannot conflict with the POSIX grammar and
// must not be stripped in POSIX mode. The --help/--version spellings are the
// same kind of nonconflicting extension.
func TestExprIssue7NonconflictingExtensionsSurvivePosixlyCorrect(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
		code int
	}{
		{"length keyword", []string{"length", "hello"}, "5\n", 0},
		{"substr keyword", []string{"substr", "hello", "2", "3"}, "ell\n", 0},
		{"index keyword", []string{"index", "hello", "l"}, "3\n", 0},
		{"match keyword", []string{"match", "hello", "l+"}, "0\n", 1},
		{"colon operator still anchors BRE", []string{"hello", ":", "l+"}, "0\n", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			rc := &tool.RunContext{
				Ctx:   context.Background(),
				Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")},
				Env:   []string{"POSIXLY_CORRECT=1"},
			}
			code := run(rc, tc.args)
			if code != tc.code {
				t.Errorf("expr %v: code=%d err=%q, want %d", tc.args, code, errb.String(), tc.code)
			}
			if out.String() != tc.want {
				t.Errorf("expr %v: out=%q, want %q", tc.args, out.String(), tc.want)
			}
		})
	}
}

// TestExprIssue7DoubleDashAsFirstTokenOnly verifies that only a leading "--"
// is a delimiter: stripping it leaves "=" without a left operand (a syntax
// error diagnosed on stderr with exit status 2), while a "--" appearing later
// is an ordinary string operand compared by "=".
func TestExprIssue7DoubleDashAsFirstTokenOnly(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")},
		Env:   []string{"POSIXLY_CORRECT=1"},
	}
	code := run(rc, []string{"--", "=", "--"})
	if code != 2 || out.String() != "" || errb.String() != "expr: syntax error\n" {
		t.Fatalf("expr -- = -- = (%q, %q, %d), want (\"\", \"expr: syntax error\\n\", 2)", out.String(), errb.String(), code)
	}
	out.Reset()
	errb.Reset()
	code = run(rc, []string{"a", "=", "--"})
	if code != 1 || out.String() != "0\n" || errb.String() != "" {
		t.Fatalf("expr a = -- = (%q, %q, %d), want (\"0\\n\", \"\", 1)", out.String(), errb.String(), code)
	}
}

func TestExprIssue7OptionLookingOperandsAndQuoteToken(t *testing.T) {
	for _, env := range [][]string{nil, {"POSIXLY_CORRECT=1"}, {"POSIXLY_CORRECT="}} {
		t.Run(strings.Join(env, ","), func(t *testing.T) {
			for _, operand := range []string{"-h", "-V"} {
				var out, errb bytes.Buffer
				rc := &tool.RunContext{
					Ctx:   context.Background(),
					Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")},
					Env:   env,
				}
				code := run(rc, []string{operand})
				if code != 0 || out.String() != operand+"\n" || errb.String() != "" {
					t.Fatalf("expr %s = (%q, %q, %d), want (%q, \"\", 0)", operand, out.String(), errb.String(), code, operand+"\n")
				}
			}

			var out, errb bytes.Buffer
			rc := &tool.RunContext{
				Ctx:   context.Background(),
				Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")},
				Env:   env,
			}
			code := run(rc, []string{"quote", "hello"})
			if code != 2 || out.String() != "" || errb.String() != "expr: syntax error\n" {
				t.Fatalf("expr quote hello = (%q, %q, %d), want (\"\", \"expr: syntax error\\n\", 2)", out.String(), errb.String(), code)
			}
		})
	}
}
