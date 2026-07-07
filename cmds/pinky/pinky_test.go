package pinkycmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestPinkyNoUsers(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}, nil)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
}

func TestPinkyHelp(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}, []string{"--help"})
	if code != 0 || !strings.Contains(out.String(), "Usage: pinky") {
		t.Fatalf("--help: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestPinkyVersion(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}, []string{"-V"})
	if code != 0 || !strings.Contains(out.String(), "qiangli/coreutils") {
		t.Fatalf("-V: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestPinkyHFlag(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}, []string{"-h"})
	if code != 0 {
		t.Fatalf("-h (no-plan): code=%d err=%q", code, errb.String())
	}
}

func TestPinkyQuick(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}, []string{"-q"})
	if code != 0 {
		t.Fatalf("-q: code=%d err=%q", code, errb.String())
	}
}

func TestPinkyLookup(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}, []string{"-i"})
	if code != 0 {
		t.Fatalf("-i: code=%d err=%q", code, errb.String())
	}
	out2, errb2 := bytes.Buffer{}, bytes.Buffer{}
	code2 := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out2, Err: &errb2}}, []string{"--lookup"})
	if code2 != 0 {
		t.Fatalf("--lookup: code=%d err=%q", code2, errb2.String())
	}
}
