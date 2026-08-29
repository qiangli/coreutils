// Package iconvcmd implements a portable, pure-Go subset of POSIX iconv(1).
//
// Supported options are -c/--discard-invalid, -f/--from-code, -t/--to-code,
// -s/--silent, and -l/--list.
// Charset names are resolved through golang.org/x/text's IANA registry; this
// includes UTF-8/16, the ISO-8859 families, Windows code pages, Shift_JIS,
// EUC-JP, Big5, GBK, and GB18030. A registered charset for which x/text has no
// implementation is rejected rather than approximated. POSIX -c omits input
// characters that cannot be converted (invalid in the input codeset or with no
// representation in the output codeset), while preserving the conversion's
// non-zero status as POSIX requires. Without -c, malformed source characters
// are likewise omitted while conversion continues, but produce a diagnostic;
// an input character with no output representation stops conversion. -s
// suppresses those character diagnostics, never file or device errors. When
// -f or -t is omitted the codeset of the current locale (LC_CTYPE, defaulting
// to the POSIX-locale US-ASCII) is used, per the Issue 7 synopsis. Suffixes such as
// //IGNORE or //TRANSLIT are explicitly rejected.
package iconvcmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/ianaindex"
	unicodeenc "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

var cmd = &tool.Tool{
	Name:     "iconv",
	Synopsis: "Convert text from one character encoding to another.",
	Usage: "iconv [-cs] -f frommap -t tomap [file...]\n" +
		"       iconv -f fromcode [-cs] [-t tocode] [file...]\n" +
		"       iconv -t tocode [-cs] [-f fromcode] [file...]\n" +
		"       iconv -l",
}

