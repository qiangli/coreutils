// Package makecmd implements the POSIX make dependency builder in pure Go.
//
// POSIX.1 Issue 7 is the normative contract. GNU make 4.3 is used only as a
// differential oracle. This package intentionally remains unregistered until
// its conformance suite and the Profile D tests close.
package makecmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{Name: "make", Synopsis: "Maintain, update, and regenerate groups of programs.", Usage: "make [-einpqrst] [-f makefile]... [-k|-S] [macro=value...] [target_name...]"}

type options struct {
	envOverride, ignore, keep, dry, print, question, noBuiltins, silent, touch bool
	files                                                                      []string
}

type origin uint8

const (
	originCommand origin = iota + 1
	originMakeflags
	originEnvironment
	originBuiltin
	originFile
)

type variable struct {
	value  string
	origin origin
}

type rule struct {
	targets, deps []string
	recipes       []string
	line          int
	inference     bool
}

type makefile struct {
	vars                     map[string]variable
	rules                    map[string][]*rule
	order                    []string
	precious, silent, ignore map[string]bool
	preciousAll              bool
	defaultRecipe            []string
	sccsRecipe               []string
	suffixes                 []string
	envOverride              bool
	posixSeen                bool
}

type engine struct {
	rc                   *tool.RunContext
	child                *tool.RunContext
	o                    options
	m                    *makefile
	visiting, done, made map[string]bool
	failed               bool
	action               bool
}

var errOutOfDate = errors.New("target is out of date")

func run(rc *tool.RunContext, args []string) int {
	restoreSignals := installMakeSignalContext(rc)
	defer restoreSignals()
	if rc.FS == nil {
		rc.FS = tool.NewLocalFS()
	}
	makeflagArgs, err := splitMakeflags(rc.Getenv("MAKEFLAGS"))
	if err != nil {
		return tool.UsageError(rc, cmd, "invalid MAKEFLAGS: %v", err)
	}
	o, mfOperands, err := parseArgs(makeflagArgs, options{})
	if err != nil {
		return tool.UsageError(rc, cmd, "MAKEFLAGS: %v", err)
	}
	o, operands, err := parseArgs(args, o)
	if err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
	}

	m := newMakefile(o.noBuiltins)
	m.envOverride = o.envOverride
	m.loadEnvironment(rc.Env)
	var targets []string
	makeflagMacros := make([][2]string, 0)
	for _, item := range mfOperands {
		if n, v, ok := macroOperand(item); ok {
			m.assign(n, v, originMakeflags, o.envOverride)
			makeflagMacros = append(makeflagMacros, [2]string{n, v})
		}
	}
	command := make([][2]string, 0)
	for _, item := range operands {
		if n, v, ok := macroOperand(item); ok {
			m.assign(n, v, originCommand, o.envOverride)
			command = append(command, [2]string{n, v})
		} else {
			targets = append(targets, item)
		}
	}
	makeflags := canonicalMakeflags(o, append(makeflagMacros, command...))
	m.assign("MAKEFLAGS", makeflags, originCommand, o.envOverride)

	files := o.files
	if len(files) == 0 {
		for _, name := range []string{"makefile", "Makefile"} {
			if _, statErr := rc.FS.Stat(rc.Path(name)); statErr == nil {
				files = []string{name}
				break
			}
		}
		if len(files) == 0 {
			// A makefile is not required when an operand can be resolved from an
			// existing file or a built-in inference rule.
		}
	}
	for _, name := range files {
		if err := m.parseFile(rc, name, map[string]bool{}, 0); err != nil {
			fmt.Fprintf(rc.Err, "make: %v\n", err)
			return 2
		}
	}
	if o.print {
		m.printDB(rc.Out)
	}
	if len(targets) == 0 {
		if len(m.order) == 0 {
			fmt.Fprintln(rc.Err, "make: no targets specified and no makefile targets found")
			return 2
		}
		targets = []string{m.order[0]}
	}

	child := *rc
	child.Env = recipeEnvironment(rc.Env, m, command, makeflags)
	e := &engine{rc: rc, child: &child, o: o, m: m, visiting: map[string]bool{}, done: map[string]bool{}, made: map[string]bool{}}
	questionOutdated := false
	for _, target := range targets {
		_, updateErr := e.update(target)
		if errors.Is(updateErr, errOutOfDate) {
			questionOutdated = true
			continue
		}
		if updateErr != nil {
			fmt.Fprintf(rc.Err, "make: %v\n", updateErr)
			e.failed = true
			if !o.keep {
				return 2
			}
		}
	}
	if e.failed {
		return 2
	}
	if o.question && questionOutdated {
		return 1
	}
	if !e.action && !o.question && !o.print {
		fmt.Fprintln(rc.Out, "make: nothing to be done")
	}
	return 0
}

