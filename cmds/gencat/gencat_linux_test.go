//go:build linux

package gencatcmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// This checks the actual system consumer, not just our own decoder. A catalog
// that round-trips internally but fails catgets is not a working gencat.
func TestGlibcCatgetsReadsGeneratedAndMergedCatalog(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("C compiler unavailable for catgets interoperability probe")
	}
	dir := t.TempDir()
	probe := filepath.Join(dir, "probe.c")
	const source = `#include <nl_types.h>
#include <stdio.h>
int main(int argc, char **argv) {
  nl_catd cat = catopen(argv[1], 0);
  if (cat == (nl_catd)-1) return 2;
  printf("%s|%s\n", catgets(cat, 2, 1, "missing"), catgets(cat, 2, 2, "deleted"));
  catclose(cat);
  return 0;
}`
	if err := os.WriteFile(probe, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	build := exec.Command(cc, probe, "-o", filepath.Join(dir, "probe"))
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compile catgets probe: %v: %s", err, output)
	}
	for name, source := range map[string]string{
		"initial.msg": "$set 2\n1 old\n2 remove\n",
		"update.msg":  "$set 2\n1 hello\\nworld\n2\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, input := range []string{"initial.msg", "update.msg"} {
		var errs bytes.Buffer
		rc := &tool.RunContext{Dir: dir, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: &errs}}
		if code := run(rc, []string{"result.cat", input}); code != 0 {
			t.Fatalf("gencat %s: code=%d: %s", input, code, errs.String())
		}
	}
	got, err := exec.Command(filepath.Join(dir, "probe"), filepath.Join(dir, "result.cat")).CombinedOutput()
	if err != nil {
		t.Fatalf("catgets probe: %v: %s", err, got)
	}
	if string(got) != "hello\nworld|deleted\n" {
		t.Fatalf("catgets got %q", got)
	}
}
