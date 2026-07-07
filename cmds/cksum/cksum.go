// Package cksumcmd implements POSIX cksum(1).
package cksumcmd

import (
	"bufio"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
	sm3hash "github.com/tjfoc/gmsm/sm3"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/sha3"
	blake3hash "lukechampine.com/blake3"
)

var cmd = &tool.Tool{
	Name:     "cksum",
	Synopsis: "Print POSIX CRC checksum and byte count for each FILE. With no FILE, or when FILE is -, read standard input.",
	Usage:    "cksum [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	algorithm := fs.StringP("algorithm", "a", "crc", "select the digest type")
	tag := fs.Bool("tag", false, "create a BSD style checksum")
	untagged := fs.Bool("untagged", false, "create a reversed style checksum, without digest type")
	raw := fs.Bool("raw", false, "emit a raw binary digest, not hexadecimal")
	base64Flag := fs.Bool("base64", false, "emit base64-encoded digests, not hexadecimal")
	length := fs.IntP("length", "l", 512, "digest length in bits for blake2b")
	check := fs.BoolP("check", "c", false, "read checksums from FILEs and check them")
	warn := fs.BoolP("warn", "w", false, "warn about improperly formatted checksum lines")
	status := fs.Bool("status", false, "don't output anything, status code shows success")
	quiet := fs.Bool("quiet", false, "don't print OK for each successfully verified file")
	strict := fs.Bool("strict", false, "exit non-zero for improperly formatted checksum lines")
	ignoreMissing := fs.Bool("ignore-missing", false, "don't fail or report status for missing files")
	zero := fs.BoolP("zero", "z", false, "end each output line with NUL, not newline")
	debug := fs.Bool("debug", false, "print CPU hardware capability detection info")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if *raw && *base64Flag {
		return tool.UsageError(rc, cmd, "the --raw and --base64 options are mutually exclusive")
	}
	if *tag && *check {
		return tool.UsageError(rc, cmd, "the --tag option is meaningless when verifying checksums")
	}
	mode, err := parseAlgorithm(*algorithm, *length, fs.Changed("length"))
	if err != nil {
		return tool.UsageError(rc, cmd, "%s", err)
	}
	if (*raw || *base64Flag || *untagged) && (mode.kind == "crc" || mode.kind == "sum") {
		return tool.UsageError(rc, cmd, "output encoding options are only supported for digest algorithms")
	}
	if len(operands) == 0 {
		operands = []string{"-"}
	}
	if *debug {
		printDebug(rc)
	}
	if *check {
		opts := cksumCheckOptions{
			warn:          *warn,
			status:        *status,
			quiet:         *quiet,
			strict:        *strict,
			ignoreMissing: *ignoreMissing,
			base64:        *base64Flag,
			untagged:      *untagged,
			zero:          *zero,
		}
		exit := 0
		for _, name := range operands {
			if checkCKSumFile(rc, mode, name, opts) != 0 {
				exit = 1
			}
		}
		return exit
	}
	exit := 0
	for _, name := range operands {
		err := printCKSumOperand(rc, mode, name, *untagged, *raw, *base64Flag, *zero)
		if err != nil {
			fmt.Fprintf(rc.Err, "cksum: %s: %s\n", name, errMsg(err))
			exit = 1
		}
	}
	return exit
}

func printDebug(rc *tool.RunContext) {
	// The Go implementation does not dispatch to CPU-specific checksum
	// kernels, but uutils treats --debug as informational and still
	// computes the checksum. Keep that contract instead of failing.
	fmt.Fprintln(rc.Out, "hardware acceleration managed by Go runtime")
}

type cksumMode struct {
	kind     string
	label    string
	bits     int
	new      func() hash.Hash
	newShake func() sha3.ShakeHash
}

