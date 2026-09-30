package localedef

import (
	"io"
	"strings"
)

// ParseSource reads category syntax without resolving copy references or
// compiling collation rules. Entries and string parts retain source order.
// Common system-source extension categories are retained for later consumers.
func ParseSource(in io.Reader) (*Source, error) {
	m := &Source{Sections: make(map[string]*Section)}
	r := newLines(in)
	var section *Section
	classes := map[string]bool{}
	for {
		s, line, err := r.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if ok, err := r.directive(s, line); ok {
			if err != nil {
				return nil, err
			}
			if len(m.Order) != 0 {
				return nil, problem(line, "character directives must precede categories")
			}
			continue
		}
		vs, err := lex(s, r.escape, r.comment)
		if err != nil {
			return nil, atLine(line, err)
		}
		if len(vs) == 0 {
			continue
		}
		key := vs[0].Text
		if section == nil {
			if _, ok := keywords[key]; !ok || len(vs) != 1 {
				return nil, problem(line, "expected locale category, got %q", key)
			}
			if m.Sections[key] != nil {
				return nil, problem(line, "duplicate category %s", key)
			}
			section = &Section{Name: key, Line: line}
			m.Sections[key] = section
			m.Order = append(m.Order, key)
			continue
		}
		if key == "END" {
			if len(vs) != 2 || vs[1].Text != section.Name {
				return nil, problem(line, "expected END %s", section.Name)
			}
			section = nil
			continue
		}
		if strings.HasPrefix(key, "LC_") {
			return nil, problem(line, "nested category %s", key)
		}
		if key == "copy" {
			if len(section.Entries) != 0 || len(vs) != 2 || vs[1].Kind != String || vs[1].Text == "" {
				return nil, problem(line, "copy requires one locale string and an otherwise empty category")
			}
			section.Copy = vs[1].Text
		} else {
			if section.Copy != "" {
				return nil, problem(line, "copy cannot be combined with other entries")
			}
			allowed := strings.Contains(" "+keywords[section.Name]+" ", " "+key+" ")
			if !allowed && !(section.Name == "LC_CTYPE" && classes[key]) && !(section.Name == "LC_COLLATE" && (vs[0].Kind == Symbol || vs[0].Kind == Ellipsis || key == "UNDEFINED")) {
				return nil, problem(line, "unknown %s keyword %q", section.Name, key)
			}
			if key == "charclass" {
				for _, v := range vs[1:] {
					if v.Kind == String {
						classes[v.Text] = true
					}
				}
			}
			if err := balanced(vs[1:]); err != nil {
				return nil, atLine(line, err)
			}
			if err := entrySyntax(section.Name, key, vs[1:]); err != nil {
				return nil, atLine(line, err)
			}
			if vs[0].Kind == Symbol {
				key = "<" + key + ">"
			}
		}
		section.Entries = append(section.Entries, Entry{Keyword: key, Values: vs[1:], Line: line})
	}
	if section != nil {
		return nil, problem(section.Line, "missing END %s", section.Name)
	}
	if len(m.Order) == 0 {
		return nil, problem(1, "no locale categories")
	}
	return m, nil
}

func atLine(line int, err error) error {
	if d, ok := err.(Diagnostic); ok {
		d.Line = line
		return d
	}
	return problem(line, "%v", err)
}
func balanced(vs []Value) error {
	depth := 0
	for _, v := range vs {
		if v.Kind != Separator {
			continue
		}
		switch v.Text {
		case "(":
			depth++
		case ")":
			depth--
			if depth < 0 {
				return problem(0, "unbalanced parentheses")
			}
		}
	}
	if depth != 0 {
		return problem(0, "unbalanced parentheses")
	}
	return nil
}

var keywords = map[string]string{
	"LC_CTYPE":    "upper lower alpha digit alnum space cntrl punct graph print xdigit blank toupper tolower charclass",
	"LC_COLLATE":  "collating-element collating-symbol order_start order_end",
	"LC_MONETARY": "int_curr_symbol currency_symbol mon_decimal_point mon_thousands_sep mon_grouping positive_sign negative_sign int_frac_digits frac_digits p_cs_precedes p_sep_by_space n_cs_precedes n_sep_by_space p_sign_posn n_sign_posn int_p_cs_precedes int_n_cs_precedes int_p_sep_by_space int_n_sep_by_space int_p_sign_posn int_n_sign_posn",
	"LC_NUMERIC":  "decimal_point thousands_sep grouping",
	"LC_TIME":     "abday day abmon mon d_t_fmt d_fmt t_fmt am_pm t_fmt_ampm era era_d_fmt era_t_fmt era_d_t_fmt alt_digits week first_weekday first_workday cal_direction timezone date_fmt ab_alt_mon alt_mon",
	"LC_MESSAGES": "yesexpr noexpr yesstr nostr",
	// These common extensions let callers read an installed en_US source without
	// discarding its additional categories. No interpretation is claimed here.
	"LC_IDENTIFICATION": "title source address contact email tel fax language territory audience application abbreviation revision date category",
	"LC_PAPER":          "height width",
	"LC_NAME":           "name_fmt name_gen name_mr name_mrs name_miss name_ms",
	"LC_ADDRESS":        "postal_fmt country_name country_post country_ab2 country_ab3 country_num country_car country_isbn lang_name lang_ab lang_term lang_lib",
	"LC_TELEPHONE":      "tel_int_fmt tel_dom_fmt int_select int_prefix",
	"LC_MEASUREMENT":    "measurement",
}

