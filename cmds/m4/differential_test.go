package m4cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// externalTimeout bounds every run of the external m4. A macro processor
// loops forever on recursive input, so the reference binary is never run
// without a deadline or with unbounded output.
const externalTimeout = 10 * time.Second

const externalOutputLimit = 1 << 20

var errOutputLimit = errors.New("output limit exceeded")

// cappedBuffer fails the write that would take it past its limit, which
// closes the pipe and stops a runaway child.
type cappedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, errOutputLimit
	}
	return b.Buffer.Write(p)
}

func runExternalM4(t *testing.T, bin, input string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), externalTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, bin, args...)
	c.Stdin = bytes.NewReader([]byte(input))
	out := &cappedBuffer{limit: externalOutputLimit}
	c.Stdout = out
	c.WaitDelay = time.Second
	err := c.Run()
	if ctx.Err() != nil {
		t.Fatalf("external m4 %v exceeded %v", args, externalTimeout)
	}
	return out.String(), err
}

func runExternalM4Result(t *testing.T, bin, input string, args ...string) (string, string, int) {
	return runExternalM4ContextResult(t, bin, "", input, args...)
}

func runExternalM4ContextResult(t *testing.T, bin, dir, input string, args ...string) (string, string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), externalTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, bin, args...)
	c.Dir = dir
	c.Stdin = strings.NewReader(input)
	out := &cappedBuffer{limit: externalOutputLimit}
	errOut := &cappedBuffer{limit: externalOutputLimit}
	c.Stdout, c.Stderr = out, errOut
	c.WaitDelay = time.Second
	err := c.Run()
	if ctx.Err() != nil {
		t.Fatalf("external m4 %v exceeded %v", args, externalTimeout)
	}
	if err == nil {
		return out.String(), errOut.String(), 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return out.String(), errOut.String(), exitErr.ExitCode()
	}
	t.Fatalf("external m4 %v could not run: %v", args, err)
	return "", "", 127
}

// cmdlineMacros are the -D/-U options of the third differential run;
// testdata/cmdline.m4 is the file that looks at them. They name nothing
// the rest of the corpus uses, so no other file changes meaning.
var cmdlineMacros = []string{"-Dcmdline_macro=set by -D", "-Dcmdline_empty", "-Dcmdline_gone=1", "-Ucmdline_gone", "-D", "cmdline_late=a=b"}

// runM4Deadline is runM4 under the same deadline as the external m4.
func runM4Deadline(t *testing.T, input string, args ...string) (string, string, int) {
	return runM4DeadlineContext(t, "", nil, input, args...)
}

func runM4DeadlineContext(t *testing.T, dir string, env []string, input string, args ...string) (string, string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), externalTimeout)
	defer cancel()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: ctx, Dir: dir, Env: env, Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &errb}, FS: tool.NewLocalFS()}
	code := run(rc, args)
	return out.String(), errb.String(), code
}

// externalM4 locates a working reference m4, skipping the test when the
// host has none (or only an unprovisioned provider stub).
func externalM4(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("m4")
	if err != nil {
		t.Skip("no external m4 on PATH")
	}
	if out, err := runExternalM4(t, bin, "define(`probe', `ok')probe\n"); err != nil || out != "ok\n" {
		t.Skipf("external m4 %s is not usable: out=%q err=%v", bin, out, err)
	}
	return bin
}

// TestDifferentialCorpus runs every testdata/*.m4 through this
// implementation and through the external m4 and compares stdout, both
// plain and with -s.
func TestDifferentialCorpus(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.m4"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no corpus: %v (err=%v)", files, err)
	}
	bin := externalM4(t)
	for _, file := range files {
		for _, args := range [][]string{{file}, {"-s", file}, append(append([]string{}, cmdlineMacros...), file)} {
			t.Run(filepath.Base(file)+"/"+args[0], func(t *testing.T) {
				want, err := runExternalM4(t, bin, "", args...)
				if err != nil {
					t.Fatalf("external m4 %v: %v", args, err)
				}
				got, errOut, code := runM4Deadline(t, "", args...)
				if code != 0 {
					t.Fatalf("m4 %v: code=%d stderr=%q", args, code, errOut)
				}
				// GNU releases disagree on some -s source locations around
				// multiline expansions and m4wrap. The corpus checks expanded
				// content; TestDifferentialIncludeDiagnosticAndSynclineIdentity
				// checks the source identity contract independently.
				if args[0] == "-s" {
					if !strings.Contains(got, "#line ") || !strings.Contains(want, "#line ") {
						t.Fatalf("m4 %v omitted synclines: got=%q want=%q", args, got, want)
					}
					got = stripSynclines(got)
					want = stripSynclines(want)
				}
				if got != want {
					t.Fatalf("m4 %v differs from %s\n got: %q\nwant: %q", args, bin, got, want)
				}
			})
		}
	}
}

