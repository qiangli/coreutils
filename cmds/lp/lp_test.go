package lpcmd

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/qiangli/coreutils/cmds/internal/session"
	"github.com/qiangli/coreutils/pkg/mailx"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

type ippReq struct {
	op    uint16
	attrs map[string][]byte
	sub   map[string][]byte // subscription group (0x06)
	data  []byte
}

// stub decodes the IPP request and answers with the given status/job-id.
func stub(t *testing.T, status uint16, jobID uint32) (*httptest.Server, *[]ippReq) {
	var mu sync.Mutex
	var got []ippReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/ipp" {
			t.Errorf("content-type %q", r.Header.Get("Content-Type"))
		}
		b, _ := io.ReadAll(r.Body)
		req := ippReq{op: binary.BigEndian.Uint16(b[2:]), attrs: map[string][]byte{}, sub: map[string][]byte{}}
		p := 8
		cur := req.attrs
		for b[p] != 0x03 {
			if b[p] <= 0x06 {
				if b[p] == 0x06 {
					cur = req.sub
				}
				p++
				continue
			}
			nl := int(binary.BigEndian.Uint16(b[p+1:]))
			name := string(b[p+3 : p+3+nl])
			p += 3 + nl
			vl := int(binary.BigEndian.Uint16(b[p:]))
			cur[name] = b[p+2 : p+2+vl]
			p += 2 + vl
		}
		req.data = b[p+1:]
		mu.Lock()
		got = append(got, req)
		mu.Unlock()
		var o bytes.Buffer
		o.Write([]byte{1, 1, byte(status >> 8), byte(status), 0, 0, 0, 1, 0x01})
		o.Write([]byte{0x47, 0, 18})
		o.WriteString("attributes-charset")
		o.Write([]byte{0, 5})
		o.WriteString("utf-8")
		o.WriteByte(0x02)
		o.Write([]byte{0x21, 0, 6})
		o.WriteString("job-id")
		o.Write([]byte{0, 4, byte(jobID >> 24), byte(jobID >> 16), byte(jobID >> 8), byte(jobID)})
		if req.op == opGetJobAttrs {
			o.Write([]byte{0x23, 0, 9})
			o.WriteString("job-state")
			o.Write([]byte{0, 4, 0, 0, 0, 9})
		}
		o.WriteByte(0x03)
		w.Header().Set("Content-Type", "application/ipp")
		w.Write(o.Bytes())
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func runLP(t *testing.T, env []string, stdin string, args ...string) (string, string, int) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(), Env: env,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	c := cmd.Run(rc, args)
	return out.String(), errb.String(), c
}

func TestPrintStdinDefaultsAndOptions(t *testing.T) {
	srv, got := stub(t, 0, 42)
	out, _, c := runLP(t, []string{"LP_IPP_URI=" + srv.URL + "/printers/x", "LPDEST=office"}, "hello", "-n", "3", "-t", "T1")
	if c != 0 || out != "request id is office-42 (1 file(s))\n" {
		t.Fatalf("code %d out %q", c, out)
	}
	r := (*got)[0]
	if r.op != 2 || string(r.data) != "hello" || string(r.attrs["job-name"]) != "T1" ||
		binary.BigEndian.Uint32(r.attrs["copies"]) != 3 || string(r.attrs["attributes-charset"]) != "utf-8" ||
		!strings.HasSuffix(string(r.attrs["printer-uri"]), "/printers/x") || r.attrs["requesting-user-name"] == nil ||
		string(r.attrs["document-format"]) != "application/octet-stream" {
		t.Fatalf("bad request %+v", r.attrs)
	}
}

func TestPrinterFallbackAndDestPrecedence(t *testing.T) {
	srv, _ := stub(t, 0, 7)
	env := []string{"LP_IPP_URI=" + srv.URL, "PRINTER=pp", "LPDEST=ld"}
	if out, _, _ := runLP(t, env, "x"); out != "request id is ld-7 (1 file(s))\n" {
		t.Fatal(out)
	}
	if out, _, _ := runLP(t, env[:2], "x"); out != "request id is pp-7 (1 file(s))\n" {
		t.Fatal(out)
	}
	if out, _, _ := runLP(t, env, "x", "-d", "dd"); out != "request id is dd-7 (1 file(s))\n" {
		t.Fatal(out)
	}
}

