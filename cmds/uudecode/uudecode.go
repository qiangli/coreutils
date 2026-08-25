// Package uudecodecmd implements decoding of the portable uuencode formats.
// Header output names follow ordinary utility pathname semantics.
package uudecodecmd

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "uudecode",
	Synopsis: "Decode a uuencoded file.",
	Usage: "uudecode [OPTION]... [FILE]...\n\n" +
		"Decode traditional or begin-base64 data from FILEs, or standard input.\n\n" +
		"  -o, --output-file=FILE  write to FILE instead of the header name\n\n" +
		"Header output names use ordinary pathname semantics.",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type header struct {
	name   string
	mode   os.FileMode
	base64 bool
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	output := fs.StringP("output-file", "o", "", "write to FILE instead of the header name")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if envPresent(rc.Env, "POSIXLY_CORRECT") && len(operands) > 1 {
		return tool.UsageError(rc, cmd, "extra operand %q", operands[1])
	}
	if *output != "" && len(operands) > 1 {
		return tool.UsageError(rc, cmd, "--output-file cannot be used with multiple input files")
	}
	if len(operands) == 0 {
		if decodeInput(rc, "standard input", rc.In, *output) {
			return 0
		}
		return 1
	}

	failed := false
	for _, operand := range operands {
		f, err := os.Open(rc.Path(operand))
		if err != nil {
			fmt.Fprintf(rc.Err, "uudecode: %s: %v\n", operand, err)
			failed = true
			continue
		}
		ok := decodeInput(rc, operand, f, *output)
		if err := f.Close(); err != nil {
			fmt.Fprintf(rc.Err, "uudecode: %s: %v\n", operand, err)
			ok = false
		}
		if !ok {
			failed = true
		}
	}
	if failed {
		return 1
	}
	return 0
}

func envPresent(env []string, key string) bool {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return true
		}
	}
	return false
}

// decodeInput scans the entire input because mailboxes commonly contain more
// than one encoded part, separated by arbitrary leading or trailing text.
func decodeInput(rc *tool.RunContext, source string, input io.Reader, override string) bool {
	r := bufio.NewReader(input)
	found := false
	for {
		line, err := readLine(r)
		if err != nil && err != io.EOF {
			fmt.Fprintf(rc.Err, "uudecode: %s: read error: %v\n", source, err)
			return false
		}
		if h, ok, parseErr := parseHeader(line); ok || parseErr != nil {
			if parseErr != nil {
				fmt.Fprintf(rc.Err, "uudecode: %s\n", parseErr)
				return false
			}
			found = true
			if !decodePart(rc, r, h, override) {
				return false
			}
		}
		if err == io.EOF {
			break
		}
	}
	if !found {
		fmt.Fprintf(rc.Err, "uudecode: %s: no 'begin' line\n", source)
	}
	return found
}

func parseHeader(line string) (header, bool, error) {
	base64Header := strings.HasPrefix(line, "begin-base64 ")
	classicHeader := strings.HasPrefix(line, "begin ")
	if !base64Header && !classicHeader {
		if strings.HasPrefix(line, "begin-") {
			return header{}, false, fmt.Errorf("unsupported begin header")
		}
		return header{}, false, nil
	}
	prefix := "begin "
	if base64Header {
		prefix = "begin-base64 "
	}
	parts := strings.SplitN(strings.TrimPrefix(line, prefix), " ", 2)
	if len(parts) != 2 || parts[1] == "" {
		return header{}, false, fmt.Errorf("malformed 'begin' line")
	}
	mode, err := parseHeaderMode(parts[0])
	if err != nil {
		return header{}, false, fmt.Errorf("malformed 'begin' line")
	}
	return header{name: parts[1], mode: mode, base64: base64Header}, true, nil
}

