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
// -m asks the server to mail the requesting user on completion, via an
// RFC 3995 subscription group (notify-recipient-uri mailto:USER@localhost,
// notify-events job-completed). -w instead waits for a job-completed state and
// writes its own terminal message; RFC 3995 notify-user-data is opaque.
// One invocation is one request: a single
// file is a Print-Job; several files are a Create-Job followed by one
// Send-Document per file (last-document set on the final one).
package lpcmd

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

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
		id, err = send(rc, uri, encode(opPrintJob, uri, user, job, *copies, opts, *mail, 0, false, docs[0]), *mail)
	} else {
		id, err = send(rc, uri, encode(opCreateJob, uri, user, job, *copies, opts, *mail, 0, false, nil), *mail)
		for i := 0; err == nil && i < len(docs); i++ {
			_, err = send(rc, uri, encode(opSendDocument, uri, user, job, 0, nil, false, id, i == len(docs)-1, docs[i]), false)
		}
	}
	if err != nil {
		fmt.Fprintf(rc.Err, "lp: %v\n", err)
		return 1
	}
	if !*silent {
		fmt.Fprintf(rc.Out, "request id is %s-%d (%d file(s))\n", d, id, len(files))
	}
	if *write {
		if err := waitCompleted(rc, uri, user, id); err != nil {
			fmt.Fprintf(rc.Err, "lp: terminal completion notification unavailable: %v\n", err)
			return 1
		}
		fmt.Fprintf(rc.Out, "request %s-%d completed\n", d, id)
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
// Create-Job carry the job attributes (and, with mail or write, a subscription
// group); Send-Document names the job by id and carries last-document.
func encode(op uint16, uri, user, job string, copies int, opts []string, mail bool, jobID uint32, last bool, doc []byte) []byte {
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
		if mail {
			b.WriteByte(0x06)
			attr(&b, 0x45, "notify-recipient-uri", []byte("mailto:"+user+"@localhost"))
			attr(&b, 0x44, "notify-events", []byte("job-completed"))
		}
	}
	b.WriteByte(0x03)
	b.Write(doc)
	return b.Bytes()
}

func send(rc *tool.RunContext, uri string, body []byte, requiredNotification bool) (uint32, error) {
	u := uri
	switch {
	case strings.HasPrefix(u, "ipps://"):
		u = "https://" + u[7:]
	case strings.HasPrefix(u, "ipp://"):
		u = "http://" + u[6:]
	}
	if strings.HasPrefix(u, "http://") && !strings.Contains(u[7:strings.IndexAny(u[7:]+"/", "/")+7], ":") &&
		strings.HasPrefix(uri, "ipp://") {
		host := u[7 : strings.IndexAny(u[7:]+"/", "/")+7]
		u = "http://" + host + ":631" + u[7+len(host):]
	}
	req, err := http.NewRequestWithContext(rc.Ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/ipp")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %s", resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if len(b) < 9 {
		return 0, fmt.Errorf("short IPP response")
	}
	if st := binary.BigEndian.Uint16(b[2:]); st >= 0x0100 {
		return 0, fmt.Errorf("IPP error status 0x%04x", st)
	}
	var id uint32
	group := byte(0)
	rejected := false
	p := 8
	for p < len(b) && b[p] != 0x03 {
		if b[p] <= 0x06 {
			group = b[p]
			rejected = rejected || group == 0x05
			p++
			continue
		}
		if p+3 > len(b) {
			return 0, fmt.Errorf("truncated IPP attribute")
		}
		tag := b[p]
		nl := int(binary.BigEndian.Uint16(b[p+1:]))
		if p+5+nl > len(b) {
			return 0, fmt.Errorf("truncated IPP attribute name")
		}
		name := string(b[p+3 : p+3+nl])
		p += 3 + nl
		vl := int(binary.BigEndian.Uint16(b[p:]))
		if p+2+vl > len(b) {
			return 0, fmt.Errorf("truncated IPP attribute value")
		}
		if tag == 0x21 && name == "job-id" && vl == 4 {
			id = binary.BigEndian.Uint32(b[p+2:])
		}
		p += 2 + vl
	}
	if id == 0 {
		return 0, fmt.Errorf("IPP response omitted job-id")
	}
	if requiredNotification && (binary.BigEndian.Uint16(b[2:]) != 0 || rejected) {
		return 0, fmt.Errorf("IPP server rejected completion mail notification")
	}
	return id, nil
}

// -w is synchronous and bounded: a process cannot promise terminal delivery
// after it exits.  A caller that needs durable asynchronous notification must
// use the printer's own notification service; lp reports that limitation by
// failing rather than pretending an opaque RFC 3995 datum reaches a terminal.
func waitCompleted(rc *tool.RunContext, uri, user string, id uint32) error {
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	for {
		state, err := jobState(rc, uri, user, id)
		if err != nil {
			return err
		}
		if state == 9 {
			return nil
		}
		select {
		case <-rc.Ctx.Done():
			return rc.Ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("job did not complete within 30 seconds")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func jobState(rc *tool.RunContext, uri, user string, id uint32) (uint32, error) {
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
	u := uri
	if strings.HasPrefix(u, "ipps://") {
		u = "https://" + u[7:]
	} else if strings.HasPrefix(u, "ipp://") {
		u = "http://" + u[6:]
		host := u[7 : strings.IndexAny(u[7:]+"/", "/")+7]
		if !strings.Contains(host, ":") {
			u = "http://" + host + ":631" + u[7+len(host):]
		}
	}
	req, err := http.NewRequestWithContext(rc.Ctx, http.MethodPost, u, bytes.NewReader(b.Bytes()))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/ipp")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK || len(data) < 9 || binary.BigEndian.Uint16(data[2:]) >= 0x0100 {
		return 0, fmt.Errorf("IPP completion query failed")
	}
	for p := 8; p < len(data) && data[p] != 0x03; {
		if data[p] <= 0x06 {
			p++
			continue
		}
		if p+3 > len(data) {
			return 0, fmt.Errorf("truncated IPP completion response")
		}
		tag, nl := data[p], int(binary.BigEndian.Uint16(data[p+1:]))
		nameStart := p + 3
		p = nameStart + nl
		if p+2 > len(data) {
			return 0, fmt.Errorf("truncated IPP completion response")
		}
		vl := int(binary.BigEndian.Uint16(data[p:]))
		if p+2+vl > len(data) {
			return 0, fmt.Errorf("truncated IPP completion response")
		}
		if tag == 0x23 && string(data[nameStart:p]) == "job-state" && vl == 4 {
			return binary.BigEndian.Uint32(data[p+2:]), nil
		}
		p += 2 + vl
	}
	return 0, fmt.Errorf("IPP completion response omitted job-state")
}
