package bc

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
)

// Interpreter executes POSIX bc source. It deliberately has no process-global
// state, so one embedded shell can safely run several calculators.
type Interpreter struct {
	Out       io.Writer
	In        *bufio.Reader
	Scale     int
	IBase     int
	OBase     int
	Mathlib   bool
	vars      map[string]Number
	arrays    map[string]map[int]Number
	funcs     map[string]*function
	callDepth int
}

func New(out io.Writer, in io.Reader) *Interpreter {
	return &Interpreter{Out: out, In: bufio.NewReader(in), IBase: 10, OBase: 10, vars: map[string]Number{}, arrays: map[string]map[int]Number{}, funcs: map[string]*function{}}
}

type function struct {
	params []param
	autos  []param
	body   []stmt
}
type param struct {
	name  string
	array bool
}

func (b *Interpreter) Execute(src string) error {
	toks, err := lex(src)
	if err != nil {
		return err
	}
	p := parser{t: toks}
	p.skipNL()
	for p.cur().k != tEOF {
		if p.cur().k == tError {
			return fmt.Errorf("%s", p.cur().s)
		}
		s, parseErr := p.statement()
		if parseErr != nil {
			if errors.Is(parseErr, io.EOF) {
				return io.EOF
			}
			return parseErr
		}
		if s != nil {
			r, runErr := s.run(b)
			if runErr != nil {
				return runErr
			}
			if r.flow == flowQuit {
				return io.EOF
			}
			if r.flow != flowNone {
				return fmt.Errorf("flow-control statement outside its valid context")
			}
		}
		if p.cur().k != tNL && p.cur().k != tEOF {
			if p.cur().k == tError {
				return fmt.Errorf("%s", p.cur().s)
			}
			return fmt.Errorf("line %d: expected statement separator near %q", p.cur().line, p.cur().s)
		}
		p.skipNL()
	}
	return nil
}

type flow int

const (
	flowNone flow = iota
	flowBreak
	flowReturn
	flowQuit
)

type result struct {
	flow  flow
	value Number
}
type stmt interface {
	run(*Interpreter) (result, error)
}
type expr interface {
	eval(*Interpreter) (Number, error)
}

type parser struct {
	t []token
	i int
}

func (p *parser) cur() token { return p.t[p.i] }
func (p *parser) take() token {
	x := p.cur()
	if p.i < len(p.t)-1 {
		p.i++
	}
	return x
}
func (p *parser) accept(s string) bool {
	if p.cur().s == s {
		p.i++
		return true
	}
	return false
}
func (p *parser) skipNL() {
	for p.cur().k == tNL {
		p.i++
	}
}
func (p *parser) want(s string) error {
	if !p.accept(s) {
		return fmt.Errorf("line %d: expected %q, got %q", p.cur().line, s, p.cur().s)
	}
	return nil
}

func (p *parser) program(stopBrace bool) ([]stmt, error) {
	var ss []stmt
	p.skipNL()
	for p.cur().k != tEOF && !(stopBrace && p.cur().s == "}") {
		if p.cur().k == tError {
			return nil, fmt.Errorf("%s", p.cur().s)
		}
		s, err := p.statement()
		if err != nil {
			return nil, err
		}
		if s != nil {
			ss = append(ss, s)
		}
		if p.cur().k != tNL && p.cur().k != tEOF && p.cur().s != "}" {
			return nil, fmt.Errorf("line %d: expected statement separator near %q", p.cur().line, p.cur().s)
		}
		p.skipNL()
	}
	return ss, nil
}

func (p *parser) block() ([]stmt, error) {
	if err := p.want("{"); err != nil {
		return nil, err
	}
	p.skipNL()
	ss, err := p.program(true)
	if err != nil {
		return nil, err
	}
	if err = p.want("}"); err != nil {
		return nil, err
	}
	return ss, nil
}

