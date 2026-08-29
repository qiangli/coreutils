// Package dfcmd implements the POSIX.1 Issue 7 (2016 Edition, XSI) df
// interface with GNU coreutils extensions where they do not conflict.
// Conformance evidence remains unverified until the full certification suite
// covers platform probes and locale behavior.
//
//   - Space is reported in units of 512-byte blocks by default; -k
//     selects 1024-byte units (POSIX XSI defaults, unconditional — not
//     gated on POSIXLY_CORRECT).
//   - -P produces the POSIX portable format: header "Filesystem
//     512-blocks Used Available Capacity Mounted on" (1024-blocks with
//     -k), one line per file system, percentage rounded up.
//   - -t is the XSI no-argument option requiring total allocated-space
//     figures in each filesystem record. The default implementation-defined
//     table already includes that field, so -t is idempotent; it never adds a
//     synthetic record. GNU --total remains a separate grand-total extension.
//     GNU type filtering is available only as --type=TYPE.
//   - The default and -t tables include the XSI-required free-file-slot field.
//     LC_MESSAGES catalogs are not yet provided; output uses the repository's
//     documented deterministic C/POSIX message vocabulary.
//
// -h/-H print human-readable sizes (GNU extension). With FILE
// arguments, only the file system containing each file is shown.
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
	"math/big"
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

type stickyWriter struct {
	writer io.Writer
	err    error
}

func (w *stickyWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.writer.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.err = err
	}
	return n, err
}

// mountEntry is one mounted file system, sizes in bytes. Filled by
// the per-platform listMounts (mounts_linux.go / mounts_darwin.go /
// mounts_windows.go).
type mountEntry struct {
	device         string
	point          string
	fstype         string
	total          uint64
	used           uint64
	avail          spaceAmount
	files          uint64
	ifree          uint64
	fileSlotsKnown bool
}

func run(rc *tool.RunContext, args []string) int {
	out := &stickyWriter{writer: rc.Out}
	child := *rc
	child.Out = out
	code := runCore(&child, args)
	if out.err != nil {
		fmt.Fprintf(rc.Err, "df: write error: %v\n", out.err)
		return 1
	}
	return code
}

