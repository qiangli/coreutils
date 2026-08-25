package paxcmd

import (
	"archive/tar"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"time"
)

type archiveKind int

const (
	archiveTar archiveKind = iota
	archiveCPIO
)

type decodedArchive struct {
	kind    archiveKind
	tarData []byte
	pax     bool
}

// decodeArchive normalizes every supported input format to tar so selection
// and PlanExtraction use one security path. The cpio reader accepts the POSIX
// Issue 7 octet-oriented format and the newc form emitted by earlier releases.
func decodeArchive(data []byte) (*decodedArchive, error) {
	if len(data) >= 6 && (string(data[:6]) == "070707" || string(data[:6]) == "070701" || string(data[:6]) == "070702") {
		tarData, err := cpioToTar(data)
		if err != nil {
			return nil, err
		}
		return &decodedArchive{kind: archiveCPIO, tarData: tarData}, nil
	}
	_, paxFormat, err := scanTar(data)
	if err != nil {
		return nil, err
	}
	return &decodedArchive{kind: archiveTar, tarData: data, pax: paxFormat || hasRawPAXHeader(data)}, nil
}

func hasRawPAXHeader(data []byte) bool {
	for off := 0; off+512 <= len(data); {
		header := data[off : off+512]
		if allZero(header) {
			return false
		}
		if header[156] == tar.TypeXHeader || header[156] == tar.TypeXGlobalHeader {
			return true
		}
		size, err := rawTarSize(header)
		if err != nil || size < 0 {
			return false
		}
		off += 512 + int((size+511)&^511)
	}
	return false
}

type countingReader struct {
	r io.Reader
	n int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.n += int64(n)
	return n, err
}

// scanTar validates a complete tar stream and returns the offset of its two
// end markers. archive/tar consumes exactly those two logical records before
// reporting EOF, so the count is safe even when the final file data is zeros.
func scanTar(data []byte) (endOffset int64, paxFormat bool, err error) {
	cr := &countingReader{r: bytes.NewReader(data)}
	tr := tar.NewReader(cr)
	for {
		h, nextErr := tr.Next()
		if nextErr == io.EOF {
			if cr.n < 1024 {
				return 0, false, fmt.Errorf("invalid tar archive: missing end-of-archive marker")
			}
			end := cr.n - 1024
			if end < 0 || end+1024 > int64(len(data)) || !allZero(data[end:end+1024]) {
				return 0, false, fmt.Errorf("invalid tar archive: missing end-of-archive marker")
			}
			return end, paxFormat, nil
		}
		if nextErr != nil {
			return 0, false, nextErr
		}
		if h.Format == tar.FormatPAX {
			paxFormat = true
		}
		if _, nextErr = io.Copy(io.Discard, tr); nextErr != nil {
			return 0, false, nextErr
		}
	}
}

// blockWriter turns the logical archive byte stream into complete physical
// writes. For a nonaligned append, prefix contains the bytes before the
// logical append point in its containing physical block; those bytes are
// emitted again as part of the first complete block rather than as a short
// write.
type blockWriter struct {
	w    io.Writer
	size int
	buf  []byte
	err  error
}

func newBlockWriter(w io.Writer, size int, prefix []byte) *blockWriter {
	buf := make([]byte, len(prefix), size)
	copy(buf, prefix)
	return &blockWriter{w: w, size: size, buf: buf}
}

func (w *blockWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	written := 0
	for len(p) > 0 {
		need := w.size - len(w.buf)
		if need > len(p) {
			need = len(p)
		}
		w.buf = append(w.buf, p[:need]...)
		p = p[need:]
		written += need
		if len(w.buf) == w.size {
			if err := w.flushBlock(); err != nil {
				return written, err
			}
		}
	}
	return written, nil
}

func (w *blockWriter) flushBlock() error {
	n, err := w.w.Write(w.buf)
	if err == nil && n != len(w.buf) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.err = err
		return err
	}
	w.buf = w.buf[:0]
	return nil
}

// Close zero-pads whatever is left to a full physical block. A stream that
// already ended on a boundary needs no padding and gets none.
func (w *blockWriter) Close() error {
	if w.err != nil {
		return w.err
	}
	if len(w.buf) == 0 {
		return nil
	}
	w.buf = append(w.buf, make([]byte, w.size-len(w.buf))...)
	return w.flushBlock()
}

// fileIdentity is the source metadata a POSIX archive header carries that
// os.FileInfo does not expose portably: the owning ids, the (device, inode)
// pair that establishes hardlink identity, and the link count.
type fileIdentity struct {
	uid, gid uint64
	dev, ino uint64
	nlink    uint64
	ok       bool
}

// devIno is a source file's hardlink identity.
type devIno struct{ dev, ino uint64 }

func (id fileIdentity) key() devIno { return devIno{id.dev, id.ino} }

type cpioEntry struct {
	name     string
	magic    string
	mode     uint64
	uid, gid uint64
	mtime    uint64
	dev, ino uint64
	rdev     uint64
	nlink    uint64
	namesize uint64
	filesize uint64
	data     []byte
}

