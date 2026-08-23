package newgrpcmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// write creates a fixture file and sets its mode EXPLICITLY. os.WriteFile's
// mode argument is masked by the process umask, so a fixture that is meant to
// be unreadable can come out readable (or vice versa) depending on the shell
// that started the test.
func write(t *testing.T, dir, name, content string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

const groupFixture = `# a comment line
root:x:0:
staff:x:20:alice,bob
secrets:$6$saltstring$digest:80:carol
malformed-line-with-too-few-fields
empty-members:x:90:
`

func TestParseGroupFile(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "group", groupFixture, 0o644)

	got, err := parseGroupFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []groupInfo{
		{Name: "root", Password: "x", GID: "0"},
		{Name: "staff", Password: "x", GID: "20", Members: []string{"alice", "bob"}},
		{Name: "secrets", Password: "$6$saltstring$digest", GID: "80", Members: []string{"carol"}},
		{Name: "empty-members", Password: "x", GID: "90"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseGroupFile:\n got %+v\nwant %+v", got, want)
	}
}

// One malformed line must not take the whole file with it: a user would
// otherwise lose access to every group because of an unrelated bad entry.
func TestParseGroupFileSkipsMalformedLinesRatherThanFailing(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "group", "broken\nstaff:x:20:alice\n", 0o644)
	got, err := parseGroupFile(path)
	if err != nil {
		t.Fatalf("a malformed line must not fail the read: %v", err)
	}
	if len(got) != 1 || got[0].Name != "staff" {
		t.Errorf("got %+v, want just the well-formed staff entry", got)
	}
}

func TestGshadowPassword(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "gshadow", "staff:!::alice\nsecrets:$6$hash:admin:carol\n", 0o600)

	if got, err := gshadowPassword(path, "secrets"); err != nil || got != "$6$hash" {
		t.Errorf("gshadowPassword(secrets) = %q, %v; want the hash", got, err)
	}
	if got, err := gshadowPassword(path, "staff"); err != nil || got != "!" {
		t.Errorf("gshadowPassword(staff) = %q, %v; want the locked sentinel", got, err)
	}
	if _, err := gshadowPassword(path, "absent"); err == nil {
		t.Error("a missing entry must be reported, not returned as an empty password")
	}
}

// The three ways a group-file "x" can resolve. The distinction between them is
// the whole point: "no password" permits nothing, "locked" permits nothing, and
// "cannot read" must not be mistaken for either — it means the host cannot
// answer the question.
func TestResolvePasswordFromShadow(t *testing.T) {
	dir := t.TempDir()
	group := write(t, dir, "group", "secrets:x:80:\nplain:$1$abc$digest:81:\n", 0o644)
	gshadow := write(t, dir, "gshadow", "secrets:$6$hash:admin:\n", 0o600)
	s := systemDB{groupFile: group, gshadowFile: gshadow}

	g, err := s.GroupByName("secrets")
	if err != nil {
		t.Fatal(err)
	}
	if g.Password != "$6$hash" || g.PasswordShadowed {
		t.Errorf("shadowed group = %+v, want the hash from the shadow database", g)
	}

	// A password living in the group file itself is used as-is; the shadow
	// database is not consulted.
	g, err = s.GroupByName("plain")
	if err != nil {
		t.Fatal(err)
	}
	if g.Password != "$1$abc$digest" {
		t.Errorf("plain group password = %q", g.Password)
	}
}

// No shadow database at all (macOS, the BSDs): the group file's own field
// stands, and "x" simply means no usable password.
func TestResolvePasswordWithoutAShadowDatabase(t *testing.T) {
	dir := t.TempDir()
	group := write(t, dir, "group", "secrets:x:80:\n", 0o644)
	s := systemDB{groupFile: group, gshadowFile: filepath.Join(dir, "does-not-exist")}

	g, err := s.GroupByName("secrets")
	if err != nil {
		t.Fatal(err)
	}
	if g.PasswordShadowed {
		t.Error("an ABSENT shadow database is not an unreadable one")
	}
	if usableGroupPassword(g.Password) {
		t.Errorf("password %q must not be usable", g.Password)
	}
}

// A shadow database that EXISTS but cannot be read is the case newgrp must not
// paper over: "x" claims a password lives somewhere this process cannot see, so
// the answer is "cannot verify", never "no password".
func TestResolvePasswordWithAnUnreadableShadowDatabase(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read a mode-000 file, so the unreadable case is not expressible")
	}
	dir := t.TempDir()
	group := write(t, dir, "group", "secrets:x:80:\n", 0o644)
	gshadow := write(t, dir, "gshadow", "secrets:$6$hash:admin:\n", 0o000)
	s := systemDB{groupFile: group, gshadowFile: gshadow}

	g, err := s.GroupByName("secrets")
	if err != nil {
		t.Fatal(err)
	}
	if !g.PasswordShadowed {
		t.Error("an unreadable shadow database must be flagged, not read as no password")
	}
	if got := authorize(userInfo{Name: "alice", GID: "1000"}, g); got != unverifiable {
		t.Errorf("authorize = %v, want unverifiable", got)
	}
}

func TestGroupLookupMisses(t *testing.T) {
	dir := t.TempDir()
	group := write(t, dir, "group", "staff:x:20:alice\n", 0o644)
	s := systemDB{groupFile: group, gshadowFile: filepath.Join(dir, "none")}

	// A name the file does not have falls through to os/user, which on any
	// test host will also not have it — the point is that the miss is reported
	// as errNoSuchGroup rather than as a read failure.
	if _, err := s.GroupByName("no-such-group-in-any-database-xyzzy"); err != errNoSuchGroup {
		t.Errorf("GroupByName miss = %v, want errNoSuchGroup", err)
	}
}

func TestIsMember(t *testing.T) {
	u := userInfo{Name: "alice", GID: "1000", Supplementary: []string{"1000", "50"}}
	for _, tc := range []struct {
		name  string
		group groupInfo
		want  bool
	}{
		{"primary group", groupInfo{GID: "1000"}, true},
		{"listed member", groupInfo{GID: "20", Members: []string{"bob", "alice"}}, true},
		{"supplementary only", groupInfo{GID: "50"}, true},
		{"unrelated group", groupInfo{GID: "70"}, false},
		{"member list names someone else", groupInfo{GID: "70", Members: []string{"bob"}}, false},
		// The two sources are checked independently: a name match with no
		// supplementary entry still counts, because a membership added since
		// the session started is not in the process's group set.
		{"name match without a supplementary entry", groupInfo{GID: "71", Members: []string{"alice"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isMember(u, tc.group); got != tc.want {
				t.Errorf("isMember = %v, want %v", got, tc.want)
			}
		})
	}
}
