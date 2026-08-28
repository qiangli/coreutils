package mailxcmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"golang.org/x/term"

	mailxpkg "github.com/qiangli/coreutils/pkg/mailx"
)

var mailxIsTerminal = func(r any) bool {
	f, ok := r.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(f.Fd()))
}

var watchMailInterrupt = func() (<-chan os.Signal, func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	return ch, func() { signal.Stop(ch) }
}

type composeRead struct {
	line string
	err  error
}

func (s *mailSession) ensureInput() {
	if s.input != nil {
		return
	}
	ch := make(chan composeRead, 1)
	s.input = ch
	s.inputOwned = true
	go func() {
		defer close(ch)
		line, err := s.reader.ReadString('\n')
		ch <- composeRead{line, err}
	}()
}

func (s *mailSession) ensureInterrupt() bool {
	if s.interrupt != nil {
		return false
	}
	s.interrupt, s.stopInterrupt = watchMailInterrupt()
	return true
}

func (s *mailSession) listCommands() {
	fmt.Fprintln(s.rc.Out, "alias alternates cd copy delete discard dp echo edit exit file folders followup from headers help hold if list mail mbox next pipe print quit reply retain save set shell size source top touch unalias undelete unset visual write z ! =")
}

func (s *mailSession) compose(to []string, subject string, record bool) error {
	return s.composeMessage(to, subject, record, true)
}

func (s *mailSession) composeMessage(to []string, subject string, record, aliases bool) error {
	if aliases {
		to = s.expandAliases(to)
	}
	if len(to) == 0 {
		return fmt.Errorf("no recipients specified")
	}
	body, to, cc, bcc, subject, send, err := s.readComposition(to, nil, nil, subject)
	if err != nil {
		return err
	}
	if !send {
		return nil
	}
	if aliases {
		to, cc, bcc = s.expandAliases(to), s.expandAliases(cc), s.expandAliases(bcc)
	}
	if err := s.deliver(body, to, cc, bcc, subject, record); err != nil {
		if deadErr := s.writeDead(body); deadErr != nil {
			return fmt.Errorf("%v; save dead letter: %w", err, deadErr)
		}
		return err
	}
	return nil
}

func (s *mailSession) expandAliases(in []string) []string {
	var out []string
	var add func(string, map[string]bool)
	add = func(a string, seen map[string]bool) {
		if seen[a] {
			return
		}
		if xs, ok := s.aliases[a]; ok {
			seen[a] = true
			for _, x := range xs {
				if !s.boolVar("metoo", false) && strings.EqualFold(x, s.user) {
					continue
				}
				add(x, seen)
			}
			return
		}
		out = append(out, a)
	}
	for _, a := range in {
		add(a, map[string]bool{})
	}
	return out
}

func (s *mailSession) deliver(body []byte, to, cc, bcc []string, subject string, record bool) error {
	all := append(append(append([]string{}, to...), cc...), bcc...)
	if len(all) == 0 {
		return fmt.Errorf("no recipients specified")
	}
	paths := make([]string, len(all))
	for i, a := range all {
		if !localAddress(a) {
			return fmt.Errorf("remote delivery is not supported: %s", a)
		}
		p, e := spoolPath(s.rc, a)
		if e != nil {
			return e
		}
		paths[i] = p
	}
	sender := s.user
	if sender == "" {
		sender = identity(s.rc)
	}
	if !localAddress(sender) {
		return fmt.Errorf("envelope sender must be a local user: %s", sender)
	}
	when := nowFn().UTC()
	headers := []mailxpkg.Header{{Name: "Date", Value: when.Format("Mon, 02 Jan 2006 15:04:05 -0700")}, {Name: "From", Value: sender}, {Name: "To", Value: strings.Join(to, ", ")}}
	if len(cc) > 0 {
		headers = append(headers, mailxpkg.Header{Name: "Cc", Value: strings.Join(cc, ", ")})
	}
	if subject != "" {
		headers = append(headers, mailxpkg.Header{Name: "Subject", Value: subject})
	}
	msg := &mailxpkg.Message{Headers: headers, Body: body}
	if e := msg.Validate(); e != nil {
		return e
	}
	for i, p := range paths {
		if e := mailxpkg.AppendMbox(p, sender, when, msg); e != nil {
			return fmt.Errorf("deliver to %s: %w", all[i], e)
		}
	}
	if record || s.vars["record"] != "" {
		file := s.vars["record"]
		if record || file == "" {
			file = all[0]
		}
		if s.boolVar("outfolder", false) {
			file = "+" + file
		}
		if e := mailxpkg.AppendMboxWithMode(s.resolve(file), sender, when, msg, 0o666); e != nil {
			return e
		}
	}
	return nil
}

