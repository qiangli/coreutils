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
