//go:build unix

package chgrpcmd

import (
	"errors"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestChgrpObservedOwnerProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	st := fi.Sys().(*syscall.Stat_t)
	realChange := changeGroup
	called := false
	var uid, gid int
	changeGroup = func(_ string, u, g int, follow bool) error {
		called = true
		if !follow {
			t.Error("regular file used no-follow operation")
		}
		uid, gid = u, g
		return nil
	}
	t.Cleanup(func() { changeGroup = realChange })
	if _, errb, code := runTool(t, dir, strconv.FormatUint(uint64(st.Gid), 10), "f"); code != 0 || errb != "" {
		t.Fatalf("chgrp self: code=%d err=%q", code, errb)
	}
	if !called {
		t.Fatal("ownership provider was not called")
	}
	if uid != int(st.Uid) || gid != int(st.Gid) {
		t.Fatalf("provider got %d:%d, want observed %d:%d", uid, gid, st.Uid, st.Gid)
	}
}

func TestChgrpTransitionFailuresContinue(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"denied", "readonly", "after"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	realChange := changeGroup
	var called []string
	changeGroup = func(path string, _, _ int, _ bool) error {
		name := filepath.Base(path)
		called = append(called, name)
		if name == "denied" {
			return &os.PathError{Op: "chown", Path: path, Err: fs.ErrPermission}
		}
		if name == "readonly" {
			return &os.PathError{Op: "chown", Path: path, Err: syscall.EROFS}
		}
		return nil
	}
	t.Cleanup(func() { changeGroup = realChange })
	_, errb, code := runTool(t, dir, currentGroup(t), "denied", "readonly", "after")
	if code != 1 || strings.Join(called, " ") != "denied readonly after" {
		t.Fatalf("code=%d calls=%v err=%q", code, called, errb)
	}
	for _, name := range []string{"denied", "readonly"} {
		if !strings.Contains(errb, "changing group of '"+name+"'") {
			t.Errorf("missing %s diagnostic: %q", name, errb)
		}
	}
}

func TestChgrpNumericGrammarAndLookupFailures(t *testing.T) {
	realLookup := lookupGroup
	lookupGroup = func(name string) (*user.Group, error) { return nil, user.UnknownGroupError(name) }
	t.Cleanup(func() { lookupGroup = realLookup })
	valid := []string{"0", "00042"}
	invalid := []string{"", "+1", "-1", "1x", "4294967295", "4294967296"}
	if strconv.IntSize == 64 {
		valid = append(valid, "4294967294")
	} else {
		invalid = append(invalid, "2147483648", "4294967294")
	}
	for _, spec := range valid {
		if _, err := parseGroup(spec); err != nil {
			t.Errorf("parseGroup(%q): %v", spec, err)
		}
	}
	for _, spec := range invalid {
		if _, err := parseGroup(spec); err == nil {
			t.Errorf("parseGroup(%q) unexpectedly succeeded", spec)
		}
	}
	lookupGroup = func(string) (*user.Group, error) { return nil, errors.New("identity service unavailable") }
	if _, err := parseGroup("42"); err == nil || !strings.Contains(err.Error(), "cannot resolve group") {
		t.Fatalf("operational lookup failure lost: %v", err)
	}
}

func TestChgrpFromNumericGrammarAndLookupFailures(t *testing.T) {
	realLookup := lookupUser
	lookupUser = func(name string) (*user.User, error) { return nil, user.UnknownUserError(name) }
	t.Cleanup(func() { lookupUser = realLookup })

	valid := []string{"0", "00042"}
	invalid := []string{"+1", "-1", "1x", "4294967295", "4294967296"}
	if strconv.IntSize == 64 {
		valid = append(valid, "4294967294")
	} else {
		invalid = append(invalid, "2147483648", "4294967294")
	}
	for _, spec := range valid {
		uid, gid, err := parseFromSpec(spec)
		if err != nil || uid < 0 || gid != -1 {
			t.Errorf("parseFromSpec(%q)=(%d,%d,%v), want a user ID and unchanged group", spec, uid, gid, err)
		}
	}
	for _, spec := range invalid {
		if _, _, err := parseFromSpec(spec); err == nil {
			t.Errorf("parseFromSpec(%q) unexpectedly succeeded", spec)
		}
	}

	lookupUser = func(string) (*user.User, error) { return nil, errors.New("identity service unavailable") }
	if _, _, err := parseFromSpec("42"); err == nil || !strings.Contains(err.Error(), "cannot resolve user '42'") {
		t.Fatalf("operational lookup failure lost: %v", err)
	}

	lookupUser = func(string) (*user.User, error) { return &user.User{Uid: "4294967295"}, nil }
	if _, _, err := parseFromSpec("named"); err == nil || !strings.Contains(err.Error(), "invalid user") {
		t.Fatalf("invalid database UID unexpectedly accepted: %v", err)
	}
}