var supportedEncodings = []string{
	"UTF-8",
	"UTF-16",
	"UTF-16BE",
	"UTF-16LE",
	"US-ASCII",
	"ISO-8859-1",
	"ISO-8859-2",
	"ISO-8859-3",
	"ISO-8859-4",
	"ISO-8859-5",
	"ISO-8859-6",
	"ISO-8859-7",
	"ISO-8859-8",
	"ISO-8859-9",
	"ISO-8859-10",
	"ISO-8859-13",
	"ISO-8859-14",
	"ISO-8859-15",
	"ISO-8859-16",
	"windows-1250",
	"windows-1251",
	"windows-1252",
	"windows-1253",
	"windows-1254",
	"windows-1255",
	"windows-1256",
	"windows-1257",
	"windows-1258",
	"IBM437",
	"IBM850",
	"CP858",
	"IBM00858",
	"IBM852",
	"IBM855",
	"IBM860",
	"IBM862",
	"IBM863",
	"IBM865",
	"IBM866",
	"IBM037",
	"IBM1047",
	"KOI8-R",
	"KOI8-U",
	"macintosh",
	"Shift_JIS",
	"EUC-JP",
	"ISO-2022-JP",
	"GBK",
	"GB18030",
	"HZ-GB-2312",
	"Big5",
	"EUC-KR",
	// Explicit spelling aliases are values too: -l must not advertise only
	// their canonical targets while lookup accepts these additional tokens.
	"UTF8", "UTF_8",
	"ASCII", "USASCII", "US_ASCII",
	"LATIN1", "LATIN-1", "LATIN_1",
	"UTF16", "UTF_16", "UTF16BE", "UTF_16BE", "UTF16LE", "UTF_16LE",
	"CP1250", "CP1251", "CP1252", "CP1253", "CP1254", "CP1255", "CP1256", "CP1257", "CP1258",
	"CP437", "CP850", "CP852", "CP855", "CP860", "CP862", "CP863", "CP865", "CP866", "CP037", "CP1047",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	fromName := fs.StringP("from-code", "f", "", "convert text from encoding FROMCODE")
	toName := fs.StringP("to-code", "t", "", "convert text to encoding TOCODE")
	silent := fs.BoolP("silent", "s", false, "suppress messages about invalid characters")
	discardInvalid := fs.BoolP("discard-invalid", "c", false, "discard invalid input characters")
	list := fs.BoolP("list", "l", false, "list all known coded character sets")
	var files []string
	var code int
	if envPresent(rc.Env, "POSIXLY_CORRECT") {
		// POSIX Utility Syntax Guideline 9 makes every argument after the
		// first operand an operand, even when it is spelled like an option.
		// Parse the original spellings so post-operand -h/-V pathnames are not
		// rewritten to their long aliases before the boundary is known.
		files, code = tool.ParseRequireOrder(rc, cmd, fs, args)
	} else {
		// Preserve the established GNU-style option permutation extension
		// outside POSIX mode.
		files, code = tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args))
	}
	if code >= 0 {
		return code
	}
	if *list {
		if fs.Changed("from-code") || fs.Changed("to-code") || fs.Changed("silent") || fs.Changed("discard-invalid") || len(files) != 0 {
			return tool.UsageError(rc, cmd, "-l is a standalone form")
		}
		for _, name := range supportedEncodings {
			line := name + "\n"
			n, err := io.WriteString(rc.Out, line)
			if err == nil && n != len(line) {
				err = io.ErrShortWrite
			}
			if err != nil {
				fmt.Fprintf(rc.Err, "iconv: write error: %v\n", err)
				return 1
			}
		}
		return 0
	}
	// No POSIX synopsis permits both -f and -t to be omitted. Fail as a usage
	// error instead of guessing that a bare invocation meant a locale copy.
	if !fs.Changed("from-code") && !fs.Changed("to-code") {
		return tool.UsageError(rc, cmd, "at least one of -f FROMCODE or -t TOCODE is required")
	}
	// POSIX synopsis: -f and -t may each be omitted, in which case the codeset
	// of the current locale (LC_CTYPE) is used. This is not a usage error.
	fromCode := *fromName
	if fromCode == "" {
		var err error
		fromCode, err = localeCodeset(rc.Env)
		if err != nil {
			fmt.Fprintf(rc.Err, "iconv: %v\n", err)
			return 1
		}
	}
	toCode := *toName
	if toCode == "" {
		var err error
		toCode, err = localeCodeset(rc.Env)
		if err != nil {
			fmt.Fprintf(rc.Err, "iconv: %v\n", err)
			return 1
		}
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	for _, codeName := range []string{fromCode, toCode} {
		if strings.Contains(codeName, "//") {
			fmt.Fprintf(rc.Err, "iconv: unsupported encoding %q\n", codeName)
			return 1
		}
	}
	fromMap := strings.ContainsRune(fromCode, '/')
	toMap := strings.ContainsRune(toCode, '/')
	if fromMap || toMap {
		if !fromMap || !toMap {
			fmt.Fprintln(rc.Err, "iconv: charmap pathname conversion requires both -f and -t charmaps")
			return 1
		}
		return runCharmapConversion(rc, fromCode, toCode, files, *discardInvalid, *silent)
	}

	from, err := lookupEncoding(fromCode)
	if err != nil {
		fmt.Fprintf(rc.Err, "iconv: %v\n", err)
		return 1
	}
	to, err := lookupEncoding(toCode)
	if err != nil {
		fmt.Fprintf(rc.Err, "iconv: %v\n", err)
		return 1
	}
	// x/text decoders are lenient — malformed source bytes become U+FFFD. The
	// decode lane therefore validates with or without -c so that -c cannot
	// change status. The encode lane needs its skipping wrapper only for -c;
	// without -c, an unrepresentable output character remains a hard error.
	discarded := &discardState{}
	newEncoder := func() transform.Transformer {
		if *discardInvalid {
			return dropInvalid{t: to.NewEncoder(), discarded: discarded}
		}
		return to.NewEncoder()
	}
	newDecoder := func() transform.Transformer {
		// x/text decoders intentionally replace malformed source bytes. POSIX
		// nevertheless requires -c not to change status, so validate on both
		// lanes and use the documented omit-on-error result when -c is absent.
		switch encodingClass(fromCode) {
		case "utf8":
			return transform.Chain(discardInvalidUTF8{discarded}, from.NewDecoder())
		case "utf16":
			return transform.Chain(&discardInvalidUTF16{discarded: discarded}, from.NewDecoder())
		case "utf16be":
			return transform.Chain(&discardInvalidUTF16{order: byteOrderBE, fixed: true, discarded: discarded}, from.NewDecoder())
		case "utf16le":
			return transform.Chain(&discardInvalidUTF16{order: byteOrderLE, fixed: true, discarded: discarded}, from.NewDecoder())
		case "gb18030":
			return &discardInvalidGB18030Decoder{enc: from, discarded: discarded}
		default:
			// These carried encodings cannot represent U+FFFD. Therefore any
			// replacement rune emitted by their lenient decoder necessarily
			// denotes malformed source input, not a literal character.
			return transform.Chain(from.NewDecoder(), dropReplacement{discarded})
		}
	}

	// Keep one encoder across all operands: iconv concatenates FILE inputs into
	// one output stream, so a BOM/state prefix must not be emitted per file.
	trackedOut := &errorTrackingWriter{w: rc.Out}
	out := transform.NewWriter(trackedOut, newEncoder())
	status := 0
	for _, name := range files {
		if err := rc.Ctx.Err(); err != nil {
			fmt.Fprintf(rc.Err, "iconv: %v\n", err)
			status = 1
			break
		}
		in, closeInput, err := openInput(rc, name)
		if err != nil {
			fmt.Fprintf(rc.Err, "iconv: %s: %v\n", name, err)
			status = 1
			continue
		}
		trackedIn := &errorTrackingReader{r: in}
		wasDiscarded := discarded.any
		_, copyErr := io.Copy(out, transform.NewReader(trackedIn, newDecoder()))
		closeErr := closeInput()
		if copyErr != nil {
			// -s suppresses diagnostics about invalid/unrepresentable
			// characters only; an output-device error must remain visible.
			if trackedOut.err != nil {
				fmt.Fprintf(rc.Err, "iconv: write error: %v\n", trackedOut.err)
			} else if trackedIn.err != nil {
				fmt.Fprintf(rc.Err, "iconv: %s: %v\n", displayName(name), trackedIn.err)
			} else if !*silent {
				fmt.Fprintf(rc.Err, "iconv: %s: %v\n", displayName(name), copyErr)
			}
			status = 1
		}
		if closeErr != nil {
			fmt.Fprintf(rc.Err, "iconv: %s: %v\n", displayName(name), closeErr)
			status = 1
		}
		if !wasDiscarded && discarded.any {
			// This implementation documents -c as omitting quietly; -s is still
			// meaningful without -c, where it suppresses conversion diagnostics.
			if !*discardInvalid && !*silent {
				fmt.Fprintf(rc.Err, "iconv: %s: invalid character sequence\n", displayName(name))
			}
			status = 1
		}
	}
	if err := out.Close(); err != nil {
		if trackedOut.err != nil || !*silent {
			fmt.Fprintf(rc.Err, "iconv: write error: %v\n", err)
		}
		status = 1
	}
	// POSIX says that the presence or absence of -c shall not affect iconv's
	// exit status. Omitted characters therefore still make the conversion
	// unsuccessful even though -c permits conversion to continue and controls
	// which bytes reach stdout.
	if discarded.any {
		status = 1
	}
	return status
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

// localeCodeset returns the codeset iconv uses for an omitted -f/-t, per the
// POSIX synopsis: the codeset of the current locale's LC_CTYPE category. A
// name of the form "lang_TERR.CODESET[@mod]" yields CODESET. C/POSIX maps to
// US-ASCII, C.UTF-8 to UTF-8, and the carried unqualified de_DE locale to
// ISO-8859-1. Any other unqualified locale fails closed.
func localeCodeset(env []string) (string, error) {
	name := locale.Resolve(env, locale.CType)
	name, _, _ = strings.Cut(name, "@")
	base, cs, hasCodeset := strings.Cut(name, ".")
	if hasCodeset && cs != "" {
		if base == "C" && encodingClass(cs) == "utf8" {
			return "UTF-8", nil
		}
		return cs, nil
	}
	switch base {
	case "C", "POSIX":
		return "US-ASCII", nil
	case "de_DE":
		// The unqualified de_DE locale in the embedded certification corpus
		// is the carried ISO-8859-1 locale, matching pkg/locale's providers.
		return "ISO-8859-1", nil
	default:
		return "", fmt.Errorf("LC_CTYPE locale %q has no carried default codeset", name)
	}
}

func encodingClass(name string) string {
	n := strings.ToUpper(strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.TrimSpace(name)))
	switch n {
	case "UTF8":
		return "utf8"
	case "UTF16":
		return "utf16"
	case "UTF16BE":
		return "utf16be"
	case "UTF16LE":
		return "utf16le"
	case "GB18030":
		return "gb18030"
	default:
		return ""
	}
}