func (s *mailSession) readComposition(to, cc, bcc []string, subject string) ([]byte, []string, []string, []string, string, bool, error) {
	var body bytes.Buffer
	escape, escapeSet := s.vars["escape"]
	if !escapeSet {
		escape = "~"
	}
	createdInterrupt := s.ensureInterrupt()
	if createdInterrupt {
		defer s.stopInterrupt()
	}
	interrupted, discardNext := false, false
	for {
		s.ensureInput()
		var line string
		var err error
		select {
		case r, ok := <-s.input:
			if s.inputOwned {
				s.input, s.inputOwned = nil, false
			}
			if !ok {
				return body.Bytes(), to, cc, bcc, subject, true, nil
			}
			line, err = r.line, r.err
			if discardNext {
				discardNext = false
				continue
			}
			interrupted = false
		case <-s.interrupt:
			if s.boolVar("ignore", false) {
				fmt.Fprintln(s.rc.Out, "@")
				discardNext = true
				continue
			}
			if !interrupted {
				fmt.Fprintln(s.rc.Out, "(Interrupt -- one more to kill letter)")
				interrupted = true
				continue
			}
			if e := s.writeDead(body.Bytes()); e != nil {
				return nil, to, cc, bcc, subject, false, e
			}
			if s.receive {
				return nil, to, cc, bcc, subject, false, nil
			}
			return nil, to, cc, bcc, subject, false, fmt.Errorf("message aborted by interrupt")
		}
		if err != nil && len(line) == 0 {
			if err == io.EOF && s.boolVar("ignoreeof", false) && mailxIsTerminal(s.rc.In) {
				fmt.Fprintln(s.rc.Out, "Use . to terminate letter")
				continue
			}
			return body.Bytes(), to, cc, bcc, subject, true, nil
		}
		trim := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if trim == "." && (s.boolVar("dot", false) || s.boolVar("ignoreeof", false)) {
			return body.Bytes(), to, cc, bcc, subject, true, nil
		}
		if escape != "" && strings.HasPrefix(trim, escape) {
			rest := strings.TrimPrefix(trim, escape)
			if rest == "" {
				body.WriteString(line)
				continue
			}
			c := rest[0]
			arg := strings.TrimSpace(rest[1:])
			switch c {
			case '.':
				return body.Bytes(), to, cc, bcc, subject, true, nil
			case 'x':
				return nil, to, cc, bcc, subject, false, nil
			case 'q':
				if e := s.writeDead(body.Bytes()); e != nil {
					return nil, to, cc, bcc, subject, false, e
				}
				return nil, to, cc, bcc, subject, false, nil
			case '?':
				fmt.Fprintln(s.rc.Out, "~! ~. ~: ~? ~A ~a ~b ~c ~d ~e ~f ~F ~h ~i ~m ~M ~p ~q ~r ~s ~t ~v ~w ~x ~|")
			case 'a', 'A':
				n := "sign"
				if c == 'A' {
					n = "Sign"
				}
				if v := s.vars[n]; v != "" {
					body.WriteString(expandSignature(v))
					body.WriteByte('\n')
				}
			case 'b':
				bcc = append(bcc, strings.Fields(arg)...)
			case 'c':
				cc = append(cc, strings.Fields(arg)...)
			case 't':
				to = append(to, strings.Fields(arg)...)
			case 's':
				subject = arg
			case 'i':
				// ~a and ~A are defined as ~i sign and ~i Sign, and the
				// sign/Sign variables recognize \t and \n, so ~i has to
				// expand them for those two variables as well.
				if v := s.vars[arg]; v != "" {
					if arg == "sign" || arg == "Sign" {
						v = expandSignature(v)
					}
					body.WriteString(v)
					body.WriteByte('\n')
				}
			case 'p':
				var preview bytes.Buffer
				fmt.Fprintf(&preview, "To: %s\nCc: %s\nBcc: %s\nSubject: %s\n\n%s", strings.Join(to, " "), strings.Join(cc, " "), strings.Join(bcc, " "), subject, body.String())
				s.writePaged(preview.Bytes())
			case 'd':
				data, e := os.ReadFile(s.resolve(s.vars["DEAD"]))
				if e != nil {
					diagnostic(s.rc, s.invoked, "%v", e)
					break
				}
				body.Write(data)
			case 'r', '<':
				data, e := s.readInsert(arg)
				if e != nil {
					diagnostic(s.rc, s.invoked, "%v", e)
				} else {
					body.Write(data)
				}
			case 'w':
				if e := appendFile(s.resolve(arg), body.Bytes()); e != nil {
					diagnostic(s.rc, s.invoked, "%v", e)
				}
			case '!':
				if e := s.runShell(arg); e != nil {
					diagnostic(s.rc, s.invoked, "%v", e)
				}
			case '|':
				out, e := s.filter(arg, body.Bytes())
				if e != nil {
					diagnostic(s.rc, s.invoked, "%v", e)
				} else {
					body.Reset()
					body.Write(out)
				}
			case 'e', 'v':
				which := "EDITOR"
				if c == 'v' {
					which = "VISUAL"
				}
				out, e := s.editData(body.Bytes(), which)
				if e != nil {
					diagnostic(s.rc, s.invoked, "%v", e)
				} else {
					body.Reset()
					body.Write(out)
				}
			case 'f', 'F', 'm', 'M':
				nums, e := s.selectMessages(strings.Fields(arg), false)
				if e != nil {
					diagnostic(s.rc, s.invoked, "%v", e)
					break
				}
				allHeaders := c == 'F' || c == 'M'
				indent := c == 'm' || c == 'M'
				for _, n := range nums {
					var d []byte
					if allHeaders {
						d = s.entries[n].Message.Bytes()
					} else {
						var b bytes.Buffer
						old := s.rc.Out
						s.rc.Out = &b
						s.printFiltered(s.entries[n])
						s.rc.Out = old
						d = b.Bytes()
					}
					if indent {
						prefix := s.vars["indentprefix"]
						for _, ln := range strings.SplitAfter(string(d), "\n") {
							if strings.TrimSpace(ln) != "" {
								body.WriteString(prefix)
							}
							body.WriteString(ln)
						}
					} else {
						body.Write(d)
					}
					s.markMessageRead(n)
				}
				s.current = nums[len(nums)-1]
			case ':', '_':
				quit, e := s.execute(arg, false)
				if e != nil {
					diagnostic(s.rc, s.invoked, "%v", e)
				} else if quit {
					s.quitRequested = true
					return nil, to, cc, bcc, subject, false, nil
				}
			case 'h':
				var abort bool
				var promptErr error
				if subject, abort, promptErr = s.promptField("Subject", subject, body.Bytes()); !abort && promptErr == nil {
					var value string
					value, abort, promptErr = s.promptField("To", strings.Join(to, " "), body.Bytes())
					to = strings.Fields(value)
				}
				if !abort && promptErr == nil {
					var value string
					value, abort, promptErr = s.promptField("Cc", strings.Join(cc, " "), body.Bytes())
					cc = strings.Fields(value)
				}
				if !abort && promptErr == nil {
					var value string
					value, abort, promptErr = s.promptField("Bcc", strings.Join(bcc, " "), body.Bytes())
					bcc = strings.Fields(value)
				}
				if promptErr != nil {
					return nil, to, cc, bcc, subject, false, promptErr
				}
				if abort {
					if s.receive {
						return nil, to, cc, bcc, subject, false, nil
					}
					return nil, to, cc, bcc, subject, false, fmt.Errorf("message aborted by interrupt")
				}
			default:
				diagnostic(s.rc, s.invoked, "unknown escape: %c", c)
			}
		} else {
			body.WriteString(line)
		}
		if err != nil {
			return body.Bytes(), to, cc, bcc, subject, true, nil
		}
	}
}

