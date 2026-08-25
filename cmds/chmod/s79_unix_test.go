//go:build unix

package chmodcmd

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func TestChmodSymbolicModeUsesInvocationUmask(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: dir, Umask: 0o027, UmaskSet: true,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	if code := cmd.Run(rc, []string{"+rw", "f"}); code != 0 || errb.Len() != 0 {
		t.Fatalf("chmod +rw: code=%d err=%q", code, errb.String())
	}
	fi, err := os.Stat(filepath.Join(dir, "f"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o640 {
		t.Fatalf("mode=%#o, want 0640 under invocation umask 027", got)
	}
}

func TestChmodSameModeFailuresContinue(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"denied", "readonly", "after"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	realChange := changeMode
	var called []string
	changeMode = func(path string, mode os.FileMode) error {
		name := filepath.Base(path)
		called = append(called, name)
		switch name {
		case "denied":
			return &os.PathError{Op: "chmod", Path: path, Err: fs.ErrPermission}
		case "readonly":
			return &os.PathError{Op: "chmod", Path: path, Err: syscall.EROFS}
		default:
			return realChange(path, mode)
		}
	}
	t.Cleanup(func() { changeMode = realChange })

	// Every file already has 0644. The provider still has to be called:
	// equality cannot bypass permission checking or later operands.
	_, errb, code := runTool(t, dir, "644", "denied", "readonly", "after")
	if code != 1 || strings.Join(called, " ") != "denied readonly after" {
		t.Fatalf("code=%d calls=%v err=%q", code, called, errb)
	}
	for _, name := range []string{"denied", "readonly"} {
		if !strings.Contains(errb, "changing permissions of '"+name+"'") {
			t.Errorf("missing %s diagnostic: %q", name, errb)
		}
	}
}

func TestChmodNativeCtimeSetIDAndHardLinkIdentity(t *testing.T) {
	dir := t.TempDir()
	path, alias := filepath.Join(dir, "file"), filepath.Join(dir, "alias")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, alias); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	before := chmodStatusChangeTime(t, path)
	waitForChmodTimestampAfter(t, dir, before)
	if _, errb, code := runTool(t, dir, "6755", "alias"); code != 0 || errb != "" {
		t.Fatalf("chmod 6755 alias: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Fatalf("hard-link permissions=%#o, want 0755", fi.Mode().Perm())
	}
	if fi.Mode()&(os.ModeSetuid|os.ModeSetgid) != os.ModeSetuid|os.ModeSetgid {
		t.Skipf("filesystem did not retain requested set-ID bits: %v", fi.Mode())
	}
	if after := chmodStatusChangeTime(t, path); !after.After(before) {
		t.Fatalf("ctime did not advance: before=%v after=%v", before, after)
	}
	if _, errb, code := runTool(t, dir, "755", "file"); code != 0 || errb != "" {
		t.Fatalf("chmod 755 file: code=%d err=%q", code, errb)
	}
	fi, err = os.Stat(alias)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		t.Fatalf("absolute octal mode left set-ID bits: %v", fi.Mode())
	}
}

func waitForChmodTimestampAfter(t *testing.T, dir string, before time.Time) {
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
		if chmodStatusChangeTime(t, marker).After(before) {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("filesystem ctime did not advance past %v", before)
		}
	}
}

func chmodStatusChangeTime(t *testing.T, path string) time.Time {
	t.Helper()
	fi, err := os.Stat(path)
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
