package mailxcmd

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	stdmail "net/mail"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	mailxpkg "github.com/qiangli/coreutils/pkg/mailx"
	"github.com/qiangli/coreutils/tool"
)

type messageState uint8

const (
	stateNew messageState = iota
	stateUnread
	stateRead
	stateDeleted
	statePreserved
	stateSaved
	stateMboxed
)

type mailSession struct {
	rc            *tool.RunContext
	invoked       *tool.Tool
	opts          options
	reader        *bufio.Reader
	input         <-chan composeRead
	inputOwned    bool
	user          string
	path          string
	previous      string
	system        bool
	entries       []mailxpkg.MboxEntry
	original      []mailxpkg.MboxEntry
	states        []messageState
	readFlags     []bool
	shown         []bool
	current       int
	headerPage    int
	vars          map[string]string
	aliases       map[string][]string
	ignore        map[string]bool
	retain        map[string]bool
	alts          map[string]bool
	cwd           string
	lastCmd       string
	lastBang      string
	active        bool
	cond          []bool
	receive       bool
	interrupt     <-chan os.Signal
	stopInterrupt func()
	quitRequested bool
}

func runReceiveSession(rc *tool.RunContext, invoked *tool.Tool, o options) int {
	s := &mailSession{
		rc: rc, invoked: invoked, opts: o, reader: bufio.NewReader(rc.In),
		user: identity(rc), vars: defaultVariables(rc), aliases: map[string][]string{},
		ignore: map[string]bool{}, retain: map[string]bool{}, alts: map[string]bool{},
		cwd: rc.Dir, active: true, receive: true,
	}
	if o.ignoreInterrupt {
		s.vars["ignore"] = ""
	}
	if o.user != "" {
		s.user = o.user
	}
	if err := s.readStartup(); err != nil {
		diagnostic(rc, invoked, "%v", err)
		return 1
	}
	if s.quitRequested {
		return 0
	}
	path := o.file
	if path == "" {
		var err error
		path, err = spoolPath(rc, s.user)
		if err != nil {
			diagnostic(rc, invoked, "%v", err)
			return 1
		}
		s.system = true
	} else {
		path = s.resolve(path)
	}
	if err := s.openMailbox(path, s.system); err != nil {
		diagnostic(rc, invoked, "%v", err)
		return 1
	}
	if o.existOnly {
		if len(s.entries) == 0 {
			return 1
		}
		return 0
	}
	if len(s.entries) == 0 {
		if !o.headersOnly {
			fmt.Fprintf(rc.Out, "No mail for %s\n", s.user)
		}
		return 0
	}
	if o.headersOnly {
		s.headers(nil)
		return 0
	}
	if !o.noInitialHeaders && s.boolVar("header", true) {
		s.headers(nil)
	}
	return s.loop()
}

func defaultVariables(rc *tool.RunContext) map[string]string {
	home := rc.Getenv("HOME")
	v := map[string]string{
		"asksub": "", "header": "", "save": "", "toplines": "5", "screen": "20",
		"indentprefix": "\t", "escape": "~", "SHELL": "sh", "LISTER": "ls", "PAGER": "more",
		"prompt": "? ",
		"DEAD":   filepath.Join(home, "dead.letter"), "MBOX": filepath.Join(home, "mbox"),
		"MAILRC": filepath.Join(home, ".mailrc"),
	}
	for _, n := range []string{"DEAD", "EDITOR", "LISTER", "MAILRC", "MBOX", "PAGER", "SHELL", "VISUAL"} {
		if x := rc.Getenv(n); x != "" {
			v[n] = x
		}
	}
	return v
}

func (s *mailSession) boolVar(name string, def bool) bool {
	_, yes := s.vars[name]
	_, no := s.vars["no"+name]
	if no {
		return false
	}
	if yes {
		return true
	}
	return def
}

func (s *mailSession) resolve(name string) string {
	if strings.HasPrefix(name, "+") && s.vars["folder"] != "" {
		folder := s.vars["folder"]
		if !filepath.IsAbs(folder) {
			folder = filepath.Join(s.rc.Getenv("HOME"), folder)
		}
		name = filepath.Join(folder, strings.TrimPrefix(name, "+"))
	}
	name = os.Expand(name, func(k string) string { return s.rc.Getenv(k) })
	if name == "~" {
		name = s.rc.Getenv("HOME")
	} else if strings.HasPrefix(name, "~/") {
		name = filepath.Join(s.rc.Getenv("HOME"), strings.TrimPrefix(name, "~/"))
	}
	if filepath.IsAbs(name) {
		return s.rc.Path(name)
	}
	base := s.cwd
	if base == "" {
		base = s.rc.Dir
	}
	p := s.rc.Path(filepath.Join(base, name))
	if matches, _ := filepath.Glob(p); len(matches) > 0 {
		return matches[0]
	}
	return p
}