func (p *parser) statement() (stmt, error) {
	t := p.cur()
	if t.k == tError {
		return nil, fmt.Errorf("%s", t.s)
	}
	if t.s == "{" {
		ss, err := p.block()
		return blockStmt(ss), err
	}
	if t.k == tString {
		p.take()
		return stringStmt(t.s), nil
	}
	if t.k == tIdent {
		switch t.s {
		case "define":
			return p.parseDefine()
		case "if":
			return p.parseIf()
		case "while":
			return p.parseWhile()
		case "for":
			return p.parseFor()
		case "break":
			p.take()
			return flowStmt(flowBreak), nil
		case "quit":
			p.take()
			// POSIX quit is lexical: it terminates even in an unselected arm
			// or function definition, without requiring the enclosing syntax
			// to be completed.
			return nil, io.EOF
		case "return":
			p.take()
			var e expr = numberExpr{fromInt64(0)}
			if p.accept("(") {
				if p.cur().s != ")" {
					var err error
					e, err = p.expression()
					if err != nil {
						return nil, err
					}
				}
				if err := p.want(")"); err != nil {
					return nil, err
				}
			}
			return returnStmt{e}, nil
		}
	}
	e, err := p.expression()
	if err != nil {
		return nil, err
	}
	return exprStmt{e: e, print: !isAssignment(e)}, nil
}

func (p *parser) parseDefine() (stmt, error) {
	p.take()
	name := p.take()
	if name.k != tIdent {
		return nil, fmt.Errorf("line %d: expected function name", name.line)
	}
	if err := p.want("("); err != nil {
		return nil, err
	}
	params, err := p.params()
	if err != nil {
		return nil, err
	}
	if err = p.want(")"); err != nil {
		return nil, err
	}
	p.skipNL()
	if err = p.want("{"); err != nil {
		return nil, err
	}
	p.skipNL()
	var autos []param
	if p.cur().s == "auto" {
		p.take()
		autos, err = p.params()
		if err != nil {
			return nil, err
		}
		p.skipNL()
	}
	body, err := p.program(true)
	if err != nil {
		return nil, err
	}
	if err = p.want("}"); err != nil {
		return nil, err
	}
	return defineStmt{name.s, &function{params: params, autos: autos, body: body}}, nil
}
func (p *parser) params() ([]param, error) {
	var ps []param
	for p.cur().k == tIdent {
		name := p.take().s
		arr := false
		if p.accept("[") {
			if err := p.want("]"); err != nil {
				return nil, err
			}
			arr = true
		}
		ps = append(ps, param{name, arr})
		if !p.accept(",") {
			break
		}
	}
	return ps, nil
}
func (p *parser) condition() (expr, error) {
	if err := p.want("("); err != nil {
		return nil, err
	}
	e, err := p.relationalExpression()
	if err != nil {
		return nil, err
	}
	if err = p.want(")"); err != nil {
		return nil, err
	}
	return e, nil
}

func (p *parser) relationalExpression() (expr, error) {
	left, err := p.expression()
	if err != nil {
		return nil, err
	}
	switch p.cur().s {
	case "==", "!=", "<", "<=", ">", ">=":
		op := p.take().s
		right, err := p.expression()
		if err != nil {
			return nil, err
		}
		return &binaryExpr{op, left, right}, nil
	default:
		return left, nil
	}
}
func (p *parser) parseIf() (stmt, error) {
	p.take()
	c, err := p.condition()
	if err != nil {
		return nil, err
	}
	p.skipNL()
	s, err := p.statement()
	if err != nil {
		return nil, err
	}
	body := []stmt{s}
	return ifStmt{c, body, nil}, err
}
func (p *parser) parseWhile() (stmt, error) {
	p.take()
	c, err := p.condition()
	if err != nil {
		return nil, err
	}
	p.skipNL()
	s, err := p.statement()
	return whileStmt{c, []stmt{s}}, err
}
func (p *parser) parseFor() (stmt, error) {
	p.take()
	if err := p.want("("); err != nil {
		return nil, err
	}
	var init, cond, post expr
	var err error
	if p.cur().k != tNL {
		init, err = p.expression()
		if err != nil {
			return nil, err
		}
	}
	if p.cur().k != tNL {
		return nil, fmt.Errorf("line %d: expected ';'", p.cur().line)
	}
	p.take()
	if p.cur().k != tNL {
		cond, err = p.relationalExpression()
		if err != nil {
			return nil, err
		}
	}
	if p.cur().k != tNL {
		return nil, fmt.Errorf("line %d: expected ';'", p.cur().line)
	}
	p.take()
	if p.cur().s != ")" {
		post, err = p.expression()
		if err != nil {
			return nil, err
		}
	}
	if err = p.want(")"); err != nil {
		return nil, err
	}
	p.skipNL()
	s, err := p.statement()
	return forStmt{init, cond, post, []stmt{s}}, err
}

