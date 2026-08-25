package paxcmd

import (
	"archive/tar"
	"bytes"
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
	return &decodedArchive{kind: archiveTar, tarData: data, pax: paxFormat}, nil
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

type blockWriter struct {
	w    io.Writer
	size int
	buf  []byte
	err  error
}

func newBlockWriter(w io.Writer, size int) *blockWriter {
	return &blockWriter{w: w, size: size, buf: make([]byte, 0, size)}
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

type cpioEntry struct {
	name     string
	mode     uint64
	uid, gid uint64
	mtime    uint64
	dev, ino uint64
	nlink    uint64
	data     []byte
}

func cpioToTar(data []byte) ([]byte, error) {
	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	offset := 0
	links := make(map[[2]uint64]string)
	trailer := false
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
			trailer = true
			break
		}
		h := &tar.Header{
			Name:    entry.name,
			Mode:    int64(entry.mode & 0o7777),
			Uid:     int(entry.uid),
			Gid:     int(entry.gid),
			ModTime: time.Unix(int64(entry.mtime), 0),
			Format:  tar.FormatPAX,
		}
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
			key := [2]uint64{entry.dev, entry.ino}
			if first, ok := links[key]; ok && entry.nlink > 1 && len(entry.data) == 0 {
				h.Typeflag = tar.TypeLink
				h.Linkname = first
			} else {
				h.Typeflag = tar.TypeReg
				h.Size = int64(len(entry.data))
				if entry.nlink > 1 {
					links[key] = h.Name
				}
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
			if _, err := tw.Write(entry.data); err != nil {
				return nil, err
			}
		}
	}
	if !trailer {
		return nil, fmt.Errorf("cpio: missing TRAILER!!! entry")
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
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
	e := cpioEntry{ino: values[0], mode: values[1], uid: values[2], gid: values[3], nlink: values[4], mtime: values[5], dev: values[7]<<32 | values[8]}
	return finishCPIOEntry(data, offset+headerSize, values[11], values[6], e, 4)
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