func decodePart(rc *tool.RunContext, r *bufio.Reader, h header, override string) bool {
	name := h.name
	toStdout := h.name == "-" || h.name == "/dev/stdout"
	if override != "" {
		name = override
		// POSIX makes /dev/stdout the sole special -o value. In particular,
		// `-o -` is an ordinary pathname, even though a header pathname of `-`
		// selects standard output.
		toStdout = override == "/dev/stdout"
	}

	decode := func(w io.Writer) error {
		if h.base64 {
			return decodeBase64(r, w)
		}
		return decodeClassic(r, w)
	}
	if toStdout {
		if err := decode(rc.Out); err != nil {
			fmt.Fprintf(rc.Err, "uudecode: %v\n", err)
			return false
		}
		return true
	}
	chmodErr, err := decodeOutput(rc.Path(name), h.mode, decode)
	if err != nil {
		fmt.Fprintf(rc.Err, "uudecode: %s: %v\n", name, err)
		return false
	}
	if chmodErr != nil {
		// POSIX explicitly makes inability to set the requested access bits
		// non-fatal: the decoded content remains the useful primary result.
		fmt.Fprintf(rc.Err, "uudecode: %s: warning: cannot set permissions: %v\n", name, chmodErr)
	}
	return true
}

var chmodDecodedFile = func(file *os.File, mode os.FileMode) error { return file.Chmod(mode) }

func decodeOutput(path string, mode os.FileMode, decode func(io.Writer) error) (chmodErr, err error) {
	path, err = resolveOutputPath(path)
	if err != nil {
		return nil, err
	}
	info, statErr := os.Stat(path)
	if statErr == nil && info.Mode().IsRegular() {
		return decodeExistingRegular(path, mode, decode)
	}
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, statErr
	}
	if statErr == nil {
		// Never open an existing FIFO or device merely to check access: that
		// can block or trigger device side effects. The platform check is
		// metadata-only; a writable special file is replaced atomically.
		if err := checkOverwrite(path); err != nil {
			return nil, err
		}
	}
	return decodeNewAtomically(path, mode, decode)
}

// decodeExistingRegular overwrites the existing file object rather than its
// directory entry. This preserves hard links and requires write permission on
// the file, but not write permission on its parent directory. The platform
// opener verifies that the held descriptor is still a regular file before it
// is truncated, avoiding FIFO/device blocking in the checked path.
func decodeExistingRegular(path string, mode os.FileMode, decode func(io.Writer) error) (chmodErr, err error) {
	file, err := openWritableRegular(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if file != nil {
			_ = file.Close()
		}
	}()
	// Decode before truncating the destination. In addition to leaving a valid
	// existing file intact on malformed input, this is required when an input
	// operand names the same file as the decoded output: the reader must be able
	// to consume the complete encoded stream before that file is overwritten.
	// Use the system temporary directory because POSIX only requires write
	// permission on an existing output file, not on its parent directory.
	staged, err := os.CreateTemp("", "uudecode-*")
	if err != nil {
		return nil, err
	}
	stagedName := staged.Name()
	defer func() {
		_ = staged.Close()
		_ = os.Remove(stagedName)
	}()
	if err := decode(staged); err != nil {
		return nil, err
	}
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if err := file.Truncate(0); err != nil {
		return nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if _, err := io.Copy(file, staged); err != nil {
		return nil, err
	}
	chmodErr = chmodDecodedFile(file, mode.Perm())
	if err := file.Close(); err != nil {
		return chmodErr, err
	}
	file = nil
	return chmodErr, nil
}

func decodeNewAtomically(path string, mode os.FileMode, decode func(io.Writer) error) (chmodErr, err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".uudecode-*")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmp != nil {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpName)
	}()
	if err := decode(tmp); err != nil {
		return nil, err
	}
	chmodErr = chmodDecodedFile(tmp, mode.Perm())
	if err := tmp.Close(); err != nil {
		return chmodErr, err
	}
	tmp = nil
	return chmodErr, os.Rename(tmpName, path)
}

