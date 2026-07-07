// Package dfcmd implements df(1) per the GNU coreutils manual for the
// commonly used reporting flags. The default (and -k) output is in
// 1024-byte blocks, rounded up; -h/-H print human-readable sizes. With
// FILE arguments, only the file system containing each file is shown.
//
// Mounted file systems are discovered by platform probes behind build
// tags: /proc/mounts + statfs on Linux, getfsstat on macOS, and
// GetLogicalDrives + GetDiskFreeSpaceEx (fixed drives) on Windows.
// Pseudo file systems with zero blocks are omitted, as GNU does by
// default.
package dfcmd

import (
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
	fstype string
	total  uint64
	used   uint64
	avail  uint64
	files  uint64
	ifree  uint64
}

func run(rc *tool.RunContext, args []string) int {
	args = normalizeBlockSizeArgs(args)
	// -k has no GNU long form; -V is a uutils alias for --version.
	rest, seenShort := extractShort(args, "kV")
	if seenShort['V'] {
		rest = append([]string{"--version"}, rest...)
	}
	fs := tool.NewFlags(cmd.Name)
	versionAlias := fs.BoolP("version-alias", "V", false, "output version information and exit")
	fs.BoolP("kibibytes", "k", false, "like --block-size=1K")
	human := fs.BoolP("human-readable", "h", false, "print sizes in powers of 1024 (e.g., 1023M)")
	blockSize := fs.StringP("block-size", "B", "", "scale sizes by SIZE before printing")
	fs.BoolP("megabytes", "M", false, "like --block-size=1M")
	si := fs.BoolP("si", "H", false, "print sizes in powers of 1000 (e.g., 1.1G)")
	portable := fs.BoolP("portability", "P", false, "use the POSIX output format")
	printType := fs.BoolP("print-type", "T", false, "print file system type")
	all := fs.BoolP("all", "a", false, "include pseudo, duplicate, and inaccessible file systems")
	inodes := fs.BoolP("inodes", "i", false, "list inode information instead of block usage")
	local := fs.BoolP("local", "l", false, "limit listing to local file systems")
	noSync := fs.Bool("no-sync", false, "do not invoke sync before getting usage info (default)")
	doSync := fs.Bool("sync", false, "invoke sync before getting usage info")
	var includeTypes, excludeTypes []string
	fs.StringArrayVarP(&includeTypes, "type", "t", nil, "limit listing to file systems of type TYPE")
	fs.StringArrayVarP(&excludeTypes, "exclude-type", "x", nil, "limit listing to file systems not of type TYPE")
	output := fs.String("output", "", "use the output format defined by FIELD_LIST, or all fields if FIELD_LIST is omitted")
	fs.Lookup("output").NoOptDefVal = defaultOutputFields
	total := fs.Bool("total", false, "produce a grand total")
	operands, code := tool.Parse(rc, cmd, fs, rest)
	if code >= 0 {
		return code
	}
	if *versionAlias {
		fmt.Fprintf(rc.Out, "%s (qiangli/coreutils) %s\n", cmd.Name, tool.Version)
		return 0
	}
	_ = noSync // accepted for uutils/GNU compatibility; no-sync is the default.
	if *doSync {
		syncFilesystems()
	}
	scale := scaleMode{blockSize: 1024, header: "1K-blocks"}
	if *si {
		scale = scaleMode{human: true, base: 1000, header: "Size"}
	}
	if *human {
		scale = scaleMode{human: true, base: 1024, header: "Size"}
	}
	if *blockSize != "" {
		size, err := parseBlockSize(*blockSize)
		if err != nil {
			return tool.UsageError(rc, cmd, "invalid --block-size argument %q", *blockSize)
		}
		scale = scaleMode{blockSize: size, header: blockHeader(size)}
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
			if !*all && m.total == 0 {
				continue // pseudo file systems (proc, sysfs, ...)
			}
			key := m.device + "\x00" + m.point
			if !*all && seen[key] {
				continue
			}
			seen[key] = true
			rows = append(rows, m)
		}
	}
	rows = filterRows(rows, includeTypes, excludeTypes, *local)
	if *total {
		rows = append(rows, totalRow(rows))
	}
	if *output != "" {
		fields := parseOutputFields(*output)
		if err := validateOutputFields(fields); err != nil {
			return tool.UsageError(rc, cmd, "%s", err)
		}
		printOutputTable(rc.Out, rows, fields, scale)
	} else {
		printTable(rc.Out, rows, tableOptions{
			scale:     scale,
			inodes:    *inodes,
			printType: *printType,
			portable:  *portable,
		})
	}
	return exit
}