var prec = map[string]int{"=": 1, "+=": 1, "-=": 1, "*=": 1, "/=": 1, "%=": 1, "^=": 1, "+": 5, "-": 5, "*": 6, "/": 6, "%": 6, "^": 7}

func (p *parser) expression() (expr, error) { return p.binary(1) }
func (p *parser) binary(min int) (expr, error) {
	left, err := p.unary()
	if err != nil {
		return nil, err
	}
	for {
		op := p.cur().s
		pr := prec[op]
		if pr < min {
			break
		}
		p.take()
		next := pr + 1
		if op == "^" || strings.HasSuffix(op, "=") {
			next = pr
		}
		right, err := p.binary(next)
		if err != nil {
			return nil, err
		}
		left = &binaryExpr{op, left, right}
	}
	return left, nil
}
func (p *parser) unary() (expr, error) {
	if p.accept("-") {
		e, err := p.unary()
		return unaryExpr{"-", e}, err
	}
	if p.accept("++") {
		e, err := p.unary()
		return incExpr{e, 1, false}, err
	}
	if p.accept("--") {
		e, err := p.unary()
		return incExpr{e, -1, false}, err
	}
	e, err := p.primary()
	if err != nil {
		return nil, err
	}
	if p.accept("++") {
		return incExpr{e, 1, true}, nil
	}
	if p.accept("--") {
		return incExpr{e, -1, true}, nil
	}
	return e, nil
}
func (p *parser) primary() (expr, error) {
	t := p.take()
	switch t.k {
	case tNumber:
		return literalExpr{t.s}, nil
	case tIdent:
		if p.accept("(") {
			var args []expr
			if p.cur().s != ")" {
				for {
					e, err := p.expression()
					if err != nil {
						return nil, err
					}
					args = append(args, e)
					if !p.accept(",") {
						break
					}
				}
			}
			if err := p.want(")"); err != nil {
				return nil, err
			}
			return callExpr{t.s, args}, nil
		}
		if p.accept("[") {
			if p.accept("]") {
				return wholeArrayExpr{name: t.s}, nil
			}
			idx, err := p.expression()
			if err != nil {
				return nil, err
			}
			if err = p.want("]"); err != nil {
				return nil, err
			}
			return variableExpr{name: t.s, index: idx}, nil
		}
		return variableExpr{name: t.s}, nil
	case tOp:
		if t.s == "(" {
			e, err := p.expression()
			if err != nil {
				return nil, err
			}
			if err = p.want(")"); err != nil {
				return nil, err
			}
			return e, nil
		}
	}
	return nil, fmt.Errorf("line %d: expected expression near %q", t.line, t.s)
}

type literalExpr struct{ s string }

func (e literalExpr) eval(b *Interpreter) (Number, error) { return ParseNumber(e.s, b.IBase) }

type numberExpr struct{ n Number }

func (e numberExpr) eval(*Interpreter) (Number, error) { return e.n.clone(), nil }

type variableExpr struct {
	name  string
	index expr
}

type wholeArrayExpr struct{ name string }

func (e wholeArrayExpr) eval(*Interpreter) (Number, error) {
	return Zero(), fmt.Errorf("array %s[] is only valid as a function argument", e.name)
}

