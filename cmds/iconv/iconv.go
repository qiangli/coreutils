// Package iconvcmd implements a portable, pure-Go subset of POSIX iconv(1).
//
// Supported options are -f/--from-code, -t/--to-code, and -s/--silent.
// Charset names are resolved through golang.org/x/text's IANA registry; this
// includes UTF-8/16, the ISO-8859 families, Windows code pages, Shift_JIS,
// EUC-JP, Big5, GBK, and GB18030. A registered charset for which x/text has no
// implementation is rejected rather than approximated. POSIX -c is recognized
// but rejected explicitly until invalid-sequence omission can be implemented
// without hiding conversion errors.
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
	Usage:    "iconv [-s] -f FROMCODE -t TOCODE [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	fromName := fs.StringP("from-code", "f", "", "convert text from encoding FROMCODE")
	toName := fs.StringP("to-code", "t", "", "convert text to encoding TOCODE")
	silent := fs.BoolP("silent", "s", false, "suppress messages about invalid characters")
	discardInvalid := fs.BoolP("discard-invalid", "c", false, "discard invalid input characters")
	files, code := tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args))
	if code >= 0 {
		return code
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
	// IANA's registry spells UTF-8 canonically, but accepting the customary
	// underscore form is useful on platforms whose locale names use it.
	name = strings.TrimSpace(name)
	if strings.EqualFold(name, "UTF_8") {
		name = "UTF-8"
	}
	if strings.EqualFold(name, "UTF-8") {
		return unicodeenc.UTF8, nil
	}
	enc, err := ianaindex.IANA.Encoding(name)
	if err != nil {
		return nil, fmt.Errorf("unsupported encoding %q", name)
	}
	if enc == nil {
		return nil, fmt.Errorf("encoding %q is registered but not supported", name)
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
