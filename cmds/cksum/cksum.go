// Package cksumcmd implements POSIX cksum(1) plus GNU's -a digest
// multiplexer. GNU semantics per coreutils 9.11: crc/crc32b/bsd/sysv
// print decimal checksums (never tagged digests), --check verifies
// only digest algorithms (auto-detected per line from the BSD tag
// when -a is not given), sha2/sha3 require --length outside check
// mode, and blake2b tags carry a length suffix when not 512.
package cksumcmd

import (
	"bufio"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
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

	"github.com/qiangli/coreutils/cmds/internal/hashenc"
	"github.com/qiangli/coreutils/tool"
	sm3hash "github.com/tjfoc/gmsm/sm3"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/sha3"
	blake3hash "lukechampine.com/blake3"
)

var cmd = &tool.Tool{
	Name:     "cksum",
	Synopsis: "Print POSIX CRC checksum and byte count for each FILE. With no FILE, or when FILE is -, read standard input.",
	Usage: "cksum [OPTION]... [FILE]...\n\n" +
		"  -r          use BSD sum algorithm\n" +
		"  -s          use System V sum algorithm",
}

func init() { cmd.Run = run; tool.Register(cmd) }

// digestFamily groups algorithms that share --length / tag-suffix
// rules.
type digestFamily int

const (
	famNone    digestFamily = iota // crc, crc32b, bsd, sysv
	famFixed                       // md5, sha1, sha224..sha512, sm3, extensions
	famSHA2                        // -a sha2 (length selects the member)
	famSHA3                        // -a sha3 (length selects the member)
	famBLAKE2B                     // -a blake2b (any multiple of 8 up to 512)
	famSHAKE                       // shake128/shake256 extension
	famBLAKE3                      // blake3 extension
)

type cksumMode struct {
	kind    string // "crc", "crc32b", "sum", "digest"
	family  digestFamily
	name    string // the --algorithm value as given
	label   string // base tag token, e.g. "SHA3", "BLAKE2b"
	bits    int    // resolved digest size; 0 = auto-detect (check mode)
	sysv    bool
	lenAlgo bool // accepts --length
	mk      func(bits int) hash.Hash
	shake   func() sha3.ShakeHash
}