func runCore(rc *tool.RunContext, args []string) int {
	posix := envPresent(rc.Env, "POSIXLY_CORRECT")
	args = normalizeBlockSizeArgs(args, posix)
	// -k has no GNU long form; -V is a uutils alias for --version.
	rest, seenShort := extractShort(args, "ktV", posix)
	if seenShort['V'] {
		rest = append([]string{"--version"}, rest...)
	}
	fs := tool.NewFlags(cmd.Name)
	versionAlias := fs.BoolP("version-alias", "V", false, "output version information and exit")
	kibi := fs.BoolP("kibibytes", "k", false, "use 1024-byte units instead of the default 512-byte units (POSIX -k)")
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
	// POSIX XSI -t is the no-argument totals option; GNU's -t TYPE
	// filter therefore keeps only its long spelling --type.
	fs.StringArrayVar(&includeTypes, "type", nil, "limit listing to file systems of type TYPE")
	fs.StringArrayVarP(&excludeTypes, "exclude-type", "x", nil, "limit listing to file systems not of type TYPE")
	output := fs.String("output", "", "use the output format defined by FIELD_LIST, or all fields if FIELD_LIST is omitted")
	fs.Lookup("output").NoOptDefVal = defaultOutputFields
	xsiTotal := fs.BoolP("xsi-total-space", "t", false, "include total allocated-space in each record (POSIX XSI)")
	gnuTotal := fs.Bool("total", false, "produce a grand total (GNU extension)")
	if posix {
		fs.SetInterspersed(false)
	}
	operands, code := tool.Parse(rc, cmd, fs, rest)
	if code >= 0 {
		return code
	}
	if *versionAlias {
		fmt.Fprintf(rc.Out, "%s (qiangli/coreutils) %s\n", cmd.Name, tool.Version)
		return 0
	}
	if *portable && (seenShort['t'] || *xsiTotal) {
		return tool.UsageError(rc, cmd, "options -P and -t are mutually exclusive")
	}
	_ = noSync // accepted for uutils/GNU compatibility; no-sync is the default.
	if *doSync {
		syncFilesystems()
	}
	// POSIX XSI default: 512-byte units; -k selects 1024-byte units.
	scale := scaleMode{blockSize: 512, header: "512-blocks"}
	if *kibi || seenShort['k'] {
		scale = scaleMode{blockSize: 1024, header: "1024-blocks"}
	}
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
	// The implementation-defined default table already contains m.total in
	// every row, which satisfies XSI -t. Only GNU --total appends a synthetic
	// aggregate row; POSIX -t never does.
	if *gnuTotal {
		rows = append(rows, totalRow(rows))
	}
	if *output != "" {
		fields := parseOutputFields(*output)
		if err := validateOutputFields(fields); err != nil {
			return tool.UsageError(rc, cmd, "%s", err)
		}
		printOutputTable(rc.Out, rows, fields, scale)
	} else {
		freeSlots := !*portable && !*human && !*si && *blockSize == "" &&
			!*printType && !*all && !*inodes && !*local && !*noSync &&
			!*doSync && len(includeTypes) == 0 && len(excludeTypes) == 0 &&
			!*gnuTotal
		if freeSlots {
			for _, row := range rows {
				if !row.fileSlotsKnown {
					fmt.Fprintf(rc.Err, "df: free file-slot counts are unavailable for %s\n", row.point)
					return 1
				}
			}
		}
		printTable(rc.Out, rows, tableOptions{
			scale:     scale,
			inodes:    *inodes,
			printType: *printType,
			portable:  *portable,
			freeSlots: freeSlots,
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
	freeSlots bool
}

func printTable(w io.Writer, rows []mountEntry, opt tableOptions) {
	if opt.portable && !opt.inodes && !opt.printType && !opt.scale.human {
		sizeHdr := opt.scale.header
		switch opt.scale.blockSize {
		case 512:
			sizeHdr = "512-blocks"
		case 1024:
			sizeHdr = "1024-blocks"
		}
		fmt.Fprintf(w, "Filesystem %s Used Available Capacity Mounted on\n", sizeHdr)
		for _, m := range rows {
			fmt.Fprintf(w, "%s %s %s %s %s %s\n",
				m.device,
				fmtValue(m.total, opt.scale),
				fmtValue(m.used, opt.scale),
				fmtSpaceValue(m.avail, opt.scale),
				usePct(m.used, m.avail),
				m.point)
		}
		return
	}

	sizeHdr, availHdr := opt.scale.header, "Available"
	if opt.scale.human {
		availHdr = "Avail"
	}
	if opt.portable && !opt.inodes && !opt.scale.human {
		// POSIX -P headers: "512-blocks" without -k, "1024-blocks"
		// with -k (also normalizes an explicit -B512 / -B1K).
		switch opt.scale.blockSize {
		case 512:
			sizeHdr = "512-blocks"
		case 1024:
			sizeHdr = "1024-blocks"
		}
	}
	if opt.inodes {
		sizeHdr, availHdr = "Inodes", "IFree"
	}
	pctHdr := "Use%"
	if opt.inodes {
		pctHdr = "IUse%"
	} else if opt.portable {
		pctHdr = "Capacity" // POSIX -P header field
	}
	type line struct{ fsys, typ, size, used, avail, pct, ifree, mnt string }
	lines := make([]line, len(rows))
	wf, ws, wu, wa, wp := len("Filesystem"), len(sizeHdr), len("Used"), len(availHdr), len(pctHdr)
	wt, wi := len("Type"), len("IFree")
	for i, m := range rows {
		l := line{
			fsys:  m.device,
			typ:   m.fstype,
			size:  fmtValue(m.total, opt.scale),
			used:  fmtValue(m.used, opt.scale),
			avail: fmtSpaceValue(m.avail, opt.scale),
			pct:   usePct(m.used, m.avail),
			ifree: strconv.FormatUint(m.ifree, 10),
			mnt:   m.point,
		}
		if opt.inodes {
			iused := inodeUsed(m)
			l.size = strconv.FormatUint(m.files, 10)
			l.used = strconv.FormatUint(iused, 10)
			l.avail = strconv.FormatUint(m.ifree, 10)
			l.pct = usePct(iused, positiveSpace(m.ifree))
		}
		wf, ws, wu = max(wf, len(l.fsys)), max(ws, len(l.size)), max(wu, len(l.used))
		wa, wp = max(wa, len(l.avail)), max(wp, len(l.pct))
		wt = max(wt, len(l.typ))
		wi = max(wi, len(l.ifree))
		lines[i] = l
	}
	fmt.Fprintf(w, "%-*s", wf, "Filesystem")
	if opt.printType {
		fmt.Fprintf(w, " %-*s", wt, "Type")
	}
	fmt.Fprintf(w, " %*s %*s %*s %*s",
		ws, sizeHdr, wu, usedHeader(opt.inodes), wa, availHdr, wp, pctHdr)
	if opt.freeSlots {
		fmt.Fprintf(w, " %*s", wi, "IFree")
	}
	fmt.Fprintln(w, " Mounted on")
	for _, l := range lines {
		fmt.Fprintf(w, "%-*s", wf, l.fsys)
		if opt.printType {
			fmt.Fprintf(w, " %-*s", wt, l.typ)
		}
		fmt.Fprintf(w, " %*s %*s %*s %*s",
			ws, l.size, wu, l.used, wa, l.avail, wp, l.pct)
		if opt.freeSlots {
			fmt.Fprintf(w, " %*s", wi, l.ifree)
		}
		fmt.Fprintf(w, " %s\n", l.mnt)
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

type spaceAmount struct {
	negative  bool
	magnitude uint64
}

func positiveSpace(n uint64) spaceAmount { return spaceAmount{magnitude: n} }

// spaceFromBlocks preserves the kernel's two's-complement representation of
// a negative f_bavail. A positive available count cannot exceed total blocks;
// a larger sign-bit value is therefore the negative value POSIX permits.
func spaceFromBlocks(blocks, total, blockSize uint64) spaceAmount {
	if blocks > total && blocks&(uint64(1)<<63) != 0 {
		return spaceAmount{negative: true, magnitude: satMul(^blocks+1, blockSize)}
	}
	return positiveSpace(satMul(blocks, blockSize))
}

func fmtSpaceValue(value spaceAmount, scale scaleMode) string {
	if !value.negative {
		return fmtValue(value.magnitude, scale)
	}
	if scale.human {
		return "-" + humanSize(value.magnitude, scale.base)
	}
	// "Next higher unit" is mathematical ceiling: -513 bytes in 512-byte
	// units is -1, not -2.
	units := uint64(0)
	if scale.blockSize != 0 {
		units = value.magnitude / scale.blockSize
	}
	if units == 0 {
		return "0"
	}
	return "-" + strconv.FormatUint(units, 10)
}

// usePct is used/(used+avail), rounded up. POSIX permits negative available
// space, which yields a positive percentage greater than 100 while the
// normally available denominator remains positive.
func usePct(used uint64, avail spaceAmount) string {
	if used == 0 && avail.magnitude == 0 {
		return "-"
	}
	// Keep the calculation exact even when the byte counters sum past
	// uint64. Filesystem counters are commonly uint64 values, and a
	// wrapped sum would otherwise produce a misleading '-' or percentage.
	numerator := new(big.Int).SetUint64(used)
	numerator.Mul(numerator, big.NewInt(100))
	denominator := new(big.Int).SetUint64(used)
	available := new(big.Int).SetUint64(avail.magnitude)
	if avail.negative {
		denominator.Sub(denominator, available)
	} else {
		denominator.Add(denominator, available)
	}
	if denominator.Sign() <= 0 {
		return "-"
	}
	quotient := new(big.Int).Quo(numerator, denominator)
	if new(big.Int).Mod(numerator, denominator).Sign() != 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient.String() + "%"
}

func inodeUsed(m mountEntry) uint64 {
	if m.files < m.ifree {
		return 0
	}
	return m.files - m.ifree
}

func divCeil(n, d uint64) uint64 {
	if d == 0 || n == 0 {
		return 0
	}
	return 1 + (n-1)/d
}

func normalizeBlockSizeArgs(args []string, requireOrder bool) []string {
	var out []string
	for i, a := range args {
		if requireOrder && (a == "-" || !strings.HasPrefix(a, "-")) {
			out = append(out, args[i:]...)
			break
		}
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
		if len(a) > 2 && a[0] == '-' && a[1] != '-' {
			// Scan the cluster, but stop at an argument-taking
			// shorthand (-B, -x): the rest of the word is that
			// flag's argument, not more flags.
			kept := []byte{'-'}
			sawM := false
			for j := 1; j < len(a); j++ {
				if a[j] == 'B' || a[j] == 'x' {
					kept = append(kept, a[j:]...)
					break
				}
				if a[j] == 'M' {
					sawM = true
					continue
				}
				kept = append(kept, a[j])
			}
			if sawM {
				out = append(out, "--block-size=1M")
				if len(kept) > 1 {
					out = append(out, string(kept))
				}
				continue
			}
		}
		out = append(out, a)
	}
	return out
}

// extractShort removes the given single-letter flags (which have no
// GNU long form) from short-flag clusters, returning the remaining
// args and the set of letters seen. Scanning stops at "--", and within
// a cluster at an argument-taking shorthand (-B, -x), whose in-word
// argument must not be mistaken for more flags.
func extractShort(args []string, chars string, requireOrder bool) ([]string, map[byte]bool) {
	found := map[byte]bool{}
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if requireOrder && (a == "-" || !strings.HasPrefix(a, "-")) {
			rest = append(rest, args[i:]...)
			break
		}
		if a == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' {
			kept := []byte{'-'}
			for j := 1; j < len(a); j++ {
				if a[j] == 'B' || a[j] == 'x' {
					kept = append(kept, a[j:]...)
					break
				}
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

func envPresent(env []string, key string) bool {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
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
		if div > ^uint64(0)/base {
			break
		}
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
	t := mountEntry{device: "total", point: "-", fstype: "-", fileSlotsKnown: true}
	for _, m := range rows {
		t.total = satAdd(t.total, m.total)
		t.used = satAdd(t.used, m.used)
		t.avail = addSpace(t.avail, m.avail)
		t.files = satAdd(t.files, m.files)
		t.ifree = satAdd(t.ifree, m.ifree)
		t.fileSlotsKnown = t.fileSlotsKnown && m.fileSlotsKnown
	}
	return t
}

func satAdd(a, b uint64) uint64 {
	if a > ^uint64(0)-b {
		return ^uint64(0)
	}
	return a + b
}

func satMul(a, b uint64) uint64 {
	if a != 0 && b > ^uint64(0)/a {
		return ^uint64(0)
	}
	return a * b
}

func addSpace(a, b spaceAmount) spaceAmount {
	if a.negative == b.negative {
		return spaceAmount{negative: a.negative, magnitude: satAdd(a.magnitude, b.magnitude)}
	}
	if a.magnitude >= b.magnitude {
		return spaceAmount{negative: a.negative, magnitude: a.magnitude - b.magnitude}
	}
	return spaceAmount{negative: b.negative, magnitude: b.magnitude - a.magnitude}
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
	"size":   "512-blocks",
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
		return usePct(iused, positiveSpace(m.ifree))
	case "size":
		return fmtValue(m.total, scale)
	case "used":
		return fmtValue(m.used, scale)
	case "avail":
		return fmtSpaceValue(m.avail, scale)
	case "pcent":
		return usePct(m.used, m.avail)
	case "file", "target":
		return m.point
	default:
		return ""
	}
}
