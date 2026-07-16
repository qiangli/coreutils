package bre

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const maxBacktrackSteps = 200000

// Regexp is a POSIX BRE matcher. Patterns without back-references or word-edge
// anchors are backed by Go's RE2 engine; patterns with \1..\9 or \<...\> use
// the bounded backtracking engine in this package.
type Regexp struct {
	re *regexp.Regexp
	bt *btProg
}

// Compile compiles a POSIX basic regular expression.
func Compile(pattern string) (*Regexp, error) {
	if !needsBacktrack(pattern) {
		t, err := ToGo(pattern)
		if err != nil {
			return nil, err
		}
		re, err := regexp.Compile(t)
		if err != nil {
			return nil, err
		}
		return &Regexp{re: re}, nil
	}
	bt, err := compileBackref(pattern, "", false)
	if err != nil {
		return nil, err
	}
	return &Regexp{bt: bt}, nil
}

// CompileWithFlags compiles a BRE with an optional RE2-style flag prefix such
// as "(?i)" or "(?im)", used by sed modifiers.
func CompileWithFlags(pattern, flags string) (*Regexp, error) {
	if !needsBacktrack(pattern) {
		t, err := ToGo(pattern)
		if err != nil {
			return nil, err
		}
		re, err := regexp.Compile(flags + t)
		if err != nil {
			return nil, err
		}
		return &Regexp{re: re}, nil
	}
	bt, err := compileBackref(pattern, flags, false)
	if err != nil {
		return nil, err
	}
	return &Regexp{bt: bt}, nil
}

// CompileEREWithFlags compiles a POSIX extended regular expression with an
// optional RE2-style flag prefix. EREs without back-references stay on RE2;
// EREs with GNU/POSIX-style \1..\9 use this package's bounded backtracker.
func CompileEREWithFlags(pattern, flags string) (*Regexp, error) {
	if !needsEREBacktrack(pattern) {
		t, err := ToGoERE(pattern)
		if err != nil {
			return nil, err
		}
		re, err := regexp.Compile(flags + t)
		if err != nil {
			return nil, err
		}
		return &Regexp{re: re}, nil
	}
	bt, err := compileBackref(pattern, flags, true)
	if err != nil {
		return nil, err
	}
	return &Regexp{bt: bt}, nil
}

// Longest makes future searches prefer the leftmost-longest match — the match
// POSIX specifies (XBD 9.1: the match is "the longest of the leftmost
// matches"), and the one GNU grep/sed report. Go's RE2 and this package's
// backtracker both default to leftmost-first, which differs whenever
// alternation can match at the same offset with different lengths: `a\|ab`
// against "ab" is leftmost-first "a" but POSIX "ab".
//
// It is opt-in because the two agree on whether a match exists at all, so a
// caller that only asks "does this line match" (grep's common path) sees no
// difference and keeps RE2's faster leftmost-first lanes. Callers that observe
// the match extent or its submatches — sed's s///, grep's -w — must set it.
//
// Like regexp.Regexp.Longest, this modifies the Regexp and must not race with
// other methods on it.
func (r *Regexp) Longest() {
	if r.re != nil {
		r.re.Longest()
		return
	}
	r.bt.longest = true
}

func (r *Regexp) MatchString(s string) bool {
	return r.FindStringIndex(s) != nil
}

func (r *Regexp) FindStringIndex(s string) []int {
	if r.re != nil {
		return r.re.FindStringIndex(s)
	}
	m := r.bt.find(s, 1)
	if len(m) == 0 {
		return nil
	}
	return m[0][:2]
}

func (r *Regexp) FindAllStringSubmatchIndex(s string, n int) [][]int {
	if r.re != nil {
		return r.re.FindAllStringSubmatchIndex(s, n)
	}
	return r.bt.find(s, n)
}

func (r *Regexp) FindAllSubmatchIndex(b []byte, n int) [][]int {
	return r.FindAllStringSubmatchIndex(string(b), n)
}

func (r *Regexp) ExpandString(dst []byte, template, src string, match []int) []byte {
	return expandTemplate(dst, template, src, match)
}

func (r *Regexp) Expand(dst []byte, template []byte, src []byte, match []int) []byte {
	return expandTemplate(dst, string(template), string(src), match)
}