func (e variableExpr) eval(b *Interpreter) (Number, error) {
	if e.index != nil {
		i, err := arrayIndex(b, e.index)
		if err != nil {
			return Zero(), err
		}
		if a := b.arrays[e.name]; a != nil {
			if v, ok := a[i]; ok {
				return v.clone(), nil
			}
		}
		return Zero(), nil
	}
	switch e.name {
	case "scale":
		return fromInt64(int64(b.Scale)), nil
	case "ibase":
		return fromInt64(int64(b.IBase)), nil
	case "obase":
		return fromInt64(int64(b.OBase)), nil
	}
	if v, ok := b.vars[e.name]; ok {
		return v.clone(), nil
	}
	return Zero(), nil
}
func arrayIndex(b *Interpreter, e expr) (int, error) {
	v, err := e.eval(b)
	if err != nil {
		return 0, err
	}
	n := new(big.Int).Set(v.n)
	if v.scale != 0 {
		n.Quo(n, pow10(v.scale))
	}
	if !n.IsInt64() || n.Sign() < 0 || n.Cmp(big.NewInt(MaxDim-1)) > 0 {
		return 0, fmt.Errorf("array index outside 0..%d", MaxDim-1)
	}
	return int(n.Int64()), nil
}
func (e variableExpr) set(b *Interpreter, v Number) error {
	if e.index != nil {
		i, err := arrayIndex(b, e.index)
		if err != nil {
			return err
		}
		if b.arrays[e.name] == nil {
			b.arrays[e.name] = map[int]Number{}
		}
		b.arrays[e.name][i] = v.clone()
		return nil
	}
	switch e.name {
	case "scale":
		n, err := truncatedInt(v, 0, MaxScale)
		if err == nil {
			b.Scale = n
		}
		return err
	case "ibase":
		n, err := truncatedInt(v, 2, 16)
		if err == nil {
			b.IBase = n
		}
		return err
	case "obase":
		n, err := truncatedInt(v, 2, MaxBase)
		if err == nil {
			b.OBase = n
		}
		return err
	}
	b.vars[e.name] = v.clone()
	return nil
}
func truncatedInt(v Number, min, max int) (int, error) {
	num := new(big.Int).Set(v.n)
	if v.scale != 0 {
		num.Quo(num, pow10(v.scale))
	}
	if !num.IsInt64() {
		return 0, fmt.Errorf("value is outside %d..%d", min, max)
	}
	n := int(num.Int64())
	if n < min || n > max {
		return 0, fmt.Errorf("value %d outside %d..%d", n, min, max)
	}
	return n, nil
}

type unaryExpr struct {
	op string
	e  expr
}

func (x unaryExpr) eval(b *Interpreter) (Number, error) {
	v, e := x.e.eval(b)
	if e != nil {
		return Zero(), e
	}
	if x.op == "-" {
		return v.Neg(), nil
	}
	if v.Sign() == 0 {
		return fromInt64(1), nil
	}
	return Zero(), nil
}

type binaryExpr struct {
	op   string
	l, r expr
}

func (x *binaryExpr) eval(b *Interpreter) (Number, error) {
	if assignmentOp(x.op) {
		lv, ok := x.l.(variableExpr)
		if !ok {
			return Zero(), fmt.Errorf("assignment requires a variable")
		}
		var rv Number
		var err error
		if x.op == "=" && lv.index == nil && (lv.name == "ibase" || lv.name == "obase") {
			if lit, ok := x.r.(literalExpr); ok && len(lit.s) == 1 && strings.ContainsRune("0123456789ABCDEF", rune(lit.s[0])) {
				rv, err = ParseNumber(lit.s, 16)
			} else {
				rv, err = x.r.eval(b)
			}
		} else {
			rv, err = x.r.eval(b)
		}
		if err != nil {
			return Zero(), err
		}
		if x.op != "=" {
			old, err := lv.eval(b)
			if err != nil {
				return Zero(), err
			}
			rv, err = arith(strings.TrimSuffix(x.op, "="), old, rv, b.Scale)
			if err != nil {
				return Zero(), err
			}
		}
		return rv, lv.set(b, rv)
	}
	a, err := x.l.eval(b)
	if err != nil {
		return Zero(), err
	}
	if x.op == "&&" && a.Sign() == 0 {
		return Zero(), nil
	}
	if x.op == "||" && a.Sign() != 0 {
		return fromInt64(1), nil
	}
	c, err := x.r.eval(b)
	if err != nil {
		return Zero(), err
	}
	switch x.op {
	case "==":
		return boolNum(a.Cmp(c) == 0), nil
	case "!=":
		return boolNum(a.Cmp(c) != 0), nil
	case "<":
		return boolNum(a.Cmp(c) < 0), nil
	case "<=":
		return boolNum(a.Cmp(c) <= 0), nil
	case ">":
		return boolNum(a.Cmp(c) > 0), nil
	case ">=":
		return boolNum(a.Cmp(c) >= 0), nil
	case "&&":
		return boolNum(c.Sign() != 0), nil
	case "||":
		return boolNum(c.Sign() != 0), nil
	}
	return arith(x.op, a, c, b.Scale)
}
func arith(op string, a, c Number, s int) (Number, error) {
	switch op {
	case "+":
		return a.Add(c), nil
	case "-":
		return a.Sub(c), nil
	case "*":
		return a.Mul(c, s), nil
	case "/":
		return a.Div(c, s)
	case "%":
		return a.Mod(c, s)
	case "^":
		return a.Pow(c, s)
	}
	return Zero(), fmt.Errorf("unknown operator %q", op)
}
func boolNum(v bool) Number {
	if v {
		return fromInt64(1)
	}
	return Zero()
}
func isAssignment(e expr) bool { x, ok := e.(*binaryExpr); return ok && assignmentOp(x.op) }
func assignmentOp(op string) bool {
	switch op {
	case "=", "+=", "-=", "*=", "/=", "%=", "^=":
		return true
	}
	return false
}

