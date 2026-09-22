package chmodcmd

// Story #682 (S245.5f) item 3: the setuid and setgid bits do not exist on a
// host with no POSIX mode bits. chmod must still ACCEPT them and exit 0
// rather than refuse — bash's test.tests sets them on a scratch file on its
// way to something else, and a hard error derails the whole fixture over a
// bit the platform cannot hold. This is the same call chmod's read-only
// projection already makes for r/x/t; the assertion here is that s rides
// along with them.

import (
	"os"
	"testing"
)

func TestSetIDBitsAreAcceptedAndProjected(t *testing.T) {
	cases := []struct {
		mode string
		old  uint32
		bits uint32      // the POSIX bits chmod computes
		want os.FileMode // what a read-only host is asked to apply
	}{
		{"g+s", 0o644, 0o2644, 0o666},
		{"u+s", 0o644, 0o4644, 0o666},
		{"ug+s", 0o644, 0o6644, 0o666},
		{"2755", 0o644, 0o2755, 0o666},
		{"4755", 0o644, 0o4755, 0o666},
		// The s bits ride along with a read-only result too: they are
		// accepted, not refused, and they change nothing that exists.
		{"g+s,a-w", 0o644, 0o2444, 0o444},
		{"g-s", 0o2644, 0o644, 0o666},
	}
	for _, c := range cases {
		change, err := parseMode(c.mode)
		if err != nil {
			t.Errorf("parseMode(%q): %v — the set-id bits must parse on every platform", c.mode, err)
			continue
		}
		bits := change.apply(c.old, false, 0o022)
		if bits != c.bits {
			t.Errorf("chmod %s on %04o: bits %04o, want %04o", c.mode, c.old, bits, c.bits)
			continue
		}
		if got := readOnlyProjection(bits); got != c.want {
			t.Errorf("chmod %s on %04o: host mode %04o, want %04o", c.mode, c.old, got, c.want)
		}
	}
}

// The projection is total over the set-id bits: no combination of them can
// make it refuse or produce a mode outside the two it has (0666 / 0444).
func TestSetIDBitsNeverEscapeTheProjection(t *testing.T) {
	for bits := uint32(0); bits <= 0o7777; bits++ {
		if got := readOnlyProjection(bits); got != 0o666 && got != 0o444 {
			t.Fatalf("readOnlyProjection(%04o) = %04o; want 0666 or 0444", bits, got)
		}
	}
}
