// Package mailxcmd implements the POSIX mailx(1) local-mail surface.
//
// It deliberately has no SMTP transport. A recipient is a local account name
// and delivery is an mbox append beneath the configured local spool. Addresses
// containing routing syntax fail before any mailbox is changed.
package mailxcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	mailxpkg "github.com/qiangli/coreutils/pkg/mailx"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "mailx",
	Synopsis: "Send and receive local mail.",
	Usage:    "mailx [-eFHiNn] [-f [file]] [-u user]\n       mailx [-Fit] [-b address] [-c address] [-r address] [-s subject] address...",
}

var mailCmd = &tool.Tool{
	Name:     "mail",
	Synopsis: "Send and receive local mail.",
	Usage:    "mail [-eFHiNn] [-f [file]] [-u user]\n       mail [-Fit] [-b address] [-c address] [-r address] [-s subject] address...",
}

var nowFn = time.Now

func init() {
	cmd.Run = func(rc *tool.RunContext, args []string) int { return runTool(rc, cmd, args) }
	mailCmd.Run = func(rc *tool.RunContext, args []string) int { return runTool(rc, mailCmd, args) }
	tool.Register(cmd)
	tool.Register(mailCmd)
}

type options struct {
	existOnly, headersOnly, noInitialHeaders      bool
	ignoreInterrupt, noSystemRC, headerRecipients bool
	fileMode, recordByRecipient                   bool
	file, user, subject, from                     string
	bcc, cc                                       []string
}

func run(rc *tool.RunContext, args []string) int {
	return runTool(rc, cmd, args)
}

func runTool(rc *tool.RunContext, invoked *tool.Tool, args []string) int {
	// A mailx session emits through many commands and prompts. Keep one
	// invocation-wide writer so every output failure, including a nil-error
	// short write, reaches the command boundary instead of being lost at an
	// individual fmt call.
	out := &stickyWriter{writer: rc.Out}
	child := *rc
	child.Out = out
	code := runToolCore(&child, invoked, args)
	if out.err != nil {
		diagnostic(rc, invoked, "write error: %v", out.err)
		return 1
	}
	return code
}

type stickyWriter struct {
	writer io.Writer
	err    error
}

func (w *stickyWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.writer.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.err = err
	}
	return n, err
}

func runToolCore(rc *tool.RunContext, invoked *tool.Tool, args []string) int {
	fs := tool.NewFlags(invoked.Name)
	o := options{}
	fs.BoolVarP(&o.existOnly, "exist", "e", false, "test for mail without producing output")
	fs.BoolVarP(&o.headersOnly, "headers", "H", false, "write a header summary and exit")
	fs.BoolVarP(&o.ignoreInterrupt, "ignore-interrupts", "i", false, "ignore interrupts while entering a message")
	fs.BoolVarP(&o.noInitialHeaders, "no-header", "N", false, "do not write the initial header summary")
	fs.BoolVarP(&o.noSystemRC, "no-system-rc", "n", false, "do not read the system startup file")
	fs.BoolVarP(&o.headerRecipients, "read-recipients", "t", false, "read recipients from message headers")
	fs.BoolVarP(&o.fileMode, "file", "f", false, "read messages from file operand, or MBOX")
	fs.BoolVarP(&o.recordByRecipient, "record", "F", false, "record sent mail in a file named for the first recipient")
	fs.StringVarP(&o.user, "user", "u", "", "read the local user's mailbox")
	fs.StringVarP(&o.subject, "subject", "s", "", "set the Subject header")
	fs.StringVarP(&o.from, "from", "r", "", "set the local envelope sender")
	var cc, bcc string
	fs.StringVarP(&cc, "cc", "c", "", "send copies to local addresses")
	fs.StringVarP(&bcc, "bcc", "b", "", "send blind copies to local addresses")
	operands, code := tool.Parse(rc, invoked, fs, args)
	if code >= 0 {
		return code
	}
	o.cc, o.bcc = splitAddresses(cc), splitAddresses(bcc)
	if o.fileMode {
		if len(operands) > 1 {
			return tool.UsageError(rc, invoked, "-f accepts at most one mailbox file operand")
		}
		if len(operands) == 1 {
			o.file = operands[0]
		} else {
			o.file = rc.Getenv("MBOX")
		}
		if o.file == "" {
			if home := rc.Getenv("HOME"); home != "" {
				o.file = filepath.Join(home, "mbox")
			} else {
				diagnostic(rc, invoked, "HOME or MBOX is required for -f")
				return 1
			}
		}
		operands = nil
	}

	sending := len(operands) > 0 || o.headerRecipients
	if sending {
		if o.file != "" || o.user != "" || o.existOnly || o.headersOnly || o.noInitialHeaders {
			return tool.UsageError(rc, invoked, "sending and mailbox-reading options cannot be combined")
		}
		return send(rc, invoked, o, operands)
	}
	if o.subject != "" || o.from != "" || len(o.cc) > 0 || len(o.bcc) > 0 {
		return tool.UsageError(rc, invoked, "recipient required when sending mail")
	}
	return receive(rc, invoked, o)
}

func splitAddresses(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
}

func localAddress(s string) bool {
	if s == "" || strings.ContainsAny(s, "@!%/:\\\r\n") || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._-", r)) {
			return false
		}
	}
	return true
}

func identity(rc *tool.RunContext) string {
	for _, key := range []string{"LOGNAME", "USER"} {
		if v := rc.Getenv(key); localAddress(v) {
			return v
		}
	}
	return "unknown"
}