// Validate checks symbolic references against a supplied charmap. Missing
// CTYPE/COLLATE symbols are warnings under POSIX 7.3; others are errors.
// Copy references remain unresolved until the compilation stage.
func Validate(src *Source, cm *Charmap) []Diagnostic {
	var ds []Diagnostic
	if cm == nil {
		return ds
	}
	for _, name := range src.Order {
		section := src.Sections[name]
		defined := map[string]bool{}
		if name == "LC_COLLATE" {
			for _, e := range section.Entries {
				if (e.Keyword == "collating-element" || e.Keyword == "collating-symbol") && len(e.Values) > 0 && e.Values[0].Kind == Symbol {
					symbol := e.Values[0].Text
					if _, ok := cm.Symbols[symbol]; ok || defined[symbol] {
						ds = append(ds, Diagnostic{Line: e.Line, Message: "duplicate collating name <" + symbol + ">"})
					}
					defined[symbol] = true
				}
			}
		}
		for _, e := range section.Entries {
			if e.Keyword == "copy" {
				continue
			}
			var check func(Value)
			check = func(v Value) {
				if v.Kind == Symbol {
					if _, ok := cm.Symbols[v.Text]; !ok && !defined[v.Text] {
						ds = append(ds, Diagnostic{Line: e.Line, Message: "undefined symbol <" + v.Text + ">", Warning: name == "LC_CTYPE" || name == "LC_COLLATE"})
					}
				}
				for _, p := range v.Parts {
					check(p)
				}
			}
			if strings.HasPrefix(e.Keyword, "<") {
				check(Value{Kind: Symbol, Text: strings.TrimSuffix(e.Keyword[1:], ">")})
			}
			for _, v := range e.Values {
				check(v)
			}
		}
	}
	return ds
}

// Check the basic operand shapes here; category defaults and semantic
// constraints (such as mutually exclusive character classes) belong to compilation.
func entrySyntax(category, key string, values []Value) error {
	if category == "LC_COLLATE" {
		switch key {
		case "collating-symbol":
			if len(values) != 1 || values[0].Kind != Symbol {
				return problem(0, "collating-symbol requires one symbolic name")
			}
		case "collating-element":
			if len(values) != 3 || values[0].Kind != Symbol || values[1].Text != "from" || values[2].Kind != String {
				return problem(0, "collating-element requires a symbolic name and from string")
			}
		case "order_end":
			if len(values) != 0 {
				return problem(0, "order_end takes no operands")
			}
		}
		return nil
	}
	if len(values) == 0 {
		return problem(0, "%s requires operands", key)
	}
	if key == "toupper" || key == "tolower" {
		char := func(v Value) bool { return v.Kind == Symbol || v.Kind == Bytes || v.Kind == Word || v.Kind == Number }
		for i := 0; i < len(values); {
			if i+4 >= len(values) || values[i].Text != "(" || !char(values[i+1]) || values[i+2].Text != "," || !char(values[i+3]) || values[i+4].Text != ")" {
				return problem(0, "%s requires (character,character) pairs", key)
			}
			i += 5
			if i < len(values) {
				if values[i].Text != ";" {
					return problem(0, "expected semicolon between pairs")
				}
				i++
			}
		}
	}
	scalarStrings := " decimal_point thousands_sep int_curr_symbol currency_symbol mon_decimal_point mon_thousands_sep positive_sign negative_sign d_t_fmt d_fmt t_fmt t_fmt_ampm era_d_fmt era_t_fmt era_d_t_fmt yesexpr noexpr yesstr nostr "
	if strings.Contains(scalarStrings, " "+key+" ") && (len(values) != 1 || values[0].Kind != String) {
		return problem(0, "%s requires one string", key)
	}
	return nil
}
