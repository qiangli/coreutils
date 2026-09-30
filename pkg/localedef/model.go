// Package localedef parses POSIX.1-2017 character maps and locale sources.
// It does not compile locales or resolve copy references.
package localedef

import "fmt"

type Kind uint8

const (
	Word Kind = iota
	String
	Symbol
	Number
	Bytes
	Separator
	Ellipsis
)

// Value retains separators and string parts so compilation can distinguish
// lists, conversion pairs, symbol references, and literal bytes without relexing.
type Value struct {
	Kind  Kind
	Text  string
	Parts []Value
}
type Entry struct {
	Keyword string
	Values  []Value
	Line    int
}
type Section struct {
	Name    string
	Line    int
	Copy    string
	Entries []Entry
}
type Source struct {
	Sections map[string]*Section
	Order    []string
}
type Charmap struct {
	CodeSet            string
	MinBytes, MaxBytes int
	Symbols            map[string][]byte
}
type Diagnostic struct {
	Line    int
	Message string
	Warning bool
}

func (d Diagnostic) Error() string { return fmt.Sprintf("line %d: %s", d.Line, d.Message) }
func problem(line int, format string, args ...any) error {
	return Diagnostic{Line: line, Message: fmt.Sprintf(format, args...)}
}
