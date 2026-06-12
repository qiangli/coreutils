// Portions adapted from https://github.com/u-root/u-root cmds/core/du/du.go (BSD-3-Clause).
// Changes: rewired to the tool framework; GNU flag set -a -b -c -d -h -s
// with GNU defaults (1024-byte block output, hard-link deduplication,
// post-order per-directory reporting); Windows fallback to apparent size.

// Package ducmd implements du(1) per the GNU coreutils manual for the
// flag subset -a -b -c -d/--max-depth -h -s. As GNU documents, sizes
// are disk usage reported in 1024-byte units by default (rounded up);
// -b switches to exact apparent sizes in bytes; -h prints
// human-readable sizes. Hard-linked files are counted once per
// invocation. Traversal does not follow symlinks.
//
// Platform note: disk usage comes from st_blocks on unix; Windows has
// no block count, so usage falls back to the apparent size there.
package ducmd

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "du",
	Synopsis: "Summarize device usage of the set of FILEs, recursively for directories.",
	Usage:    "du [OPTION]... [FILE]...",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

type duRun struct {
	rc       *tool.RunContext
	all      bool
	apparent bool // -b
	human    bool
	maxDepth int
	exit     int
	seen     map[devIno]bool
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	all := fs.BoolP("all", "a", false, "write counts for all files, not just directories")
	apparent := fs.BoolP("bytes", "b", false, "equivalent to '--apparent-size --block-size=1'")
	total := fs.BoolP("total", "c", false, "produce a grand total")
	maxDepth := fs.IntP("max-depth", "d", -1, "print the total for a directory (or file, with --all) only if it is N or fewer levels below the command line argument")
	human := fs.BoolP("human-readable", "h", false, "print sizes in human readable format (e.g., 1K 234M 2G)")
	summarize := fs.BoolP("summarize", "s", false, "display only a total for each argument")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	if *summarize && *all {
		return tool.UsageError(rc, cmd, "cannot both summarize and show all entries")
	}
	if *summarize && fs.Changed("max-depth") {
		return tool.UsageError(rc, cmd, "warning: summarizing conflicts with --max-depth=%d", *maxDepth)
	}
	if fs.Changed("max-depth") && *maxDepth < 0 {
		return tool.UsageError(rc, cmd, "invalid maximum depth '%d'", *maxDepth)
	}

	depth := math.MaxInt
	switch {
	case *summarize:
		depth = 0
	case fs.Changed("max-depth"):
		depth = *maxDepth
	}

	d := &duRun{
		rc:       rc,
		all:      *all,
		apparent: *apparent,
		human:    *human,
		maxDepth: depth,
		seen:     map[devIno]bool{},
	}

	if len(operands) == 0 {
		operands = []string{"."}
	}
	var grand int64
	for _, op := range operands {
		n, ok := d.walk(op, rc.Path(op), 0)
		if ok {
			grand += n
		}
	}
	if *total {
		d.print(grand, "total")
	}
	return d.exit
}

func (d *duRun) walk(display, full string, depth int) (int64, bool) {
	fi, err := os.Lstat(full)
	if err != nil {
		fmt.Fprintf(d.rc.Err, "du: cannot access '%s': %s\n", display, errMsg(err))
		d.exit = 1
		return 0, false
	}
	if fi.IsDir() {
		total := d.usage(fi)
		ents, rerr := os.ReadDir(full)
		if rerr != nil {
			fmt.Fprintf(d.rc.Err, "du: cannot read directory '%s': %s\n", display, errMsg(rerr))
			d.exit = 1
		}
		for _, de := range ents {
			n, _ := d.walk(joinDisplay(display, de.Name()), filepath.Join(full, de.Name()), depth+1)
			total += n
		}
		if depth <= d.maxDepth {
			d.print(total, display)
		}
		return total, true
	}
	if d.skipHardlink(fi) {
		return 0, true
	}
	total := d.usage(fi)
	// File operands are always reported; files inside the tree only
	// with --all (and within the depth limit).
	if depth == 0 || (d.all && depth <= d.maxDepth) {
		d.print(total, display)
	}
	return total, true
}

func (d *duRun) print(n int64, path string) {
	fmt.Fprintf(d.rc.Out, "%s\t%s\n", d.fmtSize(n), path)
}

func (d *duRun) fmtSize(n int64) string {
	switch {
	case d.human:
		return humanSize(uint64(n))
	case d.apparent:
		return strconv.FormatInt(n, 10)
	default:
		return strconv.FormatInt((n+1023)/1024, 10)
	}
}

func joinDisplay(dir, name string) string {
	if strings.HasSuffix(dir, "/") || strings.HasSuffix(dir, string(os.PathSeparator)) {
		return dir + name
	}
	return dir + "/" + name
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