func spoolPath(rc *tool.RunContext, user string) (string, error) {
	if !localAddress(user) {
		return "", fmt.Errorf("invalid local user %q", user)
	}
	if user == identity(rc) && rc.Getenv("MAIL") != "" {
		return rc.Path(rc.Getenv("MAIL")), nil
	}
	if root := rc.Getenv("MAILX_SPOOL"); root != "" {
		return filepath.Join(rc.Path(root), user), nil
	}
	home := rc.Getenv("HOME")
	if home == "" {
		return "", fmt.Errorf("MAILX_SPOOL or HOME is required for local delivery")
	}
	return filepath.Join(rc.Path(home), ".mailx", "spool", user), nil
}

func send(rc *tool.RunContext, invoked *tool.Tool, o options, operands []string) int {
	s := &mailSession{rc: rc, invoked: invoked, opts: o, reader: bufio.NewReader(rc.In), user: identity(rc), vars: defaultVariables(rc), aliases: map[string][]string{}, ignore: map[string]bool{}, retain: map[string]bool{}, alts: map[string]bool{}, cwd: rc.Dir, active: true}
	if o.ignoreInterrupt {
		s.vars["ignore"] = ""
	}
	if err := s.readStartup(); err != nil {
		diagnostic(rc, invoked, "%v", err)
		return 1
	}
	if s.quitRequested {
		return 0
	}
	to, cc, bcc := s.expandAliases(operands), s.expandAliases(o.cc), s.expandAliases(o.bcc)
	var body []byte
	var err error
	if mailxIsTerminal(rc.In) {
		createdInterrupt := s.ensureInterrupt()
		if createdInterrupt {
			defer s.stopInterrupt()
		}
		if o.subject == "" && s.boolVar("asksub", true) {
			var abort bool
			o.subject, abort, err = s.promptField("Subject", o.subject, nil)
			if err != nil || abort {
				if err != nil {
					diagnostic(rc, invoked, "%v", err)
				}
				return 1
			}
		}
		if s.boolVar("askcc", false) {
			var line string
			var abort bool
			line, abort, err = s.promptField("Cc", "", nil)
			if err != nil || abort {
				if err != nil {
					diagnostic(rc, invoked, "%v", err)
				}
				return 1
			}
			cc = append(cc, strings.Fields(line)...)
		}
		if s.boolVar("askbcc", false) {
			var line string
			var abort bool
			line, abort, err = s.promptField("Bcc", "", nil)
			if err != nil || abort {
				if err != nil {
					diagnostic(rc, invoked, "%v", err)
				}
				return 1
			}
			bcc = append(bcc, strings.Fields(line)...)
		}
		var sendIt bool
		body, to, cc, bcc, o.subject, sendIt, err = s.readComposition(to, cc, bcc, o.subject)
		if !sendIt {
			if s.quitRequested {
				return 0
			}
			return 1
		}
	} else {
		body, err = io.ReadAll(s.reader)
	}
	if err != nil {
		diagnostic(rc, invoked, "read message: %v", err)
		return 1
	}
	if o.headerRecipients {
		parsed, parseErr := mailxpkg.ParseMessage(body)
		if parseErr != nil {
			diagnostic(rc, invoked, "%v", parseErr)
			return 1
		}
		to = append(to, headerAddresses(parsed, "To")...)
		cc = append(cc, headerAddresses(parsed, "Cc")...)
		body = parsed.Body
		if o.subject == "" {
			if v := parsed.HeaderValues("Subject"); len(v) > 0 {
				o.subject = v[0]
			}
		}
	}
	to, cc, bcc = s.expandAliases(to), s.expandAliases(cc), s.expandAliases(bcc)
	if o.from != "" {
		s.user = o.from
	}
	if err = s.deliver(body, to, cc, bcc, o.subject, o.recordByRecipient); err != nil {
		if deadErr := s.writeDead(body); deadErr != nil {
			diagnostic(rc, invoked, "%v; save dead letter: %v", err, deadErr)
			return 1
		}
		diagnostic(rc, invoked, "%v", err)
		return 1
	}
	return 0
}

func headerAddresses(msg *mailxpkg.Message, name string) []string {
	var out []string
	for _, value := range msg.HeaderValues(name) {
		out = append(out, splitAddresses(value)...)
	}
	return out
}

func receive(rc *tool.RunContext, invoked *tool.Tool, o options) int {
	return runReceiveSession(rc, invoked, o)
}

func firstHeader(msg *mailxpkg.Message, name, fallback string) string {
	if values := msg.HeaderValues(name); len(values) > 0 {
		return values[0]
	}
	return fallback
}

func printMessage(w io.Writer, entry mailxpkg.MboxEntry) {
	fmt.Fprintln(w, entry.Envelope)
	w.Write(entry.Message.Bytes())
	if b := entry.Message.Bytes(); len(b) > 0 && b[len(b)-1] != '\n' {
		fmt.Fprintln(w)
	}
}

func saveMessages(path string, entries []mailxpkg.MboxEntry, nums []int, bodyOnly bool) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	for _, n := range nums {
		var data []byte
		if bodyOnly {
			data = entries[n].Message.Body
		} else {
			data = entries[n].Message.Bytes()
		}
		if _, err := f.Write(data); err != nil {
			_ = f.Close()
			return err
		}
		if len(data) == 0 || data[len(data)-1] != '\n' {
			if _, err := f.Write([]byte{'\n'}); err != nil {
				_ = f.Close()
				return err
			}
		}
	}
	return f.Close()
}

func diagnostic(rc *tool.RunContext, invoked *tool.Tool, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	message = strings.ReplaceAll(message, "mailx: ", "")
	fmt.Fprintf(rc.Err, "%s: %s\n", invoked.Name, message)
}