func needsBacktrack(p string) bool {
	for i := 0; i+1 < len(p); i++ {
		if p[i] == '\\' {
			n := p[i+1]
			if (n >= '1' && n <= '9') || n == '<' || n == '>' {
				return true
			}
			i++
		}
	}
	return false
}

func needsEREBacktrack(p string) bool {
	inBracket := false
	firstBracket := false
	for i := 0; i+1 < len(p); i++ {
		switch p[i] {
		case '[':
			if !inBracket {
				inBracket = true
				firstBracket = true
			}
		case ']':
			if inBracket && !firstBracket {
				inBracket = false
			}
			firstBracket = false
		case '\\':
			if !inBracket {
				n := p[i+1]
				if n >= '1' && n <= '9' {
					return true
				}
				i++
			}
			firstBracket = false
		default:
			firstBracket = false
		}
	}
	return false
}

func expandTemplate(dst []byte, template, src string, match []int) []byte {
	for i := 0; i < len(template); i++ {
		if template[i] != '$' {
			dst = append(dst, template[i])
			continue
		}
		if i+1 < len(template) && template[i+1] == '$' {
			dst = append(dst, '$')
			i++
			continue
		}
		if i+1 >= len(template) || template[i+1] != '{' {
			dst = append(dst, '$')
			continue
		}
		j := i + 2
		for j < len(template) && template[j] >= '0' && template[j] <= '9' {
			j++
		}
		if j == i+2 || j >= len(template) || template[j] != '}' {
			dst = append(dst, '$')
			continue
		}
		n, _ := strconv.Atoi(template[i+2 : j])
		if 2*n+1 < len(match) && match[2*n] >= 0 && match[2*n+1] >= 0 {
			dst = append(dst, src[match[2*n]:match[2*n+1]]...)
		}
		i = j
	}
	return dst
}

type btProg struct {
	root       btNode
	groups     int
	anchored   bool
	ignoreCase bool
	longest    bool
}

type btState struct {
	pos  int
	caps [20]int
}

type btCtx struct {
	s     string
	steps int
	limit int
}

type btNode interface {
	match(*btCtx, btState) []btState
}

type seqNode []btNode

func (n seqNode) match(ctx *btCtx, st btState) []btState {
	states := []btState{st}
	for _, child := range n {
		var next []btState
		for _, s := range states {
			next = append(next, child.match(ctx, s)...)
			if ctx.steps > ctx.limit {
				return nil
			}
		}
		if len(next) == 0 {
			return nil
		}
		states = next
	}
	return states
}

type altNode []btNode

func (n altNode) match(ctx *btCtx, st btState) []btState {
	var out []btState
	for _, child := range n {
		out = append(out, child.match(ctx, st)...)
		if ctx.steps > ctx.limit {
			return nil
		}
	}
	return out
}

type literalNode struct {
	lit        string
	ignoreCase bool
}

func (n literalNode) match(ctx *btCtx, st btState) []btState {
	ctx.steps++
	if st.pos+len(n.lit) > len(ctx.s) {
		return nil
	}
	got := ctx.s[st.pos : st.pos+len(n.lit)]
	if (n.ignoreCase && strings.EqualFold(got, n.lit)) || (!n.ignoreCase && got == n.lit) {
		st.pos += len(n.lit)
		return []btState{st}
	}
	return nil
}

// dotNode is BRE's '.'. POSIX says a period matches any character; RE2 (and
// grep, whose subject never contains one) excludes the newline. dotAll is the
// sed reading: sed's pattern space can hold embedded newlines (after N), and
// '.' matches them.
type dotNode struct {
	dotAll bool
}

func (n dotNode) match(ctx *btCtx, st btState) []btState {
	ctx.steps++
	if st.pos >= len(ctx.s) || (!n.dotAll && ctx.s[st.pos] == '\n') {
		return nil
	}
	st.pos++
	return []btState{st}
}

type anchorNode byte

func (n anchorNode) match(ctx *btCtx, st btState) []btState {
	ctx.steps++
	switch byte(n) {
	case '^':
		if st.pos == 0 {
			return []btState{st}
		}
	case '$':
		if st.pos == len(ctx.s) {
			return []btState{st}
		}
	}
	return nil
}

