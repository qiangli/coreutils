package localedef

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

type lineReader struct {
	r               *bufio.Reader
	line            int
	escape, comment byte
}

func newLines(r io.Reader) *lineReader {
	return &lineReader{r: bufio.NewReader(r), escape: '\\', comment: '#'}
}
func (r *lineReader) next() (string, int, error) {
	var b strings.Builder
	start := r.line + 1
	for {
		s, err := r.r.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", start, err
		}
		if s == "" && err == io.EOF {
			if b.Len() > 0 {
				return "", start, problem(start, "unfinished continuation")
			}
			return "", start, io.EOF
		}
		r.line++
		s = strings.TrimSuffix(strings.TrimSuffix(s, "\n"), "\r")
		if strings.HasPrefix(strings.TrimSpace(s), string(r.comment)) {
			if b.Len() == 0 {
				return "", start, nil
			}
			continue
		}
		n := 0
		for i := len(s) - 1; i >= 0 && s[i] == r.escape; i-- {
			n++
		}
		if n%2 == 1 {
			if err == io.EOF {
				return "", start, problem(start, "unfinished continuation")
			}
			b.WriteString(s[:len(s)-1])
			continue
		}
		b.WriteString(s)
		return b.String(), start, nil
	}
}
func (r *lineReader) directive(s string, line int) (bool, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return false, nil
	}
	key := strings.Trim(fields[0], "<>")
	if key != "escape_char" && key != "comment_char" {
		return false, nil
	}
	if len(fields) != 2 || len(fields[1]) != 1 {
		return true, problem(line, "%s requires one character", key)
	}
	if key == "escape_char" {
		r.escape = fields[1][0]
	} else {
		r.comment = fields[1][0]
	}
	return true, nil
}

func escaped(s string, i int, escape byte) (string, int, error) {
	if i+1 >= len(s) {
		return "", i, problem(0, "unfinished escape")
	}
	j := i + 1
	base, max, min := 8, 3, 2
	if s[j] == 'x' {
		base, max, min = 16, 2, 2
		j++
	} else if s[j] == 'd' {
		base, max, min = 10, 3, 2
		j++
	} else if s[j] < '0' || s[j] > '7' {
		return s[j : j+1], j + 1, nil
	}
	start := j
	for j < len(s) && j-start < max {
		c := s[j]
		digit := int(c - '0')
		if c >= 'a' && c <= 'f' {
			digit = int(c-'a') + 10
		}
		if c >= 'A' && c <= 'F' {
			digit = int(c-'A') + 10
		}
		if digit < 0 || digit >= base {
			break
		}
		j++
	}
	if j-start < min {
		return "", i, problem(0, "invalid numeric escape")
	}
	n, err := strconv.ParseUint(s[start:j], base, 8)
	if err != nil {
		return "", i, problem(0, "byte encoding out of range")
	}
	return string([]byte{byte(n)}), j, nil
}
func symbol(s string, i int, escape byte) (string, int, error) {
	var b strings.Builder
	for j := i + 1; j < len(s); {
		if s[j] == '>' {
			if b.Len() == 0 {
				break
			}
			return b.String(), j + 1, nil
		}
		if s[j] == escape {
			v, n, err := escaped(s, j, escape)
			if err != nil {
				return "", i, err
			}
			b.WriteString(v)
			j = n
		} else {
			b.WriteByte(s[j])
			j++
		}
	}
	return "", i, problem(0, "unterminated or empty symbolic name")
}
func lex(s string, escape, comment byte) ([]Value, error) {
	var out []Value
	for i := 0; i < len(s); {
		c := s[i]
		if c == ' ' || c == '\t' {
			i++
			continue
		}
		if c == comment {
			break
		}
		switch {
		case c == '<':
			v, n, err := symbol(s, i, escape)
			if err != nil {
				return nil, err
			}
			out = append(out, Value{Kind: Symbol, Text: v})
			i = n
		case c == '"':
			i++
			v := Value{Kind: String}
			var literal strings.Builder
			flush := func() {
				if literal.Len() > 0 {
					v.Parts = append(v.Parts, Value{Kind: Bytes, Text: literal.String()})
					literal.Reset()
				}
			}
			for i < len(s) && s[i] != '"' {
				if s[i] == escape {
					x, n, err := escaped(s, i, escape)
					if err != nil {
						return nil, err
					}
					literal.WriteString(x)
					v.Text += x
					i = n
				} else if s[i] == '<' {
					flush()
					x, n, err := symbol(s, i, escape)
					if err != nil {
						return nil, err
					}
					v.Parts = append(v.Parts, Value{Kind: Symbol, Text: x})
					v.Text += "<" + x + ">"
					i = n
				} else {
					literal.WriteByte(s[i])
					v.Text += s[i : i+1]
					i++
				}
			}
			if i == len(s) {
				return nil, problem(0, "unterminated string")
			}
			flush()
			out = append(out, v)
			i++
		case c == escape:
			v := Value{Kind: Bytes}
			for i < len(s) && s[i] == escape {
				x, n, err := escaped(s, i, escape)
				if err != nil {
					return nil, err
				}
				v.Text += x
				i = n
			}
			out = append(out, v)
		case strings.ContainsRune(";,()", rune(c)):
			out = append(out, Value{Kind: Separator, Text: s[i : i+1]})
			i++
		case strings.HasPrefix(s[i:], ".."):
			j := i
			for j < len(s) && s[j] == '.' {
				j++
			}
			if j-i != 2 && j-i != 3 {
				return nil, problem(0, "invalid ellipsis")
			}
			out = append(out, Value{Kind: Ellipsis, Text: s[i:j]})
			i = j
		default:
			j := i + 1
			for j < len(s) && !strings.ContainsRune(" \t<>\";,()", rune(s[j])) && s[j] != escape && s[j] != comment {
				j++
			}
			v := Value{Kind: Word, Text: s[i:j]}
			if _, err := strconv.Atoi(v.Text); err == nil {
				v.Kind = Number
			}
			out = append(out, v)
			i = j
		}
	}
	return out, nil
}
