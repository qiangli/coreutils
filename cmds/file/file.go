// Package filecmd implements the POSIX file(1) core interface with a small,
// deterministic built-in signature set. Unknown binary formats are reported
// honestly as "data".
package filecmd

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
	"github.com/spf13/pflag"
)

const readChunkSize = 32 * 1024

type regularFile interface {
	io.ReaderAt
	io.Closer
}

type regularFileOpener func(string) (regularFile, error)

func openOSRegularFile(path string) (regularFile, error) { return os.Open(path) }

var cmd = &tool.Tool{Name: "file", Synopsis: "Determine the type of each FILE.", Usage: "file [-dh] [-M FILE] [-m FILE] FILE...\n  or:  file -i [-h] FILE..."}

func init() { cmd.Run = run; tool.Register(cmd) }

type testSourceKind byte

const (
	defaultTests testSourceKind = iota
	additionalMagic
	replacementMagic
)

type testSource struct {
	kind testSourceKind
	name string
}

type orderedSourceValue struct {
	sources    *[]testSource
	kind       testSourceKind
	noArgument bool
}

func (v orderedSourceValue) String() string {
	if v.noArgument {
		return "false"
	}
	return ""
}
func (v orderedSourceValue) Type() string {
	if v.noArgument {
		return "bool"
	}
	return "file"
}
func (v orderedSourceValue) Set(value string) error {
	*v.sources = append(*v.sources, testSource{kind: v.kind, name: value})
	return nil
}

func addSourceFlag(fs *pflag.FlagSet, sources *[]testSource, kind testSourceKind, name, shorthand, usage string, noArgument bool) {
	fs.VarP(orderedSourceValue{sources: sources, kind: kind, noArgument: noArgument}, name, shorthand, usage)
	if noArgument {
		fs.Lookup(name).NoOptDefVal = "true"
	}
}

func run(rc *tool.RunContext, args []string) int { return runWithOpener(rc, args, openOSRegularFile) }

func runWithOpener(rc *tool.RunContext, args []string, open regularFileOpener) int {
	fs := tool.NewFlags(cmd.Name)
	brief := fs.BoolP("brief", "b", false, "do not prepend file names to output lines")
	follow := fs.BoolP("dereference", "L", false, "follow symbolic links (the POSIX default)")
	noFollow := fs.BoolP("no-dereference", "h", false, "identify symbolic links instead of following them")
	minimal := fs.BoolP("regular-file", "i", false, "identify regular files without classifying their contents")
	var sources []testSource
	addSourceFlag(fs, &sources, defaultTests, "default-tests", "d", "apply the default system tests at this position", true)
	addSourceFlag(fs, &sources, replacementMagic, "replace-magic", "M", "apply position-sensitive tests from FILE without implicit defaults", false)
	addSourceFlag(fs, &sources, additionalMagic, "magic-file", "m", "apply position-sensitive tests from FILE", false)
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing file operand")
	}
	if *minimal && len(sources) != 0 {
		return tool.UsageError(rc, cmd, "-i cannot be combined with -d, -M, or -m")
	}

	plan, err := loadTestPlan(rc, sources)
	if err != nil {
		_, _ = fmt.Fprintf(rc.Err, "file: %v\n", err)
		return 1
	}
	status := 0
	for _, name := range operands {
		forceName := false
		typ, err := identify(rc, name, *noFollow && !*follow, *minimal, plan, open)
		if err != nil {
			var formatErr *magicEvaluationError
			if errors.As(err, &formatErr) {
				_, _ = fmt.Fprintf(rc.Err, "file: %v\n", formatErr)
				status = 1
			} else {
				// XCU file: nonexistent, unreadable, or undetermined
				// operands SHALL NOT affect the exit status; the
				// standard-output line carries "cannot open".
				typ = fmt.Sprintf("cannot open %q (%v)", name, tool.SysErr(err))
				forceName = true
			}
		}
		var line string
		if *brief && !forceName {
			line = typ + "\n"
		} else {
			line = fmt.Sprintf("%s: %s\n", name, typ)
		}
		if err := writeAll(rc.Out, line); err != nil {
			_, _ = fmt.Fprintf(rc.Err, "file: write error: %v\n", err)
			return 1
		}
	}
	return status
}