func parseArgs(args []string, o options) (options, []string, error) {
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		if a == "-f" {
			if i+1 >= len(args) {
				return o, nil, fmt.Errorf("-f requires an operand")
			}
			i++
			o.files = append(o.files, args[i])
			continue
		}
		if strings.HasPrefix(a, "-f") && len(a) > 2 {
			o.files = append(o.files, a[2:])
			continue
		}
		if len(a) > 1 && a[0] == '-' {
			for pos := 1; pos < len(a); pos++ {
				c := rune(a[pos])
				switch c {
				case 'e':
					o.envOverride = true
				case 'i':
					o.ignore = true
				case 'k':
					o.keep = true
				case 'n':
					o.dry = true
				case 'p':
					o.print = true
				case 'q':
					o.question = true
				case 'r':
					o.noBuiltins = true
				case 'S':
					o.keep = false
				case 's':
					o.silent = true
				case 't':
					o.touch = true
				case 'f':
					if pos+1 < len(a) {
						o.files = append(o.files, a[pos+1:])
						pos = len(a)
					} else {
						if i+1 >= len(args) {
							return o, nil, fmt.Errorf("-f requires an operand")
						}
						i++
						o.files = append(o.files, args[i])
					}
				default:
					return o, nil, fmt.Errorf("unknown option -%c", c)
				}
			}
			continue
		}
		rest = append(rest, a)
	}
	return o, rest, nil
}

