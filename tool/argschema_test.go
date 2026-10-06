package tool

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestArgSchemaJSONSchema(t *testing.T) {
	for _, tt := range []struct {
		name   string
		schema ArgSchema
		want   string
	}{
		{"empty", ArgSchema{}, `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{},"required":[],"additionalProperties":false,"x-bashy-positionals":[],"x-bashy-shorthand":{}}`},
		{"typed", ArgSchema{
			Positionals: []ArgParameter{{Name: "path", Type: "string", Required: true, Default: "here", Enum: []string{"here", "there"}}, {Name: "count", Type: "int", Default: "2", Enum: []string{"1", "2"}}},
			Flags:       []ArgFlag{{Name: "ratio", Shorthand: "r", Type: "float", Required: true, Default: "1e-2", Enum: []string{"0.01", "2"}}, {Name: "verbose", Shorthand: "v", Type: "bool", Default: "false", Enum: []string{"true", "false"}}},
		}, `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"path":{"type":"string","default":"here","enum":["here","there"]},"count":{"type":"integer","default":2,"enum":[1,2]},"ratio":{"type":"number","default":0.01,"enum":[0.01,2]},"verbose":{"type":"boolean","default":false,"enum":[true,false]}},"required":["path","ratio"],"additionalProperties":false,"x-bashy-positionals":["path","count"],"x-bashy-shorthand":{"ratio":"-r","verbose":"-v"}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.schema.JSONSchema())
			if err != nil {
				t.Fatal(err)
			}
			var a, b any
			if err := json.Unmarshal(got, &a); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tt.want), &b); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(a, b) {
				t.Fatalf("got %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestArgSchemaArgv(t *testing.T) {
	schema := ArgSchema{
		Positionals: []ArgParameter{{Name: "source", Type: "string", Required: true, Enum: []string{"a", "b"}}, {Name: "target", Type: "string"}},
		Flags:       []ArgFlag{{Name: "verbose", Type: "bool"}, {Name: "ratio", Type: "float"}, {Name: "count", Type: "int"}},
	}
	for _, tt := range []struct {
		name   string
		schema ArgSchema
		args   map[string]any
		want   []string
		err    string
	}{
		{"empty", ArgSchema{}, nil, []string{}, ""},
		{"implicit string", ArgSchema{Positionals: []ArgParameter{{Name: "path", Default: "here"}}}, nil, []string{"here"}, ""},
		{"ordered", schema, map[string]any{"target": "out", "source": "a", "verbose": true, "ratio": 1e-7, "count": float64(3)}, []string{"--count", "3", "--ratio", "0.0000001", "--verbose", "a", "out"}, ""},
		{"false omitted", schema, map[string]any{"source": "b", "verbose": false}, []string{"b"}, ""},
		{"large integer", schema, map[string]any{"source": "a", "count": json.Number("9007199254740993")}, []string{"--count", "9007199254740993", "a"}, ""},
		{"uint", schema, map[string]any{"source": "a", "count": uint64(math.MaxUint64)}, []string{"--count", "18446744073709551615", "a"}, ""},
		{"exponent", schema, map[string]any{"source": "a", "count": json.Number("1e3"), "ratio": json.Number("1e20")}, []string{"--count", "1000", "--ratio", "100000000000000000000", "a"}, ""},
		{"missing positional", schema, nil, nil, "source"},
		{"enum", schema, map[string]any{"source": "c"}, nil, "source"},
		{"unknown", schema, map[string]any{"wat": 1}, nil, "wat"},
		{"empty unknown", ArgSchema{}, map[string]any{"wat": 1}, nil, "wat"},
		{"fraction", schema, map[string]any{"source": "a", "count": 1.5}, nil, "count"},
		{"string number", schema, map[string]any{"source": "a", "count": "3"}, nil, "count"},
		{"null", schema, map[string]any{"source": nil}, nil, "source"},
		{"bad bool", schema, map[string]any{"source": "a", "verbose": "true"}, nil, "verbose"},
		{"nan", schema, map[string]any{"source": "a", "ratio": math.NaN()}, nil, "ratio"},
		{"infinity", schema, map[string]any{"source": "a", "ratio": math.Inf(1)}, nil, "ratio"},
		{"required flag", ArgSchema{Flags: []ArgFlag{{Name: "force", Type: "bool", Required: true}}}, nil, nil, "force"},
		{"false required", ArgSchema{Flags: []ArgFlag{{Name: "force", Type: "bool", Required: true}}}, map[string]any{"force": false}, []string{}, ""},
		{"defaults", ArgSchema{Positionals: []ArgParameter{{Name: "n", Type: "int", Default: "2"}}, Flags: []ArgFlag{{Name: "yes", Type: "bool", Default: "true"}, {Name: "mode", Type: "string", Default: "safe", Enum: []string{"safe"}}}}, nil, []string{"--mode", "safe", "--yes", "2"}, ""},
		{"typed enum", ArgSchema{Flags: []ArgFlag{{Name: "n", Type: "int", Enum: []string{"1e3"}}}}, map[string]any{"n": 1000}, []string{"--n", "1000"}, ""},
		{"flag enum", ArgSchema{Flags: []ArgFlag{{Name: "n", Type: "int", Enum: []string{"1"}}}}, map[string]any{"n": 2}, nil, "n"},
		{"gap", ArgSchema{Positionals: []ArgParameter{{Name: "a", Type: "string"}, {Name: "b", Type: "string"}}}, map[string]any{"b": "later"}, nil, "b"},
		{"empty string", ArgSchema{Positionals: []ArgParameter{{Name: "a", Type: "string", Required: true}}}, map[string]any{"a": ""}, []string{""}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.schema.Argv(tt.args)
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("got %v, %v; want error naming %s", got, err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestArgSchemaDeclarationDefaults(t *testing.T) {
	for _, tt := range []struct {
		name  string
		param ArgParameter
		want  string
	}{
		{"implicit string", ArgParameter{Name: "x", Default: "here"}, `{"default":"here","type":"string"}`},
		{"invalid type", ArgParameter{Name: "x", Type: "array"}, `false`},
		{"invalid default", ArgParameter{Name: "x", Type: "int", Default: "bad"}, `false`},
		{"invalid enum", ArgParameter{Name: "x", Type: "bool", Enum: []string{"bad"}}, `false`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := ArgSchema{Positionals: []ArgParameter{tt.param}}
			prop := s.JSONSchema()["properties"].(map[string]any)["x"]
			got, err := json.Marshal(prop)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("got %s want %s", got, tt.want)
			}
		})
	}
}