func stripSynclines(output string) string {
	var content strings.Builder
	for _, line := range strings.SplitAfter(output, "\n") {
		if strings.HasPrefix(line, "#line ") {
			continue
		}
		content.WriteString(line)
	}
	return content.String()
}

func TestDifferentialM4exitSkipsWrapsAndDiversions(t *testing.T) {
	bin := externalM4(t)
	input := "before divert(1)DIV m4wrap(`WRAP')m4exit(`7') after"
	wantOut, wantErr, wantCode := runExternalM4Result(t, bin, input)
	gotOut, gotErr, gotCode := runM4Deadline(t, input)
	if gotOut != wantOut || gotErr != wantErr || gotCode != wantCode {
		t.Fatalf("m4exit differs from %s\n got: code=%d stdout=%q stderr=%q\nwant: code=%d stdout=%q stderr=%q",
			bin, gotCode, gotOut, gotErr, wantCode, wantOut, wantErr)
	}
}

func TestDifferentialIncludeDiagnosticAndSynclineIdentity(t *testing.T) {
	bin := externalM4(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "inc.m4"), []byte("inside\n[eval(`bad')]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	input := "before\ninclude(`inc.m4')after\n"
	wantOut, wantErr, wantCode := runExternalM4ContextResult(t, bin, dir, input, "-s")
	gotOut, gotErr, gotCode := runM4DeadlineContext(t, dir, nil, input, "-s")
	if gotCode != wantCode || gotOut != wantOut {
		t.Fatalf("include synclines differ from %s\n got: code=%d stdout=%q stderr=%q\nwant: code=%d stdout=%q stderr=%q",
			bin, gotCode, gotOut, gotErr, wantCode, wantOut, wantErr)
	}
	for label, stderr := range map[string]string{"m4": gotErr, "reference": wantErr} {
		if !strings.Contains(stderr, "inc.m4:2:") {
			t.Errorf("%s stderr does not retain included-file identity: %q", label, stderr)
		}
	}
}

func TestDifferentialDiagnosticBuiltins(t *testing.T) {
	bin := externalM4(t)
	input := "define(`x', `y')dumpdef(`x')errprint(`ERR')ok\n"
	wantOut, wantErr, wantCode := runExternalM4Result(t, bin, input)
	gotOut, gotErr, gotCode := runM4Deadline(t, input)
	if gotCode != wantCode || gotOut != wantOut || gotErr != wantErr {
		t.Fatalf("diagnostic builtins differ from %s\n got: code=%d stdout=%q stderr=%q\nwant: code=%d stdout=%q stderr=%q",
			bin, gotCode, gotOut, gotErr, wantCode, wantOut, wantErr)
	}

	input = "traceon(`eval')eval(`1')traceoff(`eval')[eval(`1')]\n"
	wantOut, wantErr, wantCode = runExternalM4Result(t, bin, input)
	gotOut, gotErr, gotCode = runM4Deadline(t, input)
	if gotCode != wantCode || gotOut != wantOut || strings.Count(gotErr, "m4trace:") != strings.Count(wantErr, "m4trace:") {
		t.Fatalf("trace enable/disable differs from %s\n got: code=%d stdout=%q stderr=%q\nwant: code=%d stdout=%q stderr=%q",
			bin, gotCode, gotOut, gotErr, wantCode, wantOut, wantErr)
	}
}

func TestDifferentialDocumentsPOSIXWrapOrder(t *testing.T) {
	bin := externalM4(t)
	input := "m4wrap(`W1')m4wrap(`W2')"
	wantOut, wantErr, wantCode := runExternalM4Result(t, bin, input)
	gotOut, gotErr, gotCode := runM4Deadline(t, input)
	// POSIX requires registration order. GNU m4 deliberately emits multiple
	// wraps in reverse order, so keep this known differential explicit instead
	// of weakening the certified behavior to make the general corpus green.
	if gotCode != 0 || gotOut != "W1W2" || gotErr != "" {
		t.Fatalf("POSIX wrap order: code=%d stdout=%q stderr=%q", gotCode, gotOut, gotErr)
	}
	if wantCode != 0 || wantOut != "W2W1" || wantErr != "" {
		t.Fatalf("GNU wrap-order reference changed: %s code=%d stdout=%q stderr=%q", bin, wantCode, wantOut, wantErr)
	}
}
