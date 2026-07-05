package gosed

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// FastSubstitution streams the common single-command sed form
// s/re/repl/[g] without running the full instruction VM per line.
type FastSubstitution struct {
	pattern     *regexp.Regexp
	replacement []byte
	literalFrom []byte
	literalTo   []byte
	which       int
	pflag       bool
	gflag       bool
	quiet       bool
}

// NewFastSubstitution returns a fast processor for a script that is exactly
// one unaddressed substitution command. Other scripts return nil, nil so the
// caller can use the general sed engine.
func NewFastSubstitution(program string, quiet bool) (*FastSubstitution, error) {
	pattern, replacement, mods, ok, err := parseFastSubstitution(program)
	if err != nil || !ok {
		return nil, err
	}

	subst := &FastSubstitution{quiet: quiet}
	var numbers []rune
	var flags string
	for _, char := range mods {
		switch char {
		case 'p':
			subst.pflag = true
		case 'g':
			subst.gflag = true
		case 'i', 'I':
			if !strings.ContainsRune(flags, 'i') {
				flags += "i"
			}
		case 'm', 'M':
			if !strings.ContainsRune(flags, 'm') {
				flags += "m"
			}
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			numbers = append(numbers, char)
		default:
			return nil, fmt.Errorf("Bad regexp modifier <%v>", char)
		}
	}
	if len(numbers) > 0 {
		n, _ := strconv.Atoi(string(numbers))
		if n == 0 {
			return nil, fmt.Errorf("Bad number %d on substitution", n)
		}
		subst.which = n - 1
	}

	prefix := ""
	if flags != "" {
		prefix = "(?" + flags + ")"
	}
	rx, err := compileRE(pattern, prefix)
	if err != nil {
		return nil, err
	}
	if rx.MatchString("") {
		return nil, nil
	}
	subst.pattern = rx
	subst.replacement = []byte(translateReplacement(replacement))

	if flags == "" && subst.which == 0 && isLiteralPattern(pattern) {
		if literalReplacement, ok := literalReplacement(replacement); ok {
			subst.literalFrom = []byte(pattern)
			subst.literalTo = literalReplacement
		}
	}

	return subst, nil
}