// dropInvalid wraps a transform.Transformer so that a malformed-input or
// unrepresentable-rune error skips the offending input and continues, instead
// of aborting the stream. It implements POSIX iconv -c: on the decoder it omits
// bytes that are invalid in the input codeset; on the encoder it omits runes
// with no representation in the output codeset. The skip advances by one UTF-8
// rune when the input parses as one (the encode side, whose input is the
// decoder's UTF-8 output) and by one byte otherwise (the decode side, whose
// input is arbitrary bytes), so progress is always guaranteed.
type dropInvalid struct {
	t         transform.Transformer
	discarded *discardState
}

func (d dropInvalid) Reset() { d.t.Reset() }

func (d dropInvalid) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for {
		n, s, e := d.t.Transform(dst[nDst:], src[nSrc:], atEOF)
		nDst += n
		nSrc += s
		switch e {
		case nil:
			return nDst, nSrc, nil
		case transform.ErrShortDst, transform.ErrShortSrc:
			return nDst, nSrc, e
		default:
			// Malformed input or an unrepresentable rune at src[nSrc:]; omit it.
			if nSrc >= len(src) {
				if atEOF {
					return nDst, len(src), nil // drop a trailing invalid remnant
				}
				return nDst, nSrc, transform.ErrShortSrc // may complete with more input
			}
			_, size := utf8.DecodeRune(src[nSrc:])
			if size < 1 {
				size = 1
			}
			nSrc += size
			d.discarded.any = true
		}
	}
}