func TestChgrpFromRuntimeRejectsInvalidOrUnavailableOwner(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	realUser, realChange := lookupUser, changeGroup
	var calls int
	changeGroup = func(string, int, int, bool) error {
		calls++
		return nil
	}
	t.Cleanup(func() {
		lookupUser = realUser
		changeGroup = realChange
	})

	lookupUser = func(name string) (*user.User, error) { return nil, user.UnknownUserError(name) }
	invalid := []string{"4294967295", "4294967296"}
	if strconv.IntSize == 32 {
		invalid = append(invalid, "2147483648", "4294967294")
	}
	for _, spec := range invalid {
		calls = 0
		_, errb, code := runTool(t, dir, "--from="+spec, currentGroup(t), "f")
		if code != 1 || !strings.Contains(errb, "invalid user") || calls != 0 {
			t.Errorf("--from=%s: code=%d calls=%d err=%q", spec, code, calls, errb)
		}
	}

	lookupUser = func(string) (*user.User, error) { return nil, errors.New("identity service unavailable") }
	calls = 0
	_, errb, code := runTool(t, dir, "--from=42", currentGroup(t), "f")
	if code != 1 || !strings.Contains(errb, "cannot resolve user '42': identity service unavailable") || calls != 0 {
		t.Fatalf("operational lookup failure: code=%d calls=%d err=%q", code, calls, errb)
	}
}

func TestChgrpNativeCtimeSymlinkAndHardLinkIdentity(t *testing.T) {
	dir := t.TempDir()
	target, alias, link := filepath.Join(dir, "target"), filepath.Join(dir, "alias"), filepath.Join(dir, "link")
	if err := os.WriteFile(target, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(target, alias); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if err := os.Symlink("target", link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	beforeTarget, beforeLink := chgrpChangeTime(t, target, true), chgrpChangeTime(t, link, false)
	waitForChgrpTimestampAfter(t, dir, laterTime(beforeTarget, beforeLink))
	if _, errb, code := runTool(t, dir, "-h", currentGroup(t), "link"); code != 0 || errb != "" {
		t.Fatalf("chgrp -h: code=%d err=%q", code, errb)
	}
	afterTarget, afterLink := chgrpChangeTime(t, target, true), chgrpChangeTime(t, link, false)
	if !afterLink.After(beforeLink) || !afterTarget.Equal(beforeTarget) {
		t.Fatalf("ctime selection target %v→%v link %v→%v", beforeTarget, afterTarget, beforeLink, afterLink)
	}
	if aliasTime := chgrpChangeTime(t, alias, true); !aliasTime.Equal(afterTarget) {
		t.Fatalf("hard-link ctime target=%v alias=%v", afterTarget, aliasTime)
	}
}

func waitForChgrpTimestampAfter(t *testing.T, dir string, before time.Time) {
	t.Helper()
	marker := filepath.Join(dir, "ctime-clock")
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(3 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	mode := os.FileMode(0o600)
	for {
		mode ^= 0o100
		if err := os.Chmod(marker, mode); err != nil {
			t.Fatal(err)
		}
		if chgrpChangeTime(t, marker, true).After(before) {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("filesystem ctime did not advance past %v", before)
		}
	}
}

func laterTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func chgrpChangeTime(t *testing.T, path string, follow bool) time.Time {
	t.Helper()
	stat := os.Lstat
	if follow {
		stat = os.Stat
	}
	fi, err := stat(path)
	if err != nil {
		t.Fatal(err)
	}
	v := reflect.ValueOf(fi.Sys())
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	for _, field := range []string{"Ctim", "Ctimespec"} {
		ct := v.FieldByName(field)
		if ct.IsValid() {
			return time.Unix(ct.FieldByName("Sec").Int(), ct.FieldByName("Nsec").Int())
		}
	}
	t.Skip("native stat metadata does not expose ctime")
	return time.Time{}
}
