//go:build unix

package idcmd

import (
	"os"
	"strconv"
	"testing"
)

func TestIDRealAndEffectiveSelectors(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"-u"}, strconv.Itoa(os.Geteuid())},
		{[]string{"-u", "-r"}, strconv.Itoa(os.Getuid())},
		{[]string{"-g"}, strconv.Itoa(os.Getegid())},
		{[]string{"-g", "-r"}, strconv.Itoa(os.Getgid())},
	}
	for _, tc := range tests {
		out, errb, code := runTool(t, tc.args...)
		if code != 0 || errb != "" || out != tc.want+"\n" {
			t.Errorf("id %v = (out=%q err=%q code=%d), want %q", tc.args, out, errb, code, tc.want+"\n")
		}
	}
}