func TestSilentAndOptionAndFile(t *testing.T) {
	srv, got := stub(t, 1, 9) // successful-ok-ignored-or-substituted-attributes
	env := []string{"LP_IPP_URI=" + srv.URL, "LPDEST=d"}
	out, _, c := runLP(t, env, "", "-s", "-o", "sides=two-sided-long-edge", "-c", "/dev/null")
	if c != 0 || out != "" {
		t.Fatalf("%d %q", c, out)
	}
	if string((*got)[0].attrs["sides"]) != "two-sided-long-edge" {
		t.Fatal((*got)[0].attrs)
	}
}

func TestErrors(t *testing.T) {
	srv, _ := stub(t, 0x0400, 0) // client-error-bad-request
	env := []string{"LP_IPP_URI=" + srv.URL, "LPDEST=d"}
	if _, e, c := runLP(t, env, "x"); c != 1 || !strings.Contains(e, "lp:") {
		t.Fatalf("%d %q", c, e)
	}
	if _, _, c := runLP(t, []string{"LP_IPP_URI=" + srv.URL}, "x"); c != 1 { // no destination
		t.Fatal(c)
	}
	if _, _, c := runLP(t, env, "x", "-n", "0"); c != 2 {
		t.Fatal(c)
	}
	if _, _, c := runLP(t, env, "x", "/nonexistent/f"); c != 1 {
		t.Fatal(c)
	}
	if _, _, c := runLP(t, env, "x", "--bogus"); c != 2 {
		t.Fatal(c)
	}
}

func TestCompletionDelivery(t *testing.T) {
	srv, got := stub(t, 0, 5)
	oldRead, oldWrite, oldMail := readSessions, writeTerminal, deliverMail
	t.Cleanup(func() { readSessions, writeTerminal, deliverMail = oldRead, oldWrite, oldMail })
	var terminal, mails []string
	readSessions = func([]string) ([]session.Record, error) {
		return []session.Record{{User: "bob", TTY: "pts/7", Type: "USER_PROCESS"}}, nil
	}
	writeTerminal = func(tty, msg string) error { terminal = append(terminal, tty+":"+msg); return nil }
	deliverMail = func(_ *tool.RunContext, user, msg string) error { mails = append(mails, user+":"+msg); return nil }
	env := []string{"LP_IPP_URI=" + srv.URL, "LPDEST=d", "USER=bob"}
	out, e, c := runLP(t, env, "x", "-mw")
	if c != 0 || e != "" || out != "request id is d-5 (1 file(s))\n" {
		t.Fatalf("code=%d out=%q err=%q", c, out, e)
	}
	if len(terminal) != 1 || terminal[0] != "pts/7:request d-5 completed\n" || len(mails) != 1 {
		t.Fatalf("terminal=%v mail=%v", terminal, mails)
	}
	if len(*got) != 2 || (*got)[1].op != opGetJobAttrs || len((*got)[0].sub) != 0 {
		t.Fatalf("requests=%+v", *got)
	}
	terminal = nil
	mails = nil
	readSessions = func([]string) ([]session.Record, error) { return nil, nil }
	_, e, c = runLP(t, env, "x", "-w")
	if c != 0 || e != "" || len(mails) != 1 || len(terminal) != 0 {
		t.Fatalf("fallback code=%d err=%q terminal=%v mail=%v", c, e, terminal, mails)
	}
	mails = nil
	_, e, c = runLP(t, env, "x", "-mw")
	if c != 0 || e != "" || len(mails) != 1 {
		t.Fatalf("combined fallback code=%d err=%q mail=%v", c, e, mails)
	}
	terminal = nil
	mails = nil
	readSessions = func([]string) ([]session.Record, error) { return nil, fmt.Errorf("database broken") }
	_, e, c = runLP(t, env, "x", "-w")
	if c == 0 || !strings.Contains(e, "database broken") || len(mails) != 0 {
		t.Fatalf("lookup failure code=%d err=%q mail=%v", c, e, mails)
	}
	_, e, c = runLP(t, env, "x", "-mw")
	if c == 0 || !strings.Contains(e, "database broken") || len(mails) != 1 {
		t.Fatalf("explicit mail on lookup failure code=%d err=%q mail=%v", c, e, mails)
	}
}

