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
