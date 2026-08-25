package iconvcmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/qiangli/coreutils/tool"
)

type charmapTable struct {
	bySymbol map[string][]byte
	byBytes  map[string][]string
	maxBytes int
}

func runCharmapConversion(rc *tool.RunContext, fromPath, toPath string, files []string, discard, silent bool) int {
	from, err := readCharmap(rc, fromPath)
	if err != nil {
		fmt.Fprintf(rc.Err, "iconv: %s: %v\n", fromPath, err)
		return 1
	}
	to, err := readCharmap(rc, toPath)
	if err != nil {
		fmt.Fprintf(rc.Err, "iconv: %s: %v\n", toPath, err)
		return 1
	}
	status := 0
	for _, name := range files {
		if err := rc.Ctx.Err(); err != nil {
			fmt.Fprintf(rc.Err, "iconv: %v\n", err)
			return 1
		}
		in, closeInput, err := openInput(rc, name)
		if err != nil {
			fmt.Fprintf(rc.Err, "iconv: %s: %v\n", name, err)
			status = 1
			continue
		}
		data, readErr := io.ReadAll(in)
		closeErr := closeInput()
		if readErr != nil {
			fmt.Fprintf(rc.Err, "iconv: %s: %v\n", displayName(name), readErr)
			status = 1
		}
		if closeErr != nil {
			fmt.Fprintf(rc.Err, "iconv: %s: %v\n", displayName(name), closeErr)
			status = 1
		}
		invalid := false
		for pos := 0; pos < len(data); {
			consumed, replacement := charmapMatch(from, to, data[pos:])
			if consumed == 0 || replacement == nil {
				invalid = true
				pos++
				continue
			}
			n, err := rc.Out.Write(replacement)
			if err == nil && n != len(replacement) {
				err = io.ErrShortWrite
			}
			if err != nil {
				fmt.Fprintf(rc.Err, "iconv: write error: %v\n", err)
				return 1
			}
			pos += consumed
		}
		if invalid {
			// The documented no--c result is the same omission as -c; the
			// normative distinction is that -c requires it. Status is invariant.
			if !discard && !silent {
				fmt.Fprintf(rc.Err, "iconv: %s: invalid character sequence\n", displayName(name))
			}
			status = 1
		}
	}
	return status
}

func charmapMatch(from, to *charmapTable, input []byte) (int, []byte) {
	limit := from.maxBytes
	if limit > len(input) {
		limit = len(input)
	}
	for n := limit; n > 0; n-- {
		symbols, found := from.byBytes[string(input[:n])]
		if !found {
			continue
		}
		for _, symbol := range symbols {
			if encoded, ok := to.bySymbol[symbol]; ok {
				return n, encoded
			}
		}
		return n, nil
	}
	return 0, nil
}

func readCharmap(rc *tool.RunContext, path string) (*charmapTable, error) {
	f, err := os.Open(rc.Path(path))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseCharmap(f)
}