type scaleMode struct {
	human     bool
	base      uint64
	blockSize uint64
	header    string
}

type tableOptions struct {
	scale     scaleMode
	inodes    bool
	printType bool
	portable  bool
}

func printTable(w io.Writer, rows []mountEntry, opt tableOptions) {
	sizeHdr, availHdr := opt.scale.header, "Available"
	if opt.scale.human {
		availHdr = "Avail"
	}
	if opt.portable && !opt.inodes && !opt.scale.human && opt.scale.blockSize == 1024 {
		sizeHdr = "1024-blocks"
	}
	if opt.inodes {
		sizeHdr, availHdr = "Inodes", "IFree"
	}
	type line struct{ fsys, typ, size, used, avail, pct, mnt string }
	lines := make([]line, len(rows))
	wf, ws, wu, wa, wp := len("Filesystem"), len(sizeHdr), len("Used"), len(availHdr), len("Use%")
	wt := len("Type")
	for i, m := range rows {
		l := line{
			fsys:  m.device,
			typ:   m.fstype,
			size:  fmtValue(m.total, opt.scale),
			used:  fmtValue(m.used, opt.scale),
			avail: fmtValue(m.avail, opt.scale),
			pct:   usePct(m.used, m.avail),
			mnt:   m.point,
		}
		if opt.inodes {
			iused := inodeUsed(m)
			l.size = strconv.FormatUint(m.files, 10)
			l.used = strconv.FormatUint(iused, 10)
			l.avail = strconv.FormatUint(m.ifree, 10)
			l.pct = usePct(iused, m.ifree)
		}
		wf, ws, wu = max(wf, len(l.fsys)), max(ws, len(l.size)), max(wu, len(l.used))
		wa, wp = max(wa, len(l.avail)), max(wp, len(l.pct))
		wt = max(wt, len(l.typ))
		lines[i] = l
	}
	pctHdr := "Use%"
	if opt.inodes {
		pctHdr = "IUse%"
	}
	fmt.Fprintf(w, "%-*s", wf, "Filesystem")
	if opt.printType {
		fmt.Fprintf(w, " %-*s", wt, "Type")
	}
	fmt.Fprintf(w, " %*s %*s %*s %*s %s\n",
		ws, sizeHdr, wu, usedHeader(opt.inodes), wa, availHdr, wp, pctHdr, "Mounted on")
	for _, l := range lines {
		fmt.Fprintf(w, "%-*s", wf, l.fsys)
		if opt.printType {
			fmt.Fprintf(w, " %-*s", wt, l.typ)
		}
		fmt.Fprintf(w, " %*s %*s %*s %*s %s\n",
			ws, l.size, wu, l.used, wa, l.avail, wp, l.pct, l.mnt)
	}
}

func usedHeader(inodes bool) string {
	if inodes {
		return "IUsed"
	}
	return "Used"
}

