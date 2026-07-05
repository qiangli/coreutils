package grepcmd

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func benchData() []byte {
	levels := []string{"INFO", "WARN", "ERROR", "DEBUG", "TRACE"}
	var b bytes.Buffer
	for i := 0; b.Len() < 10<<20; i++ {
		b.WriteString("0000000042 ")
		b.WriteString(levels[i%len(levels)])
		b.WriteString(" tok123 tok456 some log message text here\n")
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
		cmd.Run(rc, args) // grep exit: 0 = matches found, 1 = none (both fine)
	}
}

// BenchmarkGrepLiteral is the target: a PLAIN LITERAL pattern over 10 MB.
// GNU grep skips the regex engine for literals (memchr + Boyer-Moore); bashy
// currently runs everything through RE2 (~2× slower). Add a literal fast path
// (detect no-regex-metachars → bytes.Index per line) + fast line iteration,
// keeping every grep_test.go case byte-identical.
func BenchmarkGrepLiteral(b *testing.B) { runBench(b, "ERROR") }
func BenchmarkGrepCount(b *testing.B)   { runBench(b, "-c", "ERROR") }
func BenchmarkGrepV(b *testing.B)       { runBench(b, "-v", "ERROR") }