type discardState struct{ any bool }

// discardInvalidUTF8 omits only malformed UTF-8 encodings. In particular, a
// valid literal U+FFFD (EF BF BD) passes through unchanged; filtering decoded
// replacement runes cannot make that distinction.
type discardInvalidUTF8 struct{ discarded *discardState }

func (discardInvalidUTF8) Reset() {}

func (d discardInvalidUTF8) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		if !utf8.FullRune(src[nSrc:]) {
			if !atEOF {
				return nDst, nSrc, transform.ErrShortSrc
			}
			nSrc++ // incomplete trailing encoding
			d.discarded.any = true
			continue
		}
		_, size := utf8.DecodeRune(src[nSrc:])
		if size == 1 && src[nSrc] >= utf8.RuneSelf {
			nSrc++ // malformed encoding, not a literal RuneError
			d.discarded.any = true
			continue
		}
		if nDst+size > len(dst) {
			return nDst, nSrc, transform.ErrShortDst
		}
		copy(dst[nDst:], src[nSrc:nSrc+size])
		nDst += size
		nSrc += size
	}
	return nDst, nSrc, nil
}

type byteOrder uint8

const (
	byteOrderUnknown byteOrder = iota
	byteOrderBE
	byteOrderLE
)

// discardInvalidUTF16 validates UTF-16 code units before x/text's lenient
// decoder. The zero order means BOM-selected UTF-16; fixed orders implement
// UTF-16BE/LE. Invalid or incomplete surrogate units are omitted while valid
// U+FFFD units are preserved.
type discardInvalidUTF16 struct {
	order     byteOrder
	fixed     bool
	discarded *discardState
}

func (d *discardInvalidUTF16) Reset() {
	if !d.fixed {
		d.order = byteOrderUnknown
	}
}

