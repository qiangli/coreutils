package idcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
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
	// groups= is conditional: a process with no supplementary group
	// affiliations has only the uid= and gid= fields.
	for _, want := range []string{"uid=" + u.Uid, "gid=" + u.Gid} {
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
	want, wantErr, wantCode := runTool(t)
	out, errb, code := runTool(t, "-a")
	if code != wantCode || out != want || errb != wantErr {
		t.Errorf("-a = (out=%q err=%q code=%d), want (out=%q err=%q code=%d)", out, errb, code, want, wantErr, wantCode)
	}
	_, errb, code = runTool(t, "-a", "-u", "-g")
	if code != 2 || !strings.Contains(errb, "more than one choice") {
		t.Errorf("-a -u -g: code=%d err=%q", code, errb)
	}
}

func TestIDRejectsExtraUserOperand(t *testing.T) {
	u := current(t)
	out, errb, code := runTool(t, "-u", u.Username, u.Username)
	if code != 2 || out != "" || !strings.Contains(errb, "extra operand") {
		t.Errorf("id -u USER USER = (out=%q err=%q code=%d), want extra-operand usage error", out, errb, code)
	}
}

func TestIDPFlag(t *testing.T) {
	u := current(t)
	out, _, code := runTool(t, "-p")
	if code != 0 {
		t.Fatalf("-p: code=%d", code)
	}
	// -p changes names/formatting, but does not manufacture a groups= field
	// for a process with no supplementary group affiliations.
	for _, want := range []string{"uid=", "gid="} {
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
	// -r alone is a usage error; GNU and BSD both reject it.
	for _, args := range [][]string{{"-r"}, {"--real"}} {
		_, errb, code := runTool(t, args...)
		if code != 2 || !strings.Contains(errb, "cannot print only names or real IDs") {
			t.Errorf("id %v: code=%d err=%q", args, code, errb)
		}
	}
}

func TestIDRealGroupFlag(t *testing.T) {
	// GNU and BSD both accept -r with -G (and -n); only -r alone is rejected.
	u := current(t)
	for _, args := range [][]string{{"-r", "-G"}, {"-rG"}, {"--real", "--groups"}, {"-r", "-G", "-n"}} {
		out, errb, code := runTool(t, args...)
		if code != 0 || errb != "" {
			t.Errorf("id %v: code=%d err=%q out=%q", args, code, errb, out)
			continue
		}
		if strings.TrimSpace(out) == "" {
			t.Errorf("id %v: expected group list, got empty output", args)
		}
	}
	// -rG should still lead with the real (primary) gid.
	out, _, code := runTool(t, "-r", "-G")
	if code == 0 {
		if first := strings.SplitN(strings.TrimSpace(out), " ", 2)[0]; first != u.Gid {
			t.Errorf("id -rG = %q, want to lead with real gid %s", out, u.Gid)
		}
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

func TestIDDefaultIncludesNames(t *testing.T) {
	u := current(t)
	out, _, code := runTool(t)
	if code != 0 {
		t.Fatalf("id: code=%d", code)
	}
	idName := u.Uid + "(" + u.Username + ")"
	if !strings.Contains(out, idName) {
		t.Errorf("default id output %q missing uid with name (expected %q)", out, idName)
	}
	if !strings.Contains(out, "(") {
		t.Errorf("default id output %q should include group names in parenthesized form", out)
	}
}

// POSIX default format: the real IDs are reported as uid=/gid=, and the
// effective IDs are inserted as euid=/egid= only when they differ. The seam
// simulates a setuid invocation without the test itself being setuid.
func TestIDDefaultReportsRealAndEffectiveWhenDifferent(t *testing.T) {
	oldIDs, oldGroups := processIDsFn, processGroupIDsFn
	t.Cleanup(func() { processIDsFn, processGroupIDsFn = oldIDs, oldGroups })
	processIDsFn = func(real bool) (uid, gid string) {
		if real {
			return "1000", "1000"
		}
		return "0", "0"
	}
	processGroupIDsFn = func() ([]string, error) { return []string{"7000"}, nil }
	out, errb, code := runTool(t)
	if code != 0 || errb != "" {
		t.Fatalf("id: code=%d err=%q", code, errb)
	}
	if !strings.HasPrefix(out, "uid=1000") {
		t.Errorf("default output %q must lead with the real uid", out)
	}
	if !strings.Contains(out, " euid=0") {
		t.Errorf("default output %q must report the effective uid when it differs", out)
	}
	if !strings.Contains(out, " egid=0") {
		t.Errorf("default output %q must report the effective gid when it differs", out)
	}
	// With a deterministic supplementary group, all optional fields are
	// present and their POSIX order can be compared without treating a missing
	// field's strings.Index result (-1) as a real position.
	gi := strings.Index(out, " gid=")
	ei := strings.Index(out, " euid=")
	egi := strings.Index(out, " egid=")
	grp := strings.Index(out, " groups=")
	if gi < 0 || ei < 0 || egi < 0 || grp < 0 {
		t.Fatalf("default output is missing an ordering field: %q", out)
	}
	if !(gi < ei && ei < egi && egi < grp) {
		t.Errorf("fields must be ordered gid=, euid=, egid=, groups= in %q", out)
	}
	if strings.Count(out, "\n") != 1 {
		t.Errorf("default output is not a single line: %q", out)
	}
}

// Current-process group reporting comes from getgroups(2), not from the
// account database. The process may have changed its supplementary vector
// after login, so querying user.Current().GroupIds() would be stale.
func TestIDCurrentGroupsUseLiveProcessVector(t *testing.T) {
	oldIDs, oldGroups := processIDsFn, processGroupIDsFn
	t.Cleanup(func() { processIDsFn, processGroupIDsFn = oldIDs, oldGroups })
	processIDsFn = func(real bool) (uid, gid string) {
		if real {
			return "1000", "2000"
		}
		return "1000", "3000"
	}
	processGroupIDsFn = func() ([]string, error) {
		return []string{"7000", "7001", "7000"}, nil
	}

	out, errb, code := runTool(t, "-G")
	if code != 0 || errb != "" || strings.TrimSpace(out) != "3000 2000 7000 7001" {
		t.Fatalf("id -G: code=%d out=%q err=%q", code, out, errb)
	}
	out, errb, code = runTool(t)
	if code != 0 || errb != "" {
		t.Fatalf("id: code=%d err=%q", code, errb)
	}
	if !strings.Contains(out, " groups=7000,7001") {
		t.Fatalf("default report did not use process groups: %q", out)
	}
}

// When the effective IDs equal the real IDs (the ordinary, non-setuid case),
// no euid=/egid= fields appear.
func TestIDDefaultOmitsEffectiveWhenEqual(t *testing.T) {
	oldIDs, oldGroups := processIDsFn, processGroupIDsFn
	t.Cleanup(func() { processIDsFn, processGroupIDsFn = oldIDs, oldGroups })
	processIDsFn = func(real bool) (uid, gid string) { return "1000", "1000" }
	processGroupIDsFn = func() ([]string, error) { return nil, nil }
	out, errb, code := runTool(t)
	if code != 0 || errb != "" {
		t.Fatalf("code=%d stderr=%q", code, errb)
	}
	if !strings.HasSuffix(out, "\n") || strings.Count(out, "\n") != 1 {
		t.Fatalf("default output is not exactly one line: %q", out)
	}
	fields := strings.Split(strings.TrimSuffix(out, "\n"), " ")
	if len(fields) != 2 || !idDefaultField(fields[0], "uid", "1000") || !idDefaultField(fields[1], "gid", "1000") {
		t.Errorf("default output %q malformed", out)
	}
}

// idDefaultField accepts the two POSIX default-report forms for an ID: the
// numeric value alone, or that value decorated with a resolvable host name.
func idDefaultField(field, label, id string) bool {
	prefix := label + "=" + id
	if field == prefix {
		return true
	}
	if !strings.HasPrefix(field, prefix) {
		return false
	}
	name := strings.TrimPrefix(field, prefix)
	if len(name) <= 2 || name[0] != '(' || name[len(name)-1] != ')' {
		return false
	}
	return !strings.ContainsAny(name[1:len(name)-1], "()")
}

func TestIDDefaultField(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field string
		want  bool
	}{
		{"bare", "uid=1000", true},
		{"decorated", "uid=1000(ubuntu)", true},
		{"missing prefix", "(wrong)", false},
		{"wrong label", "gid=1000(ubuntu)", false},
		{"wrong ID", "uid=1001(ubuntu)", false},
		{"ID with matching prefix", "uid=10000(ubuntu)", false},
		{"empty name", "uid=1000()", false},
		{"missing open parenthesis", "uid=1000ubuntu)", false},
		{"missing close parenthesis", "uid=1000(ubuntu", false},
		{"extra open parenthesis", "uid=1000((ubuntu)", false},
		{"extra close parenthesis", "uid=1000(ubuntu))", false},
		{"trailing bytes", "uid=1000(ubuntu)x", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := idDefaultField(tc.field, "uid", "1000"); got != tc.want {
				t.Fatalf("idDefaultField(%q)=%v, want %v", tc.field, got, tc.want)
			}
		})
	}
}

func TestIDRealFlagWithOptions(t *testing.T) {
	u := current(t)
	out, _, code := runTool(t, "-r", "-u")
	if code != 0 {
		t.Fatalf("-r -u: code=%d", code)
	}
	if strings.TrimSpace(out) != u.Uid {
		t.Errorf("-r -u = %q, want uid %s", out, u.Uid)
	}
	out, _, code = runTool(t, "-r", "-g")
	if code != 0 {
		t.Fatalf("-r -g: code=%d", code)
	}
	if strings.TrimSpace(out) != u.Gid {
		t.Errorf("-r -g = %q, want gid %s", out, u.Gid)
	}
	out, _, code = runTool(t, "-r", "-u", "-n")
	if code != 0 {
		t.Fatalf("-r -u -n: code=%d", code)
	}
	if strings.TrimSpace(out) != u.Username {
		t.Errorf("-r -u -n = %q, want username %s", out, u.Username)
	}
}

type idFailWriter struct {
	short bool
}

func (w idFailWriter) Write(p []byte) (int, error) {
	if w.short {
		return len(p) - 1, nil
	}
	return 0, errors.New("injected output failure")
}

func TestIDOutputErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		out  io.Writer
		want string
	}{
		{"error", idFailWriter{}, "injected output failure"},
		{"short", idFailWriter{short: true}, io.ErrShortWrite.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var errOut bytes.Buffer
			rc := &tool.RunContext{
				Ctx:   context.Background(),
				Stdio: tool.Stdio{Out: tc.out, Err: &errOut},
			}
			if code := run(rc, []string{"-u"}); code != 1 || !strings.Contains(errOut.String(), tc.want) {
				t.Fatalf("code=%d err=%q", code, errOut.String())
			}
		})
	}
}

