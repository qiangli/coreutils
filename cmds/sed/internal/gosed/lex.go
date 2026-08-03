package gosed

// the lexer for SED.  The point of the lexer is to
// reliably transform the input into a series of token structs.
// These structs know the source location, and the token type, and
// any arguments to the token (e.g., a regexp's '/' argument is the
// regular expression itself).
//
// The lexer also simplifies and regularises the input, for instance
// by eliminating comments.

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode"
)

type location struct {
	line int
	pos  int
}

func (l *location) String() string {
	return fmt.Sprintf("at line %d, pos %d", l.line, l.pos)
}

const (
	tok_NUM = iota
	tok_RX
	tok_COMMA
	tok_BANG
	tok_DOLLAR
	tok_LBRACE
	tok_RBRACE
	tok_EOL
	tok_CMD
	tok_CHANGE
	tok_LABEL
	tok_RELNUM
)

type token struct {
	location
	typ    int
	letter rune
	args   []string
}

// ----------------------------------------------------------
//
//	Location-tracking reader
//
// ----------------------------------------------------------
type locReader struct {
	location
	eol bool // state for end of line, true when last rune was '\n'
	r   *bufio.Reader
}

func (lr *locReader) ReadRune() (rune, int, error) {
	r, i, err := lr.r.ReadRune()

	lr.pos++

	if lr.eol {
		lr.pos = 1
		lr.line++
		lr.eol = false
	}
	if r == '\n' {
		lr.eol = true
	}

	return r, i, err
}

func (lr *locReader) UnreadRune() error {
	lr.pos--
	lr.eol = false

	if lr.pos == 0 {
		lr.line--
		lr.eol = true
	}
	return lr.r.UnreadRune()
}

func (lr *locReader) ReadLine() (nxtl string, err error) {
	var prefix = true
	var line []byte

	var lines []string

	for prefix {
		line, prefix, err = lr.r.ReadLine()
		if err != nil {
			break
		}
		buf := make([]byte, len(line))
		copy(buf, line)
		lines = append(lines, string(buf))
	}

	nxtl = strings.Join(lines, "")

	// fixup our position information
	lr.pos += len(nxtl)
	lr.eol = true

	return
}

// ----------------------------------------------------------
// lexer functions
// ----------------------------------------------------------
func skipComment(r *locReader) (rune, error) {
	var err error
	var cur rune = ' '
	for (cur != '\n') && (err == nil) {
		cur, _, err = r.ReadRune()
	}
	return ';', err
}

func skipWS(r *locReader) (rune, error) {
	var err error
	var cur rune = ' '
	for {
		switch {
		case cur == '\n':
			return ';', err
		case cur == '#':
			return skipComment(r)
		case !unicode.IsSpace(cur):
			return cur, err
		}
		cur, _, err = r.ReadRune()
	}
}

func readNumber(r *locReader, character rune) (string, error) {
	var buffer bytes.Buffer

	var err error
	for (err == nil) && unicode.IsDigit(character) {
		buffer.WriteRune(character)
		character, _, err = r.ReadRune()
	}

	if err == nil {
		err = r.UnreadRune()
	}

	return buffer.String(), err
}

// readDelimited reads until it finds the delimter character,
// returning the string (not including the delimiter). It does
// allow the delimiter to be escaped by a backslash ('\').
// It is an error to reach EOL while looking for the delimiter.
//
// Escape state is tracked by parity, not by "the previous rune was a
// backslash": in `y/a\\/b\\/` the delimiter after `\\` closes the string,
// because the two backslashes are themselves an escaped backslash.
func readDelimited(r *locReader, delimiter rune) (string, error) {
	var buffer bytes.Buffer
	escaped := false

	for {
		character, _, err := r.ReadRune()
		if err != nil {
			if err == io.EOF {
				err = fmt.Errorf("end-of-file while looking for %c", delimiter)
			}
			return buffer.String(), err
		}

		if character == '\n' {
			return buffer.String(), fmt.Errorf("end-of-line while looking for %c", delimiter)
		}
		if !escaped && character == delimiter {
			return buffer.String(), nil
		}

		buffer.WriteRune(character)
		escaped = !escaped && character == '\\'
	}
}

// readReplacement reads the s/// replacement up to the first UNESCAPED
// delimiter, capturing it VERBATIM (backslash sequences intact) so the GNU
// replacement decoder (translateReplacement in gnu.go) can interpret \1..\9,
// &, \&, \\, \n, \t, etc. exactly as GNU sed does. A backslash-escaped
// delimiter is folded to a literal delimiter so the scan ends at the right
// place. (A \r is dropped, matching the original reader.)
func readReplacement(r *locReader, delimiter rune) (string, error) {
	var buffer bytes.Buffer
	for {
		character, _, err := r.ReadRune()
		if err != nil {
			if err == io.EOF {
				err = fmt.Errorf("end-of-file while looking for %c", delimiter)
			}
			return buffer.String(), err
		}
		switch character {
		case '\r':
			continue
		case '\n':
			return buffer.String(), fmt.Errorf("end-of-line while looking for %c", delimiter)
		case delimiter:
			return buffer.String(), nil
		case '\\':
			next, _, err := r.ReadRune()
			if err != nil {
				if err == io.EOF {
					err = fmt.Errorf("end-of-file while looking for %c", delimiter)
				}
				return buffer.String(), err
			}
			if next == delimiter {
				buffer.WriteRune(delimiter) // \<delim> → literal delimiter
			} else {
				buffer.WriteRune('\\') // keep the escape for translateReplacement
				buffer.WriteRune(next)
			}
		default:
			buffer.WriteRune(character)
		}
	}
}

