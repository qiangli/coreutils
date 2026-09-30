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
// -m (mail) is not supported. One Print-Job is sent per file operand;
// the reported request id is that of the last job.
package lpcmd

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "lp",
	Synopsis: "Submit files to a printer over IPP.",
	Usage: "lp [-c] [-d dest] [-n copies] [-o option]... [-s] [-t title] [file...]\n" +
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
	mail := fs.BoolP("m", "m", false, "send mail after printing (not supported)")
	copies := fs.IntP("n", "n", 1, "number of copies")
	var opts []string
	fs.StringArrayVarP(&opts, "o", "o", nil, "printer-specific option")
	silent := fs.BoolP("s", "s", false, "suppress the request-id message")
	title := fs.StringP("t", "t", "", "job title")
	files, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if *mail {
		return tool.NotSupported(rc, cmd, "-m")
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
	id := uint32(0)
	for _, f := range files {
		var data []byte
		var err error
		name := f
		if f == "-" {
			name = "stdin"
			data, err = io.ReadAll(rc.In)
		} else {
			data, err = os.ReadFile(rc.Path(f))
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "lp: %s: %v\n", f, err)
			return 1
		}
		job := *title
		if job == "" {
			job = name
		}
		user := rc.Getenv("USER")
		if user == "" {
			user = rc.Getenv("LOGNAME")
		}
		if user == "" {
			user = "anonymous"
		}
		req := encode(uri, user, job, *copies, opts, data)
		id, err = send(rc, uri, req)
		if err != nil {
			fmt.Fprintf(rc.Err, "lp: %v\n", err)
			return 1
		}
	}
	if !*silent {
		fmt.Fprintf(rc.Out, "request id is %s-%d (%d file(s))\n", d, id, len(files))
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

// encode builds an IPP/1.1 Print-Job (0x0002) request, request-id 1.
func encode(uri, user, job string, copies int, opts []string, doc []byte) []byte {
	var b bytes.Buffer
	b.Write([]byte{1, 1, 0, 2, 0, 0, 0, 1, 0x01})
	attr(&b, 0x47, "attributes-charset", []byte("utf-8"))
	attr(&b, 0x48, "attributes-natural-language", []byte("en"))
	attr(&b, 0x45, "printer-uri", []byte(uri))
	attr(&b, 0x42, "requesting-user-name", []byte(user))
	attr(&b, 0x42, "job-name", []byte(job))
	attr(&b, 0x49, "document-format", []byte("application/octet-stream"))
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
	b.WriteByte(0x03)
	b.Write(doc)
	return b.Bytes()
}

func send(rc *tool.RunContext, uri string, body []byte) (uint32, error) {
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
	p := 8
	for p < len(b) && b[p] != 0x03 {
		if b[p] <= 0x05 {
			p++
			continue
		}
		if p+3 > len(b) {
			break
		}
		tag := b[p]
		nl := int(binary.BigEndian.Uint16(b[p+1:]))
		if p+5+nl > len(b) {
			break
		}
		name := string(b[p+3 : p+3+nl])
		p += 3 + nl
		vl := int(binary.BigEndian.Uint16(b[p:]))
		if p+2+vl > len(b) {
			break
		}
		if tag == 0x21 && name == "job-id" && vl == 4 {
			id = binary.BigEndian.Uint32(b[p+2:])
		}
		p += 2 + vl
	}
	return id, nil
}