// MAKEFLAGS accepts the historical bare option-letter form and a
// whitespace-separated option/macro form. We emit the latter ourselves.
func splitMakeflags(s string) ([]string, error) {
	var fields []string
	var word strings.Builder
	quote := byte(0)
	escaped := false
	flush := func() {
		if word.Len() > 0 {
			fields = append(fields, word.String())
			word.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			word.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				word.WriteByte(c)
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == ' ' || c == '\t' || c == '\n' {
			flush()
			continue
		}
		word.WriteByte(c)
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("unterminated quoting")
	}
	flush()
	if len(fields) == 0 {
		return nil, nil
	}
	if fields[0] != "" && fields[0][0] != '-' && !strings.Contains(fields[0], "=") {
		fields[0] = "-" + fields[0]
	}
	return fields, nil
}

func macroOperand(s string) (string, string, bool) {
	n, v, ok := strings.Cut(s, "=")
	return n, v, ok && validName(n)
}

func validName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r == '.' || r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func canonicalMakeflags(o options, command [][2]string) string {
	var flags strings.Builder
	if o.envOverride {
		flags.WriteByte('e')
	}
	if o.ignore {
		flags.WriteByte('i')
	}
	if o.keep {
		flags.WriteByte('k')
	}
	if o.dry {
		flags.WriteByte('n')
	}
	if o.question {
		flags.WriteByte('q')
	}
	if o.noBuiltins {
		flags.WriteByte('r')
	}
	if o.silent {
		flags.WriteByte('s')
	}
	if o.touch {
		flags.WriteByte('t')
	}
	parts := []string{}
	if flags.Len() > 0 {
		parts = append(parts, "-"+flags.String())
	}
	for _, pair := range command {
		if pair[0] != "MAKEFLAGS" && pair[0] != "SHELL" {
			parts = append(parts, quoteMakeflag(pair[0]+"="+pair[1]))
		}
	}
	return strings.Join(parts, " ")
}

func quoteMakeflag(s string) string {
	if !strings.ContainsAny(s, " \t\n'\\\"") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func newMakefile(noBuiltins bool) *makefile {
	m := &makefile{vars: map[string]variable{}, rules: map[string][]*rule{}, precious: map[string]bool{}, silent: map[string]bool{}, ignore: map[string]bool{}}
	m.assign("SHELL", "/bin/sh", originBuiltin, false)
	if !noBuiltins {
		m.installBuiltins()
	}
	return m
}

func (m *makefile) installBuiltins() {
	for name, value := range map[string]string{"MAKE": "make", "AR": "ar", "ARFLAGS": "-rv", "YACC": "yacc", "YFLAGS": "", "LEX": "lex", "LFLAGS": "", "LDFLAGS": "", "CC": "c99", "CFLAGS": "-O 1", "FC": "fort77", "FFLAGS": "-O 1", "GET": "get", "GFLAGS": "", "SCCSFLAGS": "", "SCCSGETFLAGS": "-s"} {
		m.assign(name, value, originBuiltin, false)
	}
	m.suffixes = []string{".o", ".c", ".y", ".l", ".a", ".sh", ".f"}
	defs := map[string][]string{
		".c.o": {"$(CC) $(CFLAGS) -c $<"}, ".f.o": {"$(FC) $(FFLAGS) -c $<"},
		".y.o": {"$(YACC) $(YFLAGS) $<", "$(CC) $(CFLAGS) -c y.tab.c", "rm -f y.tab.c", "mv y.tab.o $@"},
		".l.o": {"$(LEX) $(LFLAGS) $<", "$(CC) $(CFLAGS) -c lex.yy.c", "rm -f lex.yy.c", "mv lex.yy.o $@"},
		".y.c": {"$(YACC) $(YFLAGS) $<", "mv y.tab.c $@"},
		".l.c": {"$(LEX) $(LFLAGS) $<", "mv lex.yy.c $@"},
		".c":   {"$(CC) $(CFLAGS) $(LDFLAGS) -o $@ $<"}, ".f": {"$(FC) $(FFLAGS) $(LDFLAGS) -o $@ $<"},
		".sh":  {"cp $< $@", "chmod a+x $@"},
		".c.a": {"$(CC) -c $(CFLAGS) $<", "$(AR) $(ARFLAGS) $@ $*.o", "rm -f $*.o"},
		".f.a": {"$(FC) -c $(FFLAGS) $<", "$(AR) $(ARFLAGS) $@ $*.o", "rm -f $*.o"},
	}
	for target, recipes := range defs {
		m.rules[target] = []*rule{{targets: []string{target}, recipes: recipes, inference: true}}
	}
	m.sccsRecipe = []string{"sccs $(SCCSFLAGS) get $(SCCSGETFLAGS) $@"}
}

func (m *makefile) loadEnvironment(env []string) {
	for _, item := range env {
		n, v, ok := strings.Cut(item, "=")
		if ok && n != "MAKEFLAGS" && n != "SHELL" {
			m.assign(n, v, originEnvironment, false)
		}
	}
}

func (m *makefile) assign(name, value string, from origin, envOverride bool) {
	old, exists := m.vars[name]
	if !exists || from <= old.origin || from == originFile && (old.origin == originBuiltin || old.origin == originFile || old.origin == originEnvironment && !envOverride) {
		m.vars[name] = variable{value: value, origin: from}
	}
}

func recipeEnvironment(base []string, m *makefile, command [][2]string, makeflags string) []string {
	env := append([]string(nil), base...)
	set := func(name, value string) {
		prefix := name + "="
		for i := len(env) - 1; i >= 0; i-- {
			if strings.HasPrefix(env[i], prefix) {
				env[i] = prefix + value
				return
			}
		}
		env = append(env, prefix+value)
	}
	set("MAKEFLAGS", makeflags)
	for _, pair := range command {
		if pair[0] != "SHELL" && pair[0] != "MAKEFLAGS" {
			set(pair[0], pair[1])
		}
	}
	for i, item := range env {
		name, _, ok := strings.Cut(item, "=")
		if ok && name != "SHELL" && name != "MAKEFLAGS" {
			if v, found := m.vars[name]; found {
				env[i] = name + "=" + m.expand(v.value, nil, map[string]bool{})
			}
		}
	}
	return env
}

func (m *makefile) parseFile(rc *tool.RunContext, name string, seen map[string]bool, depth int) error {
	if depth > 16 {
		return fmt.Errorf("include nesting exceeds 16 files")
	}
	if name == "-" {
		return m.parse(rc, rc.In, "standard input", seen, depth)
	}
	path := rc.Path(name)
	if seen[path] {
		return fmt.Errorf("recursive include of %s", name)
	}
	seen[path] = true
	defer delete(seen, path)
	f, err := rc.FS.Open(path)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	defer f.Close()
	return m.parse(rc, f, name, seen, depth)
}

func (m *makefile) parse(rc *tool.RunContext, input io.Reader, name string, seen map[string]bool, depth int) error {
	s := bufio.NewScanner(input)
	var lines []string
	for s.Scan() {
		lines = append(lines, s.Text())
	}
	if err := s.Err(); err != nil {
		return err
	}
	var current *rule
	for i := 0; i < len(lines); i++ {
		lineNo := i + 1
		raw := lines[i]
		if strings.HasPrefix(raw, "\t") {
			if current == nil {
				return fmt.Errorf("%s:%d: recipe without target", name, lineNo)
			}
			recipe := raw[1:]
			for oddTrailingSlash(recipe) && i+1 < len(lines) {
				i++
				next := lines[i]
				if strings.HasPrefix(next, "\t") {
					next = next[1:]
				}
				recipe += "\n" + next
			}
			current.recipes = append(current.recipes, recipe)
			if len(current.targets) == 1 && current.targets[0] == ".DEFAULT" {
				m.defaultRecipe = current.recipes
			}
			if len(current.targets) == 1 && current.targets[0] == ".SCCS_GET" {
				m.sccsRecipe = current.recipes
			}
			continue
		}
		for oddTrailingSlash(raw) && i+1 < len(lines) {
			raw = strings.TrimSuffix(raw, "\\") + " " + strings.TrimLeft(lines[i+1], " \t")
			i++
		}
		trim := strings.TrimSpace(raw)
		if trim == "" || strings.HasPrefix(trim, "#") {
			current = nil
			continue
		}
		if strings.HasPrefix(raw, "include ") || strings.HasPrefix(raw, "include\t") {
			value := strings.TrimSpace(stripComment(raw[len("include"):]))
			fields := strings.Fields(m.expand(value, nil, map[string]bool{}))
			if len(fields) != 1 {
				return fmt.Errorf("%s:%d: include requires one pathname", name, lineNo)
			}
			if err := m.parseFile(rc, fields[0], seen, depth+1); err != nil {
				return err
			}
			current = nil
			continue
		}
		text := stripComment(raw)
		if eq := assignmentIndex(text); eq >= 0 {
			left := strings.TrimSpace(m.expand(text[:eq], nil, map[string]bool{}))
			if !validName(left) {
				return fmt.Errorf("%s:%d: invalid macro name %q", name, lineNo, left)
			}
			m.assign(left, strings.TrimLeft(text[eq+1:], " \t"), originFile, m.envOverride)
			current = nil
			continue
		}
		colon := strings.Index(text, ":")
		if colon < 0 {
			return fmt.Errorf("%s:%d: expected target rule or macro assignment", name, lineNo)
		}
		left, right := strings.TrimSpace(text[:colon]), text[colon+1:]
		inline := ""
		hadSemi := false
		if semi := strings.Index(right, ";"); semi >= 0 {
			inline, right, hadSemi = strings.TrimLeft(right[semi+1:], " \t"), right[:semi], true
		}
		targets := strings.Fields(m.expand(left, nil, map[string]bool{}))
		deps := strings.Fields(m.expand(strings.TrimSpace(right), nil, map[string]bool{}))
		if len(targets) == 0 {
			return fmt.Errorf("%s:%d: empty target", name, lineNo)
		}
		r := &rule{targets: targets, deps: deps, line: lineNo}
		if hadSemi {
			r.recipes = append(r.recipes, inline)
		}
		current = r
		for _, target := range targets {
			switch target {
			case ".DEFAULT":
				if len(deps) != 0 {
					return fmt.Errorf("%s:%d: .DEFAULT cannot have prerequisites", name, lineNo)
				}
				m.defaultRecipe = r.recipes
			case ".IGNORE":
				if len(r.recipes) != 0 {
					return fmt.Errorf("%s:%d: .IGNORE cannot have commands", name, lineNo)
				}
				if len(deps) == 0 {
					m.ignore[""] = true
				} else {
					for _, d := range deps {
						m.ignore[d] = true
					}
				}
			case ".POSIX":
				if len(deps) != 0 || len(r.recipes) != 0 {
					return fmt.Errorf("%s:%d: .POSIX cannot have prerequisites or commands", name, lineNo)
				}
				m.posixSeen = true
			case ".SCCS_GET":
				if len(deps) != 0 {
					return fmt.Errorf("%s:%d: .SCCS_GET cannot have prerequisites", name, lineNo)
				}
				m.sccsRecipe = r.recipes
			case ".PRECIOUS":
				if len(r.recipes) != 0 {
					return fmt.Errorf("%s:%d: .PRECIOUS cannot have commands", name, lineNo)
				}
				if len(deps) == 0 {
					m.preciousAll = true
				} else {
					for _, d := range deps {
						m.precious[d] = true
					}
				}
			case ".SILENT":
				if len(r.recipes) != 0 {
					return fmt.Errorf("%s:%d: .SILENT cannot have commands", name, lineNo)
				}
				if len(deps) == 0 {
					m.silent[""] = true
				} else {
					for _, d := range deps {
						m.silent[d] = true
					}
				}
			case ".SUFFIXES":
				if len(r.recipes) != 0 {
					return fmt.Errorf("%s:%d: .SUFFIXES cannot have commands", name, lineNo)
				}
				if len(deps) == 0 {
					m.suffixes = nil
				} else {
					m.suffixes = append(m.suffixes, deps...)
				}
			default:
				r.inference = isInferenceName(target)
				if r.inference {
					if len(targets) != 1 || len(deps) != 0 {
						return fmt.Errorf("%s:%d: invalid inference rule", name, lineNo)
					}
					m.rules[target] = []*rule{r}
				} else {
					m.rules[target] = append(m.rules[target], r)
					if !strings.HasPrefix(target, ".") {
						m.order = appendUnique(m.order, target)
					}
				}
			}
		}
	}
	return nil
}

func oddTrailingSlash(s string) bool {
	n := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}
func assignmentIndex(s string) int {
	colon := strings.IndexByte(s, ':')
	for i := 0; i < len(s); i++ {
		if s[i] == '=' && (colon < 0 || i < colon) {
			return i
		}
	}
	return -1
}
func stripComment(s string) string {
	escaped := false
	for i := 0; i < len(s); i++ {
		if s[i] == '#' && !escaped {
			return s[:i]
		}
		if s[i] == '\\' {
			escaped = !escaped
		} else {
			escaped = false
		}
	}
	return s
}
func appendUnique(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}
func isInferenceName(name string) bool {
	return !strings.ContainsAny(name, "/\\") && strings.HasPrefix(name, ".") && strings.Count(name, ".") <= 2 && len(name) > 1
}

type automatic struct {
	target, member, first, stem string
	newer                       []string
}

func (m *makefile) expand(s string, a *automatic, stack map[string]bool) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '$' {
			out.WriteByte(s[i])
			i++
			continue
		}
		if i+1 >= len(s) {
			out.WriteByte('$')
			break
		}
		c := s[i+1]
		if c == '$' {
			out.WriteByte('$')
			i += 2
			continue
		}
		name := string(c)
		end := i + 2
		if c == '(' || c == '{' {
			close := byte(')')
			if c == '{' {
				close = '}'
			}
			j := i + 2
			for j < len(s) && s[j] != close {
				j++
			}
			if j == len(s) {
				out.WriteString(s[i:])
				break
			}
			name, end = s[i+2:j], j+1
		}
		base, old, replacement, subst := parseSubstitution(name)
		value := automaticValue(base, a)
		if value == "" {
			if stack[base] {
				i = end
				continue
			}
			if v, ok := m.vars[base]; ok {
				stack[base] = true
				value = m.expand(v.value, a, stack)
				delete(stack, base)
			}
		}
		if subst {
			value = suffixSubstitute(value, old, m.expand(replacement, a, stack))
		}
		out.WriteString(value)
		i = end
	}
	return out.String()
}