// Run applies the substitution to each input line and writes GNU-style output.
func (s *FastSubstitution) Run(in io.Reader, out io.Writer) error {
	br := bufio.NewReader(in)
	bw := bufio.NewWriter(out)
	var partial []byte
	var dst []byte
	var repl []byte

	for {
		line, err := br.ReadSlice('\n')
		if err == bufio.ErrBufferFull {
			partial = append(partial, line...)
			continue
		}
		if len(partial) > 0 {
			partial = append(partial, line...)
			line = partial
		}
		if len(line) > 0 {
			hasNewline := line[len(line)-1] == '\n'
			body := line
			if hasNewline {
				body = line[:len(line)-1]
			}

			var changed bool
			dst, repl, changed = s.replaceLine(dst[:0], repl[:0], body)
			if s.pflag && changed {
				if _, err := bw.Write(dst); err != nil {
					return err
				}
				if hasNewline {
					if err := bw.WriteByte('\n'); err != nil {
						return err
					}
				}
			}
			if !s.quiet {
				if _, err := bw.Write(dst); err != nil {
					return err
				}
				if hasNewline {
					if err := bw.WriteByte('\n'); err != nil {
						return err
					}
				}
			}
		}
		partial = partial[:0]

		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return bw.Flush()
}

func (s *FastSubstitution) replaceLine(dst, repl, src []byte) ([]byte, []byte, bool) {
	if s.literalFrom != nil {
		first := bytes.Index(src, s.literalFrom)
		if first < 0 {
			return src, repl, false
		}
		dst = append(dst, src[:first]...)
		dst = append(dst, s.literalTo...)
		offset := first + len(s.literalFrom)
		if !s.gflag {
			dst = append(dst, src[offset:]...)
			return dst, repl, true
		}
		for {
			next := bytes.Index(src[offset:], s.literalFrom)
			if next < 0 {
				dst = append(dst, src[offset:]...)
				return dst, repl, true
			}
			next += offset
			dst = append(dst, src[offset:next]...)
			dst = append(dst, s.literalTo...)
			offset = next + len(s.literalFrom)
		}
	}

	search := src
	offset := 0
	replaced := 0
	for {
		idx := s.pattern.FindSubmatchIndex(search)
		if idx == nil {
			dst = append(dst, src[offset:]...)
			return dst, repl, replaced > 0
		}
		start, end := offset+idx[0], offset+idx[1]
		if replaced < s.which {
			dst = append(dst, src[offset:end]...)
			offset = end
			search = src[offset:]
			replaced++
			continue
		}

		dst = append(dst, src[offset:start]...)
		repl = s.pattern.Expand(repl[:0], s.replacement, src, offsetIndexes(idx, offset))
		dst = append(dst, repl...)
		replaced++
		offset = end
		if !s.gflag {
			dst = append(dst, src[offset:]...)
			return dst, repl, true
		}
		if start == end {
			if offset >= len(src) {
				return dst, repl, true
			}
			dst = append(dst, src[offset])
			offset++
		}
		search = src[offset:]
	}
}

func offsetIndexes(idx []int, offset int) []int {
	for i := range idx {
		if idx[i] >= 0 {
			idx[i] += offset
		}
	}
	return idx
}

func parseFastSubstitution(program string) (pattern, replacement, mods string, ok bool, err error) {
	program = strings.TrimSpace(program)
	if len(program) < 2 || program[0] != 's' {
		return "", "", "", false, nil
	}
	delim := program[1]
	if delim == '\\' || delim == '\n' {
		return "", "", "", false, nil
	}
	i := 2
	pattern, i, err = readFastDelimited(program, i, delim, false)
	if err != nil {
		return "", "", "", true, err
	}
	replacement, i, err = readFastDelimited(program, i, delim, true)
	if err != nil {
		return "", "", "", true, err
	}
	mods = strings.TrimSpace(program[i:])
	if strings.ContainsAny(mods, ";\n\r\t ") {
		return "", "", "", false, nil
	}
	return pattern, replacement, mods, true, nil
}

func readFastDelimited(s string, i int, delim byte, replacement bool) (string, int, error) {
	var b strings.Builder
	for i < len(s) {
		c := s[i]
		i++
		switch c {
		case '\n':
			return "", i, fmt.Errorf("end-of-line while looking for %c", delim)
		case delim:
			return b.String(), i, nil
		case '\\':
			if i >= len(s) {
				b.WriteByte('\\')
				continue
			}
			next := s[i]
			i++
			if next == delim {
				b.WriteByte(delim)
			} else {
				b.WriteByte('\\')
				b.WriteByte(next)
			}
		case '\r':
			if !replacement {
				b.WriteByte(c)
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", i, fmt.Errorf("end-of-file while looking for %c", delim)
}

func isLiteralPattern(pattern string) bool {
	if pattern == "" {
		return false
	}
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '.', '[', '\\', '*', '^', '$':
			return false
		}
		if ExtendedRegex {
			switch pattern[i] {
			case '+', '?', '(', ')', '{', '}', '|':
				return false
			}
		}
	}
	return true
}

func literalReplacement(replacement string) ([]byte, bool) {
	var b []byte
	for i := 0; i < len(replacement); i++ {
		switch c := replacement[i]; c {
		case '&':
			return nil, false
		case '\\':
			if i+1 >= len(replacement) {
				b = append(b, '\\')
				break
			}
			n := replacement[i+1]
			i++
			switch {
			case n >= '0' && n <= '9':
				return nil, false
			case n == '&':
				b = append(b, '&')
			case n == '\\':
				b = append(b, '\\')
			case n == 'n':
				b = append(b, '\n')
			case n == 't':
				b = append(b, '\t')
			case n == 'r':
				b = append(b, '\r')
			default:
				b = append(b, n)
			}
		default:
			b = append(b, c)
		}
	}
	return b, true
}
