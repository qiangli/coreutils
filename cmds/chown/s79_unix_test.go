//go:build unix

package chowncmd

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

func TestChownTransitionFailuresContinue(t *testing.T) {
	u, dir := currentUser(t), t.TempDir()
	for _, name := range []string{"denied", "readonly", "after"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	realChange := changeOwner
	var called []string
	changeOwner = func(path string, _, _ int, _ bool) error {
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
	t.Cleanup(func() { changeOwner = realChange })
	_, errb, code := runTool(t, dir, u.Uid+":"+u.Gid, "denied", "readonly", "after")
	if code != 1 || strings.Join(called, " ") != "denied readonly after" {
		t.Fatalf("code=%d calls=%v err=%q", code, called, errb)
	}
	for _, name := range []string{"denied", "readonly"} {
		if !strings.Contains(errb, "changing ownership of '"+name+"'") {
			t.Errorf("missing %s diagnostic: %q", name, errb)
		}
	}
}

func TestChownNumericGrammarAndLookupFailures(t *testing.T) {
	realUser, realGroup := lookupUser, lookupGroup
	lookupUser = func(name string) (*user.User, error) { return nil, user.UnknownUserError(name) }
	lookupGroup = func(name string) (*user.Group, error) { return nil, user.UnknownGroupError(name) }
	t.Cleanup(func() { lookupUser, lookupGroup = realUser, realGroup })
	valid := []string{"0", "00042", "42:0007"}
	invalid := []string{"+1", "-1", "1x", "4294967295", "4294967296", "1:+2", "1:4294967295"}
	if strconv.IntSize == 64 {
		valid = append(valid, "4294967294")
	} else {
		invalid = append(invalid, "2147483648", "4294967294", "1:2147483648")
	}
	for _, spec := range valid {
		if _, _, err := parseSpec(spec); err != nil {
			t.Errorf("parseSpec(%q): %v", spec, err)
		}
	}
	for _, spec := range invalid {
		if _, _, err := parseSpec(spec); err == nil {
			t.Errorf("parseSpec(%q) unexpectedly succeeded", spec)
		}
	}
	lookupUser = func(string) (*user.User, error) { return nil, errors.New("identity service unavailable") }
	if _, _, err := parseSpec("42"); err == nil || !strings.Contains(err.Error(), "cannot resolve user") {
		t.Fatalf("operational user lookup failure lost: %v", err)
	}
	lookupUser = func(name string) (*user.User, error) { return nil, user.UnknownUserError(name) }
	lookupGroup = func(string) (*user.Group, error) { return nil, errors.New("identity service unavailable") }
	if _, _, err := parseSpec("42:7"); err == nil || !strings.Contains(err.Error(), "cannot resolve group") {
		t.Fatalf("operational group lookup failure lost: %v", err)
	}
}

func TestChownNativeCtimeSymlinkAndHardLinkIdentity(t *testing.T) {
	u, dir := currentUser(t), t.TempDir()
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
	beforeTarget, beforeLink := chownChangeTime(t, target, true), chownChangeTime(t, link, false)
	waitForChownTimestampAfter(t, dir, chownLaterTime(beforeTarget, beforeLink))
	if _, errb, code := runTool(t, dir, u.Uid+":"+u.Gid, "link"); code != 0 || errb != "" {
		t.Fatalf("chown referent: code=%d err=%q", code, errb)
	}
	afterTarget, afterLink := chownChangeTime(t, target, true), chownChangeTime(t, link, false)
	if !afterTarget.After(beforeTarget) || !afterLink.Equal(beforeLink) {
		t.Fatalf("default ctime target %v→%v link %v→%v", beforeTarget, afterTarget, beforeLink, afterLink)
	}
	if aliasTime := chownChangeTime(t, alias, true); !aliasTime.Equal(afterTarget) {
		t.Fatalf("hard-link ctime target=%v alias=%v", afterTarget, aliasTime)
	}
	waitForChownTimestampAfter(t, dir, chownLaterTime(afterTarget, afterLink))
	if _, errb, code := runTool(t, dir, "-h", u.Uid+":"+u.Gid, "link"); code != 0 || errb != "" {
		t.Fatalf("chown -h: code=%d err=%q", code, errb)
	}
	if finalLink := chownChangeTime(t, link, false); !finalLink.After(afterLink) {
		t.Fatalf("-h link ctime did not advance: %v→%v", afterLink, finalLink)
	}
	if finalTarget := chownChangeTime(t, target, true); !finalTarget.Equal(afterTarget) {
		t.Fatalf("-h changed referent ctime: %v→%v", afterTarget, finalTarget)
	}
}

func waitForChownTimestampAfter(t *testing.T, dir string, before time.Time) {
	t.Helper()
	marker := filepath.Join(dir, "ctime-clock")
	if _, err := os.Stat(marker); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(marker, nil, 0o600); err != nil {
			t.Fatal(err)
		}
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
		if chownChangeTime(t, marker, true).After(before) {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("filesystem ctime did not advance past %v", before)
		}
	}
}

func chownLaterTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func chownChangeTime(t *testing.T, path string, follow bool) time.Time {
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
