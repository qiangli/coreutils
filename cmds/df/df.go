// Package dfcmd implements df(1) per the GNU coreutils manual for the
// flag subset -h -k. The default (and -k) output is in 1024-byte
// blocks, rounded up; -h prints human-readable sizes. With FILE
// arguments, only the file system containing each file is shown.
//
// Mounted file systems are discovered by platform probes behind build
// tags: /proc/mounts + statfs on Linux, getfsstat on macOS, and
// GetLogicalDrives + GetDiskFreeSpaceEx (fixed drives) on Windows.
// Pseudo file systems with zero blocks are omitted, as GNU does by
// default.
package dfcmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "df",
	Synopsis: "Show information about the file system on which each FILE resides, or all file systems by default.",
	Usage:    "df [OPTION]... [FILE]...",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

// mountEntry is one mounted file system, sizes in bytes. Filled by
// the per-platform listMounts (mounts_linux.go / mounts_darwin.go /
// mounts_windows.go).
type mountEntry struct {
	device string
	point  string
	total  uint64
	used   uint64
	avail  uint64
}

func run(rc *tool.RunContext, args []string) int {
	// -k has no GNU long form: pre-parse it out of the clusters. It
	// selects the 1024-byte block size, which is already the default
	// here (no POSIXLY_CORRECT 512-byte mode in this userland).
	rest, _ := extractShort(args, "k")
	fs := tool.NewFlags(cmd.Name)
	human := fs.BoolP("human-readable", "h", false, "print sizes in powers of 1024 (e.g., 1023M)")
	operands, code := tool.Parse(rc, cmd, fs, rest)
	if code >= 0 {
		return code
	}

	mounts, err := listMounts()
	if err != nil {
		fmt.Fprintf(rc.Err, "df: %s\n", err)
		return 1
	}

	exit := 0
	var rows []mountEntry
	if len(operands) > 0 {
		for _, op := range operands {
			full := rc.Path(op)
			if _, serr := os.Stat(full); serr != nil {
				fmt.Fprintf(rc.Err, "df: %s: %s\n", op, errMsg(serr))
				exit = 1
				continue
			}
			idx, ok := mountForFile(full, mounts)
			if !ok {
				fmt.Fprintf(rc.Err, "df: cannot find mount point for '%s'\n", op)
				exit = 1
				continue
			}
			rows = append(rows, mounts[idx])
		}
		if len(rows) == 0 {
			fmt.Fprintln(rc.Err, "df: no file systems processed")
			return 1
		}
	} else {
		seen := map[string]bool{}
		for _, m := range mounts {
			if m.total == 0 {
				continue // pseudo file systems (proc, sysfs, ...)
			}
			key := m.device + "\x00" + m.point
			if seen[key] {
				continue
			}
			seen[key] = true
			rows = append(rows, m)
		}
	}
	printTable(rc.Out, rows, *human)
	return exit
}

func printTable(w io.Writer, rows []mountEntry, human bool) {
	sizeHdr, availHdr := "1K-blocks", "Available"
	if human {
		sizeHdr, availHdr = "Size", "Avail"
	}
	type line struct{ fsys, size, used, avail, pct, mnt string }
	lines := make([]line, len(rows))
	wf, ws, wu, wa, wp := len("Filesystem"), len(sizeHdr), len("Used"), len(availHdr), len("Use%")
	for i, m := range rows {
		l := line{
			fsys:  m.device,
			size:  fmtBlocks(m.total, human),
			used:  fmtBlocks(m.used, human),
			avail: fmtBlocks(m.avail, human),
			pct:   usePct(m.used, m.avail),
			mnt:   m.point,
		}
		wf, ws, wu = max(wf, len(l.fsys)), max(ws, len(l.size)), max(wu, len(l.used))
		wa, wp = max(wa, len(l.avail)), max(wp, len(l.pct))
		lines[i] = l
	}
	fmt.Fprintf(w, "%-*s %*s %*s %*s %*s %s\n",
		wf, "Filesystem", ws, sizeHdr, wu, "Used", wa, availHdr, wp, "Use%", "Mounted on")
	for _, l := range lines {
		fmt.Fprintf(w, "%-*s %*s %*s %*s %*s %s\n",
			wf, l.fsys, ws, l.size, wu, l.used, wa, l.avail, wp, l.pct, l.mnt)
	}
}

func fmtBlocks(b uint64, human bool) string {
	if human {
		return humanSize(b)
	}
	return strconv.FormatUint((b+1023)/1024, 10)
}

// usePct is GNU's Use%: used/(used+avail) rounded up; "-" when both
// are zero.
func usePct(used, avail uint64) string {
	if used+avail == 0 {
		return "-"
	}
	pct := (used*100 + used + avail - 1) / (used + avail)
	return strconv.FormatUint(pct, 10) + "%"
}

// extractShort removes the given single-letter flags (which have no
// GNU long form) from short-flag clusters, returning the remaining
// args and the set of letters seen. Scanning stops at "--".
func extractShort(args []string, chars string) ([]string, map[byte]bool) {
	found := map[byte]bool{}
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' {
			kept := []byte{'-'}
			for j := 1; j < len(a); j++ {
				if strings.IndexByte(chars, a[j]) >= 0 {
					found[a[j]] = true
				} else {
					kept = append(kept, a[j])
				}
			}
			if len(kept) > 1 {
				rest = append(rest, string(kept))
			}
			continue
		}
		rest = append(rest, a)
	}
	return rest, found
}

func errMsg(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

// humanSize renders n bytes in GNU --human-readable form: powers of
// 1024, at most one decimal digit, always rounding up (1025 -> 1.1K).
func humanSize(n uint64) string {
	if n < 1024 {
		return strconv.FormatUint(n, 10)
	}
	const units = "KMGTPE"
	div := uint64(1024)
	idx := 0
	for n/div >= 1024 && idx < len(units)-1 {
		div *= 1024
		idx++
	}
	whole, rem := n/div, n%div
	if whole < 10 {
		tenths := whole*10 + (rem*10+div-1)/div
		if tenths < 100 {
			return fmt.Sprintf("%d.%d%c", tenths/10, tenths%10, units[idx])
		}
		return fmt.Sprintf("10%c", units[idx])
	}
	v := whole
	if rem > 0 {
		v++
	}
	if v >= 1024 && idx < len(units)-1 {
		return fmt.Sprintf("1.0%c", units[idx+1])
	}
	return fmt.Sprintf("%d%c", v, units[idx])
}