// tagLabel is the BSD tag this mode writes with --tag output.
func (m cksumMode) tagLabel() string {
	switch m.family {
	case famSHA2:
		return fmt.Sprintf("SHA%d", m.bits)
	case famSHA3:
		return fmt.Sprintf("SHA3-%d", m.bits)
	case famBLAKE2B:
		if m.bits < 512 {
			return fmt.Sprintf("BLAKE2b-%d", m.bits)
		}
		return "BLAKE2b"
	case famBLAKE3:
		return fmt.Sprintf("BLAKE3-%d", m.bits)
	}
	return m.label
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	algorithm := fs.StringP("algorithm", "a", "crc", "select the digest type")
	tag := fs.Bool("tag", false, "create a BSD style checksum (the default for digest algorithms)")
	untagged := fs.Bool("untagged", false, "create a reversed style checksum, without digest type")
	raw := fs.Bool("raw", false, "emit a raw binary digest, not hexadecimal")
	base64Flag := fs.Bool("base64", false, "emit base64-encoded digests, not hexadecimal")
	length := fs.IntP("length", "l", 0, "digest length in bits; must not exceed the max size and must be a multiple of 8 for blake2b; must be 224, 256, 384, or 512 for sha2 or sha3")
	check := fs.BoolP("check", "c", false, "read checksums from FILEs and check them")
	warn := fs.BoolP("warn", "w", false, "warn about improperly formatted checksum lines")
	status := fs.Bool("status", false, "don't output anything, status code shows success")
	quiet := fs.Bool("quiet", false, "don't print OK for each successfully verified file")
	strict := fs.Bool("strict", false, "exit non-zero for improperly formatted checksum lines")
	ignoreMissing := fs.Bool("ignore-missing", false, "don't fail or report status for missing files")
	zero := fs.BoolP("zero", "z", false, "end each output line with NUL, not newline")
	debug := fs.Bool("debug", false, "indicate which implementation used")
	operands, code := tool.Parse(rc, cmd, fs, rewriteLegacyAlgorithmAliases(args))
	if code >= 0 {
		return code
	}
	algorithmSpecified := fs.Changed("algorithm")
	lengthSet := fs.Changed("length")
	lv := *length
	if *raw && *base64Flag {
		return tool.UsageError(rc, cmd, "--base64 and --raw are mutually exclusive")
	}
	if *tag && *check {
		return tool.UsageError(rc, cmd, "the --tag option is meaningless when verifying checksums")
	}
	if *zero && *check {
		return tool.UsageError(rc, cmd, "the --zero option is not supported when verifying checksums")
	}
	if !*check {
		switch {
		case *ignoreMissing:
			return tool.UsageError(rc, cmd, "the --ignore-missing option is meaningful only when verifying checksums")
		case *status:
			return tool.UsageError(rc, cmd, "the --status option is meaningful only when verifying checksums")
		case *warn:
			return tool.UsageError(rc, cmd, "the --warn option is meaningful only when verifying checksums")
		case *quiet:
			return tool.UsageError(rc, cmd, "the --quiet option is meaningful only when verifying checksums")
		case *strict:
			return tool.UsageError(rc, cmd, "the --strict option is meaningful only when verifying checksums")
		}
	}
	mode, err := parseAlgorithm(*algorithm)
	if err != nil {
		return tool.UsageError(rc, cmd, "%s", err)
	}
	if lv != 0 && !mode.lenAlgo {
		return tool.UsageError(rc, cmd, "--length is only supported with --algorithm blake2b, sha2, or sha3")
	}
	if lv < 0 {
		return tool.UsageError(rc, cmd, "invalid length: '%d'", lv)
	}
	switch mode.family {
	case famSHA2, famSHA3:
		if !*check && !lengthSet {
			return tool.UsageError(rc, cmd, "--algorithm=%s requires specifying --length 224, 256, 384, or 512", mode.name)
		}
		if !*check || lengthSet {
			if !validSHA2Len(lv) {
				fmt.Fprintf(rc.Err, "cksum: invalid length: '%d'\n", lv)
				return tool.UsageError(rc, cmd, "digest length for '%s' must be 224, 256, 384, or 512", mode.label)
			}
		}
		if *check {
			mode.bits = 0 // auto-detected per line
		} else {
			mode.bits = lv
		}
	case famBLAKE2B:
		if lv > 512 {
			fmt.Fprintf(rc.Err, "cksum: invalid length: '%d'\n", lv)
			return tool.UsageError(rc, cmd, "maximum digest length for 'BLAKE2b' is 512 bits")
		}
		if lv%8 != 0 {
			fmt.Fprintf(rc.Err, "cksum: invalid length: '%d'\n", lv)
			return tool.UsageError(rc, cmd, "length is not a multiple of 8")
		}
		switch {
		case *check:
			mode.bits = 0 // auto-detected per line
		case lv == 0:
			mode.bits = 512
		default:
			mode.bits = lv
		}
	case famSHAKE:
		if lv == 0 {
			lv = 512
		}
		if lv%8 != 0 {
			return tool.UsageError(rc, cmd, "invalid digest length: %d", lv)
		}
		mode.bits = lv
	case famBLAKE3:
		if lv == 0 {
			lv = 256
		}
		if lv > 1024 || lv%8 != 0 {
			return tool.UsageError(rc, cmd, "invalid digest length: %d", lv)
		}
		mode.bits = lv
	}

	if *debug {
		printDebug(rc)
	}

	if *check {
		if algorithmSpecified && mode.kind != "digest" {
			return tool.UsageError(rc, cmd, "--check is not supported with --algorithm={bsd,sysv,crc,crc32b}")
		}
		if len(operands) == 0 {
			operands = []string{"-"}
		}
		opts := cksumCheckOptions{
			warn:          *warn,
			status:        *status,
			quiet:         *quiet,
			strict:        *strict,
			ignoreMissing: *ignoreMissing,
		}
		exit := 0
		for _, name := range operands {
			if checkCKSumFile(rc, mode, !algorithmSpecified, name, opts) != 0 {
				exit = 1
			}
		}
		return exit
	}

	withName := len(operands) > 0
	if !withName {
		operands = []string{"-"}
	} else if *raw && len(operands) > 1 {
		return tool.UsageError(rc, cmd, "the --raw option is not supported with multiple files")
	}
	exit := 0
	for _, name := range operands {
		err := printCKSumOperand(rc, mode, name, withName, *untagged, *raw, *base64Flag, *zero)
		if err != nil {
			fmt.Fprintf(rc.Err, "cksum: %s: %s\n", name, hashenc.GNUErrMsg(err))
			exit = 1
		}
	}
	return exit
}