// cpioToTar converts a complete cpio archive to tar. It reads EVERY member
// before emitting anything because hardlink groups cannot be resolved in one
// pass: cpio is free to carry the file data on any member of a group (POSIX
// and GNU cpio put it on the last), while tar requires the data to precede the
// links that reference it. Buffering first lets every name in a group be
// materialized with the right content and the right link.
func cpioToTar(data []byte) ([]byte, error) {
	entries, err := readCPIOEntries(data)
	if err != nil {
		return nil, err
	}

	// first[key] is the archive-order index of the member that will carry the
	// data for its hardlink group; groupData[key] is that data, taken from
	// whichever member actually held it.
	first := make(map[devIno]int)
	groupData := make(map[devIno][]byte)
	for i, entry := range entries {
		if entry.mode&0o170000 != 0o100000 || entry.nlink <= 1 {
			continue
		}
		key := devIno{entry.dev, entry.ino}
		if _, ok := first[key]; !ok {
			first[key] = i
		}
		if len(entry.data) > 0 && len(groupData[key]) == 0 {
			groupData[key] = entry.data
		}
	}

	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	for i, entry := range entries {
		h := &tar.Header{
			Name:    entry.name,
			Mode:    int64(entry.mode & 0o7777),
			Uid:     int(entry.uid),
			Gid:     int(entry.gid),
			ModTime: time.Unix(int64(entry.mtime), 0),
			Format:  tar.FormatPAX,
			PAXRecords: map[string]string{
				"COREUTILS.cpio.c_magic":           entry.magic,
				"COREUTILS.cpio.c_dev":             strconv.FormatUint(entry.dev, 10),
				"COREUTILS.cpio.c_ino":             strconv.FormatUint(entry.ino, 10),
				"COREUTILS.cpio.c_mode":            strconv.FormatUint(entry.mode, 10),
				"COREUTILS.cpio.c_uid":             strconv.FormatUint(entry.uid, 10),
				"COREUTILS.cpio.c_gid":             strconv.FormatUint(entry.gid, 10),
				"COREUTILS.cpio.c_nlink":           strconv.FormatUint(entry.nlink, 10),
				"COREUTILS.cpio.c_rdev":            strconv.FormatUint(entry.rdev, 10),
				"COREUTILS.cpio.c_mtime":           strconv.FormatUint(entry.mtime, 10),
				"COREUTILS.cpio.c_namesize":        strconv.FormatUint(entry.namesize, 10),
				"COREUTILS.cpio.c_filesize":        strconv.FormatUint(entry.filesize, 10),
				"COREUTILS.cpio.c_name":            entry.name,
				"COREUTILS.internal.cpio.filedata": "b64:" + base64.StdEncoding.EncodeToString(entry.data),
			},
		}
		body := entry.data
		switch entry.mode & 0o170000 {
		case 0o040000:
			h.Typeflag = tar.TypeDir
			if h.Name != "" && h.Name[len(h.Name)-1] != '/' {
				h.Name += "/"
			}
		case 0o120000:
			h.Typeflag = tar.TypeSymlink
			h.Linkname = string(entry.data)
		case 0o100000:
			key := devIno{entry.dev, entry.ino}
			if head, ok := first[key]; ok && head != i {
				h.Typeflag = tar.TypeLink
				h.Linkname = entries[head].name
				body = nil
			} else {
				h.Typeflag = tar.TypeReg
				if ok {
					body = groupData[key]
				}
				h.Size = int64(len(body))
			}
		case 0o010000:
			h.Typeflag = tar.TypeFifo
		default:
			return nil, fmt.Errorf("cpio: unsupported member type %#o for %q", entry.mode&0o170000, entry.name)
		}
		if err := tw.WriteHeader(h); err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write(body); err != nil {
				return nil, err
			}
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// readCPIOEntries decodes every member up to the TRAILER!!! marker. A missing
// trailer is fatal: a truncated archive must not be extracted as if complete.
func readCPIOEntries(data []byte) ([]cpioEntry, error) {
	var entries []cpioEntry
	offset := 0
	for offset < len(data) {
		// A complete archive may contain arbitrary padding after its trailer.
		if allZero(data[offset:]) {
			break
		}
		entry, next, err := readCPIOEntry(data, offset)
		if err != nil {
			return nil, err
		}
		offset = next
		if entry.name == "TRAILER!!!" {
			return entries, nil
		}
		entries = append(entries, entry)
	}
	return nil, fmt.Errorf("cpio: missing TRAILER!!! entry")
}

func readCPIOEntry(data []byte, offset int) (cpioEntry, int, error) {
	if len(data)-offset < 6 {
		return cpioEntry{}, 0, io.ErrUnexpectedEOF
	}
	magic := string(data[offset : offset+6])
	switch magic {
	case "070707":
		return readODCEntry(data, offset)
	case "070701", "070702":
		return readNewcEntry(data, offset)
	default:
		return cpioEntry{}, 0, fmt.Errorf("cpio: invalid magic %q at offset %d", magic, offset)
	}
}

func readODCEntry(data []byte, offset int) (cpioEntry, int, error) {
	const headerSize = 76
	if len(data)-offset < headerSize {
		return cpioEntry{}, 0, io.ErrUnexpectedEOF
	}
	h := data[offset : offset+headerSize]
	fields := []struct {
		start, width int
		to           *uint64
	}{}
	var e cpioEntry
	e.magic = "070707"
	var namesize, filesize uint64
	fields = append(fields,
		struct {
			start, width int
			to           *uint64
		}{6, 6, &e.dev},
		struct {
			start, width int
			to           *uint64
		}{12, 6, &e.ino},
		struct {
			start, width int
			to           *uint64
		}{18, 6, &e.mode},
		struct {
			start, width int
			to           *uint64
		}{24, 6, &e.uid},
		struct {
			start, width int
			to           *uint64
		}{30, 6, &e.gid},
		struct {
			start, width int
			to           *uint64
		}{36, 6, &e.nlink},
		struct {
			start, width int
			to           *uint64
		}{42, 6, &e.rdev},
		struct {
			start, width int
			to           *uint64
		}{48, 11, &e.mtime},
		struct {
			start, width int
			to           *uint64
		}{59, 6, &namesize},
		struct {
			start, width int
			to           *uint64
		}{65, 11, &filesize},
	)
	for _, f := range fields {
		v, err := parseASCIIUint(h[f.start:f.start+f.width], 8)
		if err != nil {
			return cpioEntry{}, 0, fmt.Errorf("cpio: invalid octal header at offset %d: %w", offset, err)
		}
		*f.to = v
	}
	return finishCPIOEntry(data, offset+headerSize, namesize, filesize, e, 1)
}

func readNewcEntry(data []byte, offset int) (cpioEntry, int, error) {
	const headerSize = 110
	if len(data)-offset < headerSize {
		return cpioEntry{}, 0, io.ErrUnexpectedEOF
	}
	h := data[offset : offset+headerSize]
	values := make([]uint64, 13)
	for i := range values {
		v, err := parseASCIIUint(h[6+i*8:6+(i+1)*8], 16)
		if err != nil {
			return cpioEntry{}, 0, fmt.Errorf("cpio: invalid hexadecimal header at offset %d: %w", offset, err)
		}
		values[i] = v
	}
	e := cpioEntry{magic: string(h[:6]), ino: values[0], mode: values[1], uid: values[2], gid: values[3], nlink: values[4], mtime: values[5], dev: values[7]<<32 | values[8], rdev: values[9]<<32 | values[10]}
	entry, next, err := finishCPIOEntry(data, offset+headerSize, values[11], values[6], e, 4)
	if err != nil {
		return cpioEntry{}, 0, err
	}
	// 070702 is the CRC variant: the check field is the simple sum of the
	// member's data bytes, taken as unsigned chars, modulo 2^32. Verifying it
	// here means a corrupted archive is refused during decode, before the
	// extraction planner has looked at a single destination.
	if string(h[:6]) == "070702" {
		if sum := uint64(cpioDataChecksum(entry.data)); sum != values[12] {
			return cpioEntry{}, 0, fmt.Errorf("cpio: checksum mismatch for %q: header %#08x, data %#08x", entry.name, values[12], sum)
		}
	}
	return entry, next, nil
}

// cpioDataChecksum is the newc CRC: an unsigned 32-bit sum of the data bytes,
// which is what 070702 stores despite the name.
func cpioDataChecksum(data []byte) uint32 {
	var sum uint32
	for _, b := range data {
		sum += uint32(b)
	}
	return sum
}

func finishCPIOEntry(data []byte, pos int, namesize, filesize uint64, e cpioEntry, alignment int) (cpioEntry, int, error) {
	if namesize == 0 || namesize > uint64(len(data)-pos) {
		return cpioEntry{}, 0, fmt.Errorf("cpio: invalid name size %d", namesize)
	}
	nameEnd := pos + int(namesize)
	name := data[pos:nameEnd]
	if name[len(name)-1] != 0 {
		return cpioEntry{}, 0, fmt.Errorf("cpio: pathname is not NUL-terminated")
	}
	e.name = string(name[:len(name)-1])
	e.namesize = namesize
	e.filesize = filesize
	pos = align(nameEnd, alignment)
	if pos > len(data) || filesize > uint64(len(data)-pos) {
		return cpioEntry{}, 0, io.ErrUnexpectedEOF
	}
	e.data = append([]byte(nil), data[pos:pos+int(filesize)]...)
	pos = align(pos+int(filesize), alignment)
	if pos > len(data) {
		return cpioEntry{}, 0, io.ErrUnexpectedEOF
	}
	return e, pos, nil
}

func parseASCIIUint(p []byte, base int) (uint64, error) {
	if len(p) == 0 {
		return 0, fmt.Errorf("empty number")
	}
	return strconv.ParseUint(string(p), base, 64)
}

func align(n, alignment int) int {
	return (n + alignment - 1) / alignment * alignment
}

func allZero(p []byte) bool {
	for _, b := range p {
		if b != 0 {
			return false
		}
	}
	return true
}
