// Package mailxcmd implements the POSIX mailx(1) local-mail surface.
//
// It deliberately has no SMTP transport. A recipient is a local account name
// and delivery is an mbox append beneath the configured local spool. Addresses
// containing routing syntax fail before any mailbox is changed.
package mailxcmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
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
	body, err := io.ReadAll(rc.In)
	if err != nil {
		diagnostic(rc, invoked, "read message: %v", err)
		return 1
	}
	to := append([]string(nil), operands...)
	if o.headerRecipients {
		parsed, err := mailxpkg.ParseMessage(body)
		if err != nil {
			diagnostic(rc, invoked, "%v", err)
			return 1
		}
		to = append(to, headerAddresses(parsed, "To")...)
		o.cc = append(o.cc, headerAddresses(parsed, "Cc")...)
		body = parsed.Body
		if o.subject == "" {
			if v := parsed.HeaderValues("Subject"); len(v) > 0 {
				o.subject = v[0]
			}
		}
	}
	all := append(append(append([]string{}, to...), o.cc...), o.bcc...)
	if len(all) == 0 {
		return tool.UsageError(rc, invoked, "no recipients specified")
	}
	paths := make([]string, len(all))
	for i, address := range all {
		if !localAddress(address) {
			diagnostic(rc, invoked, "remote delivery is not supported: %s", address)
			return 1
		}
		paths[i], err = spoolPath(rc, address)
		if err != nil {
			diagnostic(rc, invoked, "%v", err)
			return 1
		}
	}
	sender := o.from
	if sender == "" {
		sender = identity(rc)
	}
	if !localAddress(sender) {
		diagnostic(rc, invoked, "envelope sender must be a local user: %s", sender)
		return 1
	}
	when := nowFn().UTC()
	headers := []mailxpkg.Header{
		{Name: "Date", Value: when.Format(time.RFC1123Z)},
		{Name: "From", Value: sender},
		{Name: "To", Value: strings.Join(to, ", ")},
	}
	if len(o.cc) > 0 {
		headers = append(headers, mailxpkg.Header{Name: "Cc", Value: strings.Join(o.cc, ", ")})
	}
	if o.subject != "" {
		headers = append(headers, mailxpkg.Header{Name: "Subject", Value: o.subject})
	}
	msg := &mailxpkg.Message{Headers: headers, Body: body}
	if err := msg.Validate(); err != nil {
		diagnostic(rc, invoked, "%v", err)
		return 1
	}
	for i, path := range paths {
		if err := mailxpkg.AppendMbox(path, sender, when, msg); err != nil {
			diagnostic(rc, invoked, "deliver to %s: %v", all[i], err)
			return 1
		}
	}
	if o.recordByRecipient {
		if len(to) == 0 {
			diagnostic(rc, invoked, "-F requires a To recipient")
			return 1
		}
		if err := mailxpkg.AppendMbox(rc.Path(to[0]), sender, when, msg); err != nil {
			diagnostic(rc, invoked, "record for %s: %v", to[0], err)
			return 1
		}
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
	user := o.user
	if user == "" {
		user = identity(rc)
	}
	path := o.file
	var err error
	if path == "" {
		path, err = spoolPath(rc, user)
	} else {
		path = rc.Path(path)
	}
	if err != nil {
		diagnostic(rc, invoked, "%v", err)
		return 1
	}
	entries, err := mailxpkg.ReadMbox(path)
	if err != nil {
		diagnostic(rc, invoked, "%v", err)
		return 1
	}
	if o.existOnly {
		if len(entries) == 0 {
			return 1
		}
		return 0
	}
	if len(entries) == 0 {
		if !o.headersOnly {
			fmt.Fprintf(rc.Out, "No mail for %s\n", user)
		}
		return 0
	}
	deleted := make([]bool, len(entries))
	current := 0
	if o.headersOnly {
		printHeaders(rc.Out, entries, deleted, current)
		return 0
	}
	if !o.noInitialHeaders {
		printHeaders(rc.Out, entries, deleted, current)
	}
	return commandLoop(rc, invoked, path, entries, deleted, current)
}

