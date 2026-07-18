package grepcmd

// Literal fast path: GNU grep skips its regex engine entirely when the
// pattern is a plain literal (memchr candidate scan + Boyer-Moore).
// This file is the equivalent for bashy grep: when literalPattern says
// matching is plain-substring work, searchStreamLit replaces
// searchStream — same output bytes, same summaries, same early stops —
// but matches with bytes.Index over a large reused read buffer instead
// of running RE2 on a freshly allocated string per line.

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// literalPattern reports whether matching can be served by the literal
// fast path: exactly one pattern, no case folding, no -w, and (unless
// -F) no byte that is a metacharacter in any supported dialect. Chars
// that are ordinary in BRE but special in ERE (+ ? { } ( ) |) are
// conservatively excluded so one check serves both dialects; anything
// excluded simply keeps the RE2 path. -x stays eligible (it becomes a
// whole-line equality compare). The caller compiles the RE2 form
// regardless, so every pattern-error path is unchanged.
func literalPattern(pats []string, fixed, ignoreCase, word, onlyMatching bool) ([]byte, bool) {
	if ignoreCase || word || onlyMatching || len(pats) != 1 {
		return nil, false
	}
	if !fixed && strings.ContainsAny(pats[0], `.*[]^$\+?(){}|`) {
		return nil, false
	}
	return []byte(pats[0]), true
}

// litAction tells searchStreamLit how a selected line ended the scan.
type litAction int

const (
	litContinue litAction = iota
	litStop               // stop scanning; still print summaries (maxCount, binary)
	litReturn             // return with no summaries (quiet, -l), like searchStream's returns
)

// searchStreamLit is searchStream for the literal fast path. It mirrors
// searchStream's observable behavior exactly (binary sniff, -m 0,
// summaries, error shape and ordering) with three speed changes: lines
// are walked over a big reused buffer instead of a Scanner, plain
// selects skip non-matching lines wholesale via bytes.Index on the
// region, and output lines are batched into one Write per ~64 KiB.
func (g *grepper) searchStreamLit(r io.Reader, name string) {
	if r == nil {
		r = strings.NewReader("")
	}
	const maxLine = 64 << 20 // same line cap as the RE2 path's Scanner buffer
	if g.buf == nil {
		g.buf = make([]byte, 256<<10)
	}
	buf := g.buf
	dataLen := 0
	eof := false
	// A non-EOF read error surfaces the way bufio.Scanner does: only
	// after all already-buffered data has been consumed as lines.
	var pendErr error
	fill := func() {
		for !eof && pendErr == nil {
			n, err := r.Read(buf[dataLen:])
			dataLen += n
			if err == io.EOF {
				eof = true
			} else if err != nil {
				pendErr = err
			} else if n == 0 {
				continue
			}
			return
		}
	}

	// Binary sniff: NUL within the first 32 KiB, like searchStream's Peek.
	for dataLen < 32<<10 && !eof && pendErr == nil {
		fill()
	}
	binary := bytes.IndexByte(buf[:min(dataLen, 32<<10)], 0) >= 0

	selected, lineNo := 0, 0
	stopped := false     // scan ended early via litStop; summaries still print, pendErr never surfaced
	if g.maxCount != 0 { // -m 0 selects nothing and reads nothing
		for {
			done := eof || pendErr != nil
			region := 0
			if done {
				region = dataLen // includes a final line with no '\n'
			} else if i := bytes.LastIndexByte(buf[:dataLen], '\n'); i >= 0 {
				region = i + 1 // complete lines only
			}
			if region > 0 {
				act := g.litChunk(buf[:region], name, binary, &selected, &lineNo)
				dataLen = copy(buf, buf[region:dataLen])
				if act == litReturn {
					g.flushOut()
					return
				}
				if act == litStop {
					stopped = true
					break
				}
			}
			if done {
				break
			}
			if dataLen == len(buf) { // one line overflows the buffer
				if len(buf) >= maxLine {
					g.flushOut()
					g.report(name, bufio.ErrTooLong)
					return
				}
				nb := make([]byte, min(len(buf)*2, maxLine))
				copy(nb, buf[:dataLen])
				buf = nb
				g.buf = nb
			}
			fill()
		}
		g.flushOut()
		if !stopped && pendErr != nil {
			g.report(name, pendErr)
			return
		}
	}

	if binary && selected > 0 && !g.count && !g.filesWith && !g.filesWout {
		fmt.Fprintf(g.rc.Out, "Binary file %s matches\n", name)
	}
	if g.count {
		if g.showName {
			fmt.Fprintf(g.rc.Out, "%s:%d\n", name, selected)
		} else {
			fmt.Fprintln(g.rc.Out, selected)
		}
	}
	if g.filesWout && selected == 0 {
		fmt.Fprintln(g.rc.Out, name)
		g.listedWout = true
	}
}