func expandSignature(v string) string {
	v = strings.ReplaceAll(v, `\t`, "\t")
	return strings.ReplaceAll(v, `\n`, "\n")
}

func (s *mailSession) promptField(label, current string, dead []byte) (string, bool, error) {
	interrupted, discardNext := false, false
	for {
		fmt.Fprintf(s.rc.Out, "%s: ", label)
		s.ensureInput()
		select {
		case r, ok := <-s.input:
			if s.inputOwned {
				s.input, s.inputOwned = nil, false
			}
			if discardNext {
				discardNext = false
				interrupted = false
				continue
			}
			if !ok || strings.TrimSpace(r.line) == "" {
				return current, false, nil
			}
			if r.err == io.EOF {
				return strings.TrimSpace(r.line), false, nil
			}
			return strings.TrimSpace(r.line), false, r.err
		case <-s.interrupt:
			if s.boolVar("ignore", false) {
				fmt.Fprintln(s.rc.Out, "@")
				discardNext = true
				continue
			}
			if !interrupted {
				fmt.Fprintln(s.rc.Out, "(Interrupt -- one more to kill letter)")
				interrupted = true
				continue
			}
			if err := s.writeDead(dead); err != nil {
				return current, true, err
			}
			return current, true, nil
		}
	}
}

func appendFile(path string, data []byte) error {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o666)
	if e != nil {
		return e
	}
	if e = writeBytes(f, data); e != nil {
		_ = f.Close()
		return e
	}
	return f.Close()
}
func (s *mailSession) writeDead(data []byte) error {
	if len(data) == 0 || !s.boolVar("save", true) {
		return nil
	}
	return os.WriteFile(s.resolve(s.vars["DEAD"]), data, 0o600)
}
func (s *mailSession) readInsert(arg string) ([]byte, error) {
	if strings.HasPrefix(arg, "!") {
		command := strings.TrimSpace(arg[1:])
		if s.boolVar("bang", false) {
			command = expandBang(command, s.lastBang)
		}
		s.lastBang = command
		return s.filter(command, nil)
	}
	return os.ReadFile(s.resolve(arg))
}

