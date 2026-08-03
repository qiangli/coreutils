//go:build unix

package shell

import (
	"os"
	"testing"

	_ "github.com/qiangli/coreutils/cmds/touch"
)

func TestHandlerPropagatesVirtualUmaskToTouch(t *testing.T) {
	dir := t.TempDir()
	baseline := dir + "/baseline"
	f, err := os.OpenFile(baseline, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	baseInfo, err := os.Stat(baseline)
	if err != nil {
		t.Fatal(err)
	}

	_, errb, runErr := runScript(t,
		"touch default; umask 077; touch restricted; umask 006; touch tp5",
		dir, Handler())
	if runErr != nil || errb != "" {
		t.Fatalf("run: err=%v stderr=%q", runErr, errb)
	}
	for name, want := range map[string]os.FileMode{
		"default":    baseInfo.Mode().Perm(),
		"restricted": 0o600,
		"tp5":        0o660,
	} {
		info, err := os.Stat(dir + "/" + name)
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode=%#o, want %#o", name, got, want)
		}
	}
}