var litNL = []byte{'\n'}

// litChunk scans one region whose every line is complete (only the last
// line of the stream may lack its '\n').
func (g *grepper) litChunk(data []byte, name string, binary bool, selected, lineNo *int) litAction {
	if !g.invert && !g.lineRegexp && len(g.lit) > 0 {
		// Plain substring select: hunt occurrences across the whole
		// region, touching line boundaries only around hits. Line
		// numbers are recovered by counting the newlines skipped.
		for len(data) > 0 {
			j := bytes.Index(data, g.lit)
			if j < 0 {
				if g.lineNum {
					*lineNo += bytes.Count(data, litNL)
				}
				return litContinue
			}
			ls := bytes.LastIndexByte(data[:j], '\n') + 1
			le, next := len(data), len(data)
			if k := bytes.IndexByte(data[j+len(g.lit):], '\n'); k >= 0 {
				le = j + len(g.lit) + k
				next = le + 1
			}
			if g.lineNum {
				*lineNo += bytes.Count(data[:ls], litNL) + 1
			}
			*selected++
			if act := g.litSelected(name, *lineNo, data[ls:le], binary, *selected); act != litContinue {
				return act
			}
			data = data[next:]
		}
		return litContinue
	}

	// Per-line path: -v, -x, and the empty pattern (matches every line;
	// with -x only empty lines).
	for len(data) > 0 {
		var line []byte
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			line, data = data[:i], data[i+1:]
		} else {
			line, data = data, nil
		}
		*lineNo++
		var m bool
		if g.lineRegexp {
			m = bytes.Equal(line, g.lit)
		} else {
			m = bytes.Contains(line, g.lit)
		}
		if m == g.invert {
			continue
		}
		*selected++
		if act := g.litSelected(name, *lineNo, line, binary, *selected); act != litContinue {
			return act
		}
	}
	return litContinue
}

// litSelected applies searchStream's per-selected-line logic (same
// order: quiet, -l, print-unless-count/-L with the binary break, -m).
func (g *grepper) litSelected(name string, lineNo int, line []byte, binary bool, selected int) litAction {
	g.anyMatch = true
	if g.quiet {
		return litReturn
	}
	if g.filesWith {
		g.flushOut()
		fmt.Fprintln(g.rc.Out, name)
		return litReturn
	}
	if !g.count && !g.filesWout {
		if binary {
			return litStop // one summary line after the scan
		}
		g.appendLine(name, lineNo, line)
	}
	if g.maxCount > 0 && selected >= g.maxCount {
		return litStop
	}
	return litContinue
}

// appendLine batches one output line into g.ob (printLine's shape).
func (g *grepper) appendLine(name string, n int, line []byte) {
	if g.showName {
		g.ob = append(g.ob, name...)
		g.ob = append(g.ob, ':')
	}
	if g.lineNum {
		g.ob = strconv.AppendInt(g.ob, int64(n), 10)
		g.ob = append(g.ob, ':')
	}
	g.ob = append(g.ob, line...)
	g.ob = append(g.ob, '\n')
	if len(g.ob) >= 64<<10 {
		g.flushOut()
	}
}

func (g *grepper) flushOut() {
	if len(g.ob) > 0 {
		g.rc.Out.Write(g.ob)
		g.ob = g.ob[:0]
	}
}