func rewriteLegacyAlgorithmAliases(args []string) []string {
	out := make([]string, 0, len(args))
	rest := false
	for _, arg := range args {
		if rest {
			out = append(out, arg)
			continue
		}
		if arg == "--" {
			rest = true
			out = append(out, arg)
			continue
		}
		if len(arg) <= 1 || arg[0] != '-' || arg[1] == '-' {
			out = append(out, arg)
			continue
		}
		kept := "-"
		for i := 1; i < len(arg); i++ {
			switch arg[i] {
			case 'r':
				out = append(out, "--algorithm=bsd")
			case 's':
				out = append(out, "--algorithm=sysv")
			case 'c', 'w', 'z', 'h', 'V':
				kept += string(arg[i])
			default:
				kept += arg[i:]
				i = len(arg)
			}
		}
		if kept != "-" {
			out = append(out, kept)
		}
	}
	return out
}

func printDebug(rc *tool.RunContext) {
	// The Go implementation does not dispatch to CPU-specific checksum
	// kernels; like GNU's --debug this is informational, printed to
	// stderr, and the checksum is still computed.
	fmt.Fprintln(rc.Err, "cksum: hardware acceleration managed by Go runtime")
}

func validSHA2Len(n int) bool {
	return n == 224 || n == 256 || n == 384 || n == 512
}

func sha2New(bits int) hash.Hash {
	switch bits {
	case 224:
		return sha256.New224()
	case 256:
		return sha256.New()
	case 384:
		return sha512.New384()
	default:
		return sha512.New()
	}
}

func sha3New(bits int) hash.Hash {
	switch bits {
	case 224:
		return sha3.New224()
	case 256:
		return sha3.New256()
	case 384:
		return sha3.New384()
	default:
		return sha3.New512()
	}
}

func blake2bNew(bits int) hash.Hash {
	h, err := blake2b.New(bits/8, nil)
	if err != nil {
		panic(err)
	}
	return h
}

