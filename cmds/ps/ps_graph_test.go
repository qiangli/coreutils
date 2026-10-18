package pscmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestProcessGraphStream(t *testing.T) {
	procs := []process{
		{pid: 20, ppid: 1, command: "second", args: "second -v", tty: "?"},
		{pid: 3, ppid: 1, command: "first", args: "first", tty: "pts/1"},
	}
	encode := func(ps []process) []byte {
		t.Helper()
		var b bytes.Buffer
		if err := writeProcessGraph(&b, ps); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	first := encode(procs)
	if !bytes.Equal(first, encode([]process{procs[1], procs[0]})) {
		t.Fatal("process enumeration order changed the stream")
	}
	lines := bytes.Split(bytes.TrimSpace(first), []byte{'\n'})
	if len(lines) != 7 { // header, roster, two processes, two edges, cursor
		t.Fatalf("want seven records, got %d: %s", len(lines), first)
	}
	var raw map[string]any
	if err := json.Unmarshal(lines[0], &raw); err != nil {
		t.Fatal(err)
	}
	if raw["t"] != "header" || raw["schema_version"] != "bashy-graph-v1" || raw["source"] != processGraphSource {
		t.Fatalf("invalid header: %s", lines[0])
	}
	body := append([]byte(nil), bytes.Join(lines[1:6], []byte{'\n'})...)
	body = append(body, '\n')
	sum := sha256.Sum256(body)
	rev := "sha256:" + hex.EncodeToString(sum[:])
	if raw["rev"] != rev {
		t.Fatalf("revision does not hash body: %s", first)
	}
	if !strings.Contains(string(lines[2]), `"ref":"proc:20"`) || !strings.Contains(string(lines[3]), `"ref":"proc:3"`) {
		t.Fatalf("nodes are not in canonical ref order: %s", first)
	}
	if err := json.Unmarshal(lines[6], &raw); err != nil {
		t.Fatal(err)
	}
	cursor, ok := raw["cursor"].(map[string]any)
	if raw["t"] != "cursor" || !ok || cursor["schema_version"] != "bashy-cursor-v1" {
		t.Fatalf("invalid cursor: %s", lines[6])
	}
	binding, ok := cursor["binding"].(map[string]any)
	if !ok || binding["revision"] != rev || binding["target"] != processGraphSource {
		t.Fatalf("cursor is not bound to the snapshot: %s", lines[6])
	}
}