func (s *mailSession) shellPath() string {
	sh := s.vars["SHELL"]
	if sh == "" {
		sh = "sh"
	}
	if p := s.rc.ResolveCommand(sh); p != "" {
		return p
	}
	return sh
}
func (s *mailSession) sessionInput() io.Reader {
	if s.reader != nil {
		return s.reader
	}
	return s.rc.In
}
func (s *mailSession) runShell(command string) error {
	rc := *s.rc
	rc.Dir = s.cwd
	if command == "" {
		c, e := rc.StartCommand(s.shellPath(), nil, s.sessionInput(), s.rc.Out, s.rc.Err)
		if e == nil {
			e = c.Wait()
		}
		return e
	}
	if s.boolVar("bang", false) {
		command = expandBang(command, s.lastBang)
	}
	s.lastBang = command
	c, e := rc.StartCommand(s.shellPath(), []string{"-c", command}, s.sessionInput(), s.rc.Out, s.rc.Err)
	if e == nil {
		e = c.Wait()
	}
	return e
}

func expandBang(command, previous string) string {
	var out strings.Builder
	for i := 0; i < len(command); i++ {
		if command[i] == '\\' && i+1 < len(command) && command[i+1] == '!' {
			out.WriteByte('!')
			i++
		} else if command[i] == '!' {
			out.WriteString(previous)
		} else {
			out.WriteByte(command[i])
		}
	}
	return out.String()
}
func (s *mailSession) filter(command string, in []byte) ([]byte, error) {
	if command == "" {
		command = s.vars["cmd"]
	}
	if command == "" {
		return nil, fmt.Errorf("no pipe command")
	}
	var out bytes.Buffer
	rc := *s.rc
	rc.Dir = s.cwd
	c, e := rc.StartCommand(s.shellPath(), []string{"-c", command}, bytes.NewReader(in), &out, s.rc.Err)
	if e == nil {
		e = c.Wait()
	}
	return out.Bytes(), e
}