func parseAlgorithm(name string, length int, lengthSet bool) (cksumMode, error) {
	switch strings.ToLower(name) {
	case "crc", "cksum":
		return cksumMode{kind: "crc"}, nil
	case "bsd":
		return cksumMode{kind: "sum", label: "bsd"}, nil
	case "sysv":
		return cksumMode{kind: "sum", label: "sysv"}, nil
	case "md5":
		return cksumMode{kind: "digest", label: "MD5", bits: 128, new: md5.New}, nil
	case "sha1":
		return cksumMode{kind: "digest", label: "SHA1", bits: 160, new: sha1.New}, nil
	case "sha224":
		return cksumMode{kind: "digest", label: "SHA224", bits: 224, new: sha256.New224}, nil
	case "sha2", "sha256":
		return cksumMode{kind: "digest", label: "SHA256", bits: 256, new: sha256.New}, nil
	case "sha384":
		return cksumMode{kind: "digest", label: "SHA384", bits: 384, new: sha512.New384}, nil
	case "sha512":
		return cksumMode{kind: "digest", label: "SHA512", bits: 512, new: sha512.New}, nil
	case "crc32b":
		return cksumMode{kind: "digest", label: "CRC32B", bits: 32, new: func() hash.Hash {
			return crc32.NewIEEE()
		}}, nil
	case "sha3", "sha3-256", "sha3_256":
		return cksumMode{kind: "digest", label: "SHA3-256", bits: 256, new: sha3.New256}, nil
	case "sha3-224", "sha3_224":
		return cksumMode{kind: "digest", label: "SHA3-224", bits: 224, new: sha3.New224}, nil
	case "sha3-384", "sha3_384":
		return cksumMode{kind: "digest", label: "SHA3-384", bits: 384, new: sha3.New384}, nil
	case "sha3-512", "sha3_512":
		return cksumMode{kind: "digest", label: "SHA3-512", bits: 512, new: sha3.New512}, nil
	case "shake128":
		if length <= 0 || length%8 != 0 {
			return cksumMode{}, fmt.Errorf("invalid digest length: %d", length)
		}
		return cksumMode{kind: "digest", label: "SHAKE128", bits: length, newShake: sha3.NewShake128}, nil
	case "shake256":
		if length <= 0 || length%8 != 0 {
			return cksumMode{}, fmt.Errorf("invalid digest length: %d", length)
		}
		return cksumMode{kind: "digest", label: "SHAKE256", bits: length, newShake: sha3.NewShake256}, nil
	case "sm3":
		return cksumMode{kind: "digest", label: "SM3", bits: 256, new: sm3hash.New}, nil
	case "blake2b":
		if length <= 0 || length > 512 || length%8 != 0 {
			return cksumMode{}, fmt.Errorf("invalid digest length: %d", length)
		}
		return cksumMode{kind: "digest", label: "BLAKE2b", bits: length, new: func() hash.Hash {
			h, err := blake2b.New(length/8, nil)
			if err != nil {
				panic(err)
			}
			return h
		}}, nil
	case "blake3":
		if !lengthSet {
			length = 256
		}
		if length <= 0 || length > 1024 || length%8 != 0 {
			return cksumMode{}, fmt.Errorf("invalid digest length: %d", length)
		}
		return cksumMode{kind: "digest", label: fmt.Sprintf("BLAKE3-%d", length), bits: length, new: func() hash.Hash {
			return blake3hash.New(length/8, nil)
		}}, nil
	default:
		return cksumMode{}, fmt.Errorf("invalid algorithm: %s", name)
	}
}

func printCKSumOperand(rc *tool.RunContext, mode cksumMode, name string, untagged, raw, b64, zero bool) error {
	lineEnd := "\n"
	if zero {
		lineEnd = "\x00"
	}
	switch mode.kind {
	case "crc":
		crc, size, err := cksumOperand(rc, name)
		if err != nil {
			return err
		}
		if name == "-" {
			fmt.Fprintf(rc.Out, "%d %d%s", crc, size, lineEnd)
		} else {
			fmt.Fprintf(rc.Out, "%d %d %s%s", crc, size, name, lineEnd)
		}
	case "sum":
		result, err := legacySumOperand(rc, name, mode.label == "sysv")
		if err != nil {
			return err
		}
		width := 5
		if mode.label == "sysv" {
			width = 1
		}
		if name == "-" {
			fmt.Fprintf(rc.Out, "%0*d %*d%s", width, result.checksum, width, result.blocks, lineEnd)
		} else {
			fmt.Fprintf(rc.Out, "%0*d %*d %s%s", width, result.checksum, width, result.blocks, name, lineEnd)
		}
	case "digest":
		sum, err := digestOperand(rc, mode, name)
		if err != nil {
			return err
		}
		if raw {
			_, err = rc.Out.Write(sum)
			return err
		}
		encoded := hex.EncodeToString(sum)
		if b64 {
			encoded = base64.StdEncoding.EncodeToString(sum)
		}
		if untagged {
			fmt.Fprintf(rc.Out, "%s  %s%s", encoded, name, lineEnd)
		} else {
			fmt.Fprintf(rc.Out, "%s (%s) = %s%s", mode.label, name, encoded, lineEnd)
		}
	}
	return nil
}