func TestCompletionWriteFailure(t *testing.T) {
	srv, _ := stub(t, 0, 5)
	oldRead, oldWrite := readSessions, writeTerminal
	t.Cleanup(func() { readSessions, writeTerminal = oldRead, oldWrite })
	readSessions = func([]string) ([]session.Record, error) {
		return []session.Record{{User: "bob", TTY: "pts/7", Type: "USER_PROCESS"}}, nil
	}
	writeTerminal = func(string, string) error { return io.ErrShortWrite }
	_, err, code := runLP(t, []string{"LP_IPP_URI=" + srv.URL, "LPDEST=d", "USER=bob"}, "x", "-w")
	if code == 0 || !strings.Contains(err, "short write") {
		t.Fatalf("code=%d err=%q", code, err)
	}
}

func TestCompletionMailSpool(t *testing.T) {
	srv, _ := stub(t, 0, 5)
	path := filepath.Join(t.TempDir(), "mailbox")
	_, err, code := runLP(t, []string{"LP_IPP_URI=" + srv.URL, "LPDEST=d", "USER=bob", "MAIL=" + path}, "x", "-m")
	if code != 0 {
		t.Fatal(code, err)
	}
	entries, e := mailx.ReadMbox(path)
	if e != nil || len(entries) != 1 || string(entries[0].Message.Body) != "request d-5 completed\n" {
		t.Fatalf("entries=%v err=%v", entries, e)
	}
}

func TestMalformedIPPFrames(t *testing.T) {
	valid := []byte{1, 1, 0, 0, 0, 0, 0, 1, 2, 0x21, 0, 6, 'j', 'o', 'b', '-', 'i', 'd', 0, 4, 0, 0, 0, 5, 3}
	cases := [][]byte{
		{1, 1, 0, 0, 0, 0, 0, 1, 3},              // missing id
		valid[:len(valid)-1],                     // no end tag
		append(append([]byte(nil), valid...), 0), // trailing byte
		{1, 1, 0, 0, 0, 0, 0, 1, 2, 0x21, 0, 6, 'j', 'o', 'b', '-', 'i', 'd', 0, 4, 0, 0, 0, 0, 3},
		{1, 1, 0, 0, 0, 0, 0, 1, 2, 0x21, 0, 6, 'j', 'o', 'b', '-', 'i', 'd', 0, 4, 0x80, 0, 0, 1, 3},
		{1, 1, 0, 0, 0, 0, 0, 1, 2, 0x21, 0, 6, 'j', 'o', 'b', '-', 'i', 'd', 0, 4, 0, 0, 0},
	}
	for i, body := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
		_, _, code := runLP(t, []string{"LP_IPP_URI=" + srv.URL, "LPDEST=d"}, "x")
		srv.Close()
		if code == 0 {
			t.Fatalf("case %d accepted malformed response", i)
		}
	}
}

func TestIPPSubscriptionGroupAndCompleteStateFrame(t *testing.T) {
	var frame bytes.Buffer
	frame.Write([]byte{1, 1, 0, 0, 0, 0, 0, 1, 0x02})
	attr(&frame, 0x21, "job-id", []byte{0, 0, 0, 5})
	frame.WriteByte(0x06)
	attr(&frame, 0x21, "notify-subscription-id", []byte{0, 0, 0, 9})
	frame.WriteByte(0x03)
	attrs, err := parseIPP(frame.Bytes())
	if err != nil || len(attrs) != 2 || attrs[1].group != 0x06 {
		t.Fatalf("group parse attributes=%v error=%v", attrs, err)
	}
	if id, err := positiveID(attrs); err != nil || id != 5 {
		t.Fatalf("job id %d error %v", id, err)
	}
	frame.WriteByte(0)
	if _, err := parseIPP(frame.Bytes()); err == nil {
		t.Fatal("accepted bytes after end tag")
	}
}

