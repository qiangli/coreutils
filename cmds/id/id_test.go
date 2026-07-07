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

func TestIDAliasHelpVersion(t *testing.T) {
	out, _, code := runTool(t, "-h")
	if code != 0 || !strings.Contains(out, "Usage: id") {
		t.Errorf("-h: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "-V")
	if code != 0 || !strings.Contains(out, "qiangli/coreutils") {
		t.Errorf("-V: code=%d out=%q", code, out)
	}
}

func TestIDAFlag(t *testing.T) {
	u := current(t)
	out, _, code := runTool(t, "-a", "-u", "-g")
	if code != 0 {
		t.Fatalf("-a -u -g: code=%d", code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Fatalf("-a -u -g output too short: %q", out)
	}
	if lines[0] != u.Uid || lines[1] != u.Gid {
		t.Errorf("-a -u -g lines: %q want uid=%s then gid=%s", out, u.Uid, u.Gid)
	}
}

func TestIDPFlag(t *testing.T) {
	u := current(t)
	out, _, code := runTool(t, "-p")
	if code != 0 {
		t.Fatalf("-p: code=%d", code)
	}
	for _, want := range []string{"uid=", "gid=", "groups="} {
		if !strings.Contains(out, want) {
			t.Errorf("-p output %q missing %q", out, want)
		}
	}
	outn, _, code := runTool(t, "-u", "-p")
	if code != 0 || outn != u.Username+"\n" {
		t.Errorf("-u -p = (%q, %d) want username %s", outn, code, u.Username)
	}
}

func TestIDZFlag(t *testing.T) {
	u := current(t)
	out, _, code := runTool(t, "-z")
	if code != 0 {
		t.Fatalf("-z: code=%d", code)
	}
	if !strings.Contains(out, "uid="+u.Uid) {
		t.Errorf("-z output %q missing uid", out)
	}
	if !strings.HasSuffix(out, "\x00") {
		t.Errorf("-z should end with NUL: %q", out)
	}
	if strings.Count(out, "\n") > 0 {
		t.Errorf("-z output should not contain newline: %q", out)
	}
}

func TestIDRealFlag(t *testing.T) {
	u := current(t)
	out, _, code := runTool(t, "-r")
	if code != 0 {
		t.Fatalf("-r: code=%d", code)
	}
	if !strings.Contains(out, "uid="+u.Uid) {
		t.Errorf("-r output %q missing uid", out)
	}
	out2, _, code := runTool(t, "--real")
	if code != 0 {
		t.Fatalf("--real: code=%d", code)
	}
	if out != out2 {
		t.Errorf("-r vs --real mismatch: %q vs %q", out, out2)
	}
}

func TestIDIgnoreFlag(t *testing.T) {
	_, errb, code := runTool(t, "--ignore", "no-such-user-xyzzy")
	if code != 0 {
		t.Fatalf("--ignore unknown: code=%d", code)
	}
	if strings.Contains(errb, "no such user") {
		t.Errorf("--ignore should suppress error: %q", errb)
	}
}

func TestIDNoOpFlags(t *testing.T) {
	for _, flag := range []string{"-A", "-P", "-Z", "--context"} {
		out, _, code := runTool(t, flag)
		if code != 0 {
			t.Errorf("%s: code=%d", flag, code)
		}
		if !strings.Contains(out, "uid=") {
			t.Errorf("%s output %q missing uid", flag, out)
		}
	}
}

func TestIDPWithGN(t *testing.T) {
	u := current(t)
	out, _, code := runTool(t, "-g", "-p")
	if code != 0 {
		t.Fatalf("-g -p: code=%d", code)
	}
	outn, _, _ := runTool(t, "-g", "-n")
	if out != outn {
		t.Errorf("-g -p vs -g -n: %q vs %q", out, outn)
	}

	outG, _, code := runTool(t, "-G", "-p")
	if code != 0 {
		t.Fatalf("-G -p: code=%d", code)
	}
	if strings.TrimSpace(outG) == strings.TrimSpace(outn) && outG == "" {
		t.Errorf("-G -p output should be non-empty group names: %q", outG)
	}
	_ = u
}