type wordEdgeNode byte

func (n wordEdgeNode) match(ctx *btCtx, st btState) []btState {
	ctx.steps++
	switch byte(n) {
	case '<':
		if st.pos < len(ctx.s) && isWordByte(ctx.s[st.pos]) &&
			(st.pos == 0 || !isWordByte(ctx.s[st.pos-1])) {
			return []btState{st}
		}
	case '>':
		if st.pos > 0 && isWordByte(ctx.s[st.pos-1]) &&
			(st.pos == len(ctx.s) || !isWordByte(ctx.s[st.pos])) {
			return []btState{st}
		}
	}
	return nil
}

type wordBoundaryNode byte

func (n wordBoundaryNode) match(ctx *btCtx, st btState) []btState {
	ctx.steps++
	left := st.pos > 0 && isWordByte(ctx.s[st.pos-1])
	right := st.pos < len(ctx.s) && isWordByte(ctx.s[st.pos])
	if (byte(n) == 'b') == (left != right) {
		return []btState{st}
	}
	return nil
}

type builtinClassNode byte

func (n builtinClassNode) match(ctx *btCtx, st btState) []btState {
	ctx.steps++
	if st.pos >= len(ctx.s) {
		return nil
	}
	c := ctx.s[st.pos]
	var ok bool
	switch byte(n) {
	case 'w', 'W':
		ok = isWordByte(c)
	case 's', 'S':
		ok = c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
	}
	if byte(n) == 'W' || byte(n) == 'S' {
		ok = !ok
	}
	if ok {
		st.pos++
		return []btState{st}
	}
	return nil
}

type classNode struct {
	re *regexp.Regexp
}

func (n classNode) match(ctx *btCtx, st btState) []btState {
	ctx.steps++
	if st.pos >= len(ctx.s) {
		return nil
	}
	if n.re.MatchString(ctx.s[st.pos : st.pos+1]) {
		st.pos++
		return []btState{st}
	}
	return nil
}

type groupNode struct {
	num   int
	child btNode
}

func (n groupNode) match(ctx *btCtx, st btState) []btState {
	outs := n.child.match(ctx, st)
	for i := range outs {
		outs[i].caps[2*n.num] = st.pos
		outs[i].caps[2*n.num+1] = outs[i].pos
	}
	return outs
}

type backrefNode struct {
	num        int
	ignoreCase bool
}

func (n backrefNode) match(ctx *btCtx, st btState) []btState {
	ctx.steps++
	s, e := st.caps[2*n.num], st.caps[2*n.num+1]
	if s < 0 || e < 0 {
		// The group never participated in the match (e.g. \(a\)\{0\}b\1). POSIX
		// leaves this undefined; we fail the back-reference. Note this is a
		// different state from a group that participated and matched the empty
		// string, handled just below — conflating the two was the bug.
		return nil
	}
	if e == s {
		// The group participated and matched the empty string, so the
		// back-reference matches the empty string too: POSIX XBD 9.3.6 defines
		// \n as matching "the same string as was matched by" the group, and
		// that string is "". This is what makes \(a*\)b\1 match "b".
		return []btState{st}
	}
	lit := ctx.s[s:e]
	if st.pos+len(lit) > len(ctx.s) {
		return nil
	}
	got := ctx.s[st.pos : st.pos+len(lit)]
	if (n.ignoreCase && strings.EqualFold(got, lit)) || (!n.ignoreCase && got == lit) {
		st.pos += len(lit)
		return []btState{st}
	}
	return nil
}

type repeatNode struct {
	child btNode
	min   int
	max   int
}

func (n repeatNode) match(ctx *btCtx, st btState) []btState {
	var levels [][]btState
	levels = append(levels, []btState{st})
	max := n.max
	if max < 0 || max > len(ctx.s)-st.pos {
		max = len(ctx.s) - st.pos
	}
	for i := 0; i < max; i++ {
		var next []btState
		for _, s := range levels[len(levels)-1] {
			for _, out := range n.child.match(ctx, s) {
				if out.pos == s.pos {
					continue
				}
				next = append(next, out)
			}
		}
		if len(next) == 0 || ctx.steps > ctx.limit {
			break
		}
		levels = append(levels, next)
	}
	if len(levels)-1 < n.min {
		return nil
	}
	var out []btState
	for i := len(levels) - 1; i >= n.min; i-- {
		out = append(out, levels[i]...)
	}
	return out
}