// leadingTextBlanks normalises the FIRST line of an a/i/c text argument, the
// only line where the text shares its line with the command letter.
//
// Two things happen there and nowhere else. Leading <blank>s between the
// command letter and the text are separators, not text ("a hello" appends
// "hello"), so they are dropped. And a <backslash> may then appear as the
// POSIX escape that makes the next character ordinary — the backslash is
// removed and what follows is taken literally, which is how "a\   x" keeps
// its blanks while "a   x" does not.
//
// Continuation lines are untouched: their leading blanks are part of the text.
func leadingTextBlanks(txt string) string {
	txt = strings.TrimLeft(txt, " \t")
	if strings.HasPrefix(txt, `\`) {
		txt = txt[1:]
	}
	return txt
}

// readMultiLine reads until it finds an unescaped newline. It discards the
// first line, if it is empty, because commands like "c\", "a\" and "i\" are
// intended to be used that way.
func readMultiLine(r *locReader) (string, error) {
	var lines []string
	var err error

	first := true
	hasSlash := true // does the line end in a slash?

	for hasSlash {
		txt, err := r.ReadLine()
		if err != nil {
			break
		}
		tlen := len(txt)

		// strip off the final '\', if there is one
		if tlen > 0 && txt[tlen-1] == '\\' {
			txt = txt[:tlen-1]
		} else {
			hasSlash = false
		}

		// If it's empty and the first line, forget it.
		// Otherwise, add it to the line list
		if !first || tlen > 1 {
			if first {
				txt = leadingTextBlanks(txt)
			}
			lines = append(lines, txt)
		}

		first = false
	}

	// for sed's purposes, we want a final newline...
	lines = append(lines, "")

	return strings.Join(lines, "\n"), err
}

// readIdentifier skips any whitespace, and then reads until it
// finds either a ';' or a non-alphanumeric character.  It
// returns the string it reads.
func readIdentifier(r *locReader) (string, error) {
	var buffer bytes.Buffer

	var err error
	var character rune

	character, err = skipWS(r)
	for (err == nil) && (character != ';') && !unicode.IsSpace(character) {
		buffer.WriteRune(character)
		character, _, err = r.ReadRune()
	}

	if err == nil {
		err = r.UnreadRune()
	}
	return buffer.String(), err
}

// readFilename reads the argument to r and w. Unlike labels and branch
// targets, POSIX filenames may contain blanks; the argument ends at the next
// command separator or newline.
func readFilename(r *locReader) (string, error) {
	var buffer bytes.Buffer
	character, err := skipWS(r)
	for err == nil && character != ';' && character != '\n' {
		buffer.WriteRune(character)
		character, _, err = r.ReadRune()
	}
	if err == nil && character == ';' {
		err = r.UnreadRune()
	}
	return strings.TrimRightFunc(buffer.String(), unicode.IsSpace), err
}

func readSubstitution(r *locReader) ([]string, error) {
	var ans = make([]string, 3)
	var err error

	// step 1.: get the delimiter character for substitutions
	var delimiter rune
	delimiter, _, err = r.ReadRune()
	if err != nil {
		return ans, err
	}

	// step 2.: read the regexp
	ans[0], err = readDelimited(r, delimiter)
	if err != nil {
		return ans, err
	}

	// step 3.: read the replacement
	ans[1], err = readReplacement(r, delimiter)
	if err != nil {
		return ans, err
	}

	// step 4.: read the modifiers
	ans[2], err = readIdentifier(r)

	return ans, err
}

// unescapeTranslation decodes the escapes POSIX defines for the two operands
// of the y command: `\\` is one literal backslash, `\n` is a <newline>, and a
// backslash-escaped delimiter is that delimiter as an ordinary character.
// The lengths of string1 and string2 are compared AFTER this decoding, so
// `y/abc/x\\z/` is a legal 3-for-3 translation.
//
// A backslash before anything else is undefined by POSIX; the escape is
// dropped and the character taken literally, matching how the rest of this
// engine treats `\x`. A trailing lone backslash is an error.
func unescapeTranslation(s string) (string, error) {
	if !strings.ContainsRune(s, '\\') {
		return s, nil
	}

	var buffer strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '\\' {
			buffer.WriteRune(runes[i])
			continue
		}
		if i+1 >= len(runes) {
			return "", fmt.Errorf("trailing backslash in 'y' command")
		}
		i++
		switch runes[i] {
		case 'n':
			buffer.WriteRune('\n')
		case 't':
			buffer.WriteRune('\t')
		case 'r':
			buffer.WriteRune('\r')
		default:
			buffer.WriteRune(runes[i]) // covers \\ and \<delimiter>
		}
	}
	return buffer.String(), nil
}

func readTranslation(r *locReader) ([]string, error) {
	var ans = make([]string, 2)
	var err error

	// step 1.: get the delimiter character for substitutions
	var delimiter rune
	delimiter, _, err = r.ReadRune()
	if err != nil {
		return ans, err
	}

	// step 2.: read the regexp
	ans[0], err = readDelimited(r, delimiter)
	if err != nil {
		return ans, err
	}

	// step 3.: read the replacement
	ans[1], err = readDelimited(r, delimiter)
	if err != nil {
		return ans, err
	}

	if ans[0], err = unescapeTranslation(ans[0]); err != nil {
		return ans, err
	}
	ans[1], err = unescapeTranslation(ans[1])

	return ans, err
}

func lex(r *bufio.Reader, ch chan<- *token, errch chan<- error) {
	defer close(ch)
	defer close(errch)

	rdr := locReader{}
	rdr.r = r
	rdr.eol = true

	var err error
	var cur rune

	var topLoc = rdr.location

	for err == nil {
		cur, err = skipWS(&rdr)
		if err != nil {
			break
		}

		topLoc = rdr.location // remember the start of the command

		switch cur {
		case ';':
			ch <- &token{topLoc, tok_EOL, cur, nil}
		case ',':
			ch <- &token{topLoc, tok_COMMA, cur, nil}
		case '+':
			next, _, nextErr := rdr.ReadRune()
			if nextErr != nil || !unicode.IsDigit(next) {
				if nextErr == nil {
					nextErr = fmt.Errorf("expected a number after +")
				}
				err = nextErr
				break
			}
			var num string
			num, err = readNumber(&rdr, next)
			ch <- &token{topLoc, tok_RELNUM, cur, []string{num}}
		case '{':
			ch <- &token{topLoc, tok_LBRACE, cur, nil}
		case '}':
			ch <- &token{topLoc, tok_RBRACE, cur, nil}
		case '!':
			ch <- &token{topLoc, tok_BANG, cur, nil}
		case '/':
			var rx string
			rx, err = readDelimited(&rdr, '/')
			ch <- &token{topLoc, tok_RX, cur, []string{rx}}
		case '\\':
			// POSIX \cREc: a context address delimited by any character c
			// other than <backslash> or <newline>. Inside the RE, c stands
			// for itself when escaped, which readDelimited already honours;
			// the delimiter is reported as the token letter so error
			// messages name the address the way it was written.
			var delimiter rune
			delimiter, _, err = rdr.ReadRune()
			if err != nil {
				break
			}
			if delimiter == '\n' || delimiter == '\\' {
				err = fmt.Errorf("%c cannot delimit an address %v", delimiter, &topLoc)
				break
			}
			var rx string
			rx, err = readDelimited(&rdr, delimiter)
			ch <- &token{topLoc, tok_RX, delimiter, []string{rx}}
		case '$':
			ch <- &token{topLoc, tok_DOLLAR, cur, nil}
		case ':':
			var label string
			label, err = readIdentifier(&rdr)
			ch <- &token{topLoc, tok_LABEL, cur, []string{label}}
		case 'b', 't': // branches...
			var label string
			label, err = readIdentifier(&rdr)
			ch <- &token{topLoc, tok_CMD, cur, []string{label}}
		case 's': // substitution
			var args []string
			args, err = readSubstitution(&rdr)
			ch <- &token{topLoc, tok_CMD, cur, args}
		case 'y': // translation
			var args []string
			args, err = readTranslation(&rdr)
			ch <- &token{topLoc, tok_CMD, cur, args}
		case 'c': // change
			var txt string
			txt, err = readMultiLine(&rdr)
			ch <- &token{topLoc, tok_CHANGE, cur, []string{txt}}
		case 'i', 'a': // insert or append
			var txt string
			txt, err = readMultiLine(&rdr)
			ch <- &token{topLoc, tok_CMD, cur, []string{txt}}
		case 'r', 'w':
			var fname string
			fname, err = readFilename(&rdr)
			ch <- &token{topLoc, tok_CMD, cur, []string{fname}}
		default:
			if unicode.IsDigit(cur) {
				var num string
				num, err = readNumber(&rdr, cur)
				ch <- &token{topLoc, tok_NUM, cur, []string{num}}
			} else {
				// it's just a argument-free command
				ch <- &token{topLoc, tok_CMD, cur, nil}
			}
		}
	}

	if err != io.EOF {
		errch <- fmt.Errorf("Error reading... <%s> %v", err.Error(), &topLoc)
	}
}