func TestDuplicateResponseValuesAreRejected(t *testing.T) {
	for _, name := range []string{"job-id", "job-state"} {
		t.Run(name, func(t *testing.T) {
			var frame bytes.Buffer
			frame.Write([]byte{1, 1, 0, 0, 0, 0, 0, 1, 0x02})
			tag := byte(0x21)
			if name == "job-state" {
				tag = 0x23
			}
			attr(&frame, tag, name, []byte{0, 0, 0, 5})
			attr(&frame, tag, name, []byte{0, 0, 0, 9})
			frame.WriteByte(0x03)
			attrs, err := parseIPP(frame.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			if name == "job-id" {
				if _, err := positiveID(attrs); err == nil {
					t.Fatal("accepted ambiguous job-id")
				}
			} else {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write(frame.Bytes())
				}))
				defer srv.Close()
				if _, err := jobState(context.Background(), srv.URL, "bob", 5); err == nil {
					t.Fatal("accepted ambiguous job-state")
				}
			}
		})
	}
}

func TestCompletionWaitsForActualCompletedState(t *testing.T) {
	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var response bytes.Buffer
		response.Write([]byte{1, 1, 0, 0, 0, 0, 0, 1, 0x01})
		attr(&response, 0x47, "attributes-charset", []byte("utf-8"))
		response.WriteByte(0x02)
		if binary.BigEndian.Uint16(body[2:]) == opGetJobAttrs {
			state := byte(5) // processing
			if polls.Add(1) > 1 {
				state = 9 // completed
			}
			attr(&response, 0x23, "job-state", []byte{0, 0, 0, state})
		} else {
			attr(&response, 0x21, "job-id", []byte{0, 0, 0, 5})
		}
		response.WriteByte(0x03)
		_, _ = w.Write(response.Bytes())
	}))
	defer srv.Close()
	oldMail := deliverMail
	t.Cleanup(func() { deliverMail = oldMail })
	deliverMail = func(_ *tool.RunContext, _, _ string) error {
		if polls.Load() != 2 {
			t.Errorf("mail delivered before completed state; polls=%d", polls.Load())
		}
		return nil
	}
	_, diagnostic, code := runLP(t, []string{"LP_IPP_URI=" + srv.URL, "LPDEST=d", "USER=bob"}, "x", "-m")
	if code != 0 || diagnostic != "" || polls.Load() != 2 {
		t.Fatalf("code=%d diagnostic=%q polls=%d", code, diagnostic, polls.Load())
	}
}

func TestBlockedCompletionHTTPIsCanceled(t *testing.T) {
	for _, phase := range []string{"headers", "body"} {
		t.Run(phase, func(t *testing.T) {
			entered := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if binary.BigEndian.Uint16(body[2:]) == opGetJobAttrs {
					if phase == "body" {
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte{1, 1, 0, 0})
						w.(http.Flusher).Flush()
					}
					close(entered)
					<-r.Context().Done()
					return
				}
				_, _ = w.Write([]byte{1, 1, 0, 0, 0, 0, 0, 1, 2, 0x21, 0, 6, 'j', 'o', 'b', '-', 'i', 'd', 0, 4, 0, 0, 0, 5, 3})
			}))
			defer srv.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var out, errb bytes.Buffer
			rc := &tool.RunContext{Ctx: ctx, Dir: t.TempDir(), Env: []string{"LP_IPP_URI=" + srv.URL, "LPDEST=d", "USER=bob"}, Stdio: tool.Stdio{In: strings.NewReader("x"), Out: &out, Err: &errb}}
			done := make(chan int, 1)
			go func() { done <- cmd.Run(rc, []string{"-m"}) }()
			<-entered
			cancel()
			select {
			case code := <-done:
				if code == 0 {
					t.Fatal("accepted canceled HTTP response")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("HTTP completion query ignored cancellation")
			}
		})
	}
}