func identify(rc *tool.RunContext, name string, noFollow, minimal bool, plan testPlan, open regularFileOpener) (string, error) {
	if name == "-" {
		if minimal {
			return "regular file", nil
		}
		data, err := inspectStream(rc.In, plan)
		if err != nil {
			return "", err
		}
		return plan.classifyInspected(data, localeAllowsUTF8(rc))
	}

	path := rc.Path(name)
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if noFollow {
			return describeSymlink(path)
		}
		info, err = os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return describeSymlink(path)
			}
			return "", err
		}
	}
	switch {
	case info.IsDir():
		return "directory", nil
	case info.Mode()&os.ModeNamedPipe != 0:
		return "fifo (named pipe)", nil
	case info.Mode()&os.ModeSocket != 0:
		return "socket", nil
	case info.Mode()&os.ModeDevice != 0:
		return specialDeviceType(info), nil
	case !info.Mode().IsRegular():
		return "special file", nil
	}
	if minimal {
		return "regular file", nil
	}
	f, err := open(path)
	if err != nil {
		return "", err
	}
	data, readErr := inspectReaderAt(f, plan.ranges)
	if readErr == nil {
		data, readErr = inspectDynamicPEReaderAt(f, data)
	}
	closeErr := f.Close()
	if readErr != nil {
		return "", readErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return plan.classifyInspected(data, localeAllowsUTF8(rc))
}

func describeSymlink(path string) (string, error) {
	target, err := os.Readlink(path)
	if err != nil {
		return "", err
	}
	return "symbolic link to " + target, nil
}

type byteRange struct{ start, end uint64 }
type dataChunk struct {
	start uint64
	data  []byte
}
type inspectedData struct{ chunks []dataChunk }

func mergeRanges(ranges []byteRange) []byteRange {
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	out := make([]byteRange, 0, len(ranges))
	for _, current := range ranges {
		if current.end <= current.start {
			continue
		}
		if len(out) == 0 || current.start > out[len(out)-1].end {
			out = append(out, current)
			continue
		}
		if current.end > out[len(out)-1].end {
			out[len(out)-1].end = current.end
		}
	}
	return out
}

func (d inspectedData) prefix() []byte {
	if len(d.chunks) != 0 && d.chunks[0].start == 0 {
		return d.chunks[0].data
	}
	return nil
}

func (d inspectedData) bytesAt(offset uint64, width int) ([]byte, bool) {
	if width == 0 {
		return []byte{}, true
	}
	end := offset + uint64(width)
	if end < offset {
		return nil, false
	}
	result := make([]byte, width)
	pos := offset
	for _, chunk := range d.chunks {
		chunkEnd := chunk.start + uint64(len(chunk.data))
		if chunkEnd <= pos {
			continue
		}
		if chunk.start > pos {
			return nil, false
		}
		copyEnd := min(chunkEnd, end)
		copy(result[pos-offset:copyEnd-offset], chunk.data[pos-chunk.start:copyEnd-chunk.start])
		pos = copyEnd
		if pos == end {
			return result, true
		}
	}
	return nil, false
}

func inspectReaderAt(r io.ReaderAt, ranges []byteRange) (inspectedData, error) {
	var out inspectedData
	for _, span := range mergeRanges(append([]byteRange(nil), ranges...)) {
		if span.start > uint64(^uint64(0)>>1) {
			continue // no regular file can address beyond the signed file-offset range
		}
		width := span.end - span.start
		if width > uint64(^uint(0)>>1) {
			return inspectedData{}, fmt.Errorf("magic range is too wide to inspect")
		}
		buf := make([]byte, int(width))
		n, err := r.ReadAt(buf, int64(span.start))
		if n > 0 {
			out.chunks = append(out.chunks, dataChunk{start: span.start, data: buf[:n]})
		}
		if err != nil && err != io.EOF {
			return inspectedData{}, err
		}
	}
	return out, nil
}

func inspectDynamicPEReaderAt(r io.ReaderAt, data inspectedData) (inspectedData, error) {
	span, ok := peSignatureRange(data.prefix())
	if !ok {
		return data, nil
	}
	extra, err := inspectReaderAt(r, []byteRange{span})
	if err != nil {
		return inspectedData{}, err
	}
	data.chunks = append(data.chunks, extra.chunks...)
	sort.Slice(data.chunks, func(i, j int) bool { return data.chunks[i].start < data.chunks[j].start })
	return data, nil
}