func printHeaders(w io.Writer, entries []mailxpkg.MboxEntry, deleted []bool, current int) {
	for i, entry := range entries {
		if deleted[i] {
			continue
		}
		marker := " "
		if i == current {
			marker = ">"
		}
		from := firstHeader(entry.Message, "From", "unknown")
		subject := firstHeader(entry.Message, "Subject", "")
		lines := bytes.Count(entry.Message.Body, []byte{'\n'})
		fmt.Fprintf(w, "%s%3d %-16.16s %4d/%-6d %s\n", marker, i+1, from, lines, len(entry.Message.Body), subject)
	}
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

func commandLoop(rc *tool.RunContext, invoked *tool.Tool, path string, entries []mailxpkg.MboxEntry, deleted []bool, current int) int {
	if rc.In == nil {
		return 0
	}
	sc := bufio.NewScanner(rc.In)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			line = "+"
		}
		fields := strings.Fields(line)
		verb := fields[0]
		args := fields[1:]
		switch verb {
		case "q", "quit":
			return commit(rc, invoked, path, entries, deleted)
		case "x", "xit", "exit":
			return 0
		case "h", "headers":
			printHeaders(rc.Out, entries, deleted, current)
		case "p", "print", "t", "type":
			for _, n := range messageList(args, current, len(entries)) {
				if !deleted[n] {
					printMessage(rc.Out, entries[n])
					current = n
				}
			}
		case "d", "delete", "u", "undelete":
			value := verb == "d" || verb == "delete"
			for _, n := range messageList(args, current, len(entries)) {
				deleted[n] = value
				current = n
			}
		case "+", "next":
			if n := nextMessage(deleted, current, 1); n >= 0 {
				current = n
				printMessage(rc.Out, entries[n])
			}
		case "-":
			if n := nextMessage(deleted, current, -1); n >= 0 {
				current = n
				printMessage(rc.Out, entries[n])
			}
		case "=":
			fmt.Fprintln(rc.Out, current+1)
		case "f", "from":
			for _, n := range messageList(args, current, len(entries)) {
				fmt.Fprintf(rc.Out, "%d %s %s\n", n+1, firstHeader(entries[n].Message, "From", "unknown"), firstHeader(entries[n].Message, "Subject", ""))
			}
		case "s", "save", "w", "write":
			if len(args) == 0 {
				diagnostic(rc, invoked, "filename required")
				continue
			}
			file := args[len(args)-1]
			nums := messageList(args[:len(args)-1], current, len(entries))
			if err := saveMessages(rc.Path(file), entries, nums, verb == "w" || verb == "write"); err != nil {
				diagnostic(rc, invoked, "%v", err)
			}
		default:
			if n, err := strconv.Atoi(verb); err == nil && n >= 1 && n <= len(entries) {
				current = n - 1
				printMessage(rc.Out, entries[current])
			} else {
				diagnostic(rc, invoked, "unknown command: %s", verb)
			}
		}
	}
	if err := sc.Err(); err != nil {
		diagnostic(rc, invoked, "read commands: %v", err)
		return 1
	}
	return commit(rc, invoked, path, entries, deleted)
}

func messageList(args []string, current, total int) []int {
	if len(args) == 0 {
		return []int{current}
	}
	var out []int
	for _, arg := range args {
		if arg == "*" {
			for i := 0; i < total; i++ {
				out = append(out, i)
			}
			continue
		}
		if n, err := strconv.Atoi(arg); err == nil && n >= 1 && n <= total {
			out = append(out, n-1)
		}
	}
	return out
}

func nextMessage(deleted []bool, current, step int) int {
	for n := current + step; n >= 0 && n < len(deleted); n += step {
		if !deleted[n] {
			return n
		}
	}
	return -1
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

func commit(rc *tool.RunContext, invoked *tool.Tool, path string, entries []mailxpkg.MboxEntry, deleted []bool) int {
	changed := false
	for _, value := range deleted {
		changed = changed || value
	}
	if !changed {
		return 0
	}
	if err := mailxpkg.CommitMbox(path, entries, deleted); err != nil {
		diagnostic(rc, invoked, "%v", err)
		return 1
	}
	return 0
}

func diagnostic(rc *tool.RunContext, invoked *tool.Tool, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	message = strings.ReplaceAll(message, "mailx: ", "")
	fmt.Fprintf(rc.Err, "%s: %s\n", invoked.Name, message)
}
