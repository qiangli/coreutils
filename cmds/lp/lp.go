// Package lpcmd implements POSIX lp(1) as a pure-Go IPP/1.1 client
// (RFC 8011 Print-Job over HTTP POST, application/ipp). Nothing is
// spawned; the IPP request is encoded by hand.
//
// Destination: -d, else LPDEST, else PRINTER. A destination that is
// itself an ipp://, ipp s://, http:// or https:// URI is used as is; a bare
// name maps to ipp://localhost:631/printers/<name>. The environment
// variable LP_IPP_URI, when set, overrides the resolved URI for every
// destination (the name is still used in the request-id message).
//
// -o name[=value] is sent as a keyword job attribute (value defaults
// to "true"). -c is accepted (files are always read before sending).
// -m and -w wait for job completion. Mail uses the local mailx spool; -w
// writes to a live login terminal, or mails when the user is not logged in.
// One invocation is one request: a single
// file is a Print-Job; several files are a Create-Job followed by one
// Send-Document per file (last-document set on the final one).
package lpcmd

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/qiangli/coreutils/cmds/internal/session"
	"github.com/qiangli/coreutils/pkg/mailx"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "lp",
	Synopsis: "Submit files to a printer over IPP.",
	Usage: "lp [-c] [-d dest] [-n copies] [-o option]... [-msw] [-t title] [file...]\n" +
		"Destination: -d, else $LPDEST, else $PRINTER. Bare names map to\n" +
		"ipp://localhost:631/printers/NAME; $LP_IPP_URI overrides the URI.",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	args = tool.AliasHelpVersion(args)
	fs := tool.NewFlags(cmd.Name)
	fs.SetInterspersed(false)
	_ = fs.BoolP("c", "c", false, "copy files before printing (always done)")
	dest := fs.StringP("d", "d", "", "destination printer")
	mail := fs.BoolP("m", "m", false, "send mail after printing")
	write := fs.BoolP("w", "w", false, "request a terminal message after printing")
	copies := fs.IntP("n", "n", 1, "number of copies")
	var opts []string
	fs.StringArrayVarP(&opts, "o", "o", nil, "printer-specific option")
	silent := fs.BoolP("s", "s", false, "suppress the request-id message")
	title := fs.StringP("t", "t", "", "job title")
	files, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if *copies < 1 {
		return tool.UsageError(rc, cmd, "invalid copies %d", *copies)
	}
	d := *dest
	if d == "" {
		d = rc.Getenv("LPDEST")
	}
	if d == "" {
		d = rc.Getenv("PRINTER")
	}
	if d == "" {
		fmt.Fprintln(rc.Err, "lp: no destination specified (use -d, LPDEST or PRINTER)")
		return 1
	}
	uri := resolve(d, rc.Getenv("LP_IPP_URI"))
	if len(files) == 0 {
		files = []string{"-"}
	}
	user := rc.Getenv("USER")
	if user == "" {
		user = rc.Getenv("LOGNAME")
	}
	if user == "" {
		user = "anonymous"
	}
	var docs [][]byte
	for _, f := range files {
		var data []byte
		var err error
		if f == "-" {
			data, err = io.ReadAll(rc.In)
		} else {
			data, err = os.ReadFile(rc.Path(f))
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "lp: %s: %v\n", f, err)
			return 1
		}
		docs = append(docs, data)
	}
	job := *title
	if job == "" {
		job = files[0]
		if job == "-" {
			job = "stdin"
		}
	}
	var id uint32
	var err error
	if len(docs) == 1 {
		id, err = send(rc.Ctx, uri, encode(opPrintJob, uri, user, job, *copies, opts, 0, false, docs[0]))
	} else {
		id, err = send(rc.Ctx, uri, encode(opCreateJob, uri, user, job, *copies, opts, 0, false, nil))
		for i := 0; err == nil && i < len(docs); i++ {
			var returned uint32
			returned, err = send(rc.Ctx, uri, encode(opSendDocument, uri, user, job, 0, nil, id, i == len(docs)-1, docs[i]))
			if err == nil && returned != id {
				err = fmt.Errorf("Send-Document returned job-id %d, expected %d", returned, id)
			}
		}
	}
	if err != nil {
		fmt.Fprintf(rc.Err, "lp: %v\n", err)
		return 1
	}
	if !*silent {
		if _, err := fmt.Fprintf(rc.Out, "request id is %s-%d (%d file(s))\n", d, id, len(files)); err != nil {
			fmt.Fprintf(rc.Err, "lp: write request id: %v\n", err)
			return 1
		}
	}
	if *write || *mail {
		if err := waitCompleted(rc, uri, user, id); err != nil {
			fmt.Fprintf(rc.Err, "lp: completion notification unavailable: %v\n", err)
			return 1
		}
		if err := notifyCompletion(rc, user, d, id, *mail, *write); err != nil {
			fmt.Fprintf(rc.Err, "lp: completion notification unavailable: %v\n", err)
			return 1
		}
	}
	return 0
}