func inspectStream(r io.Reader, plan testPlan) (inspectedData, error) {
	if r == nil {
		return inspectedData{}, nil
	}
	// Read the fixed context prefix first. It reveals the one dynamic built-in
	// range (the PE signature offset) before any gaps are discarded.
	data, pos, eof, err := inspectStreamRanges(r, []byteRange{{end: plan.prefixBytes}}, 0, inspectedData{})
	if err != nil || eof {
		return data, err
	}
	ranges := append([]byteRange(nil), plan.ranges...)
	if pe, ok := peSignatureRange(data.prefix()); ok {
		ranges = append(ranges, pe)
	}
	data, _, _, err = inspectStreamRanges(r, mergeRanges(ranges), pos, data)
	return data, err
}

func inspectStreamRanges(r io.Reader, ranges []byteRange, pos uint64, out inspectedData) (inspectedData, uint64, bool, error) {
	buffer := make([]byte, readChunkSize)
	for _, span := range ranges {
		if span.end <= pos {
			continue
		}
		if span.start > pos {
			n, eof, err := transferReader(r, buffer, span.start-pos, nil)
			pos += n
			if err != nil || eof {
				return out, pos, eof, err
			}
		}
		start := max(span.start, pos)
		if span.end <= start {
			continue
		}
		width := span.end - start
		if width > uint64(^uint(0)>>1) {
			return inspectedData{}, pos, false, fmt.Errorf("magic range is too wide to inspect")
		}
		captured := make([]byte, 0, int(width))
		n, eof, err := transferReader(r, buffer, width, &captured)
		if len(captured) != 0 {
			out.chunks = append(out.chunks, dataChunk{start: start, data: captured})
		}
		pos += n
		if err != nil || eof {
			return out, pos, eof, err
		}
	}
	return out, pos, false, nil
}

func transferReader(r io.Reader, buffer []byte, count uint64, captured *[]byte) (uint64, bool, error) {
	var total uint64
	for total < count {
		want := min(uint64(len(buffer)), count-total)
		n, err := r.Read(buffer[:int(want)])
		if n < 0 || n > int(want) {
			return total, false, fmt.Errorf("invalid read count %d", n)
		}
		if captured != nil {
			*captured = append(*captured, buffer[:n]...)
		}
		total += uint64(n)
		if err != nil {
			if err == io.EOF {
				return total, true, nil
			}
			return total, false, err
		}
		if n == 0 {
			return total, false, io.ErrNoProgress
		}
	}
	return total, false, nil
}

func writeAll(w io.Writer, value string) error {
	n, err := io.WriteString(w, value)
	if err == nil && n != len(value) {
		return io.ErrShortWrite
	}
	return err
}

func classifyPosition(data []byte) (string, bool) {
	return classifyPositionInspected(inspectedData{chunks: []dataChunk{{data: data}}})
}