func (s *mailSession) reply(cmd string, args []string) error {
	nums, err := s.selectMessages(args, false)
	if err != nil {
		return err
	}
	var to []string
	uppercase := cmd == "Reply" || cmd == "Followup"
	if s.boolVar("flipr", false) {
		uppercase = !uppercase
	}
	if uppercase {
		for _, i := range nums {
			to = append(to, splitAddresses(firstHeader(s.entries[i].Message, "From", ""))...)
		}
	} else {
		nums = nums[:1]
		i := nums[0]
		to = append(to, splitAddresses(firstHeader(s.entries[i].Message, "From", ""))...)
		to = append(to, headerAddresses(s.entries[i].Message, "To")...)
		to = append(to, headerAddresses(s.entries[i].Message, "Cc")...)
	}
	var filtered []string
	self, seen := strings.ToLower(s.user), map[string]bool{}
	for _, a := range to {
		k := strings.ToLower(a)
		if (!s.boolVar("metoo", false) && (k == self || s.alts[k])) || seen[k] {
			continue
		}
		seen[k] = true
		filtered = append(filtered, a)
	}
	sub := firstHeader(s.entries[nums[0]].Message, "Subject", "")
	if !strings.HasPrefix(strings.ToLower(sub), "re:") {
		sub = "Re: " + sub
	}
	s.current = nums[len(nums)-1]
	return s.composeMessage(filtered, sub, s.opts.recordByRecipient || cmd == "followup" || cmd == "Followup", false)
}

func (s *mailSession) pipe(args []string) error {
	split := 0
	for split < len(args) && s.isMessageSpec(args[split]) {
		split++
	}
	nums, e := s.selectMessages(args[:split], false)
	if e != nil {
		return e
	}
	command := strings.Join(args[split:], " ")
	var in bytes.Buffer
	for _, i := range nums {
		in.Write(s.entries[i].Message.Bytes())
		if s.boolVar("page", false) {
			in.WriteByte('\f')
		}
		s.markMessageRead(i)
	}
	s.current = nums[len(nums)-1]
	out, e := s.filter(command, in.Bytes())
	if len(out) > 0 {
		s.rc.Out.Write(out)
	}
	return e
}

func (s *mailSession) isMessageSpec(arg string) bool {
	if arg == "+" || arg == "-" || arg == "." || arg == "^" || arg == "$" || arg == "*" {
		return true
	}
	if strings.HasPrefix(arg, "/") {
		query := strings.ToLower(strings.TrimPrefix(arg, "/"))
		for i, entry := range s.entries {
			if s.states[i] != stateDeleted && strings.Contains(strings.ToLower(firstHeader(entry.Message, "Subject", "")), query) {
				return true
			}
		}
		return false
	}
	if len(arg) == 2 && arg[0] == ':' && strings.ContainsRune("dnoru", rune(arg[1])) {
		return true
	}
	if _, err := strconv.Atoi(arg); err == nil {
		return true
	}
	if isMessageRange(arg) {
		return true
	}
	for _, entry := range s.entries {
		if s.senderMatches(firstHeader(entry.Message, "From", ""), arg) {
			return true
		}
	}
	return false
}

func (s *mailSession) editData(data []byte, which string) ([]byte, error) {
	editor := s.vars[which]
	if editor == "" {
		if which == "VISUAL" {
			editor = "vi"
		} else {
			editor = "ed"
		}
	}
	f, e := os.CreateTemp(s.resolve("."), ".mailx-edit-*")
	if e != nil {
		return nil, e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = writeBytes(f, data); e != nil {
		f.Close()
		return nil, e
	}
	if e = f.Close(); e != nil {
		return nil, e
	}
	path := s.rc.ResolveCommand(editor)
	if path == "" {
		path = editor
	}
	c, e := s.rc.StartCommand(path, []string{name}, s.sessionInput(), s.rc.Out, s.rc.Err)
	if e == nil {
		e = c.Wait()
	}
	if e != nil {
		return nil, e
	}
	return os.ReadFile(name)
}
func (s *mailSession) editMessages(cmd string, args []string) error {
	nums, e := s.selectMessages(args, false)
	if e != nil {
		return e
	}
	which := "EDITOR"
	if cmd == "visual" {
		which = "VISUAL"
	}
	for _, i := range nums {
		if _, e := s.editData(s.entries[i].Message.Bytes(), which); e != nil {
			return e
		}
	}
	s.current = nums[len(nums)-1]
	return nil
}

var _ io.Reader
