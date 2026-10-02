package paxcmd

import (
	"archive/tar"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

const (
	// nameMax is the smallest {NAME_MAX} across the hosts this suite runs on.
	nameMax = 255
	// fixtureComponentWidth is the width of a fixture directory component. It
	// leaves room for the padding nearLimitComponents adds to the last one to
	// land exactly on the pathname budget.
	fixtureComponentWidth = 200
)

// nearLimitComponents builds the components of a relative pathname whose own
// joined length is exactly want (or as close below it as component sizing
// allows), with every component well inside {NAME_MAX}. The point of the
// fixture is a member pax may legally name — within an inclusive {PATH_MAX} —
// that no longer fits once it is resolved against the run directory.
func nearLimitComponents(want int) []string {
	var comps []string
	joined := 0
	for {
		next := fixtureComponentWidth
		if joined > 0 {
			next++ // the separator
		}
		if joined+next > want {
			break
		}
		comps = append(comps, strings.Repeat("p", fixtureComponentWidth))
		joined += next
	}
	if len(comps) == 0 {
		return nil
	}
	switch grow := want - joined; {
	case grow <= 0:
	case fixtureComponentWidth+grow <= nameMax:
		comps[len(comps)-1] += strings.Repeat("p", grow)
	case grow >= 2:
		comps = append(comps, strings.Repeat("p", grow-1))
	}
	return comps
}

// nearLimitSourceTree materializes a directory chain under dir one component
// at a time through directory handles, so the fixture itself never hands the
// kernel the overlong absolute spelling it is built to test. It returns the
// command-line operand (the top component) and the leaf's relative pathname.
// Host limitations skip; the caller never sees a setup failure.
func nearLimitSourceTree(t *testing.T, dir string, leaf func(*os.Root) error) (operand, member string) {
	t.Helper()
	comps := nearLimitComponents(destinationPathMax - 1 - len("/x"))
	if len(comps) < 2 {
		t.Skipf("host pathname limit %d is too small for a multi-component fixture", destinationPathMax)
	}
	member = path.Join(append(append([]string{}, comps...), "x")...)
	if len(member) >= destinationPathMax {
		t.Skipf("host pathname limit %d cannot hold a %d-byte relative member", destinationPathMax, len(member))
	}
	if len(dir)+1+len(member) < destinationPathMax {
		t.Skipf("run directory %q is too short to push the resolved member past the %d-byte limit", dir, destinationPathMax)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Skipf("root-relative directory handles unavailable: %v", err)
	}
	defer root.Close()
	current := root
	defer func() {
		if current != root {
			current.Close()
		}
	}()
	for _, c := range comps {
		if err := current.Mkdir(c, 0o700); err != nil {
			t.Skipf("host cannot create a %d-byte fixture component: %v", len(c), err)
		}
		next, err := current.OpenRoot(c)
		if err != nil {
			t.Skipf("host cannot descend into the fixture: %v", err)
		}
		if current != root {
			current.Close()
		}
		current = next
	}
	if err := leaf(current); err != nil {
		t.Skipf("host cannot create the fixture leaf: %v", err)
	}
	return comps[0], member
}

func regularLeaf(r *os.Root) error { return r.WriteFile("x", []byte("deep"), 0o600) }

func symlinkLeaf(r *os.Root) error {
	if err := r.WriteFile("referent", []byte("deep"), 0o600); err != nil {
		return err
	}
	return r.Symlink("referent", "x")
}

// archivedMember returns the header and content pax wrote for member, so the
// assertions read the archive rather than the human-facing listing.
func archivedMember(t *testing.T, archive, member string) (*tar.Header, string) {
	t.Helper()
	f, err := os.Open(archive)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer f.Close()
	tr := tar.NewReader(f)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, ""
		}
		if err != nil {
			t.Fatalf("read archive: %v", err)
		}
		if h.Name != member {
			continue
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read member: %v", err)
		}
		return h, string(body)
	}
}

// A directory operand whose descent reaches a depth that only the resolved
// absolute pathname makes overlong must still be archived in full: the member
// is a legal pathname, so pax may not drop it because it chose to join it to
// the run directory.
func TestSourceDirectoryOperandNearPathMaxIsFullyArchived(t *testing.T) {
	d := t.TempDir()
	operand, member := nearLimitSourceTree(t, d, regularLeaf)
	if _, errOut, code := exec(t, d, "", "-w", "-f", "archive.pax", operand); code != 0 || errOut != "" {
		t.Fatalf("write near-PATH_MAX directory operand = (%d, %q), want (0, \"\")", code, errOut)
	}
	h, body := archivedMember(t, filepath.Join(d, "archive.pax"), member)
	if h == nil {
		t.Fatalf("archive is missing the deep member %q", member)
	}
	if h.Typeflag != tar.TypeReg || body != "deep" {
		t.Fatalf("deep member = (type %q, %q), want a regular file holding \"deep\"", h.Typeflag, body)
	}
}