func parseSubstitution(name string) (base, old, replacement string, ok bool) {
	colon := strings.IndexByte(name, ':')
	if colon < 0 {
		return name, "", "", false
	}
	eq := strings.IndexByte(name[colon+1:], '=')
	if eq < 0 {
		return name, "", "", false
	}
	eq += colon + 1
	return name[:colon], name[colon+1 : eq], name[eq+1:], true
}
func suffixSubstitute(value, old, replacement string) string {
	var out strings.Builder
	for i := 0; i < len(value); {
		if value[i] == ' ' || value[i] == '\t' || value[i] == '\n' {
			out.WriteByte(value[i])
			i++
			continue
		}
		j := i
		for j < len(value) && value[j] != ' ' && value[j] != '\t' && value[j] != '\n' {
			j++
		}
		word := value[i:j]
		if strings.HasSuffix(word, old) {
			word = strings.TrimSuffix(word, old) + replacement
		}
		out.WriteString(word)
		i = j
	}
	return out.String()
}
func automaticValue(name string, a *automatic) string {
	if a == nil || name == "" {
		return ""
	}
	var value string
	switch name[0] {
	case '@':
		value = a.target
	case '%':
		value = a.member
	case '?':
		value = strings.Join(a.newer, " ")
	case '<':
		value = a.first
	case '*':
		value = a.stem
	default:
		return ""
	}
	if len(name) == 2 && (name[1] == 'D' || name[1] == 'F') {
		parts := strings.Fields(value)
		for i, part := range parts {
			if name[1] == 'D' {
				d := filepath.Dir(part)
				if d == "" {
					d = "."
				}
				parts[i] = d
			} else {
				parts[i] = filepath.Base(part)
			}
		}
		return strings.Join(parts, " ")
	}
	return value
}

