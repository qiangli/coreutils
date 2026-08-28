package pastecmd

import "testing"

func TestPOSIXStopsOptionParsingAtFirstOperand(t *testing.T) {
	dir := writeFiles(t, map[string]string{"input": "name\nrole\n", "-s": "alice\nadmin\n", "--ser": "lead\nstaff\n"})
	out, errOut, code := runToolDirEnv(t, dir, []string{"POSIXLY_CORRECT=1"}, "", "input", "-s", "--ser")
	if code != 0 || out != "name\talice\tlead\nrole\tadmin\tstaff\n" || errOut != "" {
		t.Fatalf("paste post-operand -s = (%q, %q, %d)", out, errOut, code)
	}
}