func (d *discardInvalidUTF16) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	order := d.order
	if order == byteOrderUnknown {
		if len(src) < 2 {
			if !atEOF {
				return 0, 0, transform.ErrShortSrc
			}
			// Never clear a discard recorded by an earlier operand:
			// the state is shared across the whole invocation.
			if len(src) != 0 {
				d.discarded.any = true
			}
			return 0, len(src), nil
		}
		switch {
		case src[0] == 0xfe && src[1] == 0xff:
			order = byteOrderBE
		case src[0] == 0xff && src[1] == 0xfe:
			order = byteOrderLE
		default:
			// Let the downstream ExpectBOM decoder issue its normal error.
			if len(dst) < 2 {
				return 0, 0, transform.ErrShortDst
			}
			copy(dst, src[:2])
			return 2, 2, nil
		}
		d.order = order
		if len(dst) < 2 {
			return 0, 0, transform.ErrShortDst
		}
		copy(dst, src[:2])
		nDst, nSrc = 2, 2
	}
	unit := func(p []byte) uint16 {
		if order == byteOrderLE {
			return uint16(p[0]) | uint16(p[1])<<8
		}
		return uint16(p[0])<<8 | uint16(p[1])
	}
	for nSrc < len(src) {
		if len(src)-nSrc < 2 {
			if !atEOF {
				return nDst, nSrc, transform.ErrShortSrc
			}
			d.discarded.any = true
			return nDst, len(src), nil
		}
		u := unit(src[nSrc:])
		size := 2
		if u >= 0xd800 && u <= 0xdbff {
			if len(src)-nSrc < 4 {
				if !atEOF {
					return nDst, nSrc, transform.ErrShortSrc
				}
				nSrc += 2
				d.discarded.any = true
				continue
			}
			lo := unit(src[nSrc+2:])
			if lo < 0xdc00 || lo > 0xdfff {
				nSrc += 2
				d.discarded.any = true
				continue
			}
			size = 4
		} else if u >= 0xdc00 && u <= 0xdfff {
			nSrc += 2
			d.discarded.any = true
			continue
		}
		if nDst+size > len(dst) {
			return nDst, nSrc, transform.ErrShortDst
		}
		copy(dst[nDst:], src[nSrc:nSrc+size])
		nDst += size
		nSrc += size
	}
	return nDst, nSrc, nil
}

// discardInvalidGB18030Decoder decodes one stateless GB18030 character at a
// time. That lets it distinguish a genuine encoded U+FFFD from an undefined
// code point that x/text's lenient decoder also renders as U+FFFD.
type discardInvalidGB18030Decoder struct {
	enc       encoding.Encoding
	discarded *discardState
}

func (d *discardInvalidGB18030Decoder) Reset() {}

func (d *discardInvalidGB18030Decoder) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		b := src[nSrc]
		size := 0
		switch {
		case b <= 0x80: // ASCII plus GB18030's single-byte euro extension
			size = 1
		case b >= 0x81 && b <= 0xfe:
			if len(src)-nSrc < 2 {
				if !atEOF {
					return nDst, nSrc, transform.ErrShortSrc
				}
				nSrc++
				d.discarded.any = true
				continue
			}
			b2 := src[nSrc+1]
			switch {
			case (b2 >= 0x40 && b2 <= 0x7e) || (b2 >= 0x80 && b2 <= 0xfe):
				size = 2
			case b2 >= 0x30 && b2 <= 0x39:
				if len(src)-nSrc < 4 {
					if !atEOF {
						return nDst, nSrc, transform.ErrShortSrc
					}
					nSrc++
					d.discarded.any = true
					continue
				}
				if src[nSrc+2] >= 0x81 && src[nSrc+2] <= 0xfe && src[nSrc+3] >= 0x30 && src[nSrc+3] <= 0x39 {
					size = 4
				}
			}
		}
		if size == 0 {
			nSrc++
			d.discarded.any = true
			continue
		}
		var decoded [utf8.UTFMax]byte
		n, consumed, decodeErr := d.enc.NewDecoder().Transform(decoded[:], src[nSrc:nSrc+size], true)
		if decodeErr != nil || consumed != size || (bytes.Equal(decoded[:n], []byte("�")) && !bytes.Equal(src[nSrc:nSrc+size], []byte{0x84, 0x31, 0xa4, 0x37})) {
			if consumed < 1 {
				consumed = 1
			}
			nSrc += consumed
			d.discarded.any = true
			continue
		}
		if nDst+n > len(dst) {
			return nDst, nSrc, transform.ErrShortDst
		}
		copy(dst[nDst:], decoded[:n])
		nDst += n
		nSrc += size
	}
	return nDst, nSrc, nil
}