func resolve(dest, override string) string {
	if override != "" {
		return override
	}
	for _, p := range []string{"ipp://", "ipps://", "http://", "https://"} {
		if strings.HasPrefix(dest, p) {
			return dest
		}
	}
	return "ipp://localhost:631/printers/" + dest
}

func attr(b *bytes.Buffer, tag byte, name string, val []byte) {
	b.WriteByte(tag)
	binary.Write(b, binary.BigEndian, uint16(len(name)))
	b.WriteString(name)
	binary.Write(b, binary.BigEndian, uint16(len(val)))
	b.Write(val)
}

const (
	opPrintJob     = 0x0002
	opCreateJob    = 0x0005
	opSendDocument = 0x0006
	opGetJobAttrs  = 0x0009
)

// encode builds an IPP/1.1 request, request-id 1. Print-Job and
// Create-Job carry job attributes; Send-Document names the job by id.
func encode(op uint16, uri, user, job string, copies int, opts []string, jobID uint32, last bool, doc []byte) []byte {
	var b bytes.Buffer
	b.Write([]byte{1, 1, byte(op >> 8), byte(op), 0, 0, 0, 1, 0x01})
	attr(&b, 0x47, "attributes-charset", []byte("utf-8"))
	attr(&b, 0x48, "attributes-natural-language", []byte("en"))
	attr(&b, 0x45, "printer-uri", []byte(uri))
	if op == opSendDocument {
		n := make([]byte, 4)
		binary.BigEndian.PutUint32(n, jobID)
		attr(&b, 0x21, "job-id", n)
	}
	attr(&b, 0x42, "requesting-user-name", []byte(user))
	if op != opSendDocument {
		attr(&b, 0x42, "job-name", []byte(job))
	}
	if op != opCreateJob {
		attr(&b, 0x49, "document-format", []byte("application/octet-stream"))
	}
	if op == opSendDocument {
		v := byte(0)
		if last {
			v = 1
		}
		attr(&b, 0x22, "last-document", []byte{v})
	}
	if op != opSendDocument {
		b.WriteByte(0x02)
		n := make([]byte, 4)
		binary.BigEndian.PutUint32(n, uint32(copies))
		attr(&b, 0x21, "copies", n)
		for _, o := range opts {
			k, v, ok := strings.Cut(o, "=")
			if !ok {
				v = "true"
			}
			attr(&b, 0x44, k, []byte(v))
		}
	}
	b.WriteByte(0x03)
	b.Write(doc)
	return b.Bytes()
}

// ippHTTP maps the IPP URI to its HTTP transport URI.
func ippHTTP(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uri // NewRequestWithContext will report the invalid URI.
	}
	if u.Scheme == "ipp" || u.Scheme == "ipps" {
		if u.Scheme == "ipp" {
			u.Scheme = "http"
		} else {
			u.Scheme = "https"
		}
		if u.Port() == "" {
			u.Host = net.JoinHostPort(u.Hostname(), "631")
		}
	}
	return u.String()
}

func exchange(ctx context.Context, uri string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ippHTTP(uri), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/ipp")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, fmt.Errorf("IPP response exceeds 1 MiB")
	}
	return data, nil
}

type ippAttribute struct {
	group, tag byte
	name       string
	value      []byte
}

// parseIPP checks the entire attribute stream before any caller accepts a
// value. IPP responses to these operations have no document data.
func parseIPP(data []byte) ([]ippAttribute, error) {
	if len(data) < 9 || data[0] != 1 || data[1] != 1 || binary.BigEndian.Uint32(data[4:8]) != 1 {
		return nil, fmt.Errorf("invalid IPP response header")
	}
	if st := binary.BigEndian.Uint16(data[2:4]); st >= 0x0100 {
		return nil, fmt.Errorf("IPP error status 0x%04x", st)
	}
	var attrs []ippAttribute
	var group byte
	for p := 8; p < len(data); {
		tag := data[p]
		p++
		if tag == 0x03 {
			if p != len(data) {
				return nil, fmt.Errorf("trailing IPP response bytes")
			}
			return attrs, nil
		}
		if tag >= 0x01 && tag <= 0x07 {
			if tag == 0x07 { // event-notification group is not a job response
				return nil, fmt.Errorf("unexpected IPP group 0x%02x", tag)
			}
			group = tag
			continue
		}
		if tag < 0x10 || group == 0 || p+2 > len(data) {
			return nil, fmt.Errorf("truncated IPP attribute")
		}
		nl := int(binary.BigEndian.Uint16(data[p:]))
		p += 2
		if p+nl+2 > len(data) {
			return nil, fmt.Errorf("truncated IPP attribute name")
		}
		name := string(data[p : p+nl])
		p += nl
		vl := int(binary.BigEndian.Uint16(data[p:]))
		p += 2
		if p+vl > len(data) {
			return nil, fmt.Errorf("truncated IPP attribute value")
		}
		attrs = append(attrs, ippAttribute{group, tag, name, data[p : p+vl]})
		p += vl
	}
	return nil, fmt.Errorf("IPP response omitted end-of-attributes tag")
}

