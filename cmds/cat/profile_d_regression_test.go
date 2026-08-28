package catcmd

import "testing"

func TestCatStopsOptionPreprocessingAndParsingAtFirstOperand(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, data string }{
		{"first", "one\n"},
		{"-n", "two\n"},
		{"--number", "three\n"},
		{"--", "four\n"},
	} {
		writeFile(t, dir, tc.name, tc.data)
	}
	out, errb, code := runTool(t, dir, "", "first", "-n", "--number", "--")
	if code != 0 || errb != "" || out != "one\ntwo\nthree\nfour\n" {
		t.Fatalf("cat option-looking operands = (%q, %q, %d)", out, errb, code)
	}
}
