//go:build !windows

package edcmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/cmds/internal/editor"
	"github.com/qiangli/coreutils/tool"
)

func runEdIn(t *testing.T, dir, input string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, FS: tool.NewLocalFS(),
		Env:   []string{"PATH=/bin:/usr/bin"},
		Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &errb}}
	return tool.Lookup("ed").Run(rc, args), out.String(), errb.String()
}

func TestPOSIXMarksMoveCopyJoinUndoAndSuffixes(t *testing.T) {
	in := "a\none\ntwo\nthree\nfour\n.\n2ka\n2,3m0\n'ap\n1,2jn\nu\n1,2t$\n$n\nQ\n"
	code, out, errb := runEdIn(t, t.TempDir(), in, "-s")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errb)
	}
	if !strings.Contains(out, "two\n") || !strings.Contains(out, "1\ttwothree\n") || !strings.HasSuffix(out, "6\tthree\n") {
		t.Fatalf("unexpected command output %q", out)
	}
}

func TestPOSIXJoinSingleAddressUsesThatLineOnly(t *testing.T) {
	code, out, errb := runEdIn(t, t.TempDir(), "a\none\ntwo\nthree\n.\n2j\n2p\nQ\n", "-s")
	if code != 0 || errb != "" || out != "two\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errb)
	}
}

func TestPOSIXAddressChainsRetainLastExpectedAddresses(t *testing.T) {
	in := "a\none\ntwo\nthree\nfour\n.\n1,2,3p\n1,2,p\n,,p\nQ\n"
	code, out, errb := runEdIn(t, t.TempDir(), in, "-s")
	if code != 0 || errb != "" || out != "two\nthree\ntwo\nfour\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errb)
	}
}

func TestPOSIXEOFEndsInputMode(t *testing.T) {
	code, out, errb := runEdIn(t, t.TempDir(), "a\nlast line\n", "-s")
	if code != 1 || errb != "" || out != "?\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errb)
	}
}

func TestPOSIXEmptyChangeAndEditUndoRules(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "replacement"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	in := "a\none\ntwo\nthree\n.\n2c\n.\n.p\nE replacement\nu\nQ\n"
	code, out, errb := runEdIn(t, dir, in, "-s")
	if code != 1 || errb != "" || out != "three\n?\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errb)
	}
}

func TestPOSIXGlobalInverseInteractiveAndPromptToggle(t *testing.T) {
	in := "a\nalpha\nbeta\nalpine\n.\ng/^a/s/a/A/p\nv/^A/d\n1,$p\nG/^A/\np\n\nP\nP\nQ\n"
	code, out, errb := runEdIn(t, t.TempDir(), in, "-s")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errb)
	}
	if strings.Contains(out, "beta") || !strings.Contains(out, "Alpha") || !strings.Contains(out, "Alpine") {
		t.Fatalf("global output %q", out)
	}
}

func TestPOSIXGlobalInputModeAndInteractiveRecall(t *testing.T) {
	in := "a\nx\ny\nx\n.\ng/x/a\\\nadded\\\n.\nG/^x$/\ns/x/X/\n&\n1,$p\nQ\n"
	code, out, errb := runEdIn(t, t.TempDir(), in, "-s")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errb)
	}
	if !strings.HasSuffix(out, "X\nadded\ny\nX\nadded\n") {
		t.Fatalf("global input/recall output=%q", out)
	}
}

func TestPOSIXReadAppendWriteAndShellForms(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	in := "0r in\nr !printf 'shell\\n'\n1,$W out\n1,$W out\nw !wc -l\n!printf 'bang\\n'\nQ\n"
	code, out, errb := runEdIn(t, dir, in, "-s")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errb)
	}
	data, err := os.ReadFile(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "file\nshell\nfile\nshell\n" {
		t.Fatalf("append output=%q", data)
	}
	if !strings.Contains(out, "2\n") || !strings.Contains(out, "bang\n") {
		t.Fatalf("shell output=%q", out)
	}
}

func TestPOSIXRepeatedEditWarningAndRememberedWriteFilename(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "replacement"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	in := "f remembered\na\nold\n.\ne replacement\ne replacement\n1p\nw other\nf\nQ\n"
	code, out, errb := runEdIn(t, dir, in, "-s")
	if code != 1 || errb != "" || out != "remembered\n?\nnew\nreplacement\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errb)
	}
}

func TestPOSIXListEscapes(t *testing.T) {
	code, out, errb := runEdIn(t, t.TempDir(), "a\n\\\t\a\v$\u0085\n.\n1l\nQ\n", "-s")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errb)
	}
	if out != "\\\\\\t\\a\\v\\$\\302\\205$\n" {
		t.Fatalf("list=%q", out)
	}
}

func TestPOSIXNonTerminalInputStopsAtFirstCommandError(t *testing.T) {
	dir := t.TempDir()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(writer, "bogus\na\nnot-written\n.\nw result\nQ\n"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, FS: tool.NewLocalFS(),
		Env:   []string{"PATH=/bin:/usr/bin"},
		Stdio: tool.Stdio{In: reader, Out: &out, Err: &errb}}
	if code := tool.Lookup("ed").Run(rc, []string{"-s"}); code != 1 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "result")); !os.IsNotExist(err) {
		t.Fatalf("commands after first error executed: stat err=%v", err)
	}
}

func TestPOSIXHangupSavesModifiedBuffer(t *testing.T) {
	signals := make(chan string, 1)
	signals <- "hangup"
	var saved []byte
	eng := &editor.Engine{
		Buffer:      editor.Buffer{Lines: []string{"recover"}, Current: 1, Dirty: true},
		Out:         io.Discard,
		ExitOnError: true,
		Signals:     signals,
		Hangup: func(data []byte) error {
			saved = append([]byte(nil), data...)
			return nil
		},
	}
	reader, writer := io.Pipe()
	defer writer.Close()
	done := make(chan int, 1)
	go func() { done <- eng.Run(reader) }()
	select {
	case code := <-done:
		if code != 1 || string(saved) != "recover\n" {
			t.Fatalf("code=%d saved=%q", code, saved)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("hangup did not interrupt command input")
	}
}

func TestPOSIXShellFilenameRecallAndQuoting(t *testing.T) {
	in := "f current\n!printf '<\\%s>\\n' %\n!printf x\n!!\n!printf '<\\%s>\\n' \\%\nQ\n"
	code, out, errb := runEdIn(t, t.TempDir(), in, "-s")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errb)
	}
	if out != "current\nprintf '<%s>\\n' current\n<current>\nxprintf x\nx<%>\n" {
		t.Fatalf("shell expansions=%q", out)
	}
}