// parseCharmap reads the XBD 6.4 mapping grammar needed for iconv's symbolic
// join. WIDTH data after END CHARMAP is intentionally irrelevant. POSIX makes
// an invalid charmap's conversion result undefined, so this is not a second
// localedef validator: it rejects malformed structure and byte tokens but does
// not prove Portable Character Set completeness, declaration consistency, or
// the same-constant-kind rule for concatenated bytes.
func parseCharmap(r io.Reader) (*charmapTable, error) {
	table := &charmapTable{bySymbol: make(map[string][]byte), byBytes: make(map[string][]string)}
	escape, comment := byte('\\'), byte('#')
	inMap, ended := false, false
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 4096), 1024*1024)
	lineNo := 0
	for s.Scan() {
		lineNo++
		line := strings.TrimSuffix(s.Text(), "\r")
		if !inMap {
			fields := strings.Fields(line)
			if len(fields) == 0 || line[0] == comment {
				continue
			}
			if line == "CHARMAP" {
				inMap = true
				continue
			}
			if len(fields) >= 2 && (fields[0] == "<escape_char>" || fields[0] == "<comment_char>") {
				if len(fields[1]) != 1 {
					return nil, fmt.Errorf("line %d: %s must be one byte", lineNo, fields[0])
				}
				if fields[0] == "<escape_char>" {
					escape = fields[1][0]
				} else {
					comment = fields[1][0]
				}
			}
			continue
		}
		if line == "END CHARMAP" {
			ended = true
			break
		}
		if line == "" || line[0] == comment {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("line %d: invalid mapping", lineNo)
		}
		encoded, err := parseCharmapEncoding(fields[1], escape)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		symbols, err := expandCharmapSymbols(fields[0], escape)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		value := append([]byte(nil), encoded...)
		for i, symbol := range symbols {
			if i > 0 && !incrementEncoding(value) {
				return nil, fmt.Errorf("line %d: range encoding overflow", lineNo)
			}
			if old, exists := table.bySymbol[symbol]; exists && !bytes.Equal(old, value) {
				return nil, fmt.Errorf("line %d: conflicting definition for %s", lineNo, symbol)
			}
			table.bySymbol[symbol] = append([]byte(nil), value...)
			table.byBytes[string(value)] = append(table.byBytes[string(value)], symbol)
			if len(value) > table.maxBytes {
				table.maxBytes = len(value)
			}
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if !inMap || !ended {
		return nil, fmt.Errorf("missing CHARMAP or END CHARMAP")
	}
	if len(table.bySymbol) == 0 {
		return nil, fmt.Errorf("empty CHARMAP")
	}
	return table, nil
}

func parseCharmapEncoding(s string, escape byte) ([]byte, error) {
	var out []byte
	for len(s) > 0 {
		if s[0] != escape || len(s) < 3 {
			return nil, fmt.Errorf("invalid encoding %q", s)
		}
		s = s[1:]
		base, width := 8, 0
		if s[0] == 'x' {
			base, width, s = 16, 2, s[1:]
		} else if s[0] == 'd' {
			base, s = 10, s[1:]
			width = digitRun(s, base, 3)
			if width < 2 {
				return nil, fmt.Errorf("invalid decimal encoding")
			}
		} else {
			width = digitRun(s, base, 3)
			if width < 2 {
				return nil, fmt.Errorf("invalid octal encoding")
			}
		}
		if len(s) < width {
			return nil, fmt.Errorf("truncated encoding")
		}
		v, err := strconv.ParseUint(s[:width], base, 8)
		if err != nil {
			return nil, fmt.Errorf("invalid encoding byte %q", s[:width])
		}
		out = append(out, byte(v))
		s = s[width:]
	}
	return out, nil
}

func digitRun(s string, base, max int) int {
	n := 0
	for n < len(s) && n < max {
		r := rune(s[n])
		valid := unicode.IsDigit(r) && (base != 8 || r < '8')
		if base == 16 {
			valid = unicode.IsDigit(r) || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F'
		}
		if !valid {
			break
		}
		n++
	}
	return n
}

func expandCharmapSymbols(field string, escape byte) ([]string, error) {
	parts := strings.Split(field, "...")
	if len(parts) == 1 {
		if !validCharmapSymbol(parts[0]) {
			return nil, fmt.Errorf("invalid symbolic name %q", field)
		}
		return []string{normalizeCharmapSymbol(parts[0], escape)}, nil
	}
	if len(parts) != 2 || !validCharmapSymbol(parts[0]) || !validCharmapSymbol(parts[1]) {
		return nil, fmt.Errorf("invalid symbolic range %q", field)
	}
	aPart := normalizeCharmapSymbol(parts[0], escape)
	bPart := normalizeCharmapSymbol(parts[1], escape)
	a, b := aPart[1:len(aPart)-1], bPart[1:len(bPart)-1]
	ai := len(a)
	for ai > 0 && a[ai-1] >= '0' && a[ai-1] <= '9' {
		ai--
	}
	bi := len(b)
	for bi > 0 && b[bi-1] >= '0' && b[bi-1] <= '9' {
		bi--
	}
	if ai == len(a) || bi != ai || a[:ai] != b[:bi] || len(a)-ai != len(b)-bi {
		return nil, fmt.Errorf("invalid symbolic range %q", field)
	}
	start, _ := strconv.Atoi(a[ai:])
	end, _ := strconv.Atoi(b[bi:])
	if end < start || end-start > 65535 {
		return nil, fmt.Errorf("invalid symbolic range %q", field)
	}
	width := len(a) - ai
	out := make([]string, 0, end-start+1)
	for i := start; i <= end; i++ {
		out = append(out, fmt.Sprintf("<%s%0*d>", a[:ai], width, i))
	}
	return out, nil
}

func validCharmapSymbol(s string) bool { return len(s) >= 3 && s[0] == '<' && s[len(s)-1] == '>' }

func normalizeCharmapSymbol(s string, escape byte) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == escape && i+1 < len(s) {
			i++
		}
		out = append(out, s[i])
	}
	return string(out)
}

func incrementEncoding(b []byte) bool {
	for i := len(b) - 1; i >= 0; i-- {
		b[i]++
		if b[i] != 0 {
			return true
		}
	}
	return false
}
