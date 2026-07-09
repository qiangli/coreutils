package bre

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const maxBacktrackSteps = 200000

// Regexp is a POSIX BRE matcher. Patterns without back-references are backed
// by Go's RE2 engine; patterns with \1..\9 use the bounded backtracking engine
// in this package.
type Regexp struct {
	re *regexp.Regexp
	bt *btProg
}

// Compile compiles a POSIX basic regular expression.
func Compile(pattern string) (*Regexp, error) {
	if !hasBackref(pattern) {
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
	bt, err := compileBackref(pattern, "")
	if err != nil {
		return nil, err
	}
	return &Regexp{bt: bt}, nil
}

// CompileWithFlags compiles a BRE with an optional RE2-style flag prefix such
// as "(?i)" or "(?im)", used by sed modifiers.
func CompileWithFlags(pattern, flags string) (*Regexp, error) {
	if !hasBackref(pattern) {
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
	bt, err := compileBackref(pattern, flags)
	if err != nil {
		return nil, err
	}
	return &Regexp{bt: bt}, nil
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

func hasBackref(p string) bool {
	for i := 0; i+1 < len(p); i++ {
		if p[i] == '\\' {
			n := p[i+1]
			if n >= '1' && n <= '9' {
				return true
			}
			i++
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

type dotNode struct{}

func (dotNode) match(ctx *btCtx, st btState) []btState {
	ctx.steps++
	if st.pos >= len(ctx.s) || ctx.s[st.pos] == '\n' {
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
	if s < 0 || e <= s {
		return nil
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
	return levels[len(levels)-1]
}

type parser struct {
	p          string
	i          int
	groups     int
	ignoreCase bool
}

func compileBackref(pattern, flags string) (*btProg, error) {
	ignoreCase := strings.Contains(flags, "i")
	p := &parser{p: pattern, ignoreCase: ignoreCase}
	root, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.i != len(pattern) {
		return nil, fmt.Errorf("unsupported BRE syntax near %q", pattern[p.i:])
	}
	prog := &btProg{root: root, groups: p.groups, ignoreCase: ignoreCase}
	if seq, ok := root.(seqNode); ok && len(seq) > 0 {
		_, prog.anchored = seq[0].(anchorNode)
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
		if p.i+1 < len(p.p) && p.p[p.i] == '\\' && p.p[p.i+1] == '|' {
			p.i += 2
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
	for p.i < len(p.p) {
		if p.p[p.i] == '\\' && p.i+1 < len(p.p) && (p.p[p.i+1] == ')' || p.p[p.i+1] == '|') {
			break
		}
		atom, err := p.parseAtom()
		if err != nil {
			return nil, err
		}
		atom, err = p.parseQuant(atom)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, atom)
	}
	return seqNode(nodes), nil
}

func (p *parser) parseAtom() (btNode, error) {
	c := p.p[p.i]
	switch c {
	case '\\':
		if p.i+1 >= len(p.p) {
			return nil, fmt.Errorf("trailing backslash (\\)")
		}
		n := p.p[p.i+1]
		switch {
		case n == '(':
			p.i += 2
			p.groups++
			num := p.groups
			if num > 9 {
				return nil, fmt.Errorf("too many capture groups for BRE back-reference matcher")
			}
			child, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if !(p.i+1 < len(p.p) && p.p[p.i] == '\\' && p.p[p.i+1] == ')') {
				return nil, fmt.Errorf("unmatched \\(")
			}
			p.i += 2
			return groupNode{num: num, child: child}, nil
		case n >= '1' && n <= '9':
			p.i += 2
			num := int(n - '0')
			if num > p.groups {
				return nil, fmt.Errorf("invalid back-reference \\%c", n)
			}
			return backrefNode{num: num, ignoreCase: p.ignoreCase}, nil
		case n == 'w' || n == 'W' || n == 's' || n == 'S' || n == 'b' || n == 'B' || n == '<' || n == '>':
			return nil, fmt.Errorf("unsupported escape \\%c in BRE back-reference matcher", n)
		default:
			p.i += 2
			return literalNode{lit: string(n), ignoreCase: p.ignoreCase}, nil
		}
	case '.':
		p.i++
		return dotNode{}, nil
	case '^':
		p.i++
		return anchorNode('^'), nil
	case '$':
		p.i++
		return anchorNode('$'), nil
	case '[':
		cls, n, err := translateBracket(p.p[p.i:])
		if err != nil {
			return nil, err
		}
		if p.ignoreCase {
			cls = "(?i)" + cls
		}
		re, err := regexp.Compile("^" + cls + "$")
		if err != nil {
			return nil, err
		}
		p.i += n
		return classNode{re: re}, nil
	default:
		p.i++
		return literalNode{lit: string(c), ignoreCase: p.ignoreCase}, nil
	}
}

func (p *parser) parseQuant(atom btNode) (btNode, error) {
	if p.i >= len(p.p) {
		return atom, nil
	}
	if p.p[p.i] == '*' {
		p.i++
		return repeatNode{child: atom, min: 0, max: -1}, nil
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
	for start := 0; start <= len(s); start++ {
		matches := p.matchAt(s, start)
		for _, st := range matches {
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
			break
		}
		if len(matches) == 0 {
			if end := leadingRepeatEnd(p.root, s, start, p.ignoreCase); end > start {
				start = end - 1
			}
		}
		if p.anchored {
			break
		}
	}
	return out
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

func leadingRepeatEnd(n btNode, s string, start int, ignoreCase bool) int {
	switch v := n.(type) {
	case seqNode:
		if len(v) == 0 {
			return start
		}
		return leadingRepeatEnd(v[0], s, start, ignoreCase)
	case groupNode:
		return leadingRepeatEnd(v.child, s, start, ignoreCase)
	case repeatNode:
		pos := start
		for {
			ctx := &btCtx{s: s, limit: maxBacktrackSteps}
			st := btState{pos: pos}
			for i := range st.caps {
				st.caps[i] = -1
			}
			outs := v.child.match(ctx, st)
			if len(outs) == 0 || outs[0].pos <= pos {
				return pos
			}
			pos = outs[0].pos
			if v.max >= 0 && pos-start >= v.max {
				return pos
			}
		}
	case literalNode:
		if start+len(v.lit) <= len(s) {
			got := s[start : start+len(v.lit)]
			if (ignoreCase && strings.EqualFold(got, v.lit)) || (!ignoreCase && got == v.lit) {
				return start + len(v.lit)
			}
		}
	case dotNode:
		if start < len(s) && s[start] != '\n' {
			return start + 1
		}
	}
	return start
}