type parser struct {
	p          string
	i          int
	groups     int
	ignoreCase bool
	dotAll     bool
	extended   bool
}

func compileBackref(pattern, flags string, extended bool) (*btProg, error) {
	// flags is an RE2 flag prefix ("", "(?i)", "(?is)", …) — the same string the
	// RE2 path prepends to its translated pattern, read here for the engine's
	// own nodes.
	ignoreCase := strings.Contains(flags, "i")
	dotAll := strings.Contains(flags, "s")
	p := &parser{p: pattern, ignoreCase: ignoreCase, dotAll: dotAll, extended: extended}
	root, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.i != len(pattern) {
		return nil, fmt.Errorf("unsupported BRE syntax near %q", pattern[p.i:])
	}
	prog := &btProg{root: root, groups: p.groups, ignoreCase: ignoreCase}
	// Only a leading '^' lets find() stop after one attempt; a leading '$' is an
	// anchorNode too, but it can still match at any offset (the empty string at
	// end of subject), so it must not short-circuit the scan.
	if seq, ok := root.(seqNode); ok && len(seq) > 0 {
		if a, ok := seq[0].(anchorNode); ok && byte(a) == '^' {
			prog.anchored = true
		}
	}
	return prog, nil
}

func (p *parser) parseExpr() (btNode, error) {
	var alts []btNode
	for {
		seq, err := p.parseSeq()
		if err != nil {
			return nil, err
		}
		alts = append(alts, seq)
		if p.isAlt() {
			p.i += 2
			if p.extended {
				p.i--
			}
			continue
		}
		break
	}
	if len(alts) == 1 {
		return alts[0], nil
	}
	return altNode(alts), nil
}

func (p *parser) parseSeq() (btNode, error) {
	var nodes []btNode
	state := posStart
	for p.i < len(p.p) {
		if p.endsSeq() {
			break
		}
		atom, nextState, quantifiable, err := p.parseAtom(state)
		if err != nil {
			return nil, err
		}
		if quantifiable {
			atom, err = p.parseQuant(atom)
			if err != nil {
				return nil, err
			}
		}
		nodes = append(nodes, atom)
		state = nextState
	}
	return seqNode(nodes), nil
}