// A legal -f pathname can become too long only after pax joins it to the
// embedding run directory. GA67 exercises the archive output path, not the
// source operand, so keep this case separate from source traversal tests.
func TestArchiveOutputNearPathMaxUsesRelativeResolution(t *testing.T) {
	d := t.TempDir()
	comps := nearLimitComponents(destinationPathMax - 1 - len("/a"))
	if len(comps) < 2 {
		t.Skip("host cannot construct a near-PATH_MAX archive name")
	}
	member := path.Join(append(comps, "a")...)
	if len(member) != destinationPathMax-1 || len(d)+1+len(member) < destinationPathMax {
		t.Skipf("fixture does not cross the absolute pathname limit: dir=%d member=%d", len(d), len(member))
	}
	root, err := os.OpenRoot(d)
	if err != nil {
		t.Skipf("root-relative operations unavailable: %v", err)
	}
	defer root.Close()
	if err := root.MkdirAll(path.Dir(member), 0o700); err != nil {
		t.Skipf("host cannot create long archive parent: %v", err)
	}
	if err := root.WriteFile(member, nil, 0o600); err != nil {
		t.Skipf("host cannot create long archive name: %v", err)
	}
	if err := os.WriteFile(filepath.Join(d, "source"), []byte("deep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, errOut, code := exec(t, d, "", "-w", "-f", member, "source"); code != 0 || errOut != "" {
		t.Fatalf("write near-PATH_MAX archive = (%d, %q), want (0, empty)", code, errOut)
	}
	f, err := root.Open(member)
	if err != nil {
		t.Fatalf("open archive through root: %v", err)
	}
	defer f.Close()
	h, err := tar.NewReader(f).Next()
	if err != nil || h.Name != "source" {
		t.Fatalf("first archived member = (%v, %v), want source", h, err)
	}
}

// Every required output format must open a legal source pathname without
// making it longer by resolving it against the embedding shell's directory.
// The cpio lane used os.ReadFile on that synthesized absolute spelling even
// after the shared walker had reached the member component by component.
func TestCPIOSourcePathNearPathMaxIsArchived(t *testing.T) {
	d := t.TempDir()
	_, member := nearLimitSourceTree(t, d, regularLeaf)
	if len(member) != destinationPathMax-1 {
		t.Fatalf("fixture pathname length = %d, want %d", len(member), destinationPathMax-1)
	}
	for _, component := range strings.Split(member, "/") {
		if len(component) > nameMax {
			t.Fatalf("fixture component length = %d, exceeds %d", len(component), nameMax)
		}
	}
	if _, errOut, code := exec(t, d, "", "-w", "-x", "cpio", "-f", "archive.cpio", member); code != 0 || errOut != "" {
		t.Fatalf("write cpio near-PATH_MAX source = (%d, %q), want (0, \"\")", code, errOut)
	}
	out, errOut, code := exec(t, d, "", "-f", "archive.cpio")
	if code != 0 || errOut != "" || !strings.Contains(out, member+"\n") {
		t.Fatalf("list cpio near-PATH_MAX source = (%d, %q, contains=%t)",
			code, errOut, strings.Contains(out, member+"\n"))
	}
}

// Physical traversal is the default: a symlink's own value, not its referent,
// must be archived. Reading that value must use the same component-wise source
// resolution as lstat and regular-file content reads.
func TestPhysicalSymlinkNearPathMaxIsArchived(t *testing.T) {
	d := t.TempDir()
	_, member := nearLimitSourceTree(t, d, symlinkLeaf)
	if len(member) != destinationPathMax-1 {
		t.Fatalf("fixture pathname length = %d, want %d", len(member), destinationPathMax-1)
	}

	if _, errOut, code := exec(t, d, "", "-w", "-f", "archive.pax", member); code != 0 || errOut != "" {
		t.Fatalf("write pax near-PATH_MAX symlink = (%d, %q), want (0, \"\")", code, errOut)
	}
	h, _ := archivedMember(t, filepath.Join(d, "archive.pax"), member)
	if h == nil || h.Typeflag != tar.TypeSymlink || h.Linkname != "referent" {
		t.Fatalf("pax symlink = (%v), want link to referent", h)
	}

	if _, errOut, code := exec(t, d, "", "-w", "-x", "cpio", "-f", "archive.cpio", member); code != 0 || errOut != "" {
		t.Fatalf("write cpio near-PATH_MAX symlink = (%d, %q), want (0, \"\")", code, errOut)
	}
	raw, err := os.ReadFile(filepath.Join(d, "archive.cpio"))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := readCPIOEntries(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.name == member {
			if entry.mode&0o170000 != 0o120000 || string(entry.data) != "referent" {
				t.Fatalf("cpio symlink = (mode %#o, %q), want symlink to referent", entry.mode, entry.data)
			}
			return
		}
	}
	t.Fatalf("cpio archive is missing deep symlink %q", member)
}

// -H resolves a symlink named as a command-line operand. The operand's own
// spelling is within {PATH_MAX}; only pax's absolute rewrite is not, so the
// followed stat must succeed and the referent's contents be archived.
func TestFollowedSymlinkOperandNearPathMaxIsArchived(t *testing.T) {
	d := t.TempDir()
	_, member := nearLimitSourceTree(t, d, symlinkLeaf)
	for _, follow := range []string{"-H", "-L"} {
		t.Run(follow, func(t *testing.T) {
			archive := "archive" + follow + ".pax"
			if _, errOut, code := exec(t, d, "", "-w", follow, "-f", archive, member); code != 0 || errOut != "" {
				t.Fatalf("write %s near-PATH_MAX symlink operand = (%d, %q), want (0, \"\")", follow, code, errOut)
			}
			h, body := archivedMember(t, filepath.Join(d, archive), member)
			if h == nil {
				t.Fatalf("%s archive is missing the operand %q", follow, member)
			}
			if h.Typeflag != tar.TypeReg || body != "deep" {
				t.Fatalf("%s member = (type %q, link %q, %q), want the referent's contents",
					follow, h.Typeflag, h.Linkname, body)
			}
		})
	}
}

// -L resolves symlinks encountered below an operand as well. The link here is
// only reached by descent, so it exercises the child branch of the walk.
func TestFollowedSymlinkBelowOperandNearPathMaxIsArchived(t *testing.T) {
	d := t.TempDir()
	operand, member := nearLimitSourceTree(t, d, symlinkLeaf)
	if _, errOut, code := exec(t, d, "", "-w", "-L", "-f", "archive.pax", operand); code != 0 || errOut != "" {
		t.Fatalf("write -L near-PATH_MAX descent = (%d, %q), want (0, \"\")", code, errOut)
	}
	h, body := archivedMember(t, filepath.Join(d, "archive.pax"), member)
	if h == nil {
		t.Fatalf("archive is missing the followed member %q", member)
	}
	if h.Typeflag != tar.TypeReg || body != "deep" {
		t.Fatalf("descended link = (type %q, link %q, %q), want the referent's contents",
			h.Typeflag, h.Linkname, body)
	}
	// The link's sibling proves the enumeration itself survived, not just the
	// one child the descent happened to follow.
	sibling := path.Join(path.Dir(member), "referent")
	if h, _ := archivedMember(t, filepath.Join(d, "archive.pax"), sibling); h == nil {
		t.Fatalf("archive is missing the deep sibling %q", sibling)
	}
}

// A source member longer than the fixed 4096 destination ceiling is still a
// legal thing to archive: the pax extended header carries any length. The
// Linux-only failure came from a sibling ("referent" beside a one-byte link
// name) landing at 4102 bytes; building that length directly reproduces it on
// hosts whose {PATH_MAX} is smaller.
func TestWriteSourceMemberBeyondFixedCeilingIsArchived(t *testing.T) {
	d := t.TempDir()
	comps := nearLimitComponents(4102 - len("/x"))
	member := path.Join(append(append([]string{}, comps...), "x")...)
	root, err := os.OpenRoot(d)
	if err != nil {
		t.Skipf("root-relative directory handles unavailable: %v", err)
	}
	defer root.Close()
	current := root
	for _, c := range comps {
		if err := current.Mkdir(c, 0o700); err != nil {
			t.Skipf("host cannot create a %d-byte fixture component: %v", len(c), err)
		}
		next, err := current.OpenRoot(c)
		if err != nil {
			t.Skipf("host cannot descend into the fixture: %v", err)
		}
		if current != root {
			current.Close()
		}
		current = next
	}
	err = regularLeaf(current)
	current.Close()
	if err != nil {
		t.Skipf("host cannot create the fixture leaf: %v", err)
	}
	if len(member) <= 4096 {
		t.Fatalf("fixture member length = %d, want > 4096", len(member))
	}
	if _, errOut, code := exec(t, d, "", "-w", "-f", "archive.pax", comps[0]); code != 0 || errOut != "" {
		t.Fatalf("write over-4096 source member = (%d, %q), want (0, \"\")", code, errOut)
	}
	h, body := archivedMember(t, filepath.Join(d, "archive.pax"), member)
	if h == nil || body != "deep" {
		t.Fatalf("over-4096 member = (%v, %q), want a regular file holding \"deep\"", h, body)
	}
}

func TestInvalidArchiveNameIgnoresTotalLength(t *testing.T) {
	long := strings.Repeat(strings.Repeat("p", 200)+"/", 25) + "x"
	if invalidPAXArchiveName(long) {
		t.Fatalf("a %d-byte name with short components is a valid archive name", len(long))
	}
	if !invalidPAXLocalDestinationName(strings.Repeat(strings.Repeat("p", 200)+"/", 21) + "x") {
		t.Fatal("destination check must still reject names beyond 4096")
	}
	for _, bad := range []string{"", "a\x00b", strings.Repeat("p", 256)} {
		if !invalidPAXArchiveName(bad) {
			t.Fatalf("invalidPAXArchiveName(%q) = false, want true", bad)
		}
	}
}
