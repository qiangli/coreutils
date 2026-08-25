package paxcmd

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func TestPreservationGrammarOrderedAndRepeated(t *testing.T) {
	cases := []struct {
		name   string
		values []string
		want   preservation
	}{
		{"default", nil, preservation{atime: true, mtime: true}},
		{"negative-times", []string{"am"}, preservation{}},
		{"everything", []string{"e"}, preservation{owner: true, mode: true, atime: true, mtime: true}},
		{"example-eme", []string{"eme"}, preservation{owner: true, mode: true, atime: true, mtime: true}},
		{"e-then-negatives", []string{"eam"}, preservation{owner: true, mode: true}},
		{"separate-enable-last", []string{"m", "e"}, preservation{owner: true, mode: true, atime: true, mtime: true}},
		{"separate-disable-last", []string{"e", "m"}, preservation{owner: true, mode: true, atime: true}},
		{"owner-mode", []string{"a", "m", "o", "p"}, preservation{owner: true, mode: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePreservation(tc.values)
			if err != nil || got != tc.want {
				t.Fatalf("policy=%+v err=%v, want %+v", got, err, tc.want)
			}
		})
	}
	for _, values := range [][]string{{""}, {"q"}, {"epx"}} {
		if _, err := parsePreservation(values); err == nil {
			t.Fatalf("parsePreservation(%q) succeeded", values)
		}
	}
}

type archiveFixture struct {
	name, body, link string
	kind             byte
	mode             int64
	uid, gid         int
	atime, mtime     time.Time
}