func (p *parser) parseAtom(state int) (btNode, int, bool, error) {
	c := p.p[p.i]
	switch c {
	case '\\':
		if p.i+1 >= len(p.p) {
			return nil, state, false, fmt.Errorf("trailing backslash (\\)")
		}
		n := p.p[p.i+1]
		switch {
		case n == '(' && !p.extended:
			p.i += 2
			p.groups++
			num := p.groups
			if num > 9 {
				return nil, state, false, fmt.Errorf("too many capture groups for BRE back-reference matcher")
			}
			child, err := p.parseExpr()
			if err != nil {
				return nil, state, false, err
			}
			if !(p.i+1 < len(p.p) && p.p[p.i] == '\\' && p.p[p.i+1] == ')') {
				return nil, state, false, fmt.Errorf("unmatched \\(")
			}
			p.i += 2
			return groupNode{num: num, child: child}, posAtom, true, nil
		case n == ')' && !p.extended:
			p.i += 2
			return literalNode{lit: ")", ignoreCase: p.ignoreCase}, posAtom, true, nil
		case n >= '1' && n <= '9':
			p.i += 2
			num := int(n - '0')
			if num > p.groups {
				return nil, state, false, fmt.Errorf("invalid back-reference \\%c", n)
			}
			return backrefNode{num: num, ignoreCase: p.ignoreCase}, posAtom, true, nil
		case n == '<' || n == '>':
			p.i += 2
			return wordEdgeNode(n), posAnchor, false, nil
		case n == 'b' || n == 'B':
			p.i += 2
			return wordBoundaryNode(n), posAnchor, false, nil
		case n == 'w' || n == 'W' || n == 's' || n == 'S':
			p.i += 2
			return builtinClassNode(n), posAtom, true, nil
		case n == '{':
			if p.extended {
				p.i += 2
				return literalNode{lit: "{", ignoreCase: p.ignoreCase}, posAtom, true, nil
			}
			return nil, state, false, fmt.Errorf("\\{ with nothing to repeat")
		case n == '}':
			if p.extended {
				p.i += 2
				return literalNode{lit: "}", ignoreCase: p.ignoreCase}, posAtom, true, nil
			}
			return nil, state, false, fmt.Errorf("unmatched \\}")
		default:
			p.i += 2
			return literalNode{lit: string(n), ignoreCase: p.ignoreCase}, posAtom, true, nil
		}
	case '(':
		if p.extended {
			p.i++
			p.groups++
			num := p.groups
			if num > 9 {
				return nil, state, false, fmt.Errorf("too many capture groups for ERE back-reference matcher")
			}
			child, err := p.parseExpr()
			if err != nil {
				return nil, state, false, err
			}
			if p.i >= len(p.p) || p.p[p.i] != ')' {
				return nil, state, false, fmt.Errorf("unmatched (")
			}
			p.i++
			return groupNode{num: num, child: child}, posAtom, true, nil
		}
		p.i++
		return literalNode{lit: "(", ignoreCase: p.ignoreCase}, posAtom, true, nil
	case ')':
		p.i++
		return literalNode{lit: ")", ignoreCase: p.ignoreCase}, posAtom, true, nil
	case '.':
		p.i++
		return dotNode{dotAll: p.dotAll}, posAtom, true, nil
	case '^':
		p.i++
		if state == posStart {
			return anchorNode('^'), posAnchor, false, nil
		}
		return literalNode{lit: "^", ignoreCase: p.ignoreCase}, posAtom, true, nil
	case '$':
		p.i++
		if p.dollarAnchors() {
			return anchorNode('$'), posAnchor, false, nil
		}
		return literalNode{lit: "$", ignoreCase: p.ignoreCase}, posAtom, true, nil
	case '[':
		cls, n, err := translateBracket(p.p[p.i:])
		if err != nil {
			return nil, state, false, err
		}
		if p.ignoreCase {
			cls = "(?i)" + cls
		}
		re, err := regexp.Compile("^" + cls + "$")
		if err != nil {
			return nil, state, false, err
		}
		p.i += n
		return classNode{re: re}, posAtom, true, nil
	case '*':
		p.i++
		return literalNode{lit: "*", ignoreCase: p.ignoreCase}, posAtom, true, nil
	case '+', '?', '{', '}', '|':
		p.i++
		return literalNode{lit: string(c), ignoreCase: p.ignoreCase}, posAtom, true, nil
	default:
		p.i++
		return literalNode{lit: string(c), ignoreCase: p.ignoreCase}, posAtom, true, nil
	}
}

func (p *parser) dollarAnchors() bool {
	if p.i == len(p.p) {
		return true
	}
	if p.extended {
		return p.p[p.i] == ')' || p.p[p.i] == '|'
	}
	return p.i+1 < len(p.p) && p.p[p.i] == '\\' && (p.p[p.i+1] == ')' || p.p[p.i+1] == '|')
}