func TestIDNamedUserOperand(t *testing.T) {
	u := current(t)
	// Output should just report uid=/gid= and groups= without euid=/egid=
	out, errb, code := runTool(t, u.Username)
	if code != 0 || errb != "" {
		t.Fatalf("id USER: code=%d err=%q", code, errb)
	}
	if strings.Contains(out, "euid=") || strings.Contains(out, "egid=") {
		t.Errorf("named user output %q should not contain euid/egid", out)
	}
	if !strings.HasPrefix(out, "uid="+u.Uid) || !strings.Contains(out, " gid="+u.Gid) {
		t.Errorf("named user output %q malformed", out)
	}

	// -u USER
	out, errb, code = runTool(t, "-u", u.Username)
	if code != 0 || errb != "" {
		t.Fatalf("-u USER: code=%d err=%q", code, errb)
	}
	if strings.TrimSpace(out) != u.Uid {
		t.Errorf("-u USER = %q, want %q", out, u.Uid)
	}

	// Public passwd/group-style fixture: the named account's initial group is
	// part of its multiple-group report, followed by supplementary groups.
	// This differs from the no-operand process form, whose groups= field comes
	// from the live supplementary getgroups(2) vector.
	oldAccountGroups := accountGroupIDsFn
	oldGroupNameByID := groupNameByIDFn
	t.Cleanup(func() {
		accountGroupIDsFn = oldAccountGroups
		groupNameByIDFn = oldGroupNameByID
	})
	groupNameByIDFn = func(string) string { return "" }
	accountGroupIDsFn = func(*user.User) ([]string, error) {
		return []string{"23001", "23002", "23003", "23002"}, nil
	}
	fixture := &user.User{Username: "primary", Uid: "22001", Gid: "23001"}
	lines, err := formatOne(fixture, false, false, false, false, false, false)
	if err != nil {
		t.Fatalf("format named fixture: %v", err)
	}
	if got, want := strings.Join(lines, "\n"), "uid=22001(primary) gid=23001 groups=23001,23002,23003"; got != want {
		t.Fatalf("named multiple-group report = %q, want %q", got, want)
	}
	accountGroupIDsFn = func(*user.User) ([]string, error) {
		return []string{"23001", "23001"}, nil
	}
	lines, err = formatOne(fixture, false, false, false, false, false, false)
	if err != nil {
		t.Fatalf("format named single-group fixture: %v", err)
	}
	if got, want := strings.Join(lines, "\n"), "uid=22001(primary) gid=23001"; got != want {
		t.Fatalf("named single-group report = %q, want no groups= field (%q)", got, want)
	}
	accountGroupIDsFn = oldAccountGroups
	groupNameByIDFn = oldGroupNameByID

	// -g USER
	out, errb, code = runTool(t, "-g", u.Username)
	if code != 0 || errb != "" {
		t.Fatalf("-g USER: code=%d err=%q", code, errb)
	}
	if strings.TrimSpace(out) != u.Gid {
		t.Errorf("-g USER = %q, want %q", out, u.Gid)
	}

	// -G USER
	out, errb, code = runTool(t, "-G", u.Username)
	if code != 0 || errb != "" {
		t.Fatalf("-G USER: code=%d err=%q", code, errb)
	}
	fields := strings.Fields(out)
	if len(fields) == 0 || fields[0] != u.Gid {
		t.Errorf("-G USER = %q, want first field to be primary GID %q", out, u.Gid)
	}
}