func (e *engine) update(target string) (bool, error) {
	if e.done[target] {
		return e.made[target], nil
	}
	if e.visiting[target] {
		return false, fmt.Errorf("dependency cycle involving %s", target)
	}
	e.visiting[target] = true
	defer delete(e.visiting, target)
	rules := e.m.rules[target]
	stem := explicitStem(target, e.m.suffixes)
	if len(rules) == 0 {
		var inferred *rule
		inferred, stem = e.infer(target)
		if inferred != nil {
			rules = []*rule{inferred}
		}
	}
	if len(rules) == 0 && len(e.m.sccsRecipe) > 0 && e.sccsNeedsGet(target) {
		rules = []*rule{{targets: []string{target}, recipes: e.m.sccsRecipe}}
	}
	if len(rules) == 0 {
		if _, err := e.targetInfo(target); err == nil {
			e.done[target] = true
			return false, nil
		}
		if len(e.m.defaultRecipe) > 0 {
			rules = []*rule{{targets: []string{target}, recipes: e.m.defaultRecipe}}
			stem = explicitStem(target, e.m.suffixes)
		} else {
			return false, fmt.Errorf("don't know how to make %s", target)
		}
	}
	var deps, recipes []string
	for _, r := range rules {
		deps = append(deps, r.deps...)
		if len(r.recipes) > 0 {
			recipes = r.recipes
		}
	}
	deps = unique(deps)
	depChanged, depFailed := false, false
	for _, dep := range deps {
		changed, err := e.update(dep)
		if err != nil {
			if e.o.keep {
				fmt.Fprintf(e.rc.Err, "make: %v\n", err)
				e.failed = true
				depFailed = true
				continue
			}
			return false, err
		}
		depChanged = depChanged || changed
	}
	if depFailed {
		return false, fmt.Errorf("target %s not remade because of errors", target)
	}
	info, statErr := e.targetInfo(target)
	needed := statErr != nil
	newer := []string{}
	for _, dep := range deps {
		di, err := e.targetInfo(dep)
		if err == nil && (statErr != nil || di.ModTime().After(info.ModTime())) {
			newer = append(newer, dep)
			needed = true
		}
		if e.made[dep] {
			needed = true
			if !contains(newer, dep) {
				newer = append(newer, dep)
			}
		}
	}
	if depChanged && statErr != nil {
		needed = true
	}
	if !needed {
		e.done[target] = true
		return false, nil
	}
	outer, member := archiveParts(target)
	first := ""
	if len(deps) > 0 {
		first = deps[0]
	}
	if len(e.m.defaultRecipe) > 0 && len(e.m.rules[target]) == 0 {
		first = target
	}
	a := &automatic{target: outer, member: member, first: first, stem: stem, newer: newer}
	if e.o.question {
		for _, line := range recipes {
			if hasPlusPrefix(line) {
				if err := e.recipe(target, e.m.expand(line, a, map[string]bool{}), true); err != nil {
					return false, err
				}
			}
		}
		return false, errOutOfDate
	}
	if e.o.touch {
		for _, line := range recipes {
			if hasPlusPrefix(line) {
				if err := e.recipe(target, e.m.expand(line, a, map[string]bool{}), true); err != nil {
					return false, err
				}
			}
		}
		if len(recipes) == 0 {
			e.done[target] = true
			e.made[target] = true
			return true, nil
		}
		if err := e.touch(target); err != nil {
			return false, err
		}
		e.done[target], e.made[target], e.action = true, true, true
		if !e.o.silent && !e.m.silent[""] && !e.m.silent[target] {
			fmt.Fprintf(e.rc.Out, "touch %s\n", target)
		}
		return true, nil
	}
	if len(recipes) == 0 {
		e.done[target] = true
		e.made[target] = true
		return true, nil
	}
	for _, line := range recipes {
		if err := e.recipe(target, e.m.expand(line, a, map[string]bool{}), false); err != nil {
			e.cleanupCancelled(target)
			return false, err
		}
	}
	e.done[target], e.made[target], e.action = true, true, true
	return true, nil
}

