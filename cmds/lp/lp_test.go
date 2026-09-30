package lpcmd

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

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

func TestMailSubscription(t *testing.T) {
	srv, got := stub(t, 0, 5)
	env := []string{"LP_IPP_URI=" + srv.URL, "LPDEST=d", "USER=bob"}
	if _, e, c := runLP(t, env, "x"); c != 0 {
		t.Fatal(c, e)
	}
	if len((*got)[0].sub) != 0 {
		t.Fatalf("subscription without -m: %v", (*got)[0].sub)
	}
	if _, e, c := runLP(t, env, "x", "-m"); c != 0 {
		t.Fatal(c, e)
	}
	s := (*got)[1].sub
	if string(s["notify-recipient-uri"]) != "mailto:bob@localhost" || string(s["notify-events"]) != "job-completed" {
		t.Fatalf("bad subscription %v", s)
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
	if c := cmd.Run(rc, []string{"-m", "-n", "2", "a", "b"}); c != 0 {
		t.Fatal(c, errb.String())
	}
	if out.String() != "request id is d-77 (2 file(s))\n" {
		t.Fatalf("out %q", out.String())
	}
	g := *got
	if len(g) != 3 || g[0].op != 5 || g[1].op != 6 || g[2].op != 6 {
		t.Fatalf("ops %+v", g)
	}
	if string(g[0].sub["notify-events"]) != "job-completed" || binary.BigEndian.Uint32(g[0].attrs["copies"]) != 2 {
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