func digestOperand(rc *tool.RunContext, mode cksumMode, name string) ([]byte, error) {
	if mode.newShake != nil {
		h := mode.newShake()
		if err := copyOperandToHash(rc, name, h); err != nil {
			return nil, err
		}
		sum := make([]byte, mode.bits/8)
		_, _ = h.Read(sum)
		return sum, nil
	}
	h := mode.new()
	if err := copyOperandToHash(rc, name, h); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func copyOperandToHash(rc *tool.RunContext, name string, h hash.Hash) error {
	if name == "-" {
		if rc.In != nil {
			if _, err := io.Copy(h, rc.In); err != nil {
				return err
			}
		}
		return nil
	}
	f, err := os.Open(rc.Path(name))
	if err != nil {
		return err
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.IsDir() {
		return errIsDirectory
	}
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	return nil
}

type cksumCheckOptions struct {
	warn          bool
	status        bool
	quiet         bool
	strict        bool
	ignoreMissing bool
	base64        bool
	untagged      bool
	zero          bool
}

func checkCKSumFile(rc *tool.RunContext, mode cksumMode, op string, opts cksumCheckOptions) int {
	var r io.Reader
	isStdin := op == "-"
	if isStdin {
		r = rc.In
		if r == nil {
			r = strings.NewReader("")
		}
	} else {
		f, err := os.Open(rc.Path(op))
		if err != nil {
			fmt.Fprintf(rc.Err, "cksum: %s: %s\n", op, errMsg(err))
			return 1
		}
		defer f.Close()
		r = f
	}

	br := bufio.NewReader(r)
	delim := byte('\n')
	if opts.zero {
		delim = 0
	}
	var valid, badFormat, mismatched, unreadable int
	exit := 0
	for {
		line, rerr := br.ReadString(delim)
		l := strings.TrimSuffix(line, string(delim))
		if l != "" && !strings.HasPrefix(l, "#") {
			entry, ok := parseCKSumCheckLine(mode, l, opts)
			if !ok || (isStdin && entry.path == "-") {
				badFormat++
			} else {
				valid++
				match, err := verifyCKSumEntry(rc, mode, entry, opts)
				switch {
				case err != nil:
					if !(opts.ignoreMissing && errors.Is(err, fs.ErrNotExist)) {
						if !opts.status {
							fmt.Fprintf(rc.Err, "cksum: %s: %s\n", entry.display, errMsg(err))
							fmt.Fprintf(rc.Out, "%s: FAILED open or read\n", entry.display)
						}
						unreadable++
						exit = 1
					}
				case match:
					if !opts.status && !opts.quiet {
						fmt.Fprintf(rc.Out, "%s: OK\n", entry.display)
					}
				default:
					if !opts.status {
						fmt.Fprintf(rc.Out, "%s: FAILED\n", entry.display)
					}
					mismatched++
					exit = 1
				}
			}
		}
		if rerr != nil {
			break
		}
	}

	if valid == 0 {
		if !opts.status {
			fmt.Fprintf(rc.Err, "cksum: %s: no properly formatted checksum lines found\n", op)
		}
		return 1
	}
	if badFormat > 0 && !opts.status {
		fmt.Fprintf(rc.Err, "cksum: WARNING: %d %s\n", badFormat,
			plural(badFormat, "line is improperly formatted", "lines are improperly formatted"))
	}
	if badFormat > 0 && opts.strict {
		exit = 1
	}
	if mismatched > 0 && !opts.status {
		fmt.Fprintf(rc.Err, "cksum: WARNING: %d %s\n", mismatched,
			plural(mismatched, "computed checksum did NOT match", "computed checksums did NOT match"))
	}
	if unreadable > 0 && !opts.status {
		fmt.Fprintf(rc.Err, "cksum: WARNING: %d %s\n", unreadable,
			plural(unreadable, "listed file could not be read", "listed files could not be read"))
	}
	return exit
}

type cksumCheckEntry struct {
	path     string
	display  string
	digest   string
	checksum uint64
	size     uint64
}

func parseCKSumCheckLine(mode cksumMode, line string, opts cksumCheckOptions) (cksumCheckEntry, bool) {
	switch mode.kind {
	case "crc", "sum":
		first, second, rest, ok := splitCKSumFields(line)
		if !ok {
			return cksumCheckEntry{}, false
		}
		checksum, err1 := strconv.ParseUint(first, 10, 64)
		size, err2 := strconv.ParseUint(second, 10, 64)
		if err1 != nil || err2 != nil {
			return cksumCheckEntry{}, false
		}
		return cksumCheckEntry{path: rest, display: rest, checksum: checksum, size: size}, true
	case "digest":
		return parseDigestCheckLine(mode, line, opts)
	default:
		return cksumCheckEntry{}, false
	}
}

func splitCKSumFields(line string) (first, second, rest string, ok bool) {
	line = strings.TrimLeft(line, " \t")
	i := strings.IndexAny(line, " \t")
	if i < 0 {
		return "", "", "", false
	}
	first = line[:i]
	line = strings.TrimLeft(line[i:], " \t")
	i = strings.IndexAny(line, " \t")
	if i < 0 {
		return "", "", "", false
	}
	second = line[:i]
	rest = strings.TrimLeft(line[i:], " \t")
	return first, second, rest, rest != ""
}

func parseDigestCheckLine(mode cksumMode, line string, opts cksumCheckOptions) (cksumCheckEntry, bool) {
	l := line
	if strings.HasPrefix(l, "\\") {
		l = l[1:]
	}
	if !opts.untagged {
		prefix := mode.label + " ("
		if rest, ok := strings.CutPrefix(l, prefix); ok {
			if i := strings.LastIndex(rest, ") = "); i >= 0 {
				name, d := rest[:i], rest[i+4:]
				if name != "" && validDigestText(d, mode.bits, opts.base64) {
					return cksumCheckEntry{path: name, display: name, digest: d}, true
				}
			}
			return cksumCheckEntry{}, false
		}
	}
	i := strings.IndexByte(l, ' ')
	if i < 0 {
		return cksumCheckEntry{}, false
	}
	d := l[:i]
	if !validDigestText(d, mode.bits, opts.base64) {
		return cksumCheckEntry{}, false
	}
	rest := l[i+1:]
	var name string
	switch {
	case strings.HasPrefix(rest, " "), strings.HasPrefix(rest, "*"):
		name = rest[1:]
	default:
		name = rest
	}
	if name == "" {
		return cksumCheckEntry{}, false
	}
	return cksumCheckEntry{path: name, display: name, digest: d}, true
}

func validDigestText(s string, bits int, b64 bool) bool {
	if b64 {
		_, err := base64.StdEncoding.DecodeString(s)
		return err == nil
	}
	return isHexN(s, bits/4)
}

func verifyCKSumEntry(rc *tool.RunContext, mode cksumMode, entry cksumCheckEntry, opts cksumCheckOptions) (bool, error) {
	switch mode.kind {
	case "crc":
		crc, size, err := cksumOperand(rc, entry.path)
		if err != nil {
			return false, err
		}
		return uint64(crc) == entry.checksum && size == entry.size, nil
	case "sum":
		result, err := legacySumOperand(rc, entry.path, mode.label == "sysv")
		if err != nil {
			return false, err
		}
		return uint64(result.checksum) == entry.checksum && result.blocks == entry.size, nil
	case "digest":
		sum, err := digestOperand(rc, mode, entry.path)
		if err != nil {
			return false, err
		}
		got := hex.EncodeToString(sum)
		if opts.base64 {
			got = base64.StdEncoding.EncodeToString(sum)
			return got == entry.digest, nil
		}
		return strings.EqualFold(got, entry.digest), nil
	default:
		return false, nil
	}
}

func isHexN(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func cksumOperand(rc *tool.RunContext, name string) (uint32, uint64, error) {
	var r io.Reader
	if name == "-" {
		if rc.In == nil {
			r = strings.NewReader("")
		} else {
			r = rc.In
		}
	} else {
		f, err := os.Open(rc.Path(name))
		if err != nil {
			return 0, 0, err
		}
		defer f.Close()
		if fi, err := f.Stat(); err == nil && fi.IsDir() {
			return 0, 0, errIsDirectory
		}
		r = f
	}
	var crc uint32
	var n uint64
	buf := make([]byte, 32*1024)
	for {
		got, err := r.Read(buf)
		for _, b := range buf[:got] {
			crc = (crc << 8) ^ cksumTable[((crc>>24)^uint32(b))&0xff]
		}
		n += uint64(got)
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, 0, err
		}
	}
	for length := n; length != 0; length >>= 8 {
		crc = (crc << 8) ^ cksumTable[((crc>>24)^uint32(length&0xff))&0xff]
	}
	return ^crc, n, nil
}

type legacySumResult struct {
	checksum uint16
	blocks   uint64
}

func legacySumOperand(rc *tool.RunContext, name string, sysv bool) (legacySumResult, error) {
	var r io.Reader
	if name == "-" {
		if rc.In == nil {
			r = strings.NewReader("")
		} else {
			r = rc.In
		}
	} else {
		f, err := os.Open(rc.Path(name))
		if err != nil {
			return legacySumResult{}, err
		}
		defer f.Close()
		if fi, err := f.Stat(); err == nil && fi.IsDir() {
			return legacySumResult{}, errIsDirectory
		}
		r = f
	}
	if sysv {
		return legacySysvSum(r)
	}
	return legacyBSDSum(r)
}

func legacyBSDSum(r io.Reader) (legacySumResult, error) {
	var checksum uint16
	var size uint64
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		for _, b := range buf[:n] {
			checksum = (checksum >> 1) + ((checksum & 1) << 15) + uint16(b)
		}
		size += uint64(n)
		if err == io.EOF {
			break
		}
		if err != nil {
			return legacySumResult{}, err
		}
	}
	return legacySumResult{checksum: checksum, blocks: legacyBlocks(size, 1024)}, nil
}

func legacySysvSum(r io.Reader) (legacySumResult, error) {
	var checksum uint32
	var size uint64
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		for _, b := range buf[:n] {
			checksum += uint32(b)
		}
		size += uint64(n)
		if err == io.EOF {
			break
		}
		if err != nil {
			return legacySumResult{}, err
		}
	}
	checksum = (checksum & 0xffff) + (checksum >> 16)
	checksum = (checksum & 0xffff) + (checksum >> 16)
	return legacySumResult{checksum: uint16(checksum), blocks: legacyBlocks(size, 512)}, nil
}

func legacyBlocks(size, block uint64) uint64 {
	if size == 0 {
		return 0
	}
	return (size + block - 1) / block
}

var errIsDirectory = fmt.Errorf("Is a directory")

func errMsg(err error) string {
	if err == errIsDirectory {
		return err.Error()
	}
	if os.IsNotExist(err) {
		return "No such file or directory"
	}
	if os.IsPermission(err) {
		return "Permission denied"
	}
	return err.Error()
}

var cksumTable = makeCKSumTable()

func makeCKSumTable() [256]uint32 {
	const poly uint32 = 0x04c11db7
	var tab [256]uint32
	for i := range tab {
		crc := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ poly
			} else {
				crc <<= 1
			}
		}
		tab[i] = crc
	}
	return tab
}
