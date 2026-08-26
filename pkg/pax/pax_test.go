package pax

import (
	"archive/tar"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadManifestCapturesKindsAndPAXRecords(t *testing.T) {
	archive := makeArchive(t, []*tar.Header{
		{Name: "dir/", Typeflag: tar.TypeDir, Format: tar.FormatUSTAR},
		{Name: strings.Repeat("nested/", 20) + "file", Typeflag: tar.TypeReg, Size: 1, Format: tar.FormatPAX, PAXRecords: map[string]string{"comment": "hello"}},
		{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "dir/file", Format: tar.FormatPAX},
	})
	members, err := ReadManifest(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 3 || members[0].Kind != KindDir || members[1].Kind != KindFile || members[1].PAXRecords["comment"] != "hello" || members[2].Kind != KindSymlink {
		t.Fatalf("manifest = %#v", members)
	}
}

func TestPlanExtractionRejectsUnsafeEntriesWithoutMutation(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.Symlink(outside, filepath.Join(root, "via-link")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	archive := makeArchive(t, []*tar.Header{
		{Name: "ok/file", Typeflag: tar.TypeReg, Size: 1},
		{Name: "/absolute", Typeflag: tar.TypeReg, Size: 1},
		{Name: "../escape", Typeflag: tar.TypeReg, Size: 1},
		{Name: "sym", Typeflag: tar.TypeSymlink, Linkname: "../../escape"},
		{Name: "dev", Typeflag: tar.TypeChar},
		{Name: "via-link/file", Typeflag: tar.TypeReg, Size: 1},
	})
	plan, err := PlanExtraction(bytes.NewReader(archive), root, OSFS{})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Unsafe || len(plan.Members) != 1 || len(plan.Rejected) != 5 {
		t.Fatalf("plan = %#v", plan)
	}
	if _, err := os.Stat(filepath.Join(root, "ok", "file")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("planner mutated root: %v", err)
	}
}

// A repeated name of the SAME kind is an ordinary archive update: the last
// occurrence wins and the earlier ones are superseded, not rejected. A
// repeated name of DIFFERENT kinds is the type-substitution attack and stays
// fatal for the whole group.
func TestPlanExtractionSupersedesSameKindDuplicatesAndRejectsKindConflicts(t *testing.T) {
	root := t.TempDir()
	archive := makeArchive(t, []*tar.Header{
		{Name: "same", Typeflag: tar.TypeReg, Size: 1},
		{Name: "same", Typeflag: tar.TypeReg, Size: 1},
		{Name: "conflict", Typeflag: tar.TypeDir},
		{Name: "conflict", Typeflag: tar.TypeSymlink, Linkname: "elsewhere"},
	})
	plan, err := PlanExtraction(bytes.NewReader(archive), root, OSFS{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Members) != 1 || plan.Members[0].Path != "same" || plan.Members[0].Index != 1 {
		t.Fatalf("members = %#v", plan.Members)
	}
	if len(plan.Superseded) != 1 || plan.Superseded[0].Path != "same" {
		t.Fatalf("superseded = %#v", plan.Superseded)
	}
	if len(plan.Rejected) != 2 {
		t.Fatalf("rejected = %#v", plan.Rejected)
	}
	for _, rejected := range plan.Rejected {
		if rejected.Path != "conflict" || !strings.Contains(rejected.Reason, "duplicate destination") {
			t.Fatalf("rejection = %#v", rejected)
		}
	}
	if !plan.Unsafe {
		t.Fatal("a kind conflict must still mark the plan unsafe")
	}

	for _, headers := range [][]*tar.Header{
		{
			{Name: "fifo-conflict", Typeflag: tar.TypeReg, Size: 1},
			{Name: "fifo-conflict", Typeflag: tar.TypeFifo},
		},
		{
			{Name: "fifo-conflict", Typeflag: tar.TypeFifo},
			{Name: "fifo-conflict", Typeflag: tar.TypeReg, Size: 1},
		},
	} {
		plan, err := PlanExtraction(bytes.NewReader(makeArchive(t, headers)), t.TempDir(), OSFS{})
		if err != nil {
			t.Fatal(err)
		}
		if !plan.Unsafe || len(plan.Members) != 0 || len(plan.Rejected) != 2 {
			t.Fatalf("FIFO/regular kind conflict plan = %#v", plan)
		}
		for _, rejected := range plan.Rejected {
			if !strings.Contains(rejected.Reason, "duplicate destination") {
				t.Fatalf("FIFO/regular rejection = %#v", rejected)
			}
		}
	}
}

func TestPlanExtractionTreatsRegularHardlinkTransitionsAsFileUpdates(t *testing.T) {
	for _, tc := range []struct {
		name    string
		headers []*tar.Header
		kind    Kind
	}{
		{
			name: "regular-to-hardlink",
			headers: []*tar.Header{
				{Name: "source", Typeflag: tar.TypeReg, Size: 1},
				{Name: "updated", Typeflag: tar.TypeReg, Size: 1},
				{Name: "updated", Typeflag: tar.TypeLink, Linkname: "source"},
			},
			kind: KindHardlink,
		},
		{
			name: "hardlink-to-regular",
			headers: []*tar.Header{
				{Name: "source", Typeflag: tar.TypeReg, Size: 1},
				{Name: "updated", Typeflag: tar.TypeLink, Linkname: "source"},
				{Name: "updated", Typeflag: tar.TypeReg, Size: 1},
			},
			kind: KindFile,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := PlanExtraction(bytes.NewReader(makeArchive(t, tc.headers)), t.TempDir(), OSFS{})
			if err != nil {
				t.Fatal(err)
			}
			if plan.Unsafe || len(plan.Rejected) != 0 {
				t.Fatalf("compatible file history rejected: %#v", plan.Rejected)
			}
			if len(plan.Superseded) != 1 || plan.Superseded[0].Path != "updated" {
				t.Fatalf("superseded = %#v", plan.Superseded)
			}
			if len(plan.Members) != 2 || plan.Members[1].Index != 2 || plan.Members[1].Kind != tc.kind {
				t.Fatalf("members = %#v", plan.Members)
			}
		})
	}
}

func TestPlanExtractionRegularHardlinkHistoryDoesNotLaunderUnsafeTarget(t *testing.T) {
	archive := makeArchive(t, []*tar.Header{
		{Name: "updated", Typeflag: tar.TypeLink, Linkname: "../escape"},
		{Name: "updated", Typeflag: tar.TypeReg, Size: 1},
	})
	plan, err := PlanExtraction(bytes.NewReader(archive), t.TempDir(), OSFS{})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Unsafe || len(plan.Rejected) != 1 || !strings.Contains(plan.Rejected[0].Reason, "hardlink target") {
		t.Fatalf("unsafe hardlink was laundered by update: %#v", plan)
	}
}

// A superseded occurrence must not launder a safety verdict: if the earlier
// copy was rejected on its own merits, the rejection is still reported.
func TestPlanExtractionSupersedeKeepsEarlierRejections(t *testing.T) {
	archive := makeArchive(t, []*tar.Header{
		{Name: "dev", Typeflag: tar.TypeChar},
		{Name: "dev", Typeflag: tar.TypeChar},
	})
	plan, err := PlanExtraction(bytes.NewReader(archive), t.TempDir(), OSFS{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Members) != 0 || len(plan.Rejected) != 2 || len(plan.Superseded) != 0 {
		t.Fatalf("plan = %#v", plan)
	}
}

// Duplicate hardlinks and duplicate symlinks are updates too, and the last
// occurrence is the one whose link target the planner validated.
func TestPlanExtractionSupersedesRepeatedSymlink(t *testing.T) {
	root := t.TempDir()
	archive := makeArchive(t, []*tar.Header{
		{Name: "file", Typeflag: tar.TypeReg, Size: 0},
		{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "file"},
		{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../escape"},
	})
	plan, err := PlanExtraction(bytes.NewReader(archive), root, OSFS{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Rejected) != 1 || !strings.Contains(plan.Rejected[0].Reason, "escapes root") {
		t.Fatalf("the surviving occurrence must be the one validated: %#v", plan.Rejected)
	}
}

func TestPlanExtractionRejectsArchiveSymlinkParentRegardlessOfOrder(t *testing.T) {
	for _, headers := range [][]*tar.Header{
		{{Name: "parent", Typeflag: tar.TypeSymlink, Linkname: "real"}, {Name: "parent/file", Typeflag: tar.TypeReg, Size: 1}},
		{{Name: "parent/file", Typeflag: tar.TypeReg, Size: 1}, {Name: "parent", Typeflag: tar.TypeSymlink, Linkname: "real"}},
	} {
		plan, err := PlanExtraction(bytes.NewReader(makeArchive(t, headers)), t.TempDir(), OSFS{})
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Members) != 1 || len(plan.Rejected) != 1 || !strings.Contains(plan.Rejected[0].Reason, "symlink parent") {
			t.Fatalf("plan = %#v", plan)
		}
	}
}

func TestPlanExtractionPropagatesLateInvalidParent(t *testing.T) {
	archive := makeArchive(t, []*tar.Header{
		{Name: "parent/child/file", Typeflag: tar.TypeReg, Size: 1},
		{Name: "parent/child", Typeflag: tar.TypeDir},
		{Name: "parent", Typeflag: tar.TypeSymlink, Linkname: "real"},
	})
	plan, err := PlanExtraction(bytes.NewReader(archive), t.TempDir(), OSFS{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Members) != 1 || len(plan.Rejected) != 2 {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestPlanExtractionHardlinkSources(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "present"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	archive := makeArchive(t, []*tar.Header{
		{Name: "forward", Typeflag: tar.TypeLink, Linkname: "source"},
		{Name: "source", Typeflag: tar.TypeReg, Size: 1},
		{Name: "existing", Typeflag: tar.TypeLink, Linkname: "present"},
		{Name: "bad-dir", Typeflag: tar.TypeLink, Linkname: "directory"},
		{Name: "directory", Typeflag: tar.TypeDir},
		{Name: "cycle-a", Typeflag: tar.TypeLink, Linkname: "cycle-b"},
		{Name: "cycle-b", Typeflag: tar.TypeLink, Linkname: "cycle-a"},
		{Name: "missing", Typeflag: tar.TypeLink, Linkname: "absent"},
	})
	plan, err := PlanExtraction(bytes.NewReader(archive), root, OSFS{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Members) != 4 || len(plan.Rejected) != 4 {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestPlanExtractionRootAndExistingTargetPolicy(t *testing.T) {
	archive := makeArchive(t, []*tar.Header{{Name: "taken", Typeflag: tar.TypeReg, Size: 1}})
	for _, root := range []string{"", ".", "relative"} {
		if _, err := PlanExtraction(bytes.NewReader(archive), root, OSFS{}); err == nil {
			t.Fatalf("root %q accepted", root)
		}
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "taken"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanExtraction(bytes.NewReader(archive), root, OSFS{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Members) != 0 || len(plan.Rejected) != 1 {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestPlanExtractionUsesInjectedFilesystem(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "safe")
	archive := makeArchive(t, []*tar.Header{{Name: "parent/file", Typeflag: tar.TypeReg, Size: 1}})
	plan, err := PlanExtraction(bytes.NewReader(archive), root, fakeFS{files: map[string]fs.FileInfo{root: fakeInfo{mode: fs.ModeDir | 0o755}, filepath.Join(root, "parent"): fakeInfo{mode: fs.ModeSymlink}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Rejected) != 1 || !strings.Contains(plan.Rejected[0].Reason, "existing symlink parent") {
		t.Fatalf("plan = %#v", plan)
	}
}

type fakeFS struct{ files map[string]fs.FileInfo }

func (f fakeFS) Lstat(path string) (fs.FileInfo, error) {
	if fi, ok := f.files[path]; ok {
		return fi, nil
	}
	return nil, fs.ErrNotExist
}
func (fakeFS) Readlink(string) (string, error) { return "", fs.ErrNotExist }

type fakeInfo struct{ mode fs.FileMode }

func (f fakeInfo) Name() string       { return "fake" }
func (f fakeInfo) Size() int64        { return 0 }
func (f fakeInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeInfo) Sys() any           { return nil }

func makeArchive(t *testing.T, headers []*tar.Header) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, header := range headers {
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("WriteHeader(%q): %v", header.Name, err)
		}
		if header.Size > 0 {
			if _, err := tw.Write(bytes.Repeat([]byte("x"), int(header.Size))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
