// Package filecmd implements a small, deterministic, pure-Go subset of file(1).
// It is a fresh implementation from public format specifications and command
// documentation. Unknown binary formats are reported honestly as "data".
package filecmd

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{Name: "file", Synopsis: "Determine the type of each FILE using a portable built-in signature set.", Usage: "file [OPTION]... FILE..."}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	brief := fs.BoolP("brief", "b", false, "do not prepend file names to output lines")
	follow := fs.BoolP("dereference", "L", false, "follow symbolic links")
	operands, code := tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args))
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing file operand")
	}
	exit := 0
	for _, name := range operands {
		typ, err := identify(rc, name, *follow)
		if err != nil {
			fmt.Fprintf(rc.Err, "file: %s: %v\n", name, err)
			exit = 1
			continue
		}
		if *brief {
			fmt.Fprintln(rc.Out, typ)
		} else {
			fmt.Fprintf(rc.Out, "%s: %s\n", name, typ)
		}
	}
	return exit
}

func identify(rc *tool.RunContext, name string, follow bool) (string, error) {
	if name == "-" {
		return classify(readPrefix(rc.In))
	}
	path := rc.Path(name)
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 && !follow {
		target, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("symbolic link to %s", target), nil
	}
	if follow {
		info, err = os.Stat(path)
		if err != nil {
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
	case info.Mode()&os.ModeDevice != 0 && info.Mode()&os.ModeCharDevice != 0:
		return specialDeviceType(info), nil
	case info.Mode()&os.ModeDevice != 0:
		return specialDeviceType(info), nil
	case !info.Mode().IsRegular():
		return "special file", nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return classify(readPrefix(f))
}

func readPrefix(r io.Reader) ([]byte, error) {
	if r == nil {
		return nil, nil
	}
	return io.ReadAll(io.LimitReader(r, 64*1024))
}

func classify(data []byte, readErr error) (string, error) {
	if readErr != nil {
		return "", readErr
	}
	if len(data) == 0 {
		return "empty", nil
	}
	if bytes.HasPrefix(data, []byte{0x7f, 'E', 'L', 'F'}) && len(data) >= 6 {
		bits := map[byte]string{1: "32-bit", 2: "64-bit"}[data[4]]
		if bits == "" {
			bits = "unknown class"
		}
		endian := map[byte]string{1: "LSB", 2: "MSB"}[data[5]]
		if endian == "" {
			endian = "unknown endian"
		}
		return fmt.Sprintf("ELF %s %s", bits, endian), nil
	}
	if len(data) >= 0x40 && data[0] == 'M' && data[1] == 'Z' {
		off := int(binary.LittleEndian.Uint32(data[0x3c:0x40]))
		if off >= 0 && off+4 <= len(data) && bytes.Equal(data[off:off+4], []byte("PE\x00\x00")) {
			return "PE executable", nil
		}
		return "DOS executable", nil
	}
	if len(data) >= 4 {
		switch binary.BigEndian.Uint32(data[:4]) {
		case 0xfeedface, 0xcefaedfe:
			return "Mach-O 32-bit", nil
		case 0xfeedfacf, 0xcffaedfe:
			return "Mach-O 64-bit", nil
		case 0xcafebabe, 0xbebafeca:
			return "Mach-O universal binary", nil
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
	} {
		if bytes.HasPrefix(data, sig.prefix) {
			return sig.typ, nil
		}
	}
	if bytes.HasPrefix(data, []byte("%PDF-")) {
		version := strings.TrimSpace(string(data[5:min(len(data), 8)]))
		return "PDF document, version " + version, nil
	}
	if len(data) >= 262 && bytes.Equal(data[257:262], []byte("ustar")) {
		return "POSIX tar archive", nil
	}
	if bytes.HasPrefix(data, []byte("#!")) {
		line := data[2:]
		if i := bytes.IndexByte(line, '\n'); i >= 0 {
			line = line[:i]
		}
		interp := strings.TrimSpace(string(line))
		if interp != "" && utf8.ValidString(interp) {
			return interp + " script, text executable", nil
		}
		return "script, text executable", nil
	}
	if isASCIIText(data) {
		return "ASCII text", nil
	}
	if utf8.Valid(data) && isText(data) {
		return "Unicode text, UTF-8 text", nil
	}
	return "data", nil
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
