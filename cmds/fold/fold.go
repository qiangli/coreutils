// Package foldcmd implements fold(1): wrap input lines.
package foldcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "fold",
	Synopsis: "Wrap input lines in each FILE, writing to standard output.\nWith no FILE, or when FILE is -, read standard input.",
	Usage:    "fold [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type countMode int

const (
	countColumns countMode = iota
	countBytes
	countCharacters
)

type encodingMode int

const (
	encodingSingleByte encodingMode = iota
	encodingLatin1
	encodingUTF8
)

type characterModel struct {
	encoding encodingMode
	width    *runewidth.Condition
}

func resolveCharacterModel(env []string) (*characterModel, error) {
	name := locale.Resolve(env, locale.CType)
	base, codeset := splitLocaleName(name)
	switch {
	case (base == "C" || base == "POSIX") && codeset == "":
		return &characterModel{encoding: encodingSingleByte}, nil
	case (base == "C" || base == "POSIX") && normalizeCodeset(codeset) == "UTF8":
		return &characterModel{encoding: encodingUTF8, width: runewidth.NewCondition()}, nil
	case strings.EqualFold(base, "de_DE") && normalizeCodeset(codeset) == "ISO88591":
		return &characterModel{encoding: encodingLatin1}, nil
	default:
		return nil, fmt.Errorf(
			"LC_CTYPE %q is unavailable; supported locales are C/POSIX, their UTF-8 aliases, and de_DE.ISO-8859-1",
			name,
		)
	}
}

func splitLocaleName(name string) (base, codeset string) {
	name, _, _ = strings.Cut(name, "@")
	base, codeset, _ = strings.Cut(name, ".")
	return base, codeset
}

func normalizeCodeset(name string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", "_", "").Replace(name))
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	posix := envPresent(rc.Env, "POSIXLY_CORRECT")
	widthValue := fs.StringP("width", "w", "80", "use WIDTH columns instead of 80")
	bytesMode := fs.BoolP("bytes", "b", false, "count bytes rather than columns")
	characters := fs.BoolP("characters", "c", false, "count characters rather than columns")
	spaces := fs.BoolP("spaces", "s", false, "break at spaces")
	args = rewriteObsoleteWidth(args, posix)
	var operands []string
	var code int
	if posix {
		operands, code = tool.ParseRequireOrder(rc, cmd, fs, args)
	} else {
		operands, code = tool.Parse(rc, cmd, fs, args)
	}
	if code >= 0 {
		return code
	}
	width, err := strconv.Atoi(*widthValue)
	if err != nil || width <= 0 {
		return tool.UsageError(rc, cmd, "invalid number of columns: %q", *widthValue)
	}
	mode := countColumns
	if *bytesMode {
		mode = countBytes
	} else if *characters {
		mode = countCharacters
	}
	if len(operands) == 0 {
		operands = []string{"-"}
	}
	model, err := resolveCharacterModel(rc.Env)
	if err != nil {
		fmt.Fprintf(rc.Err, "fold: %v\n", err)
		return 1
	}

	out := bufio.NewWriter(rc.Out)
	status := 0
	for _, name := range operands {
		r, closer, err := openInput(rc, name)
		if err != nil {
			fmt.Fprintf(rc.Err, "fold: %s: %v\n", name, err)
			status = 1
			continue
		}
		if err := foldStream(r, out, width, mode, *spaces, model); err != nil {
			fmt.Fprintf(rc.Err, "fold: %s: %v\n", name, err)
			status = 1
		}
		if closer != nil {
			closer.Close()
		}
	}
	if err := out.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "fold: write error: %v\n", err)
		return 1
	}
	return status
}