func (e *engine) sccsNeedsGet(target string) bool {
	if strings.Contains(target, "(") || strings.Contains(filepath.ToSlash(target), "/SCCS/") {
		return false
	}
	source := e.sccsPath(target)
	sourceInfo, err := e.rc.FS.Stat(source)
	if err != nil {
		return false
	}
	targetInfo, err := e.rc.FS.Stat(e.rc.Path(target))
	if err != nil {
		return true
	}
	// SCCS historically refuses to overwrite a file writable by anyone.
	return targetInfo.Mode().Perm()&0o222 == 0 && sourceInfo.ModTime().After(targetInfo.ModTime())
}

func (e *engine) sccsPath(target string) string {
	local := e.rc.Path(filepath.Join(filepath.Dir(target), "SCCS", "s."+filepath.Base(target)))
	if _, err := e.rc.FS.Stat(local); err == nil {
		return local
	}
	project := e.rc.Getenv("PROJECTDIR")
	if project == "" {
		return local
	}
	base := project
	if !filepath.IsAbs(project) {
		if account, err := user.Lookup(project); err == nil {
			found := false
			for _, child := range []string{"src", "source"} {
				candidate := filepath.Join(account.HomeDir, child)
				if fi, statErr := e.rc.FS.Stat(candidate); statErr == nil && fi.IsDir() {
					base = candidate
					found = true
					break
				}
			}
			if !found {
				base = e.rc.Path(project)
			}
		} else {
			base = e.rc.Path(project)
		}
	}
	return filepath.Join(base, "SCCS", "s."+filepath.Base(target))
}

