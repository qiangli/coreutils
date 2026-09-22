package mkfifocmd

import (
	"strings"
	"testing"
)

// The marker grammar is the wire contract with the shell's Windows open path
// (docs/windows-fifo.md); these tests run on every host on purpose.

func TestNewPipeLeafFormat(t *testing.T) {
	a, err := NewPipeLeaf()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewPipeLeaf()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatalf("two leaves collided: %q", a)
	}
	for _, leaf := range []string{a, b} {
		if !strings.HasPrefix(leaf, "bashy-fifo-") {
			t.Fatalf("leaf %q lacks the bashy-fifo- prefix", leaf)
		}
		token := strings.TrimPrefix(leaf, "bashy-fifo-")
		if len(token) != 32 {
			t.Fatalf("leaf %q token length %d, want 32", leaf, len(token))
		}
		for i := 0; i < len(token); i++ {
			c := token[i]
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				t.Fatalf("leaf %q has non-lowercase-hex byte %q", leaf, c)
			}
		}
	}
}

func TestMarkerRoundTrip(t *testing.T) {
	leaf, err := NewPipeLeaf()
	if err != nil {
		t.Fatal(err)
	}
	content := EncodeMarker(leaf)
	// docs/windows-fifo.md: a v1 marker is exactly 57 bytes, two LF lines.
	if len(content) != 57 {
		t.Fatalf("marker is %d bytes, want 57: %q", len(content), content)
	}
	if got := string(content); got != "!<bashyfifo>\n"+leaf+"\n" {
		t.Fatalf("marker bytes %q", got)
	}
	back, ok := ParseMarker(content)
	if !ok || back != leaf {
		t.Fatalf("ParseMarker(EncodeMarker(%q)) = %q, %v", leaf, back, ok)
	}
}

func TestPipePath(t *testing.T) {
	const leaf = "bashy-fifo-0123456789abcdef0123456789abcdef"
	if got := PipePath(leaf); got != `\\.\pipe\`+leaf {
		t.Fatalf("PipePath = %q", got)
	}
}

func TestParseMarkerRejects(t *testing.T) {
	const goodLeaf = "bashy-fifo-0123456789abcdef0123456789abcdef"
	good := string(EncodeMarker(goodLeaf))
	cases := []struct {
		name, content string
	}{
		{"empty", ""},
		{"magic only", "!<bashyfifo>\n"},
		{"wrong magic", "!<symlink>\n" + goodLeaf + "\n"},
		{"missing final newline", strings.TrimSuffix(good, "\n")},
		{"crlf lines", strings.ReplaceAll(good, "\n", "\r\n")},
		{"trailing junk", good + "x"},
		{"third line", good + "extra\n"},
		{"uppercase hex", "!<bashyfifo>\nbashy-fifo-0123456789ABCDEF0123456789ABCDEF\n"},
		{"token too short", "!<bashyfifo>\nbashy-fifo-0123456789abcdef\n"},
		{"token too long", "!<bashyfifo>\n" + goodLeaf + "00\n"},
		{"wrong prefix", "!<bashyfifo>\nsh-np-0123456789abcdef0123456789abcdef\n"},
		// The security cases: a leaf must never be able to escape the
		// bashy-fifo- pipe namespace.
		{"path separator", "!<bashyfifo>\nbashy-fifo-..\\0123456789abcdef0123456789abcd\n"},
		{"absolute pipe", "!<bashyfifo>\n\\\\.\\pipe\\lsass\n"},
		{"oversize", "!<bashyfifo>\n" + strings.Repeat("a", 200) + "\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if leaf, ok := ParseMarker([]byte(tc.content)); ok {
				t.Fatalf("ParseMarker accepted %q as leaf %q", tc.content, leaf)
			}
		})
	}
}
