package weave

import (
	"fmt"
	"strings"
	"testing"
)

// A VERDICT MUST NOT DISCARD THE EVIDENCE FOR ITSELF.
//
// The old trim kept the last 2000 bytes. For `go test ./...` across 200+
// packages that is exactly the wrong end: the run finishes with the
// alphabetically last packages passing, so the tail is `ok` lines plus a bare
// `FAIL`, while every `FAIL <pkg>` and `# <pkg>` build error is in the head
// that got thrown away.
//
// The stored output then reads as a failure with no cause, and the summary line
// quotes a fragment of whatever `ok` line the byte cut landed inside — the
// literal observed verdict was "suite-gate-failed — dge 2.817s".
func TestVerifyTrimKeepsFailuresNotJustTheTail(t *testing.T) {
	var b strings.Builder
	b.WriteString("# github.com/qiangli/coreutils/pkg/herald\n")
	b.WriteString("FAIL\tgithub.com/qiangli/coreutils/pkg/herald\t0.5s\n")
	for i := 0; i < 400; i++ {
		fmt.Fprintf(&b, "ok  \tgithub.com/qiangli/coreutils/pkg/filler%03d\t1.234s\n", i)
	}
	b.WriteString("FAIL\n")
	full := b.String()

	got := weaveTrimVerifyOutput(full, 2000)

	if len(got) > 2200 { // budget + the marker line
		t.Errorf("trimmed output is %d bytes, want ~2000", len(got))
	}
	if !strings.Contains(got, "FAIL\tgithub.com/qiangli/coreutils/pkg/herald") {
		t.Error("the FAILING PACKAGE was discarded — this is the whole bug")
	}
	if !strings.Contains(got, "# github.com/qiangli/coreutils/pkg/herald") {
		t.Error("the build-error line was discarded")
	}
}

// Short output is returned untouched — no marker noise on the common case.
func TestVerifyTrimLeavesShortOutputAlone(t *testing.T) {
	in := "ok  \tpkg/a\t1s\nok  \tpkg/b\t2s\n"
	if got := weaveTrimVerifyOutput(in, 2000); got != in {
		t.Errorf("short output was modified:\n%q", got)
	}
}

// A timeout marker is appended LAST, so a tail-only trim happened to keep it.
// Salient-first must keep it too — a timeout that looks like a plain failure is
// how a gate that never decided anything gets read as a verdict.
func TestVerifyTrimKeepsTheTimeoutMarker(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 400; i++ {
		fmt.Fprintf(&b, "ok  \tpkg/filler%03d\t1.234s\n", i)
	}
	b.WriteString("\n[weave: verify command timed out after 10m]")

	got := weaveTrimVerifyOutput(b.String(), 2000)
	if !strings.Contains(got, "timed out after 10m") {
		t.Error("the timeout marker was discarded; a timeout would read as a test failure")
	}
}

// When failures alone exceed the budget, keep the FIRST ones: the first failure
// is usually the cause and the rest are its consequences.
func TestVerifyTrimKeepsEarliestFailuresWhenOverBudget(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&b, "FAIL\tgithub.com/qiangli/coreutils/pkg/failing%03d\t1.0s\n", i)
	}
	got := weaveTrimVerifyOutput(b.String(), 2000)
	if !strings.Contains(got, "failing000") {
		t.Error("the FIRST failure was dropped; it is the one most likely to be the cause")
	}
}