func TestMultiFileIsOneRequest(t *testing.T) {
	srv, got := stub(t, 0, 77)
	env := []string{"LP_IPP_URI=" + srv.URL, "LPDEST=d"}
	dir := t.TempDir()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb}}
	for n, c := range map[string]string{"a": "AAA", "b": "BBB"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if c := cmd.Run(rc, []string{"-n", "2", "a", "b"}); c != 0 {
		t.Fatal(c, errb.String())
	}
	if out.String() != "request id is d-77 (2 file(s))\n" {
		t.Fatalf("out %q", out.String())
	}
	g := *got
	if len(g) != 3 || g[0].op != 5 || g[1].op != 6 || g[2].op != 6 {
		t.Fatalf("ops %+v", g)
	}
	if len(g[0].sub) != 0 || binary.BigEndian.Uint32(g[0].attrs["copies"]) != 2 {
		t.Fatalf("create-job %+v", g[0])
	}
	for i, want := range []string{"AAA", "BBB"} {
		r := g[i+1]
		if binary.BigEndian.Uint32(r.attrs["job-id"]) != 77 || string(r.data) != want {
			t.Fatalf("send-document %d: %+v", i, r)
		}
		if last := r.attrs["last-document"]; len(last) != 1 || (last[0] == 1) != (i == 1) {
			t.Fatalf("last-document %d: %v", i, last)
		}
	}
}

func TestStdinDashIsOneDocument(t *testing.T) {
	srv, got := stub(t, 0, 3)
	env := []string{"LP_IPP_URI=" + srv.URL, "LPDEST=d"}
	out, _, c := runLP(t, env, "piped", "-t", "Title", "-n", "4", "-")
	if c != 0 || out != "request id is d-3 (1 file(s))\n" {
		t.Fatal(c, out)
	}
	g := *got
	if len(g) != 1 || g[0].op != 2 || string(g[0].data) != "piped" ||
		string(g[0].attrs["job-name"]) != "Title" || binary.BigEndian.Uint32(g[0].attrs["copies"]) != 4 {
		t.Fatalf("%+v", g)
	}
	if _, _, c := runLP(t, env, "zzz"); c != 0 || string((*got)[1].attrs["job-name"]) != "stdin" {
		t.Fatalf("default title: %+v", (*got)[1].attrs)
	}
}

func TestCompletionTerminalStates(t *testing.T) {
	for _, state := range []byte{7, 8} {
		t.Run(fmt.Sprint(state), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var response bytes.Buffer
				response.Write([]byte{1, 1, 0, 0, 0, 0, 0, 1, 0x02})
				if binary.BigEndian.Uint16(body[2:]) == opGetJobAttrs {
					attr(&response, 0x23, "job-state", []byte{0, 0, 0, state})
				} else {
					attr(&response, 0x21, "job-id", []byte{0, 0, 0, 5})
				}
				response.WriteByte(0x03)
				_, _ = w.Write(response.Bytes())
			}))
			defer srv.Close()
			_, diagnostic, code := runLP(t, []string{"LP_IPP_URI=" + srv.URL, "LPDEST=d", "USER=bob"}, "x", "-m")
			if code == 0 || !strings.Contains(diagnostic, map[byte]string{7: "canceled", 8: "aborted"}[state]) {
				t.Fatalf("state %d code %d diagnostic %q", state, code, diagnostic)
			}
		})
	}
}

func TestSendDocumentRequiresResponseJobID(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := count.Add(1)
		var response bytes.Buffer
		response.Write([]byte{1, 1, 0, 0, 0, 0, 0, 1, 0x02})
		if request == 1 {
			attr(&response, 0x21, "job-id", []byte{0, 0, 0, 5})
		}
		response.WriteByte(0x03)
		_, _ = w.Write(response.Bytes())
	}))
	defer srv.Close()
	dir := t.TempDir()
	for _, file := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(file), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"LP_IPP_URI=" + srv.URL, "LPDEST=d"}, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb}}
	if code := cmd.Run(rc, []string{"a", "b"}); code == 0 || count.Load() != 2 {
		t.Fatalf("code %d requests %d stderr %q", code, count.Load(), errb.String())
	}
}

func TestTerminalPathRequiresDevice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ordinary")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeTerminal(path, "message"); err == nil {
		t.Fatal("ordinary file accepted as terminal")
	}
}
