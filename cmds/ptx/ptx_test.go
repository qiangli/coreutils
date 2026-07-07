package ptxcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, dir string, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func sp(n int) string { return strings.Repeat(" ", n) }

func TestPtxDefaultDumbFormat(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), "beta alpha\n", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	// Width 72, gap 3: the keyword column starts at half_line_width = 36.
	want := sp(29) + "beta   alpha\n" +
		sp(36) + "beta alpha\n"
	if out != want {
		t.Fatalf("out=%q want=%q", out, want)
	}
}

func TestPtxTruncationMarks(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), "aa bbb cccc dd\n", "-w", "20", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	// half=10, beforeMax=5, kaMax=8: '/' marks flag truncated context;
	// the head field wraps left context the before field dropped.
	want := sp(10) + "aa bbb/\n" +
		sp(5) + "aa   bbb cccc/\n" +
		sp(3) + "/bbb   cccc dd\n" +
		sp(3) + "cccc   dd" + sp(4) + "/bbb\n"
	if out != want {
		t.Fatalf("out=%q want=%q", out, want)
	}
}

func TestPtxTypesetModeSetsWidth100(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), "beta alpha\n", "-t", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	// -t is only a width change (100 -> half 50), not roff output.
	want := sp(43) + "beta   alpha\n" +
		sp(50) + "beta alpha\n"
	if out != want {
		t.Fatalf("out=%q want=%q", out, want)
	}
	// An explicit -w wins over -t.
	out, _, code = runTool(t, t.TempDir(), "beta alpha\n", "-t", "-w", "72", "-")
	if code != 0 || !strings.HasPrefix(out, sp(29)+"beta   alpha\n") {
		t.Fatalf("out=%q", out)
	}
}

func TestPtxSortIsCaseSensitiveAndFFoldsToUpper(t *testing.T) {
	// Without -f: byte order — "Zulu" (Z=0x5A) sorts before "alpha".
	out, errb, code := runTool(t, t.TempDir(), "Zulu alpha\n", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	want := sp(36) + "Zulu alpha\n" +
		sp(29) + "Zulu   alpha\n"
	if out != want {
		t.Fatalf("out=%q want=%q", out, want)
	}
	// With -f the fold direction is lower->UPPER: fold("a_b")="A_B" and
	// '_' (0x5F) > 'B' (0x42), so "ab" sorts first; without -f, '_'
	// (0x5F) < 'b' (0x62) puts "a_b" first.
	abFirst := sp(36) + "ab a_b\n" + sp(31) + "ab   a_b\n"
	aubFirst := sp(31) + "ab   a_b\n" + sp(36) + "ab a_b\n"
	out, _, code = runTool(t, t.TempDir(), "ab a_b\n", "-W", "[a-z_]+", "-")
	if code != 0 || out != aubFirst {
		t.Fatalf("case-sensitive out=%q want=%q", out, aubFirst)
	}
	out, _, code = runTool(t, t.TempDir(), "ab a_b\n", "-f", "-W", "[a-z_]+", "-")
	if code != 0 || out != abFirst {
		t.Fatalf("-f out=%q want=%q", out, abFirst)
	}
}

func TestPtxWordListsCaseSensitiveUnlessFold(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ignore"), []byte("Beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Without -f only the exact "Beta" is ignored.
	out, errb, code := runTool(t, dir, "beta Beta\n", "-i", "ignore", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if strings.Contains(out, "   Beta\n") || !strings.HasSuffix(out, "   beta Beta\n") {
		t.Fatalf("out=%q", out)
	}
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("want single entry, out=%q", out)
	}
	// With -f both fold together and are ignored.
	out, _, code = runTool(t, dir, "beta Beta\n", "-f", "-i", "ignore", "-")
	if code != 0 || out != "" {
		t.Fatalf("folded ignore out=%q code=%d", out, code)
	}
}

func TestPtxOnlyFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "only"), []byte("alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "alpha beta\n", "-o", "only", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if want := sp(36) + "alpha beta\n"; out != want {
		t.Fatalf("out=%q want=%q", out, want)
	}
}

func TestPtxAutoReference(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), "alpha beta\n", "-A", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	// Stdin has an empty file part: ":1:"; refMax=2, so the line width
	// shrinks to 67 and half becomes 33.
	want := ":1:" + sp(35) + "alpha beta\n" +
		":1:" + sp(27) + "alpha   beta\n"
	if out != want {
		t.Fatalf("out=%q want=%q", out, want)
	}

	// Named files use file:line references.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code = runTool(t, dir, "", "-A", "in")
	if code != 0 || !strings.Contains(out, "in:1:") || !strings.Contains(out, "in:2:") {
		t.Fatalf("out=%q", out)
	}
}

func TestPtxInputReferences(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), "r1 alpha beta\n", "-r", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	// refMax=2: reference field width 2+gap. The reference is excluded
	// from the context.
	want := "r1" + sp(36) + "alpha beta\n" +
		"r1" + sp(28) + "alpha   beta\n"
	if out != want {
		t.Fatalf("out=%q want=%q", out, want)
	}

	// -R puts the reference at the right, padded to the keyafter half.
	out, _, code = runTool(t, t.TempDir(), "r1 alpha beta\n", "-r", "-R", "-")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if !strings.HasSuffix(line, " r1") {
			t.Fatalf("line %q lacks right-side reference", line)
		}
	}
	if !strings.Contains(out, "alpha   beta") {
		t.Fatalf("out=%q", out)
	}
}

func TestPtxRoffFormat(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), "alpha beta\n", "-A", "-O", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	want := `.xx "" "" "alpha beta" "" ":1"` + "\n" +
		`.xx "" "alpha" "beta" "" ":1"` + "\n"
	if out != want {
		t.Fatalf("out=%q want=%q", out, want)
	}
	// TeX output is rejected loudly.
	_, errb, code = runTool(t, t.TempDir(), "a\n", "-T", "-")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
}

func TestPtxBreakFileAndSentenceRegexp(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "breaks"), []byte("- "), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "alpha-beta\n", "-b", "breaks", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if !strings.Contains(out, "alpha-beta\n") || strings.Count(out, "\n") != 2 {
		t.Fatalf("out=%q", out)
	}

	out, errb, code = runTool(t, dir, "alpha beta. gamma delta.\n", "-S", "[^.]+\\.", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if strings.Contains(out, "beta. gamma") {
		t.Fatalf("sentence boundary was not honored: %q", out)
	}
}

func TestPtxGapSize(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), "beta alpha\n", "-g", "2", "-")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	want := sp(30) + "beta  alpha\n" +
		sp(36) + "beta alpha\n"
	if out != want {
		t.Fatalf("out=%q want=%q", out, want)
	}
}

func TestPtxRejectsBadNumbers(t *testing.T) {
	_, errb, code := runTool(t, t.TempDir(), "", "-g", "0")
	if code != 2 || !strings.Contains(errb, "invalid gap width") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, t.TempDir(), "", "-w", "0")
	if code != 2 || !strings.Contains(errb, "invalid line width") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
}

func TestPtxMissingFile(t *testing.T) {
	_, errb, code := runTool(t, t.TempDir(), "", "missing")
	if code != 1 || !strings.Contains(errb, "cannot open") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
}