func fmtValue(b uint64, scale scaleMode) string {
	if scale.human {
		return humanSize(b, scale.base)
	}
	return strconv.FormatUint(divCeil(b, scale.blockSize), 10)
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

func inodeUsed(m mountEntry) uint64 {
	if m.files < m.ifree {
		return 0
	}
	return m.files - m.ifree
}

func divCeil(n, d uint64) uint64 {
	if d == 0 {
		return 0
	}
	return (n + d - 1) / d
}

func normalizeBlockSizeArgs(args []string) []string {
	var out []string
	for i, a := range args {
		if a == "--" {
			out = append(out, args[i:]...)
			break
		}
		if len(a) > 2 && strings.HasPrefix(a, "-B") && !strings.HasPrefix(a, "--") {
			out = append(out, "--block-size="+a[2:])
			continue
		}
		if a == "-M" {
			out = append(out, "--block-size=1M")
			continue
		}
		if len(a) > 2 && a[0] == '-' && a[1] != '-' && strings.Contains(a, "M") {
			out = append(out, "--block-size=1M")
			kept := strings.ReplaceAll(a, "M", "")
			if kept != "-" {
				out = append(out, kept)
			}
			continue
		}
		out = append(out, a)
	}
	return out
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
	return tool.SysErrString(err)
}

// humanSize renders n bytes in GNU --human-readable form: powers of
// 1024, at most one decimal digit, always rounding up (1025 -> 1.1K).
func humanSize(n, base uint64) string {
	if base == 0 {
		base = 1024
	}
	if n < base {
		return strconv.FormatUint(n, 10)
	}
	const units = "KMGTPE"
	div := base
	idx := 0
	for n/div >= base && idx < len(units)-1 {
		div *= base
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
	if v >= base && idx < len(units)-1 {
		return fmt.Sprintf("1.0%c", units[idx+1])
	}
	return fmt.Sprintf("%d%c", v, units[idx])
}

func parseBlockSize(s string) (uint64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	upper := strings.ToUpper(strings.TrimSpace(s))
	if upper == "HUMAN-READABLE" {
		return 1024, nil
	}
	mult := uint64(1)
	switch {
	case strings.HasSuffix(upper, "KIB"):
		mult, upper = 1024, strings.TrimSuffix(upper, "KIB")
	case strings.HasSuffix(upper, "MIB"):
		mult, upper = 1024*1024, strings.TrimSuffix(upper, "MIB")
	case strings.HasSuffix(upper, "GIB"):
		mult, upper = 1024*1024*1024, strings.TrimSuffix(upper, "GIB")
	case strings.HasSuffix(upper, "KB"):
		mult, upper = 1000, strings.TrimSuffix(upper, "KB")
	case strings.HasSuffix(upper, "MB"):
		mult, upper = 1000*1000, strings.TrimSuffix(upper, "MB")
	case strings.HasSuffix(upper, "GB"):
		mult, upper = 1000*1000*1000, strings.TrimSuffix(upper, "GB")
	case strings.HasSuffix(upper, "K"):
		mult, upper = 1024, strings.TrimSuffix(upper, "K")
	case strings.HasSuffix(upper, "M"):
		mult, upper = 1024*1024, strings.TrimSuffix(upper, "M")
	case strings.HasSuffix(upper, "G"):
		mult, upper = 1024*1024*1024, strings.TrimSuffix(upper, "G")
	}
	if upper == "" {
		upper = "1"
	}
	n, err := strconv.ParseUint(upper, 10, 64)
	if err != nil || n == 0 {
		return 0, fmt.Errorf("invalid size")
	}
	if n > ^uint64(0)/mult {
		return 0, fmt.Errorf("size overflow")
	}
	return n * mult, nil
}

func blockHeader(size uint64) string {
	if size == 1024 {
		return "1K-blocks"
	}
	return strconv.FormatUint(size, 10) + "B-blocks"
}

func filterRows(rows []mountEntry, include, exclude []string, localOnly bool) []mountEntry {
	includeSet, excludeSet := stringSet(include), stringSet(exclude)
	out := rows[:0]
	for _, m := range rows {
		typ := strings.ToLower(m.fstype)
		if len(includeSet) > 0 && !includeSet[typ] {
			continue
		}
		if excludeSet[typ] {
			continue
		}
		if localOnly && isRemoteType(typ) {
			continue
		}
		out = append(out, m)
	}
	return out
}

func stringSet(vals []string) map[string]bool {
	if len(vals) == 0 {
		return nil
	}
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		m[strings.ToLower(v)] = true
	}
	return m
}

func isRemoteType(t string) bool {
	switch t {
	case "9p", "afs", "cifs", "ncpfs", "nfs", "nfs4", "smbfs", "sshfs":
		return true
	default:
		return false
	}
}

func totalRow(rows []mountEntry) mountEntry {
	t := mountEntry{device: "total", point: "-", fstype: "-"}
	for _, m := range rows {
		t.total = satAdd(t.total, m.total)
		t.used = satAdd(t.used, m.used)
		t.avail = satAdd(t.avail, m.avail)
		t.files = satAdd(t.files, m.files)
		t.ifree = satAdd(t.ifree, m.ifree)
	}
	return t
}

func satAdd(a, b uint64) uint64 {
	if a > ^uint64(0)-b {
		return ^uint64(0)
	}
	return a + b
}

const defaultOutputFields = "source,fstype,itotal,iused,iavail,ipcent,size,used,avail,pcent,file,target"

func parseOutputFields(s string) []string {
	parts := strings.Split(s, ",")
	fields := fieldsFromParts(parts)
	if len(fields) == 0 {
		return strings.Split(defaultOutputFields, ",")
	}
	return fields
}

func fieldsFromParts(parts []string) []string {
	var fields []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			fields = append(fields, p)
		}
	}
	return fields
}