func positiveID(attrs []ippAttribute) (uint32, error) {
	for _, a := range attrs {
		if a.group == 0x02 && a.name == "job-id" {
			if a.tag != 0x21 || len(a.value) != 4 {
				return 0, fmt.Errorf("invalid IPP job-id")
			}
			id := int32(binary.BigEndian.Uint32(a.value))
			if id <= 0 {
				return 0, fmt.Errorf("nonpositive IPP job-id")
			}
			return uint32(id), nil
		}
	}
	return 0, fmt.Errorf("IPP response omitted job-id")
}

func send(ctx context.Context, uri string, body []byte) (uint32, error) {
	data, err := exchange(ctx, uri, body)
	if err != nil {
		return 0, err
	}
	attrs, err := parseIPP(data)
	if err != nil {
		return 0, err
	}
	return positiveID(attrs)
}

// A deadline on the context covers the HTTP headers, response body and polling.
func waitCompleted(rc *tool.RunContext, uri, user string, id uint32) error {
	ctx, cancel := context.WithTimeout(rc.Ctx, 30*time.Second)
	defer cancel()
	for {
		state, err := jobState(ctx, uri, user, id)
		if err != nil {
			return err
		}
		switch state {
		case 9:
			return nil
		case 7:
			return fmt.Errorf("job %d was canceled", id)
		case 8:
			return fmt.Errorf("job %d was aborted", id)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func jobState(ctx context.Context, uri, user string, id uint32) (uint32, error) {
	var b bytes.Buffer
	b.Write([]byte{1, 1, 0, opGetJobAttrs, 0, 0, 0, 1, 0x01})
	attr(&b, 0x47, "attributes-charset", []byte("utf-8"))
	attr(&b, 0x48, "attributes-natural-language", []byte("en"))
	attr(&b, 0x45, "printer-uri", []byte(uri))
	n := make([]byte, 4)
	binary.BigEndian.PutUint32(n, id)
	attr(&b, 0x21, "job-id", n)
	attr(&b, 0x42, "requesting-user-name", []byte(user))
	b.WriteByte(0x03)
	data, err := exchange(ctx, uri, b.Bytes())
	if err != nil {
		return 0, err
	}
	attrs, err := parseIPP(data)
	if err != nil {
		return 0, err
	}
	for _, a := range attrs {
		if a.group == 0x02 && a.name == "job-state" {
			if a.tag != 0x23 || len(a.value) != 4 {
				return 0, fmt.Errorf("invalid IPP job-state")
			}
			state := binary.BigEndian.Uint32(a.value)
			if state < 3 || state > 9 {
				return 0, fmt.Errorf("invalid IPP job-state %d", state)
			}
			return state, nil
		}
	}
	return 0, fmt.Errorf("IPP completion response omitted job-state")
}

var readSessions = func(env []string) ([]session.Record, error) {
	path := session.DefaultFileForEnv(env)
	if path == "" {
		return nil, fmt.Errorf("login database unavailable")
	}
	return session.ReadEnv(path, env) // explicit path keeps lookup failure distinct from absence
}

var writeTerminal = func(tty, message string) error {
	path := session.TTYPath(tty)
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return fmt.Errorf("%s is not a terminal device", path)
	}
	if !term.IsTerminal(int(f.Fd())) {
		return fmt.Errorf("%s is not a terminal", path)
	}
	n, err := io.WriteString(f, message)
	if err == nil && n != len(message) {
		return io.ErrShortWrite
	}
	return err
}

var deliverMail = func(rc *tool.RunContext, user, message string) error {
	if user == "" || strings.ContainsAny(user, "@!%/:\\\r\n") || user == "." || user == ".." {
		return fmt.Errorf("invalid local mail recipient %q", user)
	}
	path := rc.Getenv("MAIL")
	if path == "" {
		if root := rc.Getenv("MAILX_SPOOL"); root != "" {
			path = filepath.Join(root, user)
		} else if home := rc.Getenv("HOME"); home != "" {
			path = filepath.Join(home, ".mailx", "spool", user)
		} else {
			return fmt.Errorf("MAIL, MAILX_SPOOL or HOME is required for local mail")
		}
	}
	msg := &mailx.Message{Headers: []mailx.Header{{Name: "To", Value: user}, {Name: "Subject", Value: "lp job completion"}}, Body: []byte(message)}
	return mailx.LocalMboxTransport{MailboxPath: rc.Path(path), Sender: user}.Deliver(rc.Ctx, msg, []string{user})
}

func notifyCompletion(rc *tool.RunContext, user, dest string, id uint32, mail, write bool) error {
	message := fmt.Sprintf("request %s-%d completed\n", dest, id)
	if mail {
		if err := deliverMail(rc, user, message); err != nil {
			return err
		}
	}
	if write {
		records, err := readSessions(rc.Env)
		if err != nil {
			return fmt.Errorf("read login sessions: %w", err)
		}
		found := false
		for _, record := range records {
			if session.IsUser(record) && record.User == user && record.TTY != "" {
				found = true
				if err := writeTerminal(record.TTY, message); err == nil {
					break
				} else {
					return fmt.Errorf("write terminal %s: %w", record.TTY, err)
				}
			}
		}
		if !found && !mail {
			return deliverMail(rc, user, message)
		}
	}
	return nil
}
