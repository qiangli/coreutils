// Package iconvcmd implements a portable, pure-Go subset of POSIX iconv(1).
//
// Supported options are -f/--from-code, -t/--to-code, -s/--silent, and -l/--list.
// Charset names are resolved through golang.org/x/text's IANA registry; this
// includes UTF-8/16, the ISO-8859 families, Windows code pages, Shift_JIS,
// EUC-JP, Big5, GBK, and GB18030. A registered charset for which x/text has no
// implementation is rejected rather than approximated. POSIX -c is recognized
// but rejected explicitly until invalid-sequence omission can be implemented
// without hiding conversion errors. Suffixes such as //IGNORE or //TRANSLIT
// are explicitly rejected.
package iconvcmd

import (
	"fmt"
	"io"
	"os"
	"strings"

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
	if *discardInvalid {
		fmt.Fprintln(rc.Err, "iconv: option '-c' is not supported")
		return 2
	}
	if *fromName == "" {
		return tool.UsageError(rc, cmd, "missing source encoding; use -f FROMCODE")
	}
	if *toName == "" {
		return tool.UsageError(rc, cmd, "missing destination encoding; use -t TOCODE")
	}

	from, err := lookupEncoding(*fromName)
	if err != nil {
		fmt.Fprintf(rc.Err, "iconv: %v\n", err)
		return 1
	}
	to, err := lookupEncoding(*toName)
	if err != nil {
		fmt.Fprintf(rc.Err, "iconv: %v\n", err)
		return 1
	}
	if len(files) == 0 {
		files = []string{"-"}
	}

	// Keep one encoder across all operands: iconv concatenates FILE inputs into
	// one output stream, so a BOM/state prefix must not be emitted per file.
	trackedOut := &errorTrackingWriter{w: rc.Out}
	out := transform.NewWriter(trackedOut, to.NewEncoder())
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
		_, copyErr := io.Copy(out, transform.NewReader(in, from.NewDecoder()))
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