// parseAlgorithm resolves an --algorithm value. GNU's own values are
// matched exactly (GNU uses exact argmatch); the extension values
// (blake3, shake128/256, the sha3-NNN spellings, cksum) stay
// case-insensitive.
func parseAlgorithm(name string) (cksumMode, error) {
	switch name {
	case "crc":
		return cksumMode{kind: "crc", family: famNone, name: name, label: "CRC"}, nil
	case "crc32b":
		return cksumMode{kind: "crc32b", family: famNone, name: name, label: "CRC32B"}, nil
	case "bsd":
		return cksumMode{kind: "sum", family: famNone, name: name, label: "BSD"}, nil
	case "sysv":
		return cksumMode{kind: "sum", family: famNone, name: name, label: "SYSV", sysv: true}, nil
	case "md5":
		return cksumMode{kind: "digest", family: famFixed, name: name, label: "MD5", bits: 128, mk: func(int) hash.Hash { return md5.New() }}, nil
	case "sha1":
		return cksumMode{kind: "digest", family: famFixed, name: name, label: "SHA1", bits: 160, mk: func(int) hash.Hash { return sha1.New() }}, nil
	case "sha224":
		return cksumMode{kind: "digest", family: famFixed, name: name, label: "SHA224", bits: 224, mk: func(int) hash.Hash { return sha256.New224() }}, nil
	case "sha256":
		return cksumMode{kind: "digest", family: famFixed, name: name, label: "SHA256", bits: 256, mk: func(int) hash.Hash { return sha256.New() }}, nil
	case "sha384":
		return cksumMode{kind: "digest", family: famFixed, name: name, label: "SHA384", bits: 384, mk: func(int) hash.Hash { return sha512.New384() }}, nil
	case "sha512":
		return cksumMode{kind: "digest", family: famFixed, name: name, label: "SHA512", bits: 512, mk: func(int) hash.Hash { return sha512.New() }}, nil
	case "sha2":
		return cksumMode{kind: "digest", family: famSHA2, name: name, label: "SHA2", lenAlgo: true, mk: sha2New}, nil
	case "sha3":
		return cksumMode{kind: "digest", family: famSHA3, name: name, label: "SHA3", lenAlgo: true, mk: sha3New}, nil
	case "blake2b":
		return cksumMode{kind: "digest", family: famBLAKE2B, name: name, label: "BLAKE2b", lenAlgo: true, mk: blake2bNew}, nil
	case "sm3":
		return cksumMode{kind: "digest", family: famFixed, name: name, label: "SM3", bits: 256, mk: func(int) hash.Hash { return sm3hash.New() }}, nil
	}
	// Extensions beyond GNU's set (lenient, case-insensitive).
	switch strings.ToLower(name) {
	case "cksum":
		return cksumMode{kind: "crc", family: famNone, name: name, label: "CRC"}, nil
	case "sha3-224", "sha3_224":
		return cksumMode{kind: "digest", family: famFixed, name: name, label: "SHA3-224", bits: 224, mk: func(int) hash.Hash { return sha3.New224() }}, nil
	case "sha3-256", "sha3_256":
		return cksumMode{kind: "digest", family: famFixed, name: name, label: "SHA3-256", bits: 256, mk: func(int) hash.Hash { return sha3.New256() }}, nil
	case "sha3-384", "sha3_384":
		return cksumMode{kind: "digest", family: famFixed, name: name, label: "SHA3-384", bits: 384, mk: func(int) hash.Hash { return sha3.New384() }}, nil
	case "sha3-512", "sha3_512":
		return cksumMode{kind: "digest", family: famFixed, name: name, label: "SHA3-512", bits: 512, mk: func(int) hash.Hash { return sha3.New512() }}, nil
	case "shake128":
		return cksumMode{kind: "digest", family: famSHAKE, name: name, label: "SHAKE128", lenAlgo: true, shake: sha3.NewShake128}, nil
	case "shake256":
		return cksumMode{kind: "digest", family: famSHAKE, name: name, label: "SHAKE256", lenAlgo: true, shake: sha3.NewShake256}, nil
	case "blake3":
		return cksumMode{kind: "digest", family: famBLAKE3, name: name, label: "BLAKE3", lenAlgo: true, mk: func(bits int) hash.Hash {
			return blake3hash.New(bits/8, nil)
		}}, nil
	}
	return cksumMode{}, fmt.Errorf("invalid algorithm: %s", name)
}

func printCKSumOperand(rc *tool.RunContext, mode cksumMode, name string, withName, untagged, raw, b64, zero bool) error {
	lineEnd := "\n"
	if zero {
		lineEnd = "\x00"
	}
	suffix := ""
	if withName {
		suffix = " " + name
	}
	switch mode.kind {
	case "crc":
		crc, size, err := cksumOperand(rc, name)
		if err != nil {
			return err
		}
		if raw {
			var be [4]byte
			binary.BigEndian.PutUint32(be[:], crc)
			_, err = rc.Out.Write(be[:])
			return err
		}
		fmt.Fprintf(rc.Out, "%d %d%s%s", crc, size, suffix, lineEnd)
	case "crc32b":
		crc, size, err := crc32bOperand(rc, name)
		if err != nil {
			return err
		}
		if raw {
			var be [4]byte
			binary.BigEndian.PutUint32(be[:], crc)
			_, err = rc.Out.Write(be[:])
			return err
		}
		// GNU dispatches crc32b to the same decimal untagged output
		// as crc.
		fmt.Fprintf(rc.Out, "%d %d%s%s", crc, size, suffix, lineEnd)
	case "sum":
		result, err := legacySumOperand(rc, name, mode.sysv)
		if err != nil {
			return err
		}
		if raw {
			var be [2]byte
			binary.BigEndian.PutUint16(be[:], result.checksum)
			_, err = rc.Out.Write(be[:])
			return err
		}
		if mode.sysv {
			fmt.Fprintf(rc.Out, "%d %d%s%s", result.checksum, result.blocks, suffix, lineEnd)
		} else {
			fmt.Fprintf(rc.Out, "%05d %5d%s%s", result.checksum, result.blocks, suffix, lineEnd)
		}
	case "digest":
		sum, err := digestOperand(rc, mode, mode.bits, name)
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
		outName, prefix := name, ""
		if !zero {
			outName, prefix = hashenc.EscapeFilename(name)
		}
		if untagged {
			fmt.Fprintf(rc.Out, "%s%s  %s%s", prefix, encoded, outName, lineEnd)
		} else {
			fmt.Fprintf(rc.Out, "%s%s (%s) = %s%s", prefix, mode.tagLabel(), outName, encoded, lineEnd)
		}
	}
	return nil
}

