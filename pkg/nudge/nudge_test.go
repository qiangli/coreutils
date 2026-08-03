package nudge

import (
	"bytes"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/interp"
)

func TestSuggestBuiltinAndRouting(t *testing.T) {
	cases := []struct {
		desc      string
		args      []string
		isBuiltin bool
		want      string // substring expected in the suggestion, "" = no hint
	}{
		{"cd", []string{"cd", "/tmp"}, true, "awd DIR"},
		{"pushd", []string{"pushd", "x"}, true, "awd DIR"},
		{"grep recursive", []string{"grep", "-rn", "foo", "."}, false, "--agentic"},
		{"grep recursive suggests ast", []string{"grep", "-r", "Foo", "."}, false, "ast refs"},
		{"grep non-recursive", []string{"grep", "foo", "a.go"}, false, ""},
		{"grep already agentic", []string{"grep", "-r", "--agentic", "foo", "."}, false, ""},
		{"grep already json", []string{"grep", "-r", "--json", "foo", "."}, false, ""},
		{"find", []string{"find", ".", "-name", "*.go"}, false, "ast symbols"},
		{"ls no hint", []string{"ls", "-la"}, false, ""},
	}
	for _, c := range cases {
		got := Suggest(c.args, c.isBuiltin)
		if c.want == "" {
			if got != "" {
				t.Errorf("%s: expected no hint, got %q", c.desc, got)
			}
			continue
		}
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: hint %q does not contain %q", c.desc, got, c.want)
		}
	}
}

func TestOnAuditRateLimitedOncePerTool(t *testing.T) {
	t.Setenv("BASHY_HINTS", "on")
	var buf bytes.Buffer
	n := New(&buf)
	ev := interp.AuditEvent{Args: []string{"cd", "/tmp"}, IsBuiltin: true}
	n.OnAudit(ev)
	n.OnAudit(ev) // second time must be suppressed
	if got := strings.Count(buf.String(), "awd DIR"); got != 1 {
		t.Fatalf("expected exactly one hint, got %d in %q", got, buf.String())
	}
}

func TestOnAuditSilentWhenDisabled(t *testing.T) {
	t.Setenv("BASHY_HINTS", "off")
	var buf bytes.Buffer
	New(&buf).OnAudit(interp.AuditEvent{Args: []string{"cd", "/tmp"}, IsBuiltin: true})
	if buf.Len() != 0 {
		t.Fatalf("expected no output when disabled, got %q", buf.String())
	}
}

func TestSuggestFailureBSDIdioms(t *testing.T) {
	cases := []struct {
		desc string
		args []string
		want string
	}{
		{"BSD stat format", []string{"stat", "-f", "%Sm", "file"}, "-f is --file-system in GNU; BSD's format string is -c/--format"},
		{"GNU stat filesystem", []string{"stat", "-f", "."}, ""},
		{"GNU stat format", []string{"stat", "-c", "%y", "file"}, ""},
		{"BSD sed in-place", []string{"sed", "-i", "", "-e", "s/x/y/", "file"}, "GNU sed takes -i with no argument; BSD's `-i ''` is just `-i`"},
		{"GNU sed in-place", []string{"sed", "-i", "-e", "s/x/y/", "file"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			if got := SuggestFailure(tc.args); got != tc.want {
				t.Fatalf("SuggestFailure(%q) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

func TestOnFailureAgentOutputExact(t *testing.T) {
	var buf bytes.Buffer
	OnFailure(&buf, []string{"stat", "-f", "%Sm", "file"}, []string{"BASHY_AGENTIC=1", "BASHY_HINTS=on"})
	const want = "{\"schema_version\":\"bashy-hint-v1\",\"kind\":\"hint\",\"tool\":\"stat\",\"suggest\":\"-f is --file-system in GNU; BSD's format string is -c/--format\",\"off\":\"BASHY_HINTS=off\"}\n"
	if got := buf.String(); got != want {
		t.Fatalf("OnFailure output:\n got %q\nwant %q", got, want)
	}
}

func TestOnFailureSilentWhenDisabled(t *testing.T) {
	var buf bytes.Buffer
	OnFailure(&buf, []string{"sed", "-i", "", "-e", "s/x/y/", "file"}, []string{"BASHY_AGENTIC=1", "BASHY_HINTS=off"})
	if buf.Len() != 0 {
		t.Fatalf("expected no output when disabled, got %q", buf.String())
	}
}

// The tty-messaging family is deliberately unimplemented, so an agent that
// reaches for it must be told where the working equivalent is rather than just
// getting "command not found".
func TestSuggest_TtyMessagingPointsAtTheBoard(t *testing.T) {
	for _, name := range []string{"wall", "write", "mesg", "talk"} {
		got := Suggest([]string{name, "someone", "hi"}, false)
		if got == "" {
			t.Errorf("%s: no hint; an unimplemented tool must name its replacement", name)
			continue
		}
		if !strings.Contains(got, "bashy mb") {
			t.Errorf("%s: hint does not point at the board: %s", name, got)
		}
	}
	// A tool that IS implemented must not be annotated.
	if got := Suggest([]string{"ls", "-l"}, false); got != "" {
		t.Errorf("ls must not be hinted: %s", got)
	}
}
