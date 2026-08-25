// Package iconvcmd implements a portable, pure-Go subset of POSIX iconv(1).
//
// Supported options are -c/--discard-invalid, -f/--from-code, -t/--to-code,
// -s/--silent, and -l/--list.
// Charset names are resolved through golang.org/x/text's IANA registry; this
// includes UTF-8/16, the ISO-8859 families, Windows code pages, Shift_JIS,
// EUC-JP, Big5, GBK, and GB18030. A registered charset for which x/text has no
// implementation is rejected rather than approximated. POSIX -c omits input
// characters that cannot be converted (invalid in the input codeset or with no
// representation in the output codeset) instead of failing. When -f or -t is
// omitted the codeset of the current locale (LC_CTYPE, defaulting to the
// POSIX-locale US-ASCII) is used, per the Issue 7 synopsis. Suffixes such as
// //IGNORE or //TRANSLIT are explicitly rejected.
package iconvcmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/ianaindex"
	unicodeenc "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

var cmd = &tool.Tool{
	Name:     "iconv",
	Synopsis: "Convert text from one character encoding to another.",
	Usage:    "iconv [-l] [-s] -f FROMCODE -t TOCODE [FILE]...",
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
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	fromName := fs.StringP("from-code", "f", "", "convert text from encoding FROMCODE")
	toName := fs.StringP("to-code", "t", "", "convert text to encoding TOCODE")
	silent := fs.BoolP("silent", "s", false, "suppress messages about invalid characters")
	discardInvalid := fs.BoolP("discard-invalid", "c", false, "discard invalid input characters")
	list := fs.BoolP("list", "l", false, "list all known coded character sets")
	files, code := tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args))
	if code >= 0 {
		return code
	}
	if *list {
		for _, name := range supportedEncodings {
			fmt.Fprintln(rc.Out, name)
		}
		return 0
	}
	// POSIX synopsis: -f and -t may each be omitted, in which case the codeset
	// of the current locale (LC_CTYPE) is used. This is not a usage error.
	fromCode := *fromName
	if fromCode == "" {
		fromCode = localeCodeset(rc.Env)
	}
	toCode := *toName
	if toCode == "" {
		toCode = localeCodeset(rc.Env)
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
	if len(files) == 0 {
		files = []string{"-"}
	}

	// -c omits input characters that cannot be converted. The x/text decoders
	// are lenient — a byte invalid in the input codeset becomes U+FFFD rather
	// than an error — so the two conversion-loss paths are handled separately:
	// on the decode side dropReplacement omits those U+FFFD substitutions
	// (bytes invalid in the input codeset), and on the encode side dropInvalid
	// omits runes with no representation in the output codeset, which is the
	// path that actually errors.
	newEncoder := func() transform.Transformer {
		if *discardInvalid {
			return dropInvalid{to.NewEncoder()}
		}
		return to.NewEncoder()
	}
	newDecoder := func() transform.Transformer {
		if *discardInvalid {
			return transform.Chain(from.NewDecoder(), dropReplacement{})
		}
		return from.NewDecoder()
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
		_, copyErr := io.Copy(out, transform.NewReader(in, newDecoder()))
		if closeErr := closeInput(); copyErr == nil {
			copyErr = closeErr
		}
		if copyErr != nil {
			// -s suppresses diagnostics about invalid/unrepresentable
			// characters only; an output-device error must remain visible.
			if trackedOut.err != nil {
				fmt.Fprintf(rc.Err, "iconv: write error: %v\n", trackedOut.err)
			} else if !*silent {
				fmt.Fprintf(rc.Err, "iconv: %s: %v\n", displayName(name), copyErr)
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
	return status
}

// localeCodeset returns the codeset iconv uses for an omitted -f/-t, per the
// POSIX synopsis: the codeset of the current locale's LC_CTYPE category. A
// name of the form "lang_TERR.CODESET[@mod]" yields CODESET; the C/POSIX
// locale (or a name with no codeset) yields US-ASCII, the codeset of the
// portable character set, matching the deterministic LC_ALL=C contract.
func localeCodeset(env []string) string {
	name := locale.Resolve(env, locale.CType)
	name, _, _ = strings.Cut(name, "@")
	if _, cs, ok := strings.Cut(name, "."); ok && cs != "" {
		return cs
	}
	return "US-ASCII"
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
	t transform.Transformer
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
			d.t.Reset()
		}
	}
}

// dropReplacement removes U+FFFD runes from a UTF-8 stream. Under iconv -c it
// omits the replacement characters the (lenient) decoder emitted for bytes that
// were invalid in the input codeset, so those bytes are omitted from the output
// rather than surviving as U+FFFD when the output codeset can represent it.
type dropReplacement struct{}

func (dropReplacement) Reset() {}

func (dropReplacement) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		if !atEOF && !utf8.FullRune(src[nSrc:]) {
			return nDst, nSrc, transform.ErrShortSrc
		}
		r, size := utf8.DecodeRune(src[nSrc:])
		if r == utf8.RuneError { // both real U+FFFD and stray invalid bytes
			nSrc += size
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

func (w *errorTrackingWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
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
		case "437", "850", "852", "855", "858", "860", "862", "863", "865", "866", "037", "1047":
			name = "ibm" + num
		}
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