// rewriteObsoleteWidth implements the obsolete option syntax
// (fold -72 == fold -w 72), which GNU fold accepts anywhere on the
// command line; the last width given wins.
func rewriteObsoleteWidth(args []string, requireOrder bool) []string {
	out := make([]string, 0, len(args))
	for i, a := range args {
		if a == "--" {
			out = append(out, args[i:]...)
			break
		}
		if requireOrder && (a == "-" || !strings.HasPrefix(a, "-")) {
			// POSIX Utility Syntax Guideline 9 ends option recognition at the
			// first operand. In particular, a later filename such as -3 must
			// not be rewritten into fold's obsolete width syntax.
			out = append(out, args[i:]...)
			break
		}
		if len(a) >= 2 && a[0] == '-' && allDigits(a[1:]) {
			out = append(out, "-w"+a[1:])
			continue
		}
		out = append(out, a)
	}
	return out
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

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

type inputCharacter struct {
	raw   []byte
	rune  rune
	valid bool
}

type characterReader struct {
	r     *bufio.Reader
	model *characterModel
}

func (r *characterReader) next() (inputCharacter, error) {
	if r.model.encoding != encodingUTF8 {
		b, err := r.r.ReadByte()
		if err != nil {
			return inputCharacter{}, err
		}
		return inputCharacter{raw: []byte{b}, rune: rune(b), valid: true}, nil
	}
	ch, size, err := r.r.ReadRune()
	if err != nil {
		return inputCharacter{}, err
	}
	if err := r.r.UnreadRune(); err != nil {
		return inputCharacter{}, err
	}
	raw := make([]byte, size)
	if _, err := io.ReadFull(r.r, raw); err != nil {
		return inputCharacter{}, err
	}
	valid := ch != utf8.RuneError || size > 1
	return inputCharacter{raw: raw, rune: ch, valid: valid}, nil
}

// foldStream retains the exact input bytes for every decoded character. The
// locale controls character boundaries and display widths, never output
// transcoding. This is load-bearing for single-byte locales and malformed
// UTF-8: neither may turn into the UTF-8 encoding of U+FFFD.
func foldStream(
	r io.Reader, w io.Writer, width int, mode countMode, spaces bool,
	model *characterModel,
) error {
	reader := &characterReader{r: bufio.NewReader(r), model: model}
	var line []inputCharacter
	col := 0

	adjust := func(c int, ch inputCharacter) int {
		if mode == countBytes {
			return c + len(ch.raw)
		}
		if len(ch.raw) == 1 {
			switch ch.raw[0] {
			case '\b':
				if c > 0 {
					c--
				}
				return c
			case '\r':
				return 0
			case '\t':
				return c + 8 - c%8
			}
		}
		if mode == countCharacters || model.encoding != encodingUTF8 || !ch.valid {
			return c + 1
		}
		charWidth := model.width.RuneWidth(ch.rune)
		if charWidth < 0 {
			charWidth = 1
		}
		return c + charWidth
	}

	writeLine := func(chars []inputCharacter, newline bool) error {
		for _, ch := range chars {
			if err := writeFull(w, ch.raw); err != nil {
				return err
			}
		}
		if newline {
			return writeFull(w, []byte{'\n'})
		}
		return nil
	}

	for {
		ch, err := reader.next()
		if err == io.EOF {
			if len(line) > 0 {
				return writeLine(line, false)
			}
			return nil
		}
		if err != nil {
			return err
		}
		if len(ch.raw) == 1 && ch.raw[0] == '\n' {
			if err := writeLine(line, true); err != nil {
				return err
			}
			line = line[:0]
			col = 0
			continue
		}
	rescan:
		newCol := adjust(col, ch)
		if newCol > width {
			if spaces {
				if i := lastBlank(line); i >= 0 {
					if err := writeLine(line[:i+1], true); err != nil {
						return err
					}
					line = append(line[:0], line[i+1:]...)
					col = 0
					for _, previous := range line {
						col = adjust(col, previous)
					}
					goto rescan
				}
			}
			if len(line) == 0 {
				if err := writeFull(w, []byte{'\n'}); err != nil {
					return err
				}
				line = append(line, ch)
				col = newCol
				continue
			}
			if err := writeLine(line, true); err != nil {
				return err
			}
			line = line[:0]
			col = 0
			if adjust(0, ch) > width {
				line = append(line, ch)
				col = adjust(0, ch)
				continue
			}
			goto rescan
		}
		line = append(line, ch)
		col = newCol
	}
}

func writeFull(w io.Writer, p []byte) error {
	n, err := w.Write(p)
	if err == nil && n != len(p) {
		return io.ErrShortWrite
	}
	return err
}

func lastBlank(chars []inputCharacter) int {
	for i := len(chars) - 1; i >= 0; i-- {
		if len(chars[i].raw) == 1 && (chars[i].raw[0] == ' ' || chars[i].raw[0] == '\t') {
			return i
		}
	}
	return -1
}

func openInput(rc *tool.RunContext, name string) (io.Reader, io.Closer, error) {
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
	return f, f, nil
}