// resolveOutputPath follows a final symlink because POSIX describes overwrite
// behavior in terms of the file to which the pathname resolves. It also
// resolves a dangling final link to its intended creation pathname. No target
// is opened, so FIFOs and devices cannot block or trigger open side effects.
func resolveOutputPath(path string) (string, error) {
	current := filepath.Clean(path)
	for links := 0; links < 255; links++ {
		info, err := os.Lstat(current)
		if err != nil {
			if !os.IsNotExist(err) {
				return "", err
			}
			// Resolve existing symlinks in the parent even when the leaf does
			// not exist, so rename creates the file reached by the pathname.
			if parent, parentErr := filepath.EvalSymlinks(filepath.Dir(current)); parentErr == nil {
				current = filepath.Join(parent, filepath.Base(current))
			}
			return current, nil
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return current, nil
		}
		target, err := os.Readlink(current)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(current), target)
		}
		current = filepath.Clean(target)
	}
	return "", fmt.Errorf("too many symbolic links in output pathname %q", path)
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), err
}

func decodeClassic(r *bufio.Reader, out io.Writer) error {
	for {
		line, err := readLine(r)
		if err != nil && err != io.EOF {
			return fmt.Errorf("read error: %w", err)
		}
		if line == "end" {
			return fmt.Errorf("malformed data: missing zero-length line before 'end'")
		}
		if line == "" && err == io.EOF {
			return fmt.Errorf("short file")
		}
		if line == "" {
			return fmt.Errorf("malformed empty data line")
		}
		n := int(dec(line[0]))
		if n == 0 {
			next, nextErr := readLine(r)
			if nextErr != nil && nextErr != io.EOF {
				return fmt.Errorf("read error: %w", nextErr)
			}
			if next != "end" {
				return fmt.Errorf("malformed data: expected 'end'")
			}
			return nil
		}
		if n > 45 {
			return fmt.Errorf("invalid data line length %d", n)
		}
		need := ((n + 2) / 3) * 4
		if len(line)-1 < need {
			return fmt.Errorf("short data line")
		}
		decoded := make([]byte, 0, n)
		for i := 1; i <= need; i += 4 {
			for j := 0; j < 4; j++ {
				c := line[i+j]
				if c < 0x20 || c > 0x60 {
					return fmt.Errorf("invalid encoded character 0x%02x", c)
				}
			}
			v := uint32(dec(line[i]))<<18 | uint32(dec(line[i+1]))<<12 | uint32(dec(line[i+2]))<<6 | uint32(dec(line[i+3]))
			decoded = append(decoded, byte(v>>16), byte(v>>8), byte(v))
		}
		if _, err := out.Write(decoded[:n]); err != nil {
			return fmt.Errorf("write error: %w", err)
		}
		if err == io.EOF {
			return fmt.Errorf("short file")
		}
	}
}

func decodeBase64(r *bufio.Reader, out io.Writer) error {
	var quantum [4]byte
	n := 0
	padded := false
	for {
		line, err := readLine(r)
		if err != nil && err != io.EOF {
			return fmt.Errorf("read error: %w", err)
		}
		if line == "====" {
			if n != 0 {
				return fmt.Errorf("malformed base64 data")
			}
			return nil
		}
		for i := 0; i < len(line); i++ {
			c := line[i]
			if !base64Char(c) {
				continue // POSIX requires non-base64 characters to be ignored.
			}
			if padded {
				return fmt.Errorf("malformed base64 data")
			}
			quantum[n] = c
			n++
			if n != len(quantum) {
				continue
			}
			var decoded [3]byte
			count, decErr := base64.StdEncoding.Decode(decoded[:], quantum[:])
			if decErr != nil {
				return fmt.Errorf("malformed base64 data")
			}
			if _, writeErr := out.Write(decoded[:count]); writeErr != nil {
				return fmt.Errorf("write error: %w", writeErr)
			}
			padded = quantum[2] == '=' || quantum[3] == '='
			n = 0
		}
		if err == io.EOF {
			return fmt.Errorf("short base64 data")
		}
	}
}

func base64Char(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '/' || c == '='
}

func dec(b byte) byte { return (b - 0x20) & 0x3f }