func makeAttributeArchive(t *testing.T, members ...archiveFixture) string {
	t.Helper()
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	for _, m := range members {
		h := &tar.Header{
			Name: m.name, Typeflag: m.kind, Linkname: m.link,
			Mode: m.mode, Uid: m.uid, Gid: m.gid,
			AccessTime: m.atime, ModTime: m.mtime, Format: tar.FormatPAX,
		}
		if m.kind == tar.TypeReg || m.kind == 0 {
			h.Typeflag = tar.TypeReg
			h.Size = int64(len(m.body))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size != 0 {
			if _, err := io.WriteString(tw, m.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return raw.String()
}

func execPaxContext(t *testing.T, dir, stdin string, mask os.FileMode, args ...string) (string, string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: dir, Umask: mask, UmaskSet: true,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errOut},
	}
	code := run(rc, args)
	return out.String(), errOut.String(), code
}

func TestExtractDefaultCreationAndPreservedMode(t *testing.T) {
	atime := time.Unix(1000000000, 123000000)
	mtime := time.Unix(1000000100, 456000000)
	raw := makeAttributeArchive(t, archiveFixture{
		name: "file", body: "body", mode: 0o777, atime: atime, mtime: mtime,
	})

	defaultDir := t.TempDir()
	_, errOut, code := execPaxContext(t, defaultDir, raw, 0o027, "-r")
	if code != 0 || errOut != "" {
		t.Fatalf("default code=%d stderr=%q", code, errOut)
	}
	fi, err := os.Stat(filepath.Join(defaultDir, "file"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o750 {
		t.Fatalf("default mode=%#o, want archived mode filtered by umask (0750)", got)
	}
	if fi.ModTime().UnixMicro() != mtime.UnixMicro() {
		t.Fatalf("default mtime=%v, want %v", fi.ModTime(), mtime)
	}
	gotA, ok := sourceAccessTime(fi)
	if ok && gotA.UnixMicro() != atime.UnixMicro() {
		t.Fatalf("default atime=%v, want %v", gotA, atime)
	}

	preserveDir := t.TempDir()
	_, errOut, code = execPaxContext(t, preserveDir, raw, 0o077, "-r", "-p", "p")
	if code != 0 || errOut != "" {
		t.Fatalf("-p p code=%d stderr=%q", code, errOut)
	}
	fi, err = os.Stat(filepath.Join(preserveDir, "file"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o777 {
		t.Fatalf("preserved mode=%#o, want 0777", got)
	}
}

func TestRepeatedPOptionsReachExtractionInCommandOrder(t *testing.T) {
	raw := makeAttributeArchive(t, archiveFixture{
		name: "file", body: "x", mode: 0o600, uid: 123, gid: 456,
		atime: time.Unix(1000000000, 0), mtime: time.Unix(1000000100, 0),
	})
	oldChown, oldTimes := chownExtractedFn, timesExtractedFn
	defer func() { chownExtractedFn, timesExtractedFn = oldChown, oldTimes }()
	chownExtractedFn = func(string, int, int, bool) error { return nil }

	for _, tc := range []struct {
		name  string
		args  []string
		wantM bool
	}{
		{"disable-last", []string{"-r", "-p", "e", "-p", "m"}, false},
		{"enable-last", []string{"-r", "-p", "m", "-p", "e"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotA, gotM bool
			timesExtractedFn = func(_ string, _ time.Time, setA bool, _ time.Time, setM, _ bool) error {
				gotA, gotM = setA, setM
				return nil
			}
			_, errOut, code := execPaxContext(t, t.TempDir(), raw, 0o022, tc.args...)
			if code != 0 || errOut != "" || !gotA || gotM != tc.wantM {
				t.Fatalf("code=%d stderr=%q times=(%v,%v), want m=%v", code, errOut, gotA, gotM, tc.wantM)
			}
		})
	}
}

func TestNegativeTimeCharacteristicsReachExtraction(t *testing.T) {
	raw := makeAttributeArchive(t, archiveFixture{
		name: "file", body: "x", mode: 0o600,
		atime: time.Unix(1000000000, 0), mtime: time.Unix(1000000100, 0),
	})
	oldTimes := timesExtractedFn
	defer func() { timesExtractedFn = oldTimes }()
	var gotA, gotM bool
	timesExtractedFn = func(_ string, _ time.Time, setA bool, _ time.Time, setM, _ bool) error {
		gotA, gotM = setA, setM
		return nil
	}
	_, errOut, code := execPaxContext(t, t.TempDir(), raw, 0o022, "-r", "-p", "am")
	if code != 0 || errOut != "" || gotA || gotM {
		t.Fatalf("code=%d stderr=%q times=(%v,%v), want both disabled", code, errOut, gotA, gotM)
	}
}

func TestPreservationFailuresKeepFilesClearSetIDAndContinue(t *testing.T) {
	raw := makeAttributeArchive(t,
		archiveFixture{name: "first", body: "one", mode: 0o6755, uid: 123, gid: 456, mtime: time.Unix(1000000100, 0)},
		archiveFixture{name: "second", body: "two", mode: 0o644, uid: 123, gid: 456, mtime: time.Unix(1000000200, 0)},
	)
	oldChown := chownExtractedFn
	defer func() { chownExtractedFn = oldChown }()
	chownExtractedFn = func(string, int, int, bool) error { return errors.New("ownership denied") }

	d := t.TempDir()
	_, errOut, code := execPaxContext(t, d, raw, 0o022, "-r", "-p", "e")
	if code != 1 || !strings.Contains(errOut, "preserve owner: ownership denied") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	for name, body := range map[string]string{"first": "one", "second": "two"} {
		if got := string(mustRead(t, filepath.Join(d, name))); got != body {
			t.Fatalf("%s body=%q", name, got)
		}
	}
	fi, err := os.Stat(filepath.Join(d, "first"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		t.Fatalf("set-ID survived failed ownership preservation: %s", fi.Mode())
	}
}

func TestModeAndTimeFailuresKeepExtractedFile(t *testing.T) {
	raw := makeAttributeArchive(t, archiveFixture{
		name: "file", body: "body", mode: 0o755,
		atime: time.Unix(1000000000, 0), mtime: time.Unix(1000000100, 0),
	})
	oldChmod, oldTimes := chmodExtractedFn, timesExtractedFn
	defer func() { chmodExtractedFn, timesExtractedFn = oldChmod, oldTimes }()

	chmodExtractedFn = func(string, os.FileMode) error { return errors.New("chmod denied") }
	d := t.TempDir()
	_, errOut, code := execPaxContext(t, d, raw, 0o022, "-r", "-p", "p")
	if code != 1 || !strings.Contains(errOut, "preserve mode: chmod denied") {
		t.Fatalf("mode code=%d stderr=%q", code, errOut)
	}
	if got := string(mustRead(t, filepath.Join(d, "file"))); got != "body" {
		t.Fatalf("mode-failure body=%q", got)
	}

	chmodExtractedFn = oldChmod
	timesExtractedFn = func(string, time.Time, bool, time.Time, bool, bool) error {
		return errors.New("utimens denied")
	}
	d = t.TempDir()
	_, errOut, code = execPaxContext(t, d, raw, 0o022, "-r")
	if code != 1 || !strings.Contains(errOut, "preserve times: utimens denied") {
		t.Fatalf("time code=%d stderr=%q", code, errOut)
	}
	if got := string(mustRead(t, filepath.Join(d, "file"))); got != "body" {
		t.Fatalf("time-failure body=%q", got)
	}
}

func TestDirectoryAttributesFinalizeAfterChildren(t *testing.T) {
	mtime := time.Unix(1000000500, 0)
	raw := makeAttributeArchive(t,
		archiveFixture{name: "dir/", kind: tar.TypeDir, mode: 0o701, mtime: mtime},
		archiveFixture{name: "dir/child", body: "x", mode: 0o600, mtime: time.Unix(1000000600, 0)},
	)
	d := t.TempDir()
	_, errOut, code := execPaxContext(t, d, raw, 0o077, "-r", "-p", "p")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	fi, err := os.Stat(filepath.Join(d, "dir"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o701 || fi.ModTime().Unix() != mtime.Unix() {
		t.Fatalf("directory mode=%#o mtime=%v", fi.Mode().Perm(), fi.ModTime())
	}
}

func TestRestrictiveDirectoryModeIsDeferredUntilChildrenExist(t *testing.T) {
	mtime := time.Unix(1000000700, 0)
	raw := makeAttributeArchive(t,
		archiveFixture{name: "locked/", kind: tar.TypeDir, mode: 0, mtime: mtime},
		archiveFixture{name: "locked/child", body: "x", mode: 0o600, mtime: mtime},
	)
	d := t.TempDir()
	_, errOut, code := execPaxContext(t, d, raw, 0, "-r")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	fi, err := os.Stat(filepath.Join(d, "locked"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0 {
		t.Fatalf("final directory mode=%#o, want 0", fi.Mode().Perm())
	}
	if err := os.Chmod(filepath.Join(d, "locked"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(t, filepath.Join(d, "locked", "child"))); got != "x" {
		t.Fatalf("child=%q", got)
	}
}

func TestDirectoryFinalizationUsesDepthAndLastDuplicate(t *testing.T) {
	raw := makeAttributeArchive(t,
		archiveFixture{name: "outer/inner/", kind: tar.TypeDir, mode: 0o711, mtime: time.Unix(1000000100, 0)},
		archiveFixture{name: "outer/", kind: tar.TypeDir, mode: 0o755, mtime: time.Unix(1000000200, 0)},
		archiveFixture{name: "outer/inner/", kind: tar.TypeDir, mode: 0o700, mtime: time.Unix(1000000300, 0)},
	)
	d := t.TempDir()
	_, errOut, code := execPaxContext(t, d, raw, 0, "-r", "-p", "p")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	inner, err := os.Stat(filepath.Join(d, "outer", "inner"))
	if err != nil {
		t.Fatal(err)
	}
	if inner.Mode().Perm() != 0o700 {
		t.Fatalf("last duplicate directory mode=%#o, want 0700", inner.Mode().Perm())
	}
}

func TestSymlinkPreservationUsesNoFollowOwnerAndTimes(t *testing.T) {
	raw := makeAttributeArchive(t, archiveFixture{
		name: "link", kind: tar.TypeSymlink, link: "target", mode: 0o777,
		uid: 123, gid: 456, atime: time.Unix(1000000000, 0), mtime: time.Unix(1000000100, 0),
	})
	oldChown, oldChmod, oldTimes := chownExtractedFn, chmodExtractedFn, timesExtractedFn
	defer func() { chownExtractedFn, chmodExtractedFn, timesExtractedFn = oldChown, oldChmod, oldTimes }()
	var ownerSymlink, timesSymlink bool
	chmodCalls := 0
	chownExtractedFn = func(_ string, uid, gid int, symlink bool) error {
		ownerSymlink = symlink
		if uid != 123 || gid != 456 {
			t.Fatalf("owner=(%d,%d)", uid, gid)
		}
		return nil
	}
	chmodExtractedFn = func(string, os.FileMode) error { chmodCalls++; return nil }
	timesExtractedFn = func(_ string, _ time.Time, setA bool, _ time.Time, setM, symlink bool) error {
		timesSymlink = symlink
		if !setA || !setM {
			t.Fatalf("time selection=(%v,%v)", setA, setM)
		}
		return nil
	}
	_, errOut, code := execPaxContext(t, t.TempDir(), raw, 0o022, "-r", "-p", "e")
	if code != 0 || errOut != "" || !ownerSymlink || !timesSymlink || chmodCalls != 0 {
		t.Fatalf("code=%d stderr=%q ownerSymlink=%v timesSymlink=%v chmodCalls=%d", code, errOut, ownerSymlink, timesSymlink, chmodCalls)
	}
}

func TestCopyLinkSetIDFallbackAndPreserveEverythingLink(t *testing.T) {
	d := t.TempDir()
	source := filepath.Join(d, "source")
	if err := os.WriteFile(source, []byte("body"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(source, 0o755|os.ModeSetuid|os.ModeSetgid); err != nil {
		t.Skipf("set-ID mode unavailable: %v", err)
	}

	defaultDest := filepath.Join(d, "default")
	if err := os.Mkdir(defaultDest, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, errOut, code := execPaxContext(t, d, "", 0o022, "-r", "-w", "-l", "source", "default"); code != 0 || errOut != "" {
		t.Fatalf("default code=%d stderr=%q", code, errOut)
	}
	srcInfo, _ := os.Stat(source)
	dstInfo, err := os.Stat(filepath.Join(defaultDest, "source"))
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(srcInfo, dstInfo) || dstInfo.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		t.Fatalf("default -l did not copy-and-clear set-ID: same=%v mode=%s", os.SameFile(srcInfo, dstInfo), dstInfo.Mode())
	}

	preserveDest := filepath.Join(d, "preserve")
	if err := os.Mkdir(preserveDest, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, errOut, code := execPaxContext(t, d, "", 0o022, "-r", "-w", "-l", "-p", "e", "source", "preserve"); code != 0 || errOut != "" {
		t.Fatalf("preserve code=%d stderr=%q", code, errOut)
	}
	preserved, err := os.Stat(filepath.Join(preserveDest, "source"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(srcInfo, preserved) {
		t.Fatal("-p e -l did not retain the possible hard link")
	}
}

func TestCopyLinkFallbackAppliesRequestedAttributes(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "source"), []byte("body"), 0o751); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(d, "dest")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	oldLink, oldChown := linkSourceFn, chownExtractedFn
	defer func() { linkSourceFn, chownExtractedFn = oldLink, oldChown }()
	linkSourceFn = func(string, string) error { return errors.New("cross-device link") }
	chownCalls := 0
	chownExtractedFn = func(string, int, int, bool) error { chownCalls++; return nil }
	_, errOut, code := execPaxContext(t, d, "", 0o077, "-r", "-w", "-l", "-p", "op", "source", "dest")
	if code != 0 || errOut != "" || chownCalls != 1 {
		t.Fatalf("code=%d stderr=%q chownCalls=%d", code, errOut, chownCalls)
	}
	fi, err := os.Stat(filepath.Join(dest, "source"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o751 {
		t.Fatalf("fallback mode=%#o, want 0751", fi.Mode().Perm())
	}
}

func TestOrdinaryCopyPreservesEverythingThroughArchiveLane(t *testing.T) {
	d := t.TempDir()
	source := filepath.Join(d, "source")
	if err := os.WriteFile(source, []byte("body"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(source, 0o751); err != nil {
		t.Fatal(err)
	}
	mtime := time.Unix(1000000800, 0)
	if err := os.Chtimes(source, time.Unix(1000000700, 0), mtime); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(d, "dest")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	oldChown := chownExtractedFn
	defer func() { chownExtractedFn = oldChown }()
	chownCalls := 0
	chownExtractedFn = func(_ string, uid, gid int, symlink bool) error {
		chownCalls++
		if symlink || uid < 0 || gid < 0 {
			t.Fatalf("owner uid=%d gid=%d symlink=%v", uid, gid, symlink)
		}
		return nil
	}
	_, errOut, code := execPaxContext(t, d, "", 0o077, "-r", "-w", "-p", "e", "source", "dest")
	if code != 0 || errOut != "" || chownCalls != 1 {
		t.Fatalf("code=%d stderr=%q chownCalls=%d", code, errOut, chownCalls)
	}
	fi, err := os.Stat(filepath.Join(dest, "source"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o751 || fi.ModTime().Unix() != mtime.Unix() {
		t.Fatalf("copied mode=%#o mtime=%v", fi.Mode().Perm(), fi.ModTime())
	}
}