func (e *engine) infer(target string) (*rule, string) {
	archive, member := archiveParts(target)
	if member != "" && strings.HasSuffix(archive, ".a") && strings.HasSuffix(member, ".o") {
		stem := strings.TrimSuffix(member, ".o")
		for _, from := range e.m.suffixes {
			if rs := e.m.rules[from+".a"]; len(rs) > 0 {
				source := stem + from
				if _, err := e.rc.FS.Stat(e.rc.Path(source)); err == nil {
					r := *rs[len(rs)-1]
					r.targets = []string{target}
					r.deps = []string{source}
					return &r, stem
				}
			}
		}
		return nil, stem
	}
	for _, to := range e.m.suffixes {
		if !strings.HasSuffix(target, to) {
			continue
		}
		stem := strings.TrimSuffix(target, to)
		for _, from := range e.m.suffixes {
			if rs := e.m.rules[from+to]; len(rs) > 0 {
				source := stem + from
				if _, err := e.rc.FS.Stat(e.rc.Path(source)); err == nil {
					r := *rs[len(rs)-1]
					r.targets = []string{target}
					r.deps = []string{source}
					return &r, stem
				}
			}
		}
		return nil, stem
	}
	if filepath.Ext(target) == "" {
		for _, from := range e.m.suffixes {
			if rs := e.m.rules[from]; len(rs) > 0 {
				source := target + from
				if _, err := e.rc.FS.Stat(e.rc.Path(source)); err == nil {
					r := *rs[len(rs)-1]
					r.targets = []string{target}
					r.deps = []string{source}
					return &r, target
				}
			}
		}
	}
	return nil, ""
}
func explicitStem(target string, suffixes []string) string {
	for _, suffix := range suffixes {
		if strings.HasSuffix(target, suffix) {
			return strings.TrimSuffix(target, suffix)
		}
	}
	return target
}

func (e *engine) recipe(target, line string, specialMode bool) error {
	silent := e.o.silent || e.m.silent[""] || e.m.silent[target]
	ignore := e.o.ignore || e.m.ignore[""] || e.m.ignore[target]
	force := false
	for len(line) > 0 {
		switch line[0] {
		case '@':
			silent = true
		case '-':
			ignore = true
		case '+':
			force = true
		default:
			goto prefixesDone
		}
		line = line[1:]
	}
prefixesDone:
	if !silent || e.o.dry {
		fmt.Fprintln(e.rc.Out, line)
	}
	if (e.o.dry || specialMode) && !force {
		e.action = true
		return nil
	}
	shell := e.m.expand(e.m.vars["SHELL"].value, nil, map[string]bool{})
	if shell == "" {
		shell = "/bin/sh"
	}
	path := e.child.ResolveCommand(shell)
	if path == "" {
		path = shell
	}
	args := []string{"-c", line}
	if !ignore {
		args = []string{"-e", "-c", line}
	}
	process, err := e.child.StartCommand(path, args, e.rc.In, e.rc.Out, e.rc.Err)
	if err == nil {
		err = process.Wait()
	}
	e.action = true
	if err != nil && !ignore {
		return fmt.Errorf("target %s: %w", target, err)
	}
	return nil
}
func hasPlusPrefix(line string) bool {
	for len(line) > 0 {
		switch line[0] {
		case '@', '-':
			line = line[1:]
		case '+':
			return true
		default:
			return false
		}
	}
	return false
}
func (e *engine) touch(target string) error {
	archive, member := archiveParts(target)
	if member != "" {
		return e.touchArchiveMember(archive, member)
	}
	path := e.rc.Path(target)
	now := time.Now()
	if _, err := e.rc.FS.Stat(path); err == nil {
		return e.rc.FS.Chtimes(path, now, now)
	}
	f, err := e.rc.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		return err
	}
	return f.Close()
}

