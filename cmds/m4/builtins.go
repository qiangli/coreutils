package m4cmd

import (
	"crypto/rand"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// builtinFunc implements a built-in macro. It returns the text to be
// rescanned; fn is non-nil only for defn of a built-in.
type builtinFunc func(p *processor, name string, args []argument) (text string, fn *macro)

var builtins = map[string]*macro{}

func builtin(name string, blind bool, fn builtinFunc) {
	builtins[name] = &macro{fn: fn, name: name, blind: blind}
}

func init() {
	builtin("define", true, func(p *processor, _ string, args []argument) (string, *macro) {
		p.defineFromArgs(args, false)
		return "", nil
	})
	builtin("pushdef", true, func(p *processor, _ string, args []argument) (string, *macro) {
		p.defineFromArgs(args, true)
		return "", nil
	})
	builtin("undefine", true, func(p *processor, _ string, args []argument) (string, *macro) {
		for _, a := range args {
			delete(p.macros, a.text)
		}
		return "", nil
	})
	builtin("popdef", true, func(p *processor, _ string, args []argument) (string, *macro) {
		for _, a := range args {
			if st := p.macros[a.text]; len(st) > 1 {
				p.macros[a.text] = st[:len(st)-1]
			} else {
				delete(p.macros, a.text)
			}
		}
		return "", nil
	})
	builtin("defn", true, biDefn)
	builtin("ifdef", true, func(p *processor, name string, args []argument) (string, *macro) {
		switch {
		case !p.atLeast(name, args, 2):
		case p.lookup(args[0].text) != nil:
			return args[1].text, nil
		case len(args) > 2:
			return args[2].text, nil
		}
		return "", nil
	})
	builtin("ifelse", true, biIfelse)
	builtin("shift", true, func(p *processor, _ string, args []argument) (string, *macro) {
		if len(args) < 2 {
			return "", nil
		}
		return joinArgs(args[1:], p.lquote, p.rquote), nil
	})
	builtin("dnl", false, func(p *processor, _ string, _ []argument) (string, *macro) {
		for {
			c, ok := p.peekAt(0)
			if !ok {
				break
			}
			if p.next(); c == '\n' {
				break
			}
		}
		return "", nil
	})
	builtin("changequote", false, func(p *processor, _ string, args []argument) (string, *macro) {
		// No arguments restore the defaults; a null start quote turns
		// quoting off; a missing or null end quote is the default one.
		p.lquote, p.rquote = "`", "'"
		if len(args) > 0 && (len(args) > 1 || args[0].present) {
			p.lquote = args[0].text
			if len(args) > 1 && args[1].text != "" {
				p.rquote = args[1].text
			}
		}
		return "", nil
	})
	builtin("changecom", false, func(p *processor, _ string, args []argument) (string, *macro) {
		// No arguments (or a null start) turn comments off; a missing or
		// null end delimiter is a newline.
		p.bcomm, p.ecomm = "", "\n"
		if len(args) > 0 {
			p.bcomm = args[0].text
			if len(args) > 1 && args[1].text != "" {
				p.ecomm = args[1].text
			}
		}
		return "", nil
	})
	builtin("len", true, func(p *processor, _ string, args []argument) (string, *macro) {
		return strconv.Itoa(len(args[0].text)), nil
	})
	builtin("index", true, func(p *processor, name string, args []argument) (string, *macro) {
		if !p.atLeast(name, args, 2) {
			return "", nil
		}
		return strconv.Itoa(strings.Index(args[0].text, args[1].text)), nil
	})
	builtin("substr", true, biSubstr)
	builtin("translit", true, biTranslit)
	builtin("incr", true, func(p *processor, name string, args []argument) (string, *macro) {
		return p.addTo(name, args, 1), nil
	})
	builtin("decr", true, func(p *processor, name string, args []argument) (string, *macro) {
		return p.addTo(name, args, -1), nil
	})
	builtin("eval", true, biEval)

	builtin("divert", false, func(p *processor, name string, args []argument) (string, *macro) {
		n := int32(0)
		ok := true
		if len(args) > 0 && args[0].text != "" {
			n, ok = p.number(name, args[0].text)
		}
		if !ok {
			return "", nil
		}
		if p.divnum != int(n) {
			p.outLine, p.outSeq = -1, 0
		}
		p.divnum = int(n)
		return "", nil
	})
	builtin("divnum", false, func(p *processor, _ string, _ []argument) (string, *macro) { return strconv.Itoa(p.divnum), nil })
	builtin("undivert", false, func(p *processor, name string, args []argument) (string, *macro) {
		undiv := func(n int) {
			if n > 0 && n < len(p.diversions) {
				s := p.diversions[n].String()
				p.diversions[n].Reset()
				p.writeOutput(s)
			}
		}
		if len(args) == 0 {
			for i := 1; i < len(p.diversions); i++ {
				undiv(i)
			}
			return "", nil
		}
		for _, a := range args {
			n, ok := p.number(name, a.text)
			if ok {
				undiv(int(n))
			}
		}
		return "", nil
	})
	builtin("include", true, func(p *processor, name string, args []argument) (string, *macro) {
		if !p.atLeast(name, args, 1) {
			return "", nil
		}
		data, err := p.readNamedFile(args[0].text)
		if err != nil {
			p.fatalf("%s: %v", args[0].text, err)
		}
		p.pushIncluded(args[0].text, data)
		return "", nil
	})
	builtin("sinclude", true, func(p *processor, _ string, args []argument) (string, *macro) {
		if len(args) == 0 {
			return "", nil
		}
		data, err := p.readNamedFile(args[0].text)
		if err != nil {
			return "", nil
		}
		p.pushIncluded(args[0].text, data)
		return "", nil
	})
	builtin("syscmd", true, func(p *processor, name string, args []argument) (string, *macro) {
		if !p.atLeast(name, args, 1) {
			return "", nil
		}
		// syscmd writes directly to the invocation stream. Flush m4's buffered
		// prefix first so observable output remains in source order.
		if err := p.out.Flush(); err != nil {
			p.fatalf("write error: %v", err)
		}
		p.sysval = p.runSystem(args[0].text)
		return "", nil
	})
	builtin("sysval", false, func(p *processor, _ string, _ []argument) (string, *macro) { return strconv.Itoa(p.sysval), nil })
	builtin("maketemp", true, biMaketemp)
	builtin("mkstemp", true, biMkstemp)
	builtin("m4exit", false, func(p *processor, name string, args []argument) (string, *macro) {
		code := int32(0)
		var ok bool = true
		if len(args) > 0 {
			code, ok = p.number(name, args[0].text)
		}
		if ok {
			p.exitSet = true
			p.exitCode = int(code)
			if code != 0 {
				p.failed = true
			}
		}
		panic(fatal{})
	})
	builtin("m4wrap", true, func(p *processor, _ string, args []argument) (string, *macro) {
		if len(args) > 0 {
			p.wraps = append(p.wraps, args[0].text)
		}
		return "", nil
	})
	builtin("errprint", true, func(p *processor, _ string, args []argument) (string, *macro) {
		for _, a := range args {
			fmt.Fprint(p.rc.Err, a.text)
		}
		return "", nil
	})
	builtin("dumpdef", false, biDumpdef)
	builtin("traceon", false, func(p *processor, _ string, args []argument) (string, *macro) {
		if len(args) == 0 {
			p.traceAll = true
			p.trace = map[string]bool{}
		} else {
			for _, a := range args {
				p.trace[a.text] = true
			}
		}
		return "", nil
	})
	builtin("traceoff", false, func(p *processor, _ string, args []argument) (string, *macro) {
		if len(args) == 0 {
			p.traceAll = false
			p.trace = map[string]bool{}
		} else {
			for _, a := range args {
				p.trace[a.text] = false
			}
		}
		return "", nil
	})
}

// atLeast warns and reports false when a built-in has too few arguments.
func (p *processor) atLeast(name string, args []argument, n int) bool {
	if len(args) >= n {
		return true
	}
	p.warnf("Warning: too few arguments to builtin `%s'", name)
	return false
}

// number parses a decimal integer argument of a built-in. A null string
// counts as 0 with a warning; anything else non-numeric is an error and
// the built-in expands to nothing.
func (p *processor) number(name, s string) (int32, bool) {
	if s == "" {
		p.warnf("empty string treated as 0 in builtin `%s'", name)
		return 0, true
	}
	v, err := strconv.ParseInt(strings.TrimLeft(s, " \t\n"), 10, 64)
	if err != nil {
		p.warnf("non-numeric argument to builtin `%s'", name)
		return 0, false
	}
	return int32(v), true
}

func (p *processor) defineFromArgs(args []argument, push bool) {
	name := args[0].text
	if name == "" {
		return
	}
	m := &macro{}
	if len(args) > 1 {
		if args[1].fn != nil {
			m = args[1].fn
		} else {
			m.text = args[1].text
		}
	}
	p.define(name, m, push)
}

// biDefn yields the quoted definition of each named macro. A built-in
// has no text: alone, its definition is handed back as fn so define and
// pushdef can install it under another name.
func biDefn(p *processor, _ string, args []argument) (string, *macro) {
	var b strings.Builder
	for _, a := range args {
		m := p.lookup(a.text)
		switch {
		case m == nil:
		case m.fn != nil:
			if len(args) == 1 {
				return "", m
			}
		default:
			b.WriteString(p.lquote + m.text + p.rquote)
		}
	}
	return b.String(), nil
}

// biIfelse compares arguments pairwise: with three or four it is a plain
// if-then[-else]; with more, a failed comparison drops the first three
// and starts over on the rest.
func biIfelse(p *processor, name string, args []argument) (string, *macro) {
	if len(args) == 1 {
		return "", nil
	}
	if !p.atLeast(name, args, 3) {
		return "", nil
	}
	for {
		if args[0].text == args[1].text {
			return args[2].text, nil
		}
		switch len(args) {
		case 3:
			return "", nil
		case 4, 5:
			return args[3].text, nil
		}
		args = args[3:]
	}
}

func biSubstr(p *processor, name string, args []argument) (string, *macro) {
	if !p.atLeast(name, args, 2) {
		return "", nil
	}
	s := args[0].text
	start, ok := p.number(name, args[1].text)
	if !ok {
		return "", nil
	}
	length := int32(len(s))
	if len(args) > 2 {
		if length, ok = p.number(name, args[2].text); !ok {
			return "", nil
		}
	}
	if start < 0 || int(start) >= len(s) || length <= 0 {
		return "", nil
	}
	end := int(start) + int(length)
	if end > len(s) {
		end = len(s)
	}
	return s[start:end], nil
}

// biTranslit maps each character of the first argument that occurs in
// the second to the character at the same position in the third, or
// deletes it when the third is too short. POSIX leaves '-' between two
// characters unspecified; it is read as the range between them.
func biTranslit(p *processor, name string, args []argument) (string, *macro) {
	if !p.atLeast(name, args, 2) {
		return "", nil
	}
	from := expandRanges(args[1].text)
	var to string
	if len(args) > 2 {
		to = expandRanges(args[2].text)
	}
	const unmapped, deleted = -1, -2
	var table [256]int
	for i := range table {
		table[i] = unmapped
	}
	for i := 0; i < len(from); i++ {
		if table[from[i]] != unmapped {
			continue
		}
		if i < len(to) {
			table[from[i]] = int(to[i])
		} else {
			table[from[i]] = deleted
		}
	}
	var b strings.Builder
	for i := 0; i < len(args[0].text); i++ {
		switch c := args[0].text[i]; table[c] {
		case unmapped:
			b.WriteByte(c)
		case deleted:
		default:
			b.WriteByte(byte(table[c]))
		}
	}
	return b.String(), nil
}

func expandRanges(s string) string {
	if !strings.Contains(s, "-") {
		return s
	}
	var b []byte
	for i := 0; i < len(s); i++ {
		if s[i] != '-' || i == 0 || i+1 == len(s) {
			b = append(b, s[i])
			continue
		}
		lo, hi := int(s[i-1]), int(s[i+1])
		i++
		for c := lo; c != hi; {
			if lo < hi {
				c++
			} else {
				c--
			}
			b = append(b, byte(c))
		}
	}
	return string(b)
}

func (p *processor) addTo(name string, args []argument, delta int32) string {
	v, ok := p.number(name, args[0].text)
	if !ok {
		return ""
	}
	return strconv.Itoa(int(v + delta))
}

// biEval evaluates the first argument as an integer expression and
// formats it in the radix of the second (default 10) with at least as
// many digits as the third asks for.
func biEval(p *processor, name string, args []argument) (string, *macro) {
	radix, width := int32(10), int32(1)
	var ok bool
	if len(args) > 1 && args[1].text != "" {
		if radix, ok = p.number(name, args[1].text); !ok {
			return "", nil
		}
		if radix < 1 || radix > 36 {
			p.warnf("radix in builtin `%s' out of range (radix = %d)", name, radix)
			return "", nil
		}
	}
	if len(args) > 2 && args[2].text != "" {
		if width, ok = p.number(name, args[2].text); !ok {
			return "", nil
		}
		if width < 0 {
			p.warnf("negative width to builtin `%s'", name)
			return "", nil
		}
	}
	if args[0].text == "" {
		p.warnf("empty string treated as 0 in builtin `%s'", name)
	}
	v, err := evalExpr(args[0].text)
	if err != "" {
		p.warnf("%s in eval: %s", err, args[0].text)
		p.failed = true
		return "", nil
	}
	mag := uint64(v)
	sign := ""
	if v < 0 {
		mag, sign = uint64(-int64(v)), "-"
	}
	var digits string
	if radix == 1 {
		digits = strings.Repeat("1", int(mag))
	} else {
		digits = strings.ToUpper(strconv.FormatUint(mag, int(radix)))
	}
	if pad := int(width) - len(digits); pad > 0 {
		digits = strings.Repeat("0", pad) + digits
	}
	return sign + digits, nil
}

func biMaketemp(p *processor, name string, args []argument) (string, *macro) {
	if !p.atLeast(name, args, 1) {
		return "", nil
	}
	tmpl := args[0].text
	last := len(tmpl)
	first := last
	for first > 0 && tmpl[first-1] == 'X' {
		first--
	}
	if first == last {
		return tmpl, nil
	}
	return tmpl[:first] + strconv.Itoa(os.Getpid()), nil
}

func biMkstemp(p *processor, name string, args []argument) (string, *macro) {
	if !p.atLeast(name, args, 1) {
		return "", nil
	}
	tmpl := args[0].text
	width := len(tmpl) - len(strings.TrimRight(tmpl, "X"))
	if width < 6 {
		p.warnf("%s: template must end in XXXXXX", name)
		p.failed = true
		return "", nil
	}
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	for attempt := 0; attempt < 100; attempt++ {
		suffix := make([]byte, width)
		if _, err := rand.Read(suffix); err != nil {
			p.warnf("%s: random suffix: %v", name, err)
			p.failed = true
			return "", nil
		}
		for i := range suffix {
			suffix[i] = letters[int(suffix[i])%len(letters)]
		}
		// Keep the caller's relative directory spelling in the expansion.
		candidate := tmpl[:len(tmpl)-width] + string(suffix)
		f, err := os.OpenFile(p.rc.Path(candidate), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
		if os.IsExist(err) {
			continue
		}
		if err == nil {
			err = f.Close()
		}
		if err == nil {
			return candidate, nil
		}
		p.warnf("%s: %v", name, err)
		p.failed = true
		return "", nil
	}
	p.warnf("%s: cannot create unique file", name)
	p.failed = true
	return "", nil
}

func biDumpdef(p *processor, _ string, args []argument) (string, *macro) {
	names := args
	if len(names) == 0 {
		for n := range p.macros {
			names = append(names, argument{text: n})
		}
	}
	for _, a := range names {
		if m := p.lookup(a.text); m != nil {
			if m.fn != nil {
				fmt.Fprintf(p.rc.Err, "%s:\t<%s>\n", a.text, m.name)
			} else {
				fmt.Fprintf(p.rc.Err, "%s:\t%s\n", a.text, m.text)
			}
		}
	}
	return "", nil
}