type incExpr struct {
	e     expr
	delta int
	post  bool
}

func (x incExpr) eval(b *Interpreter) (Number, error) {
	lv, ok := x.e.(variableExpr)
	if !ok {
		return Zero(), fmt.Errorf("increment requires a variable")
	}
	old, err := lv.eval(b)
	if err != nil {
		return Zero(), err
	}
	n := old.Add(fromInt64(int64(x.delta)))
	if err = lv.set(b, n); err != nil {
		return Zero(), err
	}
	if x.post {
		return old, nil
	}
	return n, nil
}

type callExpr struct {
	name string
	args []expr
}

func (x callExpr) eval(b *Interpreter) (Number, error) {
	if b.Mathlib && b.funcs[x.name] == nil && len(x.name) == 1 && strings.Contains("scael", x.name) {
		if len(x.args) != 1 {
			return Zero(), fmt.Errorf("%s requires one argument", x.name)
		}
		v, err := x.args[0].eval(b)
		if err != nil {
			return Zero(), err
		}
		return b.mathCall(x.name, v)
	}
	if b.Mathlib && b.funcs[x.name] == nil && x.name == "j" {
		if len(x.args) != 2 {
			return Zero(), fmt.Errorf("j requires two arguments")
		}
		order, err := x.args[0].eval(b)
		if err != nil {
			return Zero(), err
		}
		arg, err := x.args[1].eval(b)
		if err != nil {
			return Zero(), err
		}
		return b.besselCall(order, arg)
	}
	switch x.name {
	case "length":
		if len(x.args) != 1 {
			return Zero(), fmt.Errorf("length requires one argument")
		}
		v, e := x.args[0].eval(b)
		if e != nil {
			return Zero(), e
		}
		return fromInt64(int64(v.Length())), nil
	case "scale":
		if len(x.args) != 1 {
			return Zero(), fmt.Errorf("scale requires one argument")
		}
		v, e := x.args[0].eval(b)
		if e != nil {
			return Zero(), e
		}
		return fromInt64(int64(v.Scale())), nil
	case "sqrt":
		if len(x.args) != 1 {
			return Zero(), fmt.Errorf("sqrt requires one argument")
		}
		v, e := x.args[0].eval(b)
		if e != nil {
			return Zero(), e
		}
		return v.Sqrt(b.Scale)
	}
	f := b.funcs[x.name]
	if f == nil {
		return Zero(), fmt.Errorf("undefined function %s", x.name)
	}
	if len(x.args) != len(f.params) {
		return Zero(), fmt.Errorf("function %s expects %d arguments", x.name, len(f.params))
	}
	// Evaluate every actual argument before installing any formal binding.
	// This is observable when a later argument names the same global as an
	// earlier formal. Whole arrays are copied because POSIX passes by value.
	values := make([]Number, len(x.args))
	arrayValues := make([]map[int]Number, len(x.args))
	for i, p := range f.params {
		if p.array {
			a, ok := x.args[i].(wholeArrayExpr)
			if !ok {
				return Zero(), fmt.Errorf("array parameter %s requires an array", p.name)
			}
			cp := map[int]Number{}
			for k, v := range b.arrays[a.name] {
				cp[k] = v.clone()
			}
			arrayValues[i] = cp
		} else {
			v, err := x.args[i].eval(b)
			if err != nil {
				return Zero(), err
			}
			values[i] = v
		}
	}

	// Save and restore dynamic bindings; POSIX function parameters and auto
	// variables are local, all other variables remain global.
	savedV := map[string]Number{}
	hadV := map[string]bool{}
	savedA := map[string]map[int]Number{}
	hadA := map[string]bool{}
	for i, p := range f.params {
		if p.array {
			savedA[p.name], hadA[p.name] = b.arrays[p.name]
			b.arrays[p.name] = arrayValues[i]
		} else {
			savedV[p.name], hadV[p.name] = b.vars[p.name]
			b.vars[p.name] = values[i]
		}
	}
	for _, p := range f.autos {
		if p.array {
			savedA[p.name], hadA[p.name] = b.arrays[p.name]
			b.arrays[p.name] = map[int]Number{}
		} else {
			savedV[p.name], hadV[p.name] = b.vars[p.name]
			b.vars[p.name] = Zero()
		}
	}
	b.callDepth++
	r, err := b.exec(f.body)
	b.callDepth--
	for n, v := range savedV {
		if hadV[n] {
			b.vars[n] = v
		} else {
			delete(b.vars, n)
		}
	}
	for n, v := range savedA {
		if hadA[n] {
			b.arrays[n] = v
		} else {
			delete(b.arrays, n)
		}
	}
	if err != nil {
		return Zero(), err
	}
	if r.flow == flowReturn {
		return r.value, nil
	}
	if r.flow == flowQuit {
		return Zero(), io.EOF
	}
	return Zero(), nil
}