func (s *mailSession) readStartup() error {
	if !s.opts.noSystemRC {
		name := s.rc.Getenv("MAILX_SYSTEM_RC")
		if name == "" {
			name = "/etc/mail.rc"
		}
		if err := s.sourceFile(name, true, true); err != nil {
			return err
		}
	}
	return s.sourceFile(s.vars["MAILRC"], true, true)
}

func (s *mailSession) sourceFile(name string, missingOK, startup bool) error {
	if name == "" {
		return nil
	}
	f, err := s.rc.FS.Open(s.resolve(name))
	if err != nil {
		if missingOK && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("source %s: %w", name, err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	oldActive, oldCond := s.active, append([]bool(nil), s.cond...)
	s.active, s.cond = true, nil
	defer func() { s.active, s.cond = oldActive, oldCond }()
	var logical string
	for sc.Scan() {
		line := sc.Text()
		if prefix, yes := mailContinuation(line); yes {
			logical += prefix
			continue
		}
		line = strings.TrimSpace(logical + line)
		logical = ""
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		quit, err := s.execute(line, startup)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if quit {
			s.quitRequested = true
			return nil
		}
		if s.quitRequested {
			return nil
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if len(s.cond) != 0 {
		return fmt.Errorf("unterminated if")
	}
	return nil
}

func (s *mailSession) openMailbox(path string, system bool) error {
	entries, err := mailxpkg.ReadMbox(path)
	if err != nil {
		return err
	}
	s.path, s.system, s.entries = path, system, cloneMboxEntries(entries)
	s.original = entries
	s.states = make([]messageState, len(entries))
	s.readFlags = make([]bool, len(entries))
	s.shown = make([]bool, len(entries))
	for i, e := range entries {
		if !system {
			s.states[i] = stateRead
			continue
		}
		status := strings.Join(e.Message.HeaderValues("Status"), "")
		switch {
		case strings.Contains(status, "R"):
			s.states[i] = stateRead
			s.readFlags[i] = true
		case strings.Contains(status, "O"):
			s.states[i] = stateUnread
		default:
			s.states[i] = stateNew
		}
	}
	s.current = -1
	for _, want := range []messageState{stateNew, stateUnread, stateRead} {
		for i, st := range s.states {
			if st == want {
				s.current = i
				break
			}
		}
		if s.current >= 0 {
			break
		}
	}
	screen, _ := strconv.Atoi(s.vars["screen"])
	if screen <= 0 {
		screen = 20
	}
	if s.current >= 0 {
		s.headerPage = (s.current / screen) * screen
	}
	return nil
}

func (s *mailSession) loop() int {
	s.ensureInterrupt()
	defer s.stopInterrupt()
	for {
		s.ensureInput()
		if prompt, yes := s.vars["prompt"]; yes && prompt != "" && !s.boolVar("noprompt", false) {
			fmt.Fprint(s.rc.Out, prompt)
		}
		var r composeRead
		var ok bool
		select {
		case r, ok = <-s.input:
			if s.inputOwned {
				s.input, s.inputOwned = nil, false
			}
		case <-s.interrupt:
			fmt.Fprintln(s.rc.Out)
			continue
		}
		if !ok {
			return s.finish(true)
		}
		line, err := r.line, r.err
		if err != nil && len(line) == 0 {
			if errors.Is(err, io.EOF) {
				return s.finish(true)
			}
			diagnostic(s.rc, s.invoked, "read commands: %v", err)
			return 1
		}
		for {
			if prefix, yes := mailContinuation(strings.TrimSuffix(line, "\n")); yes {
				s.ensureInput()
				next, more := <-s.input
				if s.inputOwned {
					s.input, s.inputOwned = nil, false
				}
				if !more {
					line = prefix
					break
				}
				line = prefix + next.line
				err = next.err
				continue
			}
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			line = "next"
		}
		quit, xerr := s.execute(line, false)
		if xerr != nil {
			diagnostic(s.rc, s.invoked, "%v", xerr)
		}
		if quit {
			if s.lastCmd == "exit" {
				return 0
			}
			return s.finish(true)
		}
		if s.quitRequested {
			if s.lastCmd == "exit" {
				return 0
			}
			return s.finish(true)
		}
		if err != nil {
			return s.finish(true)
		}
	}
}

func (s *mailSession) execute(line string, startup bool) (bool, error) {
	fields, splitErr := splitMailWords(line)
	if splitErr != nil {
		return false, splitErr
	}
	if len(fields) == 0 {
		return false, nil
	}
	word := fields[0].text
	args := make([]string, len(fields)-1)
	for i := range args {
		args[i] = fields[i+1].text
	}
	if word == "z+" || word == "z-" {
		args = []string{word[1:]}
		word = "z"
	}
	if strings.HasPrefix(word, "#") {
		return false, nil
	}
	cmd, err := mailCommand(word)
	if err != nil {
		return false, err
	}
	literalPlus := func(i int) {
		if i >= 0 && i < len(args) && fields[i+1].protectedLeading && strings.HasPrefix(args[i], "+") {
			args[i] = s.rc.Path(filepath.Join(s.cwd, args[i]))
		}
	}
	switch cmd {
	case "cd", "file", "source":
		literalPlus(0)
	case "save", "copy", "write":
		literalPlus(len(args) - 1)
	}
	if cmd == "if" {
		if len(args) != 1 || (args[0] != "s" && args[0] != "r") {
			return false, fmt.Errorf("if requires s or r")
		}
		parent := s.active
		s.cond = append(s.cond, parent)
		s.active = parent && ((args[0] == "r") == s.receive)
		return false, nil
	}
	if cmd == "else" {
		if len(s.cond) == 0 {
			return false, fmt.Errorf("else without if")
		}
		parent := s.cond[len(s.cond)-1]
		s.active = parent && !s.active
		return false, nil
	}
	if cmd == "endif" {
		if len(s.cond) == 0 {
			return false, fmt.Errorf("endif without if")
		}
		s.active = s.cond[len(s.cond)-1]
		s.cond = s.cond[:len(s.cond)-1]
		return false, nil
	}
	if !s.active {
		return false, nil
	}
	if startup && map[string]bool{"bang": true, "hold": true, "mail": true, "preserve": true, "reply": true, "Reply": true, "shell": true, "visual": true, "edit": true, "copy-author": true, "followup": true, "Followup": true}[cmd] {
		return false, fmt.Errorf("%s is invalid in a startup file", word)
	}
	s.lastCmd = cmd
	switch cmd {
	case "alias":
		return false, s.cmdAlias(args)
	case "unalias":
		for _, a := range args {
			delete(s.aliases, a)
		}
		return false, nil
	case "alternates":
		if len(args) == 0 {
			s.printNameSet(s.alts)
			return false, nil
		}
		for _, a := range args {
			s.alts[strings.ToLower(a)] = true
		}
		return false, nil
	case "set":
		return false, s.cmdSet(args)
	case "unset":
		if len(args) == 0 {
			return false, fmt.Errorf("unset requires at least one name")
		}
		for _, a := range args {
			delete(s.vars, a)
			delete(s.vars, "no"+a)
		}
		return false, nil
	case "discard":
		if len(args) == 0 {
			s.printNameSet(s.ignore)
			return false, nil
		}
		for _, a := range args {
			s.ignore[strings.ToLower(a)] = true
			delete(s.retain, strings.ToLower(a))
		}
		return false, nil
	case "retain":
		if len(args) == 0 {
			s.printNameSet(s.retain)
			return false, nil
		}
		for _, a := range args {
			s.retain[strings.ToLower(a)] = true
			delete(s.ignore, strings.ToLower(a))
		}
		return false, nil
	case "source":
		if len(args) != 1 {
			return false, fmt.Errorf("source requires a file")
		}
		return false, s.sourceFile(args[0], false, startup)
	case "cd":
		return false, s.cmdCD(args)
	case "echo":
		fmt.Fprintln(s.rc.Out, strings.Join(args, " "))
		return false, nil
	case "list":
		if len(args) != 0 {
			return false, fmt.Errorf("list accepts no arguments")
		}
		s.listCommands()
		return false, nil
	case "headers":
		if len(args) > 0 {
			nums, e := s.selectMessages(args, false)
			if e != nil {
				return false, e
			}
			screen, _ := strconv.Atoi(s.vars["screen"])
			if screen <= 0 {
				screen = 20
			}
			s.headerPage = (nums[0] / screen) * screen
			for i := s.headerPage; i < len(s.entries) && i < s.headerPage+screen; i++ {
				if s.states[i] != stateDeleted {
					s.current = i
					break
				}
			}
		}
		s.headers(nil)
		return false, nil
	case "from":
		nums, e := s.selectMessages(args, false)
		if e == nil {
			s.headers(nums)
			s.current = nums[len(nums)-1]
		}
		return false, e
	case "print", "Print", "top":
		return false, s.show(cmd, args)
	case "next":
		return false, s.next(args)
	case "delete", "dp":
		return false, s.mark(args, stateDeleted, cmd == "dp")
	case "undelete":
		return false, s.undelete(args)
	case "hold", "preserve":
		return false, s.mark(args, statePreserved, false)
	case "mbox":
		return false, s.mark(args, stateMboxed, false)
	case "touch":
		return false, s.mark(args, stateRead, false)
	case "save", "copy", "write", "save-author", "copy-author":
		return false, s.save(cmd, args)
	case "size":
		return false, s.size(args)
	case "file":
		return false, s.changeFile(args)
	case "folders":
		if len(args) != 0 {
			return false, fmt.Errorf("folders accepts no arguments")
		}
		return false, s.folders()
	case "mail":
		return false, s.compose(args, "", false)
	case "reply", "Reply", "followup", "Followup":
		return false, s.reply(cmd, args)
	case "pipe":
		return false, s.pipe(args)
	case "shell":
		if len(args) != 0 {
			return false, fmt.Errorf("shell accepts no arguments")
		}
		return false, s.runShell("")
	case "bang":
		return false, s.runShell(strings.TrimSpace(strings.TrimPrefix(line, "!")))
	case "edit", "visual":
		return false, s.editMessages(cmd, args)
	case "help":
		if len(args) != 0 {
			return false, fmt.Errorf("help accepts no arguments")
		}
		s.listCommands()
		return false, nil
	case "number":
		if len(args) != 0 {
			return false, fmt.Errorf("= accepts no arguments")
		}
		if s.current >= 0 {
			fmt.Fprintln(s.rc.Out, s.current+1)
		}
		return false, nil
	case "scroll":
		if len(args) > 1 || (len(args) == 1 && args[0] != "+" && args[0] != "-") {
			return false, fmt.Errorf("z accepts only + or -")
		}
		s.scrollHeaders(args)
		return false, nil
	case "exit":
		if len(args) != 0 {
			return false, fmt.Errorf("exit accepts no arguments")
		}
		return true, nil
	case "quit":
		if len(args) != 0 {
			return false, fmt.Errorf("quit accepts no arguments")
		}
		return true, nil
	}
	return false, fmt.Errorf("unsupported command %s", word)
}

func mailCommand(w string) (string, error) {
	if strings.HasPrefix(w, "!") {
		return "bang", nil
	}
	if c, ok := map[string]string{"=": "number", "#": "comment", "?": "help", "+": "next", "|": "pipe", "dp": "dp", "dt": "dp", "cd": "cd", "folders": "folders", "z": "scroll"}[w]; ok {
		return c, nil
	}
	type commandName struct{ min, full, canon string }
	names := []commandName{
		{"a", "alias", "alias"}, {"g", "group", "alias"}, {"alt", "alternates", "alternates"},
		{"ch", "chdir", "cd"}, {"c", "copy", "copy"}, {"C", "Copy", "copy-author"},
		{"d", "delete", "delete"}, {"di", "discard", "discard"}, {"ig", "ignore", "discard"},
		{"ec", "echo", "echo"}, {"e", "edit", "edit"}, {"ex", "exit", "exit"}, {"x", "xit", "exit"},
		{"fi", "file", "file"}, {"fold", "folder", "file"}, {"fo", "followup", "followup"}, {"F", "Followup", "Followup"},
		{"f", "from", "from"}, {"h", "headers", "headers"}, {"hel", "help", "help"},
		{"ho", "hold", "hold"}, {"pre", "preserve", "hold"}, {"i", "if", "if"}, {"el", "else", "else"}, {"en", "endif", "endif"},
		{"l", "list", "list"}, {"m", "mail", "mail"}, {"mb", "mbox", "mbox"}, {"n", "next", "next"},
		{"pi", "pipe", "pipe"}, {"P", "Print", "Print"}, {"T", "Type", "Print"}, {"p", "print", "print"}, {"t", "type", "print"},
		{"q", "quit", "quit"}, {"R", "Reply", "Reply"}, {"R", "Respond", "Reply"}, {"r", "reply", "reply"}, {"r", "respond", "reply"},
		{"ret", "retain", "retain"}, {"s", "save", "save"}, {"S", "Save", "save-author"}, {"se", "set", "set"},
		{"sh", "shell", "shell"}, {"si", "size", "size"}, {"so", "source", "source"}, {"to", "top", "top"}, {"tou", "touch", "touch"},
		{"una", "unalias", "unalias"}, {"u", "undelete", "undelete"}, {"uns", "unset", "unset"}, {"v", "visual", "visual"}, {"w", "write", "write"},
	}
	var found string
	for _, n := range names {
		if len(w) >= len(n.min) && len(w) <= len(n.full) && strings.HasPrefix(n.full, w) {
			if found != "" && found != n.canon {
				return "", fmt.Errorf("ambiguous command %s", w)
			}
			found = n.canon
		}
	}
	if found == "" {
		return "", fmt.Errorf("unknown command: %s", w)
	}
	return found, nil
}

func (s *mailSession) cmdAlias(a []string) error {
	if len(a) == 0 {
		keys := make([]string, 0, len(s.aliases))
		for k := range s.aliases {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(s.rc.Out, "%s\t%s\n", k, strings.Join(s.aliases[k], " "))
		}
		return nil
	}
	if len(a) == 1 {
		fmt.Fprintln(s.rc.Out, strings.Join(s.aliases[a[0]], " "))
		return nil
	}
	s.aliases[a[0]] = append(s.aliases[a[0]], a[1:]...)
	return nil
}
func (s *mailSession) cmdSet(a []string) error {
	if len(a) == 0 {
		keys := make([]string, 0, len(s.vars))
		for k := range s.vars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(s.rc.Out, "%s=%s\n", k, s.vars[k])
		}
		return nil
	}
	for _, x := range a {
		n, v, ok := strings.Cut(x, "=")
		if !ok {
			v = ""
		}
		if n == "ask" {
			n = "asksub"
		} else if n == "noask" {
			n = "noasksub"
		}
		if strings.HasPrefix(n, "no") && v == "" {
			delete(s.vars, strings.TrimPrefix(n, "no"))
			s.vars[n] = ""
		} else {
			delete(s.vars, "no"+n)
			s.vars[n] = v
		}
	}
	return nil
}
func (s *mailSession) cmdCD(a []string) error {
	if len(a) > 1 {
		return fmt.Errorf("cd accepts one directory")
	}
	n := s.rc.Getenv("HOME")
	if len(a) == 1 {
		n = a[0]
	}
	p := s.resolve(n)
	fi, e := s.rc.FS.Stat(p)
	if e != nil {
		return e
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", n)
	}
	s.cwd = p
	return nil
}

func (s *mailSession) headers(nums []int) {
	if nums == nil {
		start := s.headerPage
		screen, _ := strconv.Atoi(s.vars["screen"])
		if screen <= 0 {
			screen = 20
		}
		for i := start; i < len(s.entries) && i < start+screen; i++ {
			if s.states[i] != stateDeleted {
				nums = append(nums, i)
			}
		}
	}
	for _, i := range nums {
		e := s.entries[i]
		mark := " "
		if i == s.current {
			mark = ">"
		}
		st := map[messageState]string{stateNew: "N", stateUnread: "U", stateRead: "R", stateDeleted: "D", statePreserved: "P", stateSaved: "S", stateMboxed: "M"}[s.states[i]]
		party := firstHeader(e.Message, "From", "unknown")
		if s.boolVar("showto", false) && strings.EqualFold(party, s.user) {
			party = firstHeader(e.Message, "To", party)
		}
		fmt.Fprintf(s.rc.Out, "%s%s%3d %-16.16s %4d/%-6d %s\n", mark, st, i+1, party, bytes.Count(e.Message.Body, []byte{'\n'}), len(e.Message.Body), firstHeader(e.Message, "Subject", ""))
	}
}

func (s *mailSession) scrollHeaders(args []string) {
	screen, _ := strconv.Atoi(s.vars["screen"])
	if screen <= 0 {
		screen = 20
	}
	if len(args) > 0 && args[0] == "-" {
		s.headerPage -= screen
	} else {
		s.headerPage += screen
	}
	if s.headerPage < 0 {
		s.headerPage = 0
	}
	if s.headerPage >= len(s.entries) && len(s.entries) > 0 {
		s.headerPage = ((len(s.entries) - 1) / screen) * screen
	}
	s.headers(nil)
}

func (s *mailSession) selectMessages(args []string, deletedOnly bool) ([]int, error) {
	if len(args) == 0 {
		if s.current < 0 {
			return nil, fmt.Errorf("no applicable messages")
		}
		return []int{s.current}, nil
	}
	seen := map[int]bool{}
	var out []int
	add := func(i int) {
		if i >= 0 && i < len(s.entries) && (!deletedOnly || s.states[i] == stateDeleted) && (deletedOnly || s.states[i] != stateDeleted) && !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
	}
	for _, a := range args {
		switch {
		case a == "*":
			for i := range s.entries {
				add(i)
			}
		case a == ".":
			add(s.current)
		case a == "^":
			for i := range s.entries {
				before := len(out)
				add(i)
				if len(out) > before {
					break
				}
			}
		case a == "$":
			for i := len(s.entries) - 1; i >= 0; i-- {
				before := len(out)
				add(i)
				if len(out) > before {
					break
				}
			}
		case a == "+":
			for i := s.current + 1; i < len(s.entries); i++ {
				before := len(out)
				add(i)
				if len(out) > before {
					break
				}
			}
		case a == "-":
			for i := s.current - 1; i >= 0; i-- {
				before := len(out)
				add(i)
				if len(out) > before {
					break
				}
			}
		case strings.HasPrefix(a, "/"):
			q := strings.ToLower(a[1:])
			for i, e := range s.entries {
				if strings.Contains(strings.ToLower(firstHeader(e.Message, "Subject", "")), q) {
					add(i)
				}
			}
		case strings.HasPrefix(a, ":") && len(a) == 2:
			for i, st := range s.states {
				ok := map[byte]bool{'d': st == stateDeleted, 'n': st == stateNew, 'o': st != stateRead && st != stateNew, 'r': st == stateRead, 'u': st == stateUnread}[a[1]]
				if ok {
					if deletedOnly {
						if st == stateDeleted {
							if !seen[i] {
								seen[i] = true
								out = append(out, i)
							}
						}
					} else {
						add(i)
					}
				}
			}
		case strings.Contains(a, "-"):
			lo, hi, ok := strings.Cut(a, "-")
			x, e1 := strconv.Atoi(lo)
			y, e2 := strconv.Atoi(hi)
			if !ok || e1 != nil || e2 != nil {
				return nil, fmt.Errorf("invalid message range %s", a)
			}
			for i := x; i <= y; i++ {
				add(i - 1)
			}
		default:
			if n, e := strconv.Atoi(a); e == nil {
				add(n - 1)
			} else {
				for i, e := range s.entries {
					if s.senderMatches(firstHeader(e.Message, "From", ""), a) {
						add(i)
					}
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no applicable messages")
	}
	sort.Ints(out)
	return out, nil
}

func (s *mailSession) senderMatches(from, query string) bool {
	from, query = strings.TrimSpace(from), strings.TrimSpace(query)
	if strings.EqualFold(from, query) {
		return true
	}
	address := from
	if parsed, err := stdmail.ParseAddress(from); err == nil {
		address = parsed.Address
	}
	if strings.EqualFold(address, query) {
		return true
	}
	if !s.boolVar("allnet", false) {
		return false
	}
	return strings.EqualFold(mailLogin(address), mailLogin(query))
}

func mailLogin(address string) string {
	if i := strings.LastIndex(address, "!"); i >= 0 {
		address = address[i+1:]
	}
	if i := strings.IndexAny(address, "@%"); i >= 0 {
		address = address[:i]
	}
	return address
}

func (s *mailSession) show(cmd string, args []string) error {
	nums, e := s.selectMessages(args, false)
	if e != nil {
		return e
	}
	for _, i := range nums {
		switch cmd {
		case "top":
			n, _ := strconv.Atoi(s.vars["toplines"])
			if n <= 0 {
				n = 5
			}
			data := strings.Split(string(s.entries[i].Message.Bytes()), "\n")
			if len(data) > n {
				data = data[:n]
			}
			fmt.Fprintln(s.rc.Out, strings.Join(data, "\n"))
		case "Print":
			var out bytes.Buffer
			printMessage(&out, s.entries[i])
			s.writePaged(out.Bytes())
		default:
			s.printFiltered(s.entries[i])
		}
		s.markMessageRead(i)
		s.shown[i] = true
		s.current = i
	}
	return nil
}
func (s *mailSession) printFiltered(e mailxpkg.MboxEntry) {
	var out bytes.Buffer
	fmt.Fprintln(&out, e.Envelope)
	for _, h := range e.Message.Headers {
		n := strings.ToLower(h.Name)
		show := !s.ignore[n]
		if len(s.retain) > 0 {
			show = s.retain[n]
		}
		if show {
			fmt.Fprintf(&out, "%s: %s\n", h.Name, h.Value)
		}
	}
	fmt.Fprintln(&out)
	out.Write(e.Message.Body)
	if len(e.Message.Body) > 0 && e.Message.Body[len(e.Message.Body)-1] != '\n' {
		fmt.Fprintln(&out)
	}
	s.writePaged(out.Bytes())
}
func (s *mailSession) writePaged(data []byte) {
	crt, _ := strconv.Atoi(s.vars["crt"])
	if crt > 0 && mailxIsTerminal(s.rc.Out) && bytes.Count(data, []byte{'\n'}) > crt {
		paged, err := s.filter(s.vars["PAGER"], data)
		if err == nil {
			s.rc.Out.Write(paged)
			return
		}
	}
	s.rc.Out.Write(data)
}
func (s *mailSession) next(args []string) error {
	if len(args) > 0 {
		nums, e := s.selectMessages(args, false)
		if e != nil {
			return e
		}
		s.current = nums[0]
	} else if s.current >= 0 && !s.shown[s.current] {
	} else {
		found := -1
		for i := s.current + 1; i < len(s.entries); i++ {
			if s.states[i] != stateDeleted {
				found = i
				break
			}
		}
		if found < 0 {
			fmt.Fprintln(s.rc.Out, "At EOF")
			return nil
		}
		s.current = found
	}
	return s.show("print", []string{strconv.Itoa(s.current + 1)})
}
func (s *mailSession) mark(args []string, st messageState, autoprint bool) error {
	nums, e := s.selectMessages(args, false)
	if e != nil {
		return e
	}
	for _, i := range nums {
		if st == stateRead {
			s.markMessageRead(i)
		} else if s.states[i] != statePreserved || st == stateDeleted {
			s.states[i] = st
		}
		if st == stateMboxed {
			if len(s.readFlags) < len(s.entries) {
				s.readFlags = append(s.readFlags, make([]bool, len(s.entries)-len(s.readFlags))...)
			}
			s.readFlags[i] = true
		}
		if st == statePreserved || st == stateMboxed || st == stateRead {
			s.shown[i] = true
		}
		s.current = i
	}
	after := false
	if st == stateDeleted {
		after = s.chooseAfter(nums[len(nums)-1])
	}
	if autoprint {
		if !after {
			fmt.Fprintln(s.rc.Out, "At EOF")
			return nil
		}
		return s.show("print", []string{strconv.Itoa(s.current + 1)})
	}
	if s.boolVar("autoprint", false) {
		if s.current >= 0 {
			return s.show("print", []string{strconv.Itoa(s.current + 1)})
		}
	}
	return nil
}

func (s *mailSession) markMessageRead(i int) {
	if len(s.readFlags) < len(s.entries) {
		s.readFlags = append(s.readFlags, make([]bool, len(s.entries)-len(s.readFlags))...)
	}
	s.readFlags[i] = true
	if s.states[i] == stateNew || s.states[i] == stateUnread || s.states[i] == stateRead {
		s.states[i] = stateRead
	}
}
func (s *mailSession) chooseAfter(last int) bool {
	s.current = -1
	for i := last + 1; i < len(s.entries); i++ {
		if s.states[i] != stateDeleted {
			s.current = i
			return true
		}
	}
	for i := last - 1; i >= 0; i-- {
		if s.states[i] != stateDeleted {
			s.current = i
			return false
		}
	}
	return false
}
func (s *mailSession) undelete(args []string) error {
	var nums []int
	var e error
	if len(args) == 0 {
		for i := s.current + 1; i < len(s.entries); i++ {
			if s.states[i] == stateDeleted {
				nums = []int{i}
				break
			}
		}
		if len(nums) == 0 {
			for i := s.current - 1; i >= 0; i-- {
				if s.states[i] == stateDeleted {
					nums = []int{i}
					break
				}
			}
		}
		if len(nums) == 0 {
			return fmt.Errorf("no applicable messages")
		}
	} else {
		nums, e = s.selectMessages(args, true)
	}
	if e != nil {
		return e
	}
	for _, i := range nums {
		s.states[i] = stateRead
		s.readFlags[i] = true
		s.current = i
	}
	if s.boolVar("autoprint", false) {
		return s.show("print", []string{strconv.Itoa(s.current + 1)})
	}
	return nil
}

func (s *mailSession) save(cmd string, args []string) error {
	if len(args) == 0 {
		if cmd == "save" || cmd == "copy" {
			args = []string{s.vars["MBOX"]}
		} else if cmd != "save-author" && cmd != "copy-author" {
			return fmt.Errorf("filename required")
		}
	}
	var file string
	sel := args
	if cmd == "save-author" || cmd == "copy-author" {
		file = ""
	} else {
		file = args[len(args)-1]
		sel = args[:len(args)-1]
	}
	nums, e := s.selectMessages(sel, false)
	if e != nil {
		return e
	}
	if file == "" {
		file = authorFile(s.entries[nums[0]])
	}
	body := cmd == "write"
	path := s.resolve(file)
	if body {
		if e = saveMessages(path, s.entries, nums, true); e != nil {
			return e
		}
	} else {
		for _, i := range nums {
			sender := firstHeader(s.entries[i].Message, "From", "unknown")
			if e = mailxpkg.AppendMbox(path, sender, nowFn(), s.entries[i].Message); e != nil {
				return e
			}
		}
	}
	if cmd == "save" || cmd == "save-author" || cmd == "write" {
		for _, i := range nums {
			if s.states[i] != statePreserved {
				s.states[i] = stateSaved
			}
		}
	} else if cmd == "copy" || cmd == "copy-author" {
		for _, i := range nums {
			s.markMessageRead(i)
			s.shown[i] = true
		}
	}
	s.current = nums[len(nums)-1]
	return nil
}
func authorFile(e mailxpkg.MboxEntry) string {
	a := firstHeader(e.Message, "From", "unknown")
	if i := strings.IndexAny(a, "@!% "); i >= 0 {
		a = a[:i]
	}
	return a
}
func (s *mailSession) size(args []string) error {
	nums, e := s.selectMessages(args, false)
	if e != nil {
		return e
	}
	for _, i := range nums {
		fmt.Fprintf(s.rc.Out, "%d: %d\n", i+1, len(s.entries[i].Message.Bytes()))
	}
	s.current = nums[len(nums)-1]
	return nil
}

func (s *mailSession) changeFile(args []string) error {
	if len(args) == 0 {
		fmt.Fprintf(s.rc.Out, "%s: %d messages\n", s.path, len(s.entries))
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("file accepts one mailbox")
	}
	name, err := s.mailboxName(args[0])
	if err != nil {
		return err
	}
	if code := s.finish(true); code != 0 {
		return fmt.Errorf("cannot commit mailbox")
	}
	s.previous = s.path
	return s.openMailbox(name, name == mustSpoolPath(s, s.user))
}

func (s *mailSession) mailboxName(name string) (string, error) {
	switch {
	case name == "%":
		return mustSpoolPath(s, s.user), nil
	case strings.HasPrefix(name, "%"):
		user := strings.TrimPrefix(name, "%")
		if !localAddress(user) {
			return "", fmt.Errorf("invalid mailbox user %q", user)
		}
		return mustSpoolPath(s, user), nil
	case name == "#":
		if s.previous == "" {
			return "", fmt.Errorf("no previous mailbox")
		}
		return s.previous, nil
	case name == "&":
		return s.resolve(s.vars["MBOX"]), nil
	default:
		return s.resolve(name), nil
	}
}
func mustSpoolPath(s *mailSession, user string) string { p, _ := spoolPath(s.rc, user); return p }

func (s *mailSession) printNameSet(set map[string]bool) {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintln(s.rc.Out, strings.Join(keys, " "))
}
func shellQuote(x string) string { return "'" + strings.ReplaceAll(x, "'", "'\\''") + "'" }

type mailWord struct {
	text             string
	protectedLeading bool
}

func splitMailWords(line string) ([]mailWord, error) {
	var out []mailWord
	var b strings.Builder
	quote := byte(0)
	escaped := false
	started := false
	protectedLeading := false
	flush := func() {
		if started {
			out = append(out, mailWord{text: b.String(), protectedLeading: protectedLeading})
			b.Reset()
			started = false
			protectedLeading = false
		}
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				if b.Len() == 0 && c == '+' {
					protectedLeading = true
				}
				b.WriteByte(c)
			}
			started = true
			continue
		}
		if escaped {
			if b.Len() == 0 && c == '+' {
				protectedLeading = true
			}
			b.WriteByte(c)
			escaped = false
			started = true
			continue
		}
		switch c {
		case '\\':
			escaped = true
			started = true
		case '\'', '"':
			quote = c
			started = true
		case ' ', '\t':
			flush()
		default:
			b.WriteByte(c)
			started = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unmatched quote")
	}
	if escaped {
		b.WriteByte('\\')
	}
	flush()
	return out, nil
}

func mailContinuation(line string) (string, bool) {
	n := 0
	for i := len(line) - 1; i >= 0 && line[i] == '\\'; i-- {
		n++
	}
	if n%2 == 1 {
		return line[:len(line)-1], true
	}
	return line, false
}
func (s *mailSession) folders() error {
	folder := s.vars["folder"]
	if folder == "" {
		folder = s.cwd
	} else if !filepath.IsAbs(folder) {
		folder = filepath.Join(s.rc.Getenv("HOME"), folder)
	}
	return s.runShell("cd " + shellQuote(s.resolve(folder)) + " && " + s.vars["LISTER"])
}

func (s *mailSession) finish(commit bool) int {
	if !commit {
		return 0
	}
	keep := make([]bool, len(s.entries))
	for i := range keep {
		keep[i] = true
	}
	var move []int
	for i, st := range s.states {
		switch st {
		case stateDeleted:
			keep[i] = false
		case stateMboxed:
			keep[i] = false
			move = append(move, i)
		case stateSaved:
			if s.system {
				keep[i] = false
				if s.boolVar("keepsave", false) {
					move = append(move, i)
				}
			}
		case stateRead:
			if s.system && !s.boolVar("hold", false) {
				keep[i] = false
				move = append(move, i)
			} else if s.system {
				setMessageStatus(s.entries[i].Message, "RO")
			}
		case stateNew, stateUnread:
			if s.system {
				setMessageStatus(s.entries[i].Message, "O")
			}
		case statePreserved:
			if s.system {
				if i < len(s.readFlags) && s.readFlags[i] {
					setMessageStatus(s.entries[i].Message, "RO")
				} else {
					setMessageStatus(s.entries[i].Message, "O")
				}
			}
		}
	}
	if len(move) > 0 {
		mbox := s.resolve(s.vars["MBOX"])
		if filepath.Clean(mbox) == filepath.Clean(s.path) {
			for _, i := range move {
				keep[i] = true
				setMessageStatus(s.entries[i].Message, "RO")
			}
			move = nil
		}
	}
	if len(move) > 0 {
		mbox := s.resolve(s.vars["MBOX"])
		var moved []mailxpkg.MboxEntry
		for _, i := range move {
			moved = append(moved, s.entries[i])
		}
		if e := mailxpkg.MergeMbox(mbox, moved, s.boolVar("append", false)); e != nil {
			diagnostic(s.rc, s.invoked, "save to MBOX: %v", e)
			return 1
		}
	}
	if e := mailxpkg.CommitMboxChangesKeep(s.path, s.original, s.entries, keep, s.boolVar("keep", false)); e != nil {
		diagnostic(s.rc, s.invoked, "%v", e)
		return 1
	}
	return 0
}

func cloneMboxEntries(entries []mailxpkg.MboxEntry) []mailxpkg.MboxEntry {
	out := make([]mailxpkg.MboxEntry, len(entries))
	for i, e := range entries {
		out[i].Envelope = e.Envelope
		if e.Message != nil {
			m, err := mailxpkg.ParseMessage(e.Message.Bytes())
			if err == nil {
				out[i].Message = m
			}
		}
	}
	return out
}

func setMessageStatus(m *mailxpkg.Message, value string) {
	if m == nil {
		return
	}
	var headers []mailxpkg.Header
	for _, h := range m.Headers {
		if strings.EqualFold(h.Name, "Status") {
			continue
		}
		h.Raw = nil
		headers = append(headers, h)
	}
	headers = append(headers, mailxpkg.Header{Name: "Status", Value: value})
	m.Headers, m.RawHeaders, m.Separator = headers, nil, nil
}
