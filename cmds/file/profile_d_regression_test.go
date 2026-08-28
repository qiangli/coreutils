package filecmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPOSIXStopsOptionParsingAtFirstOperand(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"input", "-h", "--no-deref", "--"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, errOut, code := invokeEnv(t, dir, "", []string{"POSIXLY_CORRECT=1"}, "input", "-h", "--no-deref", "--")
	if code != 0 || out != "input: empty\n-h: empty\n--no-deref: empty\n--: empty\n" || errOut != "" {
		t.Fatalf("file post-operand options = (%q, %q, %d)", out, errOut, code)
	}
}

func TestProfileDMagicRightArrowContinuation(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "file_1_right_arrow_offset.tmp", []byte("some_string_data right_arrow_offset"))
	put(t, dir, "magic", []byte(strings.Join([]string{
		"0\ts\tsome_string_data\tMessage: %s",
		">17\ts\tright_arrow_offset\t Message2: %s",
	}, "\n")+"\n"))
	for _, flag := range []string{"-m", "-M"} {
		out, errOut, code := invoke(t, dir, "", flag, "magic", "file_1_right_arrow_offset.tmp")
		want := "file_1_right_arrow_offset.tmp: Message: some_string_data Message2: right_arrow_offset\n"
		if code != 0 || out != want || errOut != "" {
			t.Fatalf("file %s right-arrow magic = (%q, %q, %d), want %q", flag, out, errOut, code, want)
		}
	}
}
