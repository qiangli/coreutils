package sedcmd

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func benchData() []byte {
	var b bytes.Buffer
	line := []byte("0000000042 INFO tok123 tok456 some log message text here\n")
	for b.Len() < 10<<20 {
		b.Write(line)
	}
	return b.Bytes()
}

func runBench(b *testing.B, args ...string) {
	data := benchData()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			FS:    tool.NewLocalFS(),
			Stdio: tool.Stdio{In: bytes.NewReader(data), Out: io.Discard, Err: io.Discard},
		}
		if code := cmd.Run(rc, args); code != 0 {
			b.Fatalf("exit %d", code)
		}
	}
}

// BenchmarkSedSubst is the target: a global substitution over 10 MB (~1.8×
// slower than GNU sed). Add a fast path for simple `s///` (literal or single
// regex, no addresses) + buffered line I/O, keeping every sed_test.go case
// byte-identical.
func BenchmarkSedSubst(b *testing.B)   { runBench(b, "s/tok/TOK/g") }
func BenchmarkSedLiteral(b *testing.B) { runBench(b, "s/message/MSG/") }