func TestIDNamedUserOperandCombinations(t *testing.T) {
	u := current(t)
	// -un USER
	out, errb, code := runTool(t, "-u", "-n", u.Username)
	if code != 0 || errb != "" {
		t.Fatalf("-u -n USER: code=%d err=%q", code, errb)
	}
	if strings.TrimSpace(out) != u.Username {
		t.Errorf("-u -n USER = %q, want %q", out, u.Username)
	}

	// -gn USER
	out, errb, code = runTool(t, "-g", "-n", u.Username)
	if code != 0 || errb != "" {
		t.Fatalf("-g -n USER: code=%d err=%q", code, errb)
	}
	if got, want := strings.TrimSpace(out), groupName(u.Gid); got != want {
		t.Errorf("-g -n USER = %q, want resolved-name-or-ID fallback %q", got, want)
	}

	// -Gn USER
	out, errb, code = runTool(t, "-G", "-n", u.Username)
	if code != 0 || errb != "" {
		t.Fatalf("-G -n USER: code=%d err=%q", code, errb)
	}
	gids, err := u.GroupIds()
	if err != nil {
		t.Fatalf("GroupIds: %v", err)
	}
	wantGroups := uniqueNonempty(append([]string{u.Gid}, gids...))
	for i := range wantGroups {
		wantGroups[i] = groupName(wantGroups[i])
	}
	if got, want := strings.TrimSpace(out), strings.Join(wantGroups, " "); got != want {
		t.Errorf("-G -n USER = %q, want resolved-name-or-ID fallbacks %q", got, want)
	}

	// -run USER (POSIX: -r is ignored if no difference, but it's valid)
	out, errb, code = runTool(t, "-r", "-u", "-n", u.Username)
	if code != 0 || errb != "" {
		t.Fatalf("-r -u -n USER: code=%d err=%q", code, errb)
	}
	if strings.TrimSpace(out) != u.Username {
		t.Errorf("-r -u -n USER = %q, want %q", out, u.Username)
	}
}
