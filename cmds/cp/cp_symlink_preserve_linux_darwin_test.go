//go:build darwin || linux

package cpcmd

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestCpPreservePhysicalSymlinkMetadataWithoutMutatingReferent(t *testing.T) {
	dir := t.TempDir()
	referent := filepath.Join(dir, "referent")
	write(t, referent, "payload")
	link := filepath.Join(dir, "link")
	if err := os.Symlink("referent", link); err != nil {
		t.Fatal(err)
	}

	referentTime := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	linkAtime := time.Date(2012, 3, 4, 5, 6, 7, 0, time.UTC)
	linkMtime := time.Date(2013, 4, 5, 6, 7, 8, 0, time.UTC)
	if err := os.Chtimes(referent, referentTime, referentTime); err != nil {
		t.Fatal(err)
	}
	if err := preserveLinkTimes(link, linkAtime, linkMtime); err != nil {
		t.Skipf("filesystem cannot set symlink timestamps: %v", err)
	}
	sourceInfo, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	wantAtime, ok := atime(sourceInfo)
	if !ok {
		t.Fatal("source symlink access time unavailable")
	}

	oldLchown := lchownFn
	var lchownPath string
	lchownFn = func(path string, uid, gid int) error {
		lchownPath = path
		return oldLchown(path, uid, gid)
	}
	t.Cleanup(func() { lchownFn = oldLchown })

	_, errb, code := runTool(t, dir, "-pP", "link", "copy")
	if code != 0 || errb != "" {
		t.Fatalf("cp -pP link copy: code=%d err=%q", code, errb)
	}
	dest := filepath.Join(dir, "copy")
	if lchownPath != dest {
		t.Fatalf("Lchown path=%q, want destination symlink %q", lchownPath, dest)
	}

	destInfo, err := os.Lstat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if destInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("destination mode=%v, want symlink", destInfo.Mode())
	}
	gotAtime, ok := atime(destInfo)
	if !ok || gotAtime.Unix() != wantAtime.Unix() || destInfo.ModTime().Unix() != sourceInfo.ModTime().Unix() {
		t.Fatalf("destination link times=(%v,%v), want (%v,%v)",
			gotAtime, destInfo.ModTime(), wantAtime, sourceInfo.ModTime())
	}
	sourceStat := sourceInfo.Sys().(*syscall.Stat_t)
	destStat := destInfo.Sys().(*syscall.Stat_t)
	if sourceStat.Uid != destStat.Uid || sourceStat.Gid != destStat.Gid {
		t.Fatalf("destination owner=%d:%d, want source owner=%d:%d",
			destStat.Uid, destStat.Gid, sourceStat.Uid, sourceStat.Gid)
	}

	referentInfo, err := os.Stat(referent)
	if err != nil {
		t.Fatal(err)
	}
	referentAtime, ok := atime(referentInfo)
	if !ok || referentAtime.Unix() != referentTime.Unix() || referentInfo.ModTime().Unix() != referentTime.Unix() {
		t.Fatalf("referent times mutated through symlink: atime=%v mtime=%v want=%v",
			referentAtime, referentInfo.ModTime(), referentTime)
	}
}