func classifyPositionInspected(inspected inspectedData) (string, bool) {
	data := inspected.prefix()
	if bytes.HasPrefix(data, []byte{0x7f, 'E', 'L', 'F'}) && len(data) >= 6 {
		bits := map[byte]string{1: "32-bit", 2: "64-bit"}[data[4]]
		if bits == "" {
			bits = "unknown class"
		}
		endian := map[byte]string{1: "LSB", 2: "MSB"}[data[5]]
		if endian == "" {
			endian = "unknown endian"
		}
		description := fmt.Sprintf("ELF %s %s", bits, endian)
		if len(data) >= 18 {
			var typ uint16
			if data[5] == 2 {
				typ = binary.BigEndian.Uint16(data[16:18])
			} else {
				typ = binary.LittleEndian.Uint16(data[16:18])
			}
			if typ == 2 || typ == 3 {
				description += " executable"
			}
		}
		return description, true
	}
	if len(data) >= 0x40 && data[0] == 'M' && data[1] == 'Z' {
		off := uint64(binary.LittleEndian.Uint32(data[0x3c:0x40]))
		if signature, ok := inspected.bytesAt(off, 4); ok && bytes.Equal(signature, []byte("PE\x00\x00")) {
			return "PE executable", true
		}
		return "DOS executable", true
	}
	if len(data) >= 4 {
		switch binary.BigEndian.Uint32(data[:4]) {
		case 0xfeedface, 0xcefaedfe:
			return "Mach-O 32-bit", true
		case 0xfeedfacf, 0xcffaedfe:
			return "Mach-O 64-bit", true
		case 0xcafebabe, 0xbebafeca:
			return "Mach-O universal binary", true
		}
	}
	for _, sig := range []struct {
		prefix []byte
		typ    string
	}{
		{[]byte("\x89PNG\r\n\x1a\n"), "PNG image data"}, {[]byte("\xff\xd8\xff"), "JPEG image data"},
		{[]byte("GIF87a"), "GIF image data, version 87a"}, {[]byte("GIF89a"), "GIF image data, version 89a"},
		{[]byte("PK\x03\x04"), "Zip archive data"}, {[]byte("PK\x05\x06"), "Zip archive data (empty)"},
		{[]byte("\x1f\x8b"), "gzip compressed data"},
		{[]byte("!<arch>\n"), "current ar archive"},
		{[]byte("070701"), "ASCII cpio archive"}, {[]byte("070702"), "ASCII cpio archive"},
		{[]byte("070707"), "ASCII cpio archive"},
	} {
		if bytes.HasPrefix(data, sig.prefix) {
			return sig.typ, true
		}
	}
	if bytes.HasPrefix(data, []byte("%PDF-")) {
		version := strings.TrimSpace(string(data[5:min(len(data), 8)]))
		return "PDF document, version " + version, true
	}
	if len(data) >= 262 && bytes.Equal(data[257:262], []byte("ustar")) {
		return "POSIX tar archive", true
	}
	return "", false
}

func peSignatureRange(prefix []byte) (byteRange, bool) {
	if len(prefix) < 0x40 || prefix[0] != 'M' || prefix[1] != 'Z' {
		return byteRange{}, false
	}
	start := uint64(binary.LittleEndian.Uint32(prefix[0x3c:0x40]))
	return byteRange{start: start, end: start + 4}, true
}

func classifyContext(data []byte, utf8Locale bool) string {
	if bytes.HasPrefix(data, []byte("#!")) {
		line := data[2:]
		if i := bytes.IndexByte(line, '\n'); i >= 0 {
			line = line[:i]
		}
		interp := strings.TrimSpace(string(line))
		if interp != "" && utf8.ValidString(interp) {
			return interp + " commands text"
		}
		return "commands text"
	}
	if isASCIIText(data) {
		text := string(data)
		if looksLikeC(text) {
			return "c program text"
		}
		if looksLikeFortran(text) {
			return "fortran program text"
		}
		return "ASCII text"
	}
	if utf8Locale && utf8.Valid(data) && isText(data) {
		return "Unicode text, UTF-8 text"
	}
	return "data"
}

func looksLikeC(text string) bool {
	compact := strings.ReplaceAll(text, "\t", " ")
	return strings.Contains(compact, "#include") ||
		strings.Contains(compact, "int main(") || strings.Contains(compact, "int main (") ||
		(strings.Contains(compact, "#define") && strings.Contains(compact, "{"))
}

func looksLikeFortran(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}
		fields := strings.Fields(strings.ToLower(line))
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "program", "subroutine", "function":
			return len(fields) > 1
		}
	}
	return false
}

func localeAllowsUTF8(rc *tool.RunContext) bool {
	name := strings.ToLower(locale.Resolve(rc.Env, locale.CType))
	return strings.Contains(name, "utf-8") || strings.Contains(name, "utf8")
}

// classify remains the direct built-in classifier used by focused signature
// tests. An invocation uses testPlan.classify so option ordering is preserved.
func classify(data []byte, readErr error) (string, error) {
	if readErr != nil {
		return "", readErr
	}
	if len(data) == 0 {
		return "empty", nil
	}
	if typ, ok := classifyPosition(data); ok {
		return typ, nil
	}
	return classifyContext(data, true), nil
}

func isASCIIText(data []byte) bool {
	for _, b := range data {
		if b >= 0x80 || b == 0 || (b < 0x20 && b != '\n' && b != '\r' && b != '\t' && b != '\f' && b != '\b') {
			return false
		}
	}
	return true
}

func isText(data []byte) bool {
	for _, r := range string(data) {
		if r == 0 || (r < 0x20 && r != '\n' && r != '\r' && r != '\t' && r != '\f' && r != '\b') {
			return false
		}
	}
	return true
}
