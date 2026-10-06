package tool

import (
	"encoding/json"
	"testing"
)

func TestDocumentAsMCPLegacyJSON(t *testing.T) {
	got, err := json.Marshal(DocumentAsMCP("cd", "cd [-L|-P] [dir] — change the working directory"))
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"tool":{"name":"cd","description":"cd [-L|-P] [dir] — change the working directory","inputSchema":{"additionalProperties":false,"properties":{"args":{"description":"Command arguments in order, including options and operands.","items":{"type":"string"},"type":"array"}},"type":"object"}},"call":{"jsonrpc":"2.0","method":"tools/call","params":{"name":"cd","arguments":{"args":[]}}}}`
	if string(got) != want {
		t.Fatalf("legacy JSON changed:\ngot  %s\nwant %s", got, want)
	}
}

func TestApplyEffects(t *testing.T) {
	for _, tt := range []struct {
		name                  string
		effects               []string
		readOnly, destructive bool
	}{
		{"empty", nil, true, false}, {"pure", []string{"pure"}, true, false},
		{"read", []string{"pure", "read"}, true, false}, {"write", []string{"read", "write"}, false, false},
		{"destroy", []string{"destroy", "read"}, false, true}, {"unknown", []string{"unknown"}, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := DocumentAsMCP("test", "test").Tool
			d.ApplyEffects([]string{"destroy"}, nil)
			d.Meta["other"] = "kept"
			d.Annotations.Title = "title"
			d.ApplyEffects(tt.effects, []string{"linux"})
			if *d.Annotations.ReadOnlyHint != tt.readOnly || *d.Annotations.DestructiveHint != tt.destructive {
				t.Fatalf("wrong hints: %+v", d.Annotations)
			}
			if d.Meta["other"] != "kept" || d.Annotations.Title != "title" {
				t.Fatal("lost existing metadata")
			}
			got, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			var wire struct {
				Annotations map[string]any `json:"annotations"`
				Meta        map[string]any `json:"_meta"`
			}
			if err := json.Unmarshal(got, &wire); err != nil {
				t.Fatal(err)
			}
			if wire.Annotations["readOnlyHint"] != tt.readOnly || wire.Annotations["destructiveHint"] != tt.destructive {
				t.Fatalf("lost explicit booleans: %s", got)
			}
			effects, _ := json.Marshal(wire.Meta["bashy.effects"])
			want, _ := json.Marshal(tt.effects)
			if string(effects) != string(want) {
				t.Fatalf("effects %s want %s", effects, want)
			}
			if wire.Meta["bashy.os"].([]any)[0] != "linux" {
				t.Fatalf("OS metadata: %s", got)
			}
		})
	}
}

func TestDocumentAsMCPSchema(t *testing.T) {
	s := ArgSchema{Positionals: []ArgParameter{{Name: "path", Type: "string", Required: true}}}
	d := DocumentAsMCPSchema("cat", "read files", s)
	got, err := json.Marshal(d.Tool.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(s.JSONSchema())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("schema %s want %s", got, want)
	}
	if d.Tool.Name != "cat" || d.Tool.Description != "read files" || d.Call.JSONRPC != "2.0" || d.Call.Method != "tools/call" || d.Call.Params.Name != "cat" {
		t.Fatalf("bad documentation: %+v", d)
	}
	if len(d.Call.Params.Arguments) != 0 || d.Call.Params.Arguments == nil {
		t.Fatalf("bad example arguments: %#v", d.Call.Params.Arguments)
	}
}
