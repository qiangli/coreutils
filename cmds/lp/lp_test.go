package lpcmd

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

type ippReq struct {
	op    uint16
	attrs map[string][]byte
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
		req := ippReq{op: binary.BigEndian.Uint16(b[2:]), attrs: map[string][]byte{}}
		p := 8
		for b[p] != 0x03 {
			if b[p] <= 0x05 {
				p++
				continue
			}
			nl := int(binary.BigEndian.Uint16(b[p+1:]))
			name := string(b[p+3 : p+3+nl])
			p += 3 + nl
			vl := int(binary.BigEndian.Uint16(b[p:]))
			req.attrs[name] = b[p+2 : p+2+vl]
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
	if _, _, c := runLP(t, env, "x", "-m"); c != 2 {
		t.Fatal(c)
	}
	if _, _, c := runLP(t, env, "x", "--bogus"); c != 2 {
		t.Fatal(c)
	}
}
