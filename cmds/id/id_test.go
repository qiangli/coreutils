package idcmd

import (
	"bytes"
	"context"
	"os/user"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func current(t *testing.T) *user.User {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Skipf("user.Current: %v", err)
	}
	return u
}

func TestIDDefault(t *testing.T) {
	u := current(t)
	out, errb, code := runTool(t)
	if code != 0 || errb != "" {
		t.Fatalf("id: code=%d err=%q", code, errb)
	}
	for _, want := range []string{"uid=" + u.Uid, "gid=" + u.Gid, "groups="} {
		if !strings.Contains(out, want) {
			t.Errorf("id output %q missing %q", out, want)
		}
	}
	if strings.Count(out, "\n") != 1 {
		t.Errorf("id output is not a single line: %q", out)
	}
}

func TestIDOnlyFlags(t *testing.T) {
	u := current(t)
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"-u"}, u.Uid + "\n"},
		{[]string{"-g"}, u.Gid + "\n"},
		{[]string{"-u", "-n"}, u.Username + "\n"},
	}
	for _, c := range cases {
		out, _, code := runTool(t, c.args...)
		if code != 0 || out != c.want {
			t.Errorf("id %q = (%q, %d), want (%q, 0)", c.args, out, code, c.want)
		}
	}

	out, _, code := runTool(t, "-G")
	if code != 0 {
		t.Fatalf("-G: code=%d", code)
	}
	found := false
	for _, f := range strings.Fields(out) {
		if f == u.Gid {
			found = true
		}
	}
	if !found {
		t.Errorf("-G output %q missing primary gid %s", out, u.Gid)
	}
	if !strings.HasPrefix(out, u.Gid) {
		t.Errorf("-G output %q does not lead with the effective gid", out)
	}

	out, _, code = runTool(t, "-G", "-n")
	if code != 0 || strings.TrimSpace(out) == "" {
		t.Errorf("-Gn = (%q, %d), want non-empty names", out, code)
	}
}

func TestIDNamedUser(t *testing.T) {
	u := current(t)
	out, _, code := runTool(t, "-u", u.Username)
	if code != 0 || out != u.Uid+"\n" {
		t.Errorf("id -u %s = (%q, %d), want (%q, 0)", u.Username, out, code, u.Uid+"\n")
	}
}

func TestIDErrors(t *testing.T) {
	_, errb, code := runTool(t, "no-such-user-xyzzy")
	if code != 1 || !strings.Contains(errb, "no such user") {
		t.Errorf("unknown user: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "-u", "-g")
	if code != 2 || !strings.Contains(errb, "more than one choice") {
		t.Errorf("-u -g: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "-n")
	if code != 2 || !strings.Contains(errb, "cannot print only names") {
		t.Errorf("-n alone: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestIDHelp(t *testing.T) {
	out, _, code := runTool(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: id") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
}