func (e *engine) touchArchiveMember(archive, wanted string) error {
	f, err := e.rc.OpenFile(e.rc.Path(archive), os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	magic := make([]byte, 8)
	if _, err = io.ReadFull(f, magic); err != nil || string(magic) != "!<arch>\n" {
		return fmt.Errorf("%s is not an archive", archive)
	}
	for {
		headerAt, seekErr := f.Seek(0, io.SeekCurrent)
		if seekErr != nil {
			return seekErr
		}
		header := make([]byte, 60)
		_, err = io.ReadFull(f, header)
		if errors.Is(err, io.EOF) {
			return os.ErrNotExist
		}
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(strings.TrimSpace(string(header[0:16])), "/")
		size, parseErr := strconv.ParseInt(strings.TrimSpace(string(header[48:58])), 10, 64)
		if parseErr != nil {
			return parseErr
		}
		if name == wanted {
			if _, err = f.Seek(headerAt+16, io.SeekStart); err != nil {
				return err
			}
			_, err = io.WriteString(f, fmt.Sprintf("%-12d", time.Now().Unix()))
			return err
		}
		if _, err = f.Seek(size+(size&1), io.SeekCurrent); err != nil {
			return err
		}
	}
}
func (e *engine) cleanupCancelled(target string) {
	if e.rc.Ctx == nil || e.rc.Ctx.Err() == nil || e.o.dry || e.o.print || e.o.question || e.m.preciousAll || e.m.precious[target] {
		return
	}
	outer, _ := archiveParts(target)
	path := e.rc.Path(outer)
	if fi, err := e.rc.FS.Stat(path); err == nil && !fi.IsDir() {
		if err := e.rc.FS.Remove(path); err == nil {
			fmt.Fprintf(e.rc.Err, "make: removed %s\n", target)
		}
	}
}
func contains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
func unique(items []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	return out
}
func archiveParts(target string) (string, string) {
	open := strings.IndexByte(target, '(')
	if open > 0 && strings.HasSuffix(target, ")") {
		return target[:open], target[open+1 : len(target)-1]
	}
	return target, ""
}
func (e *engine) targetInfo(target string) (os.FileInfo, error) {
	archive, member := archiveParts(target)
	if member == "" {
		return e.rc.FS.Stat(e.rc.Path(target))
	}
	return archiveMemberInfo(e.rc.FS, e.rc.Path(archive), member)
}

type memberInfo struct {
	name string
	size int64
	mode os.FileMode
	mod  time.Time
}

func (m memberInfo) Name() string       { return m.name }
func (m memberInfo) Size() int64        { return m.size }
func (m memberInfo) Mode() os.FileMode  { return m.mode }
func (m memberInfo) ModTime() time.Time { return m.mod }
func (m memberInfo) IsDir() bool        { return false }
func (m memberInfo) Sys() any           { return nil }
func archiveMemberInfo(fs *tool.LocalFS, archive, wanted string) (os.FileInfo, error) {
	f, err := fs.Open(archive)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	magic := make([]byte, 8)
	if _, err = io.ReadFull(f, magic); err != nil || string(magic) != "!<arch>\n" {
		return nil, fmt.Errorf("%s is not an archive", archive)
	}
	for {
		header := make([]byte, 60)
		_, err = io.ReadFull(f, header)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if string(header[58:60]) != "`\n" {
			return nil, fmt.Errorf("invalid archive header")
		}
		name := strings.TrimSuffix(strings.TrimSpace(string(header[0:16])), "/")
		stamp, _ := strconv.ParseInt(strings.TrimSpace(string(header[16:28])), 10, 64)
		size, sizeErr := strconv.ParseInt(strings.TrimSpace(string(header[48:58])), 10, 64)
		if sizeErr != nil {
			return nil, sizeErr
		}
		if name == wanted {
			return memberInfo{name: wanted, size: size, mode: 0o644, mod: time.Unix(stamp, 0)}, nil
		}
		if _, err = f.Seek(size+(size&1), io.SeekCurrent); err != nil {
			return nil, err
		}
	}
	return nil, os.ErrNotExist
}
func (m *makefile) printDB(w io.Writer) {
	names := make([]string, 0, len(m.vars))
	for name := range m.vars {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(w, "%s = %s\n", name, m.vars[name].value)
	}
	fmt.Fprintf(w, ".SUFFIXES: %s\n", strings.Join(m.suffixes, " "))
	if m.posixSeen {
		fmt.Fprintln(w, ".POSIX:")
	}
	printSpecial := func(name string, values map[string]bool, all bool) {
		if all {
			fmt.Fprintf(w, "%s:\n", name)
			return
		}
		items := make([]string, 0, len(values))
		for item := range values {
			if item != "" {
				items = append(items, item)
			}
		}
		sort.Strings(items)
		if len(items) > 0 {
			fmt.Fprintf(w, "%s: %s\n", name, strings.Join(items, " "))
		}
	}
	printSpecial(".IGNORE", m.ignore, m.ignore[""])
	printSpecial(".SILENT", m.silent, m.silent[""])
	printSpecial(".PRECIOUS", m.precious, m.preciousAll)
	printRecipes := func(target string, recipes []string) {
		fmt.Fprintf(w, "%s:\n", target)
		for _, line := range recipes {
			fmt.Fprintf(w, "\t%s\n", line)
		}
	}
	if len(m.defaultRecipe) > 0 {
		printRecipes(".DEFAULT", m.defaultRecipe)
	}
	if len(m.sccsRecipe) > 0 {
		printRecipes(".SCCS_GET", m.sccsRecipe)
	}
	targets := make([]string, 0, len(m.rules))
	for target := range m.rules {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	for _, target := range targets {
		for _, r := range m.rules[target] {
			fmt.Fprintf(w, "%s: %s\n", target, strings.Join(r.deps, " "))
			for _, line := range r.recipes {
				fmt.Fprintf(w, "\t%s\n", line)
			}
		}
	}
}
