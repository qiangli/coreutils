// Package editor provides the reusable line-buffer and command engine used by
// ed.  It deliberately has no dependency on the command registry or process
// globals so later ex/vi front ends can share the editing state.
package editor

import "fmt"

// Buffer is a one-based, line-oriented edit buffer. Current is zero only when
// the buffer is empty (or when an append is positioned before line one).
type Buffer struct {
	Lines   []string
	Current int
	Dirty   bool
}

// Clone returns an independent snapshot suitable for ed's one-command undo.
func (b Buffer) Clone() Buffer {
	b.Lines = append([]string(nil), b.Lines...)
	return b
}

func (b *Buffer) Last() int { return len(b.Lines) }

func (b *Buffer) Reset(lines []string, dirty bool) {
	b.Lines = append(b.Lines[:0], lines...)
	b.Current = len(lines)
	b.Dirty = dirty
}

func (b *Buffer) Append(after int, lines []string) error {
	if after < 0 || after > len(b.Lines) {
		return fmt.Errorf("invalid address")
	}
	if len(lines) == 0 {
		b.Current = after
		return nil
	}
	copyLines := append([]string(nil), lines...)
	b.Lines = append(b.Lines, make([]string, len(copyLines))...)
	copy(b.Lines[after+len(copyLines):], b.Lines[after:len(b.Lines)-len(copyLines)])
	copy(b.Lines[after:], copyLines)
	b.Current = after + len(copyLines)
	b.Dirty = true
	return nil
}

func (b *Buffer) Delete(first, last int) error {
	if first < 1 || last < first || last > len(b.Lines) {
		return fmt.Errorf("invalid address")
	}
	copy(b.Lines[first-1:], b.Lines[last:])
	b.Lines = b.Lines[:len(b.Lines)-(last-first+1)]
	if len(b.Lines) == 0 {
		b.Current = 0
	} else if first <= len(b.Lines) {
		b.Current = first
	} else {
		b.Current = len(b.Lines)
	}
	b.Dirty = true
	return nil
}

func (b *Buffer) Change(first, last int, lines []string) error {
	if len(b.Lines) == 0 && (first == 0 || first == 1) && (last == 0 || last == 1) {
		return b.Append(0, lines)
	}
	if first == 0 {
		first = 1
	}
	if last == 0 {
		last = 1
	}
	if err := b.Delete(first, last); err != nil {
		return err
	}
	return b.Append(first-1, lines)
}

func splitText(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	s := string(data)
	if s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	if s == "" {
		return []string{""}
	}
	lines := make([]string, 0, 1)
	for {
		for i := 0; i < len(s); i++ {
			if s[i] == '\n' {
				lines = append(lines, s[:i])
				s = s[i+1:]
				goto next
			}
		}
		return append(lines, s)
	next:
	}
}

func joinLines(lines []string) []byte {
	if len(lines) == 0 {
		return nil
	}
	n := len(lines)
	for _, line := range lines {
		n += len(line)
	}
	out := make([]byte, 0, n)
	for _, line := range lines {
		out = append(out, line...)
		out = append(out, '\n')
	}
	return out
}