type exprStmt struct {
	e     expr
	print bool
}

func (s exprStmt) run(b *Interpreter) (result, error) {
	v, e := s.e.eval(b)
	if e == nil && s.print {
		_, e = fmt.Fprintln(b.Out, v.wrappedStringBase(b.OBase))
	}
	return result{}, e
}

type stringStmt string

func (s stringStmt) run(b *Interpreter) (result, error) {
	_, err := fmt.Fprint(b.Out, string(s))
	return result{}, err
}

type blockStmt []stmt

func (s blockStmt) run(b *Interpreter) (result, error) { return b.exec([]stmt(s)) }

type flowStmt flow

func (s flowStmt) run(*Interpreter) (result, error) { return result{flow: flow(s)}, nil }

type returnStmt struct{ e expr }

func (s returnStmt) run(b *Interpreter) (result, error) {
	v, e := s.e.eval(b)
	return result{flow: flowReturn, value: v}, e
}

type defineStmt struct {
	name string
	f    *function
}

func (s defineStmt) run(b *Interpreter) (result, error) { b.funcs[s.name] = s.f; return result{}, nil }

type ifStmt struct {
	c       expr
	yes, no []stmt
}

func (s ifStmt) run(b *Interpreter) (result, error) {
	v, e := s.c.eval(b)
	if e != nil {
		return result{}, e
	}
	if v.Sign() != 0 {
		return b.exec(s.yes)
	}
	return b.exec(s.no)
}

type whileStmt struct {
	c    expr
	body []stmt
}

func (s whileStmt) run(b *Interpreter) (result, error) {
	for {
		v, e := s.c.eval(b)
		if e != nil {
			return result{}, e
		}
		if v.Sign() == 0 {
			return result{}, nil
		}
		r, e := b.exec(s.body)
		if e != nil {
			return r, e
		}
		switch r.flow {
		case flowBreak:
			return result{}, nil
		case flowReturn, flowQuit:
			return r, nil
		}
	}
}

type forStmt struct {
	init, cond, post expr
	body             []stmt
}

func (s forStmt) run(b *Interpreter) (result, error) {
	if s.init != nil {
		if _, e := s.init.eval(b); e != nil {
			return result{}, e
		}
	}
	for {
		if s.cond != nil {
			v, e := s.cond.eval(b)
			if e != nil {
				return result{}, e
			}
			if v.Sign() == 0 {
				return result{}, nil
			}
		}
		r, e := b.exec(s.body)
		if e != nil {
			return r, e
		}
		if r.flow == flowBreak {
			return result{}, nil
		}
		if r.flow == flowReturn || r.flow == flowQuit {
			return r, nil
		}
		if s.post != nil {
			if _, e = s.post.eval(b); e != nil {
				return result{}, e
			}
		}
	}
}
func (b *Interpreter) exec(ss []stmt) (result, error) {
	for _, s := range ss {
		r, e := s.run(b)
		if e != nil {
			return r, e
		}
		if r.flow != flowNone {
			return r, nil
		}
	}
	return result{}, nil
}
