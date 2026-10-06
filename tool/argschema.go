package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// ArgSchema mirrors interp.CommandSchema without coupling documentation to the interpreter.
type ArgSchema struct {
	Positionals []ArgParameter
	Flags       []ArgFlag
}

// ArgParameter describes a positional argument. Type is string, int, float or bool;
// an empty Type means string, as in interp.CommandParameter.
// An empty Default means no default, as in interp.CommandParameter.
type ArgParameter struct {
	Name     string
	Type     string
	Required bool
	Default  string
	Enum     []string
}

// ArgFlag describes a named argument; Shorthand excludes the leading dash.
type ArgFlag struct {
	Name      string
	Shorthand string
	Type      string
	Required  bool
	Default   string
	Enum      []string
}

// JSONSchema projects a declared schema into JSON Schema 2020-12. Invalid
// declarations produce a rejecting property schema rather than widening inputs.
func (s ArgSchema) JSONSchema() map[string]any {
	properties := map[string]any{}
	required := []string{}
	positionals := []string{}
	shorthands := map[string]string{}
	add := func(p ArgParameter) {
		types := map[string]string{"": "string", "string": "string", "int": "integer", "float": "number", "bool": "boolean"}
		typ, ok := types[p.Type]
		if !ok {
			properties[p.Name] = false
			return
		}
		prop := map[string]any{"type": typ}
		properties[p.Name] = prop
		if p.Required {
			required = append(required, p.Name)
		}
		if p.Default != "" {
			v, err := schemaLiteral(p.Type, p.Default)
			if err != nil {
				properties[p.Name] = false
				return
			}
			prop["default"] = v
		}
		if len(p.Enum) > 0 {
			values := make([]any, 0, len(p.Enum))
			for _, e := range p.Enum {
				v, err := schemaLiteral(p.Type, e)
				if err != nil {
					properties[p.Name] = false
					return
				}
				values = append(values, v)
			}
			prop["enum"] = values
		}
	}
	for _, p := range s.Positionals {
		add(p)
		positionals = append(positionals, p.Name)
	}
	for _, f := range s.Flags {
		add(ArgParameter{f.Name, f.Type, f.Required, f.Default, f.Enum})
		if f.Shorthand != "" {
			shorthands[f.Name] = "-" + f.Shorthand
		}
	}
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object",
		"properties": properties, "required": required, "additionalProperties": false,
		"x-bashy-positionals": positionals, "x-bashy-shorthand": shorthands,
	}
}

// Argv validates arguments and emits flags in name order, followed by positional
// operands in declaration order. Defaults fill omitted fields before validation.
func (s ArgSchema) Argv(args map[string]any) ([]string, error) {
	known := map[string]bool{}
	for _, p := range s.Positionals {
		known[p.Name] = true
	}
	for _, f := range s.Flags {
		known[f.Name] = true
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !known[k] {
			return nil, fmt.Errorf("unknown argument %q", k)
		}
	}
	value := func(p ArgParameter) (string, bool, error) {
		v, ok := args[p.Name]
		if !ok && p.Default != "" {
			var err error
			v, err = schemaLiteral(p.Type, p.Default)
			if err != nil {
				return "", false, fmt.Errorf("%s: %w", p.Name, err)
			}
			ok = true
		}
		if !ok {
			if p.Required {
				return "", false, fmt.Errorf("missing required argument %q", p.Name)
			}
			return "", false, nil
		}
		text, err := schemaValue(p.Type, v)
		if err != nil {
			return "", false, fmt.Errorf("%s: %w", p.Name, err)
		}
		if len(p.Enum) > 0 {
			matched := false
			for _, e := range p.Enum {
				literal, err := schemaLiteral(p.Type, e)
				if err != nil {
					return "", false, fmt.Errorf("%s: invalid enum: %w", p.Name, err)
				}
				candidate, _ := schemaValue(p.Type, literal)
				matched = matched || candidate == text
			}
			if !matched {
				return "", false, fmt.Errorf("%s: value %q is not in enum", p.Name, text)
			}
		}
		return text, true, nil
	}
	flags := append([]ArgFlag(nil), s.Flags...)
	sort.Slice(flags, func(i, j int) bool { return flags[i].Name < flags[j].Name })
	argv := []string{}
	for _, f := range flags {
		text, ok, err := value(ArgParameter{f.Name, f.Type, f.Required, f.Default, f.Enum})
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if f.Type == "bool" {
			if text == "true" {
				argv = append(argv, "--"+f.Name)
			}
		} else {
			argv = append(argv, "--"+f.Name, text)
		}
	}
	gap := false
	for _, p := range s.Positionals {
		text, ok, err := value(p)
		if err != nil {
			return nil, err
		}
		if !ok {
			gap = true
			continue
		}
		if gap {
			return nil, fmt.Errorf("%s: cannot follow an omitted positional", p.Name)
		}
		argv = append(argv, text)
	}
	return argv, nil
}

func schemaLiteral(typ, text string) (any, error) {
	switch typ {
	case "", "string":
		return text, nil
	case "bool":
		if text == "true" {
			return true, nil
		}
		if text == "false" {
			return false, nil
		}
	case "int", "float":
		normalized, err := schemaValue(typ, json.Number(text))
		if err != nil {
			return nil, err
		}
		return json.Number(normalized), nil
	}
	return nil, fmt.Errorf("invalid %s literal %q", typ, text)
}

// schemaValue accepts JSON values and Go numeric values, without accepting
// strings as numbers or losing integer precision through float64 conversion.
func schemaValue(typ string, value any) (string, error) {
	if typ == "" || typ == "string" {
		if v, ok := value.(string); ok {
			return v, nil
		}
	}
	if typ == "bool" {
		if v, ok := value.(bool); ok {
			return strconv.FormatBool(v), nil
		}
	}
	if typ == "int" || typ == "float" {
		var text string
		if n, ok := value.(json.Number); ok {
			text = string(n)
		} else if value != nil {
			v := reflect.ValueOf(value)
			switch v.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				text = strconv.FormatInt(v.Int(), 10)
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				text = strconv.FormatUint(v.Uint(), 10)
			case reflect.Float32, reflect.Float64:
				text = strconv.FormatFloat(v.Float(), 'f', -1, v.Type().Bits())
			}
		}
		if text != "" && json.Valid([]byte(text)) && !strings.ContainsAny(text, "\"[]{}") {
			if typ == "int" {
				n, ok := new(big.Rat).SetString(text)
				if ok && n.IsInt() {
					return n.Num().String(), nil
				}
			} else {
				n, err := strconv.ParseFloat(text, 64)
				if err == nil && !math.IsInf(n, 0) && !math.IsNaN(n) {
					return strconv.FormatFloat(n, 'f', -1, 64), nil
				}
			}
		}
	}
	return "", fmt.Errorf("expected %s, got %v (%T)", typ, value, value)
}