func validateOutputFields(fields []string) error {
	for _, f := range fields {
		if _, ok := outputHeaders[f]; !ok {
			return fmt.Errorf("unknown field %q", f)
		}
	}
	return nil
}

var outputHeaders = map[string]string{
	"source": "Filesystem",
	"fstype": "Type",
	"itotal": "Inodes",
	"iused":  "IUsed",
	"iavail": "IFree",
	"ipcent": "IUse%",
	"size":   "1K-blocks",
	"used":   "Used",
	"avail":  "Avail",
	"pcent":  "Use%",
	"file":   "File",
	"target": "Mounted on",
}

func printOutputTable(w io.Writer, rows []mountEntry, fields []string, scale scaleMode) {
	widths := make([]int, len(fields))
	values := make([][]string, len(rows))
	for i, f := range fields {
		h := outputHeaders[f]
		if f == "size" {
			h = scale.header
		}
		widths[i] = len(h)
	}
	for r, m := range rows {
		values[r] = make([]string, len(fields))
		for c, f := range fields {
			v := outputValue(m, f, scale)
			values[r][c] = v
			widths[c] = max(widths[c], len(v))
		}
	}
	for i, f := range fields {
		h := outputHeaders[f]
		if f == "size" {
			h = scale.header
		}
		if i > 0 {
			fmt.Fprint(w, " ")
		}
		fmt.Fprintf(w, "%*s", widths[i], h)
	}
	fmt.Fprintln(w)
	for _, row := range values {
		for i, v := range row {
			if i > 0 {
				fmt.Fprint(w, " ")
			}
			fmt.Fprintf(w, "%*s", widths[i], v)
		}
		fmt.Fprintln(w)
	}
}

func outputValue(m mountEntry, field string, scale scaleMode) string {
	iused := inodeUsed(m)
	switch field {
	case "source":
		return m.device
	case "fstype":
		return m.fstype
	case "itotal":
		return strconv.FormatUint(m.files, 10)
	case "iused":
		return strconv.FormatUint(iused, 10)
	case "iavail":
		return strconv.FormatUint(m.ifree, 10)
	case "ipcent":
		return usePct(iused, m.ifree)
	case "size":
		return fmtValue(m.total, scale)
	case "used":
		return fmtValue(m.used, scale)
	case "avail":
		return fmtValue(m.avail, scale)
	case "pcent":
		return usePct(m.used, m.avail)
	case "file", "target":
		return m.point
	default:
		return ""
	}
}