func (p *parser) parseQuant(atom btNode) (btNode, error) {
	if p.i >= len(p.p) {
		return atom, nil
	}
	if p.p[p.i] == '*' {
		p.i++
		return repeatNode{child: atom, min: 0, max: -1}, nil
	}
	if p.extended {
		switch p.p[p.i] {
		case '+':
			p.i++
			return repeatNode{child: atom, min: 1, max: -1}, nil
		case '?':
			p.i++
			return repeatNode{child: atom, min: 0, max: 1}, nil
		case '{':
			end := strings.IndexByte(p.p[p.i+1:], '}')
			if end < 0 {
				return nil, fmt.Errorf("unmatched {")
			}
			inner := p.p[p.i+1 : p.i+1+end]
			norm, ok := normalizeInterval(inner)
			if !ok {
				return nil, fmt.Errorf("invalid interval {%s}", inner)
			}
			min, max, err := parseInterval(norm)
			if err != nil {
				return nil, err
			}
			p.i += 1 + end + 1
			return repeatNode{child: atom, min: min, max: max}, nil
		}
		return atom, nil
	}
	if p.p[p.i] != '\\' || p.i+1 >= len(p.p) {
		return atom, nil
	}
	switch p.p[p.i+1] {
	case '+':
		p.i += 2
		return repeatNode{child: atom, min: 1, max: -1}, nil
	case '?':
		p.i += 2
		return repeatNode{child: atom, min: 0, max: 1}, nil
	case '{':
		end := strings.Index(p.p[p.i+2:], `\}`)
		if end < 0 {
			return nil, fmt.Errorf("unmatched \\{")
		}
		inner := p.p[p.i+2 : p.i+2+end]
		norm, ok := normalizeInterval(inner)
		if !ok {
			return nil, fmt.Errorf("invalid interval \\{%s\\}", inner)
		}
		min, max, err := parseInterval(norm)
		if err != nil {
			return nil, err
		}
		p.i += 2 + end + 2
		return repeatNode{child: atom, min: min, max: max}, nil
	}
	return atom, nil
}

func (p *parser) isAlt() bool {
	if p.extended {
		return p.i < len(p.p) && p.p[p.i] == '|'
	}
	return p.i+1 < len(p.p) && p.p[p.i] == '\\' && p.p[p.i+1] == '|'
}

func (p *parser) endsSeq() bool {
	if p.extended {
		return p.p[p.i] == ')' || p.p[p.i] == '|'
	}
	return p.p[p.i] == '\\' && p.i+1 < len(p.p) && (p.p[p.i+1] == ')' || p.p[p.i+1] == '|')
}

func parseInterval(s string) (int, int, error) {
	if comma := strings.IndexByte(s, ','); comma >= 0 {
		min, _ := strconv.Atoi(s[:comma])
		if comma == len(s)-1 {
			return min, -1, nil
		}
		max, _ := strconv.Atoi(s[comma+1:])
		return min, max, nil
	}
	n, _ := strconv.Atoi(s)
	return n, n, nil
}

func (p *btProg) find(s string, n int) [][]int {
	var out [][]int
	limit := n
	// Every offset is tried in turn: the first that matches is the leftmost
	// match, as POSIX requires. (An earlier "skip past the leading repeat on
	// failure" shortcut lived here. It is unsound on precisely the patterns
	// this engine exists for: it assumes the position the pattern resumes at
	// after a leading \(a*\) does not depend on where that group started, but a
	// back-reference makes the rest of the match depend on both the group's text
	// and its offset. It made \(a*\)b\1 skip the leftmost match "aba" in "aaba"
	// and report "b" instead.)
	for start := 0; start <= len(s); start++ {
		st, ok := p.pick(p.matchAt(s, start))
		if ok {
			match := make([]int, 2*(p.groups+1))
			for i := range match {
				match[i] = -1
			}
			match[0], match[1] = start, st.pos
			copy(match[2:], st.caps[2:2*(p.groups+1)])
			out = append(out, match)
			if limit > 0 && len(out) >= limit {
				return out
			}
			if st.pos > start {
				start = st.pos - 1
			}
		}
		if p.anchored {
			break
		}
	}
	return out
}

// pick chooses which of the states a successful match at one offset produced to
// report. The backtracker yields them in greedy-preference (leftmost-first)
// order, so the default is the first. Under longest, POSIX wants the greatest
// end offset; ties keep the earliest such state, which preserves the greedy
// subexpression assignment that the yield order already encodes.
func (p *btProg) pick(sts []btState) (btState, bool) {
	if len(sts) == 0 {
		return btState{}, false
	}
	best := sts[0]
	if p.longest {
		for _, st := range sts[1:] {
			if st.pos > best.pos {
				best = st
			}
		}
	}
	return best, true
}

func (p *btProg) matchAt(s string, start int) []btState {
	var st btState
	st.pos = start
	for i := range st.caps {
		st.caps[i] = -1
	}
	ctx := &btCtx{s: s, limit: maxBacktrackSteps}
	outs := p.root.match(ctx, st)
	if ctx.steps > ctx.limit {
		return nil
	}
	return outs
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}