// dropReplacement removes U+FFFD runes from a UTF-8 stream. Under iconv -c it
// omits the replacement characters the (lenient) decoder emitted for bytes that
// were invalid in the input codeset, so those bytes are omitted from the output
// rather than surviving as U+FFFD when the output codeset can represent it.
type dropReplacement struct{ discarded *discardState }

func (dropReplacement) Reset() {}

func (d dropReplacement) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		if !atEOF && !utf8.FullRune(src[nSrc:]) {
			return nDst, nSrc, transform.ErrShortSrc
		}
		r, size := utf8.DecodeRune(src[nSrc:])
		if r == utf8.RuneError { // both real U+FFFD and stray invalid bytes
			nSrc += size
			d.discarded.any = true
			continue
		}
		if nDst+size > len(dst) {
			return nDst, nSrc, transform.ErrShortDst
		}
		copy(dst[nDst:], src[nSrc:nSrc+size])
		nDst += size
		nSrc += size
	}
	return nDst, nSrc, nil
}

type errorTrackingWriter struct {
	w   io.Writer
	err error
}

type errorTrackingReader struct {
	r   io.Reader
	err error
}

func (r *errorTrackingReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if err != nil && err != io.EOF {
		r.err = err
	}
	return n, err
}

func (w *errorTrackingWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.err = err
	}
	return n, err
}

func lookupEncoding(name string) (encoding.Encoding, error) {
	raw := name
	name = strings.TrimSpace(name)
	if strings.Contains(name, "//") {
		return nil, fmt.Errorf("unsupported encoding %q", raw)
	}
	upper := strings.ToUpper(name)
	switch upper {
	case "UTF8", "UTF_8":
		name = "UTF-8"
	case "ASCII", "USASCII", "US_ASCII":
		return ianaindex.IANA.Encoding("US-ASCII")
	case "LATIN1", "LATIN-1", "LATIN_1":
		name = "ISO-8859-1"
	case "UTF16", "UTF_16":
		name = "UTF-16"
	case "UTF16BE", "UTF_16BE":
		name = "UTF-16BE"
	case "UTF16LE", "UTF_16LE":
		name = "UTF-16LE"
	}
	if strings.HasPrefix(upper, "CP") && len(upper) > 2 {
		num := upper[2:]
		switch num {
		case "1250", "1251", "1252", "1253", "1254", "1255", "1256", "1257", "1258":
			name = "windows-" + num
		case "858":
			return charmap.CodePage858, nil
		case "437", "850", "852", "855", "860", "862", "863", "865", "866", "037", "1047":
			name = "ibm" + num
		}
	}
	if strings.EqualFold(name, "IBM00858") {
		return charmap.CodePage858, nil
	}
	if !isAdvertisedEncoding(name) {
		return nil, fmt.Errorf("unsupported encoding %q", raw)
	}
	if strings.EqualFold(name, "UTF-8") {
		return unicodeenc.UTF8, nil
	}
	enc, err := ianaindex.IANA.Encoding(name)
	if err != nil {
		return nil, fmt.Errorf("unsupported encoding %q", raw)
	}
	if enc == nil {
		return nil, fmt.Errorf("encoding %q is registered but not supported", raw)
	}
	return enc, nil
}

func isAdvertisedEncoding(name string) bool {
	for _, supported := range supportedEncodings {
		if strings.EqualFold(name, supported) {
			return true
		}
	}
	return false
}

func openInput(rc *tool.RunContext, name string) (io.Reader, func() error, error) {
	if name == "-" {
		return rc.In, func() error { return nil }, nil
	}
	f, err := os.Open(rc.Path(name))
	if err != nil {
		return nil, nil, err
	}
	return f, f.Close, nil
}

func displayName(name string) string {
	if name == "-" {
		return "standard input"
	}
	return name
}