func digestOperand(rc *tool.RunContext, mode cksumMode, bits int, name string) ([]byte, error) {
	if mode.shake != nil {
		h := mode.shake()
		if err := copyOperandToHash(rc, name, h); err != nil {
			return nil, err
		}
		sum := make([]byte, bits/8)
		_, _ = h.Read(sum)
		return sum, nil
	}
	h := mode.mk(bits)
	if err := copyOperandToHash(rc, name, h); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func copyOperandToHash(rc *tool.RunContext, name string, h io.Writer) error {
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
		return hashenc.ErrIsDirectory
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
}

// checkTagTemplates lists the digest algorithms a plain `cksum -c`
// (no -a) auto-detects from BSD tags, keyed by base tag token. GNU
// refuses to auto-detect the non-digest formats (CRC, CRC32B, BSD,
// SYSV): those tags stay misformatted.
var checkTagTemplates = map[string]cksumMode{}

func init() {
	for _, n := range []string{"md5", "sha1", "sha224", "sha256", "sha384", "sha512", "sha2", "sha3", "blake2b", "sm3"} {
		m, err := parseAlgorithm(n)
		if err != nil {
			panic(err)
		}
		if m.family == famSHA2 || m.family == famSHA3 || m.family == famBLAKE2B {
			m.bits = 0 // suffix / default selects the length
		}
		checkTagTemplates[m.label] = m
	}
}

type cksumCheckEntry struct {
	mode    cksumMode
	bits    int
	path    string
	display string
	digest  string
	b64     bool
}

func checkCKSumFile(rc *tool.RunContext, mode cksumMode, autoDetect bool, op string, opts cksumCheckOptions) int {
	var r io.Reader
	isStdin := op == "-"
	display := op
	if isStdin {
		display = "'standard input'"
		r = rc.In
		if r == nil {
			r = strings.NewReader("")
		}
	} else {
		f, err := os.Open(rc.Path(op))
		if err != nil {
			fmt.Fprintf(rc.Err, "cksum: %s: %s\n", op, hashenc.GNUErrMsg(err))
			return 1
		}
		defer f.Close()
		r = f
	}

	br := bufio.NewReader(r)
	var valid, badFormat, mismatched, unreadable int
	lineNo := 0
	curLabel := mode.label
	exit := 0
	for {
		line, rerr := br.ReadString('\n')
		lineNo++
		l := strings.TrimSuffix(line, "\n")
		if l != "" && !strings.HasPrefix(l, "#") {
			entry, ok := parseCKSumCheckLine(mode, autoDetect, l)
			if !ok || (isStdin && entry.path == "-") {
				badFormat++
				if opts.warn {
					fmt.Fprintf(rc.Err, "cksum: %s: %d: improperly formatted %s checksum line\n",
						display, lineNo, curLabel)
				}
			} else {
				curLabel = entry.mode.label
				valid++
				match, err := verifyCKSumEntry(rc, entry)
				switch {
				case err != nil:
					if !(opts.ignoreMissing && errors.Is(err, fs.ErrNotExist)) {
						if !opts.status {
							fmt.Fprintf(rc.Err, "cksum: %s: %s\n", entry.display, hashenc.GNUErrMsg(err))
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
			fmt.Fprintf(rc.Err, "cksum: %s: no properly formatted checksum lines found\n", display)
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

// parseCKSumCheckLine parses one --check line. Without -a only the
// BSD tagged format is accepted and the algorithm comes from the tag;
// with -a both tagged (label must match) and untagged formats are
// accepted.
func parseCKSumCheckLine(mode cksumMode, autoDetect bool, line string) (cksumCheckEntry, bool) {
	escaped := false
	l := line
	if strings.HasPrefix(l, "\\") {
		escaped = true
		l = l[1:]
	}

	if entry, ok := parseTaggedCKSumLine(mode, autoDetect, l, escaped); ok {
		return entry, true
	}
	if autoDetect {
		// GNU only supports the tagged format without -a.
		return cksumCheckEntry{}, false
	}
	return parseUntaggedCKSumLine(mode, l, escaped)
}

// parseTaggedCKSumLine handles "TAG[-BITS] (name) = digest" (with the
// OpenSSL "TAG(name)= digest" spacing also accepted, as in GNU).
func parseTaggedCKSumLine(mode cksumMode, autoDetect bool, l string, escaped bool) (cksumCheckEntry, bool) {
	// Extract the tag token (ends at ' ', '-' or '(').
	i := 0
	for i < len(l) && l[i] != ' ' && l[i] != '-' && l[i] != '(' {
		i++
	}
	token := l[:i]
	rest := l[i:]

	var tmpl cksumMode
	switch {
	case autoDetect:
		m, ok := checkTagTemplates[token]
		if !ok {
			return cksumCheckEntry{}, false
		}
		tmpl = m
	case mode.family == famSHA2:
		// -a sha2 accepts SHA2[-BITS] and the SHA224..SHA512 tags.
		switch token {
		case "SHA2":
			tmpl = mode
		case "SHA224", "SHA256", "SHA384", "SHA512":
			tmpl = mode
			tmpl.bits, _ = strconv.Atoi(token[3:])
		default:
			return cksumCheckEntry{}, false
		}
	default:
		// The mode's own tag; a label like "SHA3-256" splits into
		// token "SHA3" + required suffix.
		wantToken, wantSuffix, _ := strings.Cut(mode.tagTokenForCheck(), "-")
		if token != wantToken {
			return cksumCheckEntry{}, false
		}
		tmpl = mode
		if wantSuffix != "" {
			// Fixed extension label with a mandatory suffix.
			if !strings.HasPrefix(rest, "-"+wantSuffix) {
				return cksumCheckEntry{}, false
			}
			rest = rest[1+len(wantSuffix):]
		}
	}

	bits := tmpl.bits
	// Optional "-BITS" suffix for the variable-length families.
	if strings.HasPrefix(rest, "-") {
		switch tmpl.family {
		case famSHA2, famSHA3, famBLAKE2B, famBLAKE3:
		default:
			return cksumCheckEntry{}, false
		}
		j := 1
		for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
			j++
		}
		n, err := strconv.Atoi(rest[1:j])
		if j == 1 || err != nil {
			return cksumCheckEntry{}, false
		}
		switch tmpl.family {
		case famSHA2, famSHA3:
			if !validSHA2Len(n) {
				return cksumCheckEntry{}, false
			}
		case famBLAKE2B:
			if n <= 0 || n%8 != 0 || n > 512 {
				return cksumCheckEntry{}, false
			}
		case famBLAKE3:
			if n <= 0 || n%8 != 0 || n > 1024 {
				return cksumCheckEntry{}, false
			}
		}
		bits = n
		rest = rest[j:]
	}
	if bits == 0 {
		// No suffix: GNU falls back to the family's maximum size.
		bits = 512
	}

	rest = strings.TrimPrefix(rest, " ")
	body, found := strings.CutPrefix(rest, "(")
	if !found {
		return cksumCheckEntry{}, false
	}
	end := strings.LastIndexByte(body, ')')
	if end <= 0 {
		return cksumCheckEntry{}, false
	}
	name := body[:end]
	after := strings.TrimLeft(body[end+1:], " \t")
	after, found = strings.CutPrefix(after, "=")
	if !found {
		return cksumCheckEntry{}, false
	}
	d := strings.TrimLeft(after, " \t")
	isB64, ok := validCKSumDigest(d, bits)
	if !ok {
		return cksumCheckEntry{}, false
	}
	return cksumCheckEntry{
		mode:    tmpl,
		bits:    bits,
		path:    hashenc.UnescapeFilename(escaped, name),
		display: name,
		digest:  d,
		b64:     isB64,
	}, true
}

// tagTokenForCheck is the tag a check line must carry for this mode.
func (m cksumMode) tagTokenForCheck() string {
	switch m.family {
	case famSHA3:
		return "SHA3"
	case famBLAKE2B:
		return "BLAKE2b"
	}
	return m.label
}

func parseUntaggedCKSumLine(mode cksumMode, l string, escaped bool) (cksumCheckEntry, bool) {
	i := strings.IndexByte(l, ' ')
	if i < 0 {
		return cksumCheckEntry{}, false
	}
	d := l[:i]
	bits := mode.bits
	var isB64 bool
	if bits == 0 {
		// Auto-detect the length from the hex-digit count (GNU does
		// this for blake2b, sha2 and sha3).
		if !isHexStr(d) || len(d) < 2 || len(d)%2 != 0 {
			return cksumCheckEntry{}, false
		}
		n := len(d) * 4
		switch mode.family {
		case famSHA2, famSHA3:
			if !validSHA2Len(n) {
				return cksumCheckEntry{}, false
			}
		case famBLAKE2B:
			if n > 512 {
				return cksumCheckEntry{}, false
			}
		}
		bits = n
	} else {
		var ok bool
		isB64, ok = validCKSumDigest(d, bits)
		if !ok {
			return cksumCheckEntry{}, false
		}
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
	return cksumCheckEntry{
		mode:    mode,
		bits:    bits,
		path:    hashenc.UnescapeFilename(escaped, name),
		display: name,
		digest:  d,
		b64:     isB64,
	}, true
}

// validCKSumDigest reports whether s is a plausible digest of the
// given bit length: hex of bits/4 characters, or (GNU cksum accepts
// this in check lines regardless of --base64) base64 of the binary
// digest length.
func validCKSumDigest(s string, bits int) (isB64, ok bool) {
	if isHexN(s, bits/4) {
		return false, true
	}
	if len(s) == base64.StdEncoding.EncodedLen(bits/8) {
		if _, err := base64.StdEncoding.DecodeString(s); err == nil {
			return true, true
		}
	}
	return false, false
}

func verifyCKSumEntry(rc *tool.RunContext, entry cksumCheckEntry) (bool, error) {
	sum, err := digestOperand(rc, entry.mode, entry.bits, entry.path)
	if err != nil {
		return false, err
	}
	if entry.b64 {
		return base64.StdEncoding.EncodeToString(sum) == entry.digest, nil
	}
	return strings.EqualFold(hex.EncodeToString(sum), entry.digest), nil
}

func isHexN(s string, n int) bool {
	return len(s) == n && isHexStr(s)
}

func isHexStr(s string) bool {
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
	r, closeFn, err := openOperand(rc, name)
	if err != nil {
		return 0, 0, err
	}
	if closeFn != nil {
		defer closeFn()
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

// crc32bOperand computes the standard IEEE CRC-32 ("crc32b") and the
// byte count.
func crc32bOperand(rc *tool.RunContext, name string) (uint32, uint64, error) {
	r, closeFn, err := openOperand(rc, name)
	if err != nil {
		return 0, 0, err
	}
	if closeFn != nil {
		defer closeFn()
	}
	h := crc32.NewIEEE()
	n, err := io.Copy(h, r)
	if err != nil {
		return 0, 0, err
	}
	return h.Sum32(), uint64(n), nil
}

func openOperand(rc *tool.RunContext, name string) (io.Reader, func() error, error) {
	if name == "-" {
		if rc.In == nil {
			return strings.NewReader(""), nil, nil
		}
		return rc.In, nil, nil
	}
	f, err := os.Open(rc.Path(name))
	if err != nil {
		return nil, nil, err
	}
	if fi, err := f.Stat(); err == nil && fi.IsDir() {
		_ = f.Close()
		return nil, nil, hashenc.ErrIsDirectory
	}
	return f, f.Close, nil
}

type legacySumResult struct {
	checksum uint16
	blocks   uint64
}

func legacySumOperand(rc *tool.RunContext, name string, sysv bool) (legacySumResult, error) {
	r, closeFn, err := openOperand(rc, name)
	if err != nil {
		return legacySumResult{}, err
	}
	if closeFn != nil {
		defer closeFn()
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
