package localecmd

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

const (
	// Bashy sets this to the native, host-owned locale executable in Windows
	// fixture processes. It is deliberately a path, rather than a command name:
	// resolving "locale" through PATH could recurse into this multicall binary.
	hostLocalePathEnv = "BASHY_HOST_LOCALE"
	// Some Git Bash installations omit installed legacy locales from `locale -a`.
	// The fixture provisioner may add candidate names here (semicolon or newline
	// separated); every candidate is still verified against the host service.
	hostLocaleNamesEnv = "BASHY_HOST_LOCALE_NAMES"
)

type hostLocaleRunner func(env []string, args []string) (stdout, stderr string, err error)

type hostLocaleProvider struct {
	run        hostLocaleRunner
	candidates []string
}

func (p hostLocaleProvider) names() []string {
	out, _, err := p.run(nil, []string{"-a"})
	seen := make(map[string]bool)
	if err == nil {
		for _, name := range strings.Fields(out) {
			seen[name] = true
		}
	}
	for _, name := range p.candidates {
		if name != "" {
			seen[name] = true
		}
	}
	return sortedKeys(seen)
}

// serves is intentionally exhaustive. A service which returns success while
// silently falling back to C (as Git Bash does for Big5-HKSCS) is not a locale
// provider for that name and must not affect `locale -a`.
func (p hostLocaleProvider) serves(name string) bool {
	if !p.selected(name) {
		return false
	}
	if !p.matchesRequestedCharmap(name) {
		return false
	}
	for _, category := range categories {
		if _, err := p.query(name, category); err != nil {
			return false
		}
	}
	return true
}

// matchesRequestedCharmap checks the data the service actually selected, not
// merely the label echoed by `locale`. Git Bash can echo a requested name yet
// supply C or plain Big5 data for it. Keep this deliberately small and
// explicit: an unknown spelling is not evidence that two encodings are equal.
func (p hostLocaleProvider) matchesRequestedCharmap(name string) bool {
	_, requested := splitLocaleName(name)
	want, ok := canonicalHostCodeset(requested)
	if !ok {
		return false
	}
	keywords, err := p.query(name, "LC_CTYPE")
	if err != nil {
		return false
	}
	var charmaps []keyword
	for _, keyword := range keywords {
		if keyword.Name == "charmap" {
			charmaps = append(charmaps, keyword)
		}
	}
	if len(charmaps) != 1 || charmaps[0].Kind != kindString || len(charmaps[0].Values) != 1 {
		return false
	}
	got, ok := canonicalHostCodeset(charmaps[0].Values[0])
	return ok && got == want
}

// canonicalHostCodeset recognizes only the aliases required by the provisioned
// locale corpus. In particular, BIG5-HKSCS is distinct from BIG5: accepting
// the latter for zh_HK.big5hkscs would silently advertise a fallback.
func canonicalHostCodeset(codeset string) (string, bool) {
	normalized := strings.ToUpper(codeset)
	normalized = strings.NewReplacer("-", "", "_", "", ".", "").Replace(normalized)
	switch normalized {
	case "UTF8":
		return "UTF8", true
	case "ISO88591":
		return "ISO88591", true
	case "BIG5":
		return "BIG5", true
	case "BIG5HKSCS":
		return "BIG5HKSCS", true
	case "CP932", "SJIS", "SHIFT", "SHIFTJIS":
		return "CP932", true
	case "CP1251", "WINDOWS1251":
		return "CP1251", true
	default:
		return "", false
	}
}

// selected verifies the host did not silently substitute C for an unsupported
// locale. Exit status alone is insufficient on Git Bash.
func (p hostLocaleProvider) selected(name string) bool {
	out, _, err := p.run([]string{"LC_ALL=" + name}, nil)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if key != "LC_CTYPE" || !ok {
			continue
		}
		values, _, err := parseHostLocaleValue(value)
		return err == nil && len(values) == 1 && values[0] == name
	}
	return false
}

func (p hostLocaleProvider) query(name, category string) ([]keyword, error) {
	out, errOut, err := p.run([]string{"LC_ALL=" + name}, []string{"-k", category})
	if err != nil {
		if errOut != "" {
			return nil, fmt.Errorf("host locale %s: %s", category, strings.TrimSpace(errOut))
		}
		return nil, fmt.Errorf("host locale %s: %w", category, err)
	}
	return parseHostLocaleKeywordsForLocale(name, category, out)
}

func parseHostLocaleKeywords(category, out string) ([]keyword, error) {
	return parseHostLocaleKeywordsForLocale("", category, out)
}

func parseHostLocaleKeywordsForLocale(locale, category, out string) ([]keyword, error) {
	var result []keyword
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if line == "" {
			continue // LC_COLLATE legitimately has no scalar keywords.
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || name == "" {
			return nil, fmt.Errorf("malformed host locale output %q", line)
		}
		values, kind, err := parseHostLocaleValueForLocale(locale, value)
		if err != nil {
			return nil, fmt.Errorf("host locale %s: %w", name, err)
		}
		result = append(result, keyword{Name: name, Category: categoryForHostKeyword(category, name), Kind: kind, Values: values})
	}
	return result, nil
}

func categoryForHostKeyword(requested, name string) string {
	if name == "charmap" || name == "code_set_name" || name == "mb_cur_max" {
		return "LC_CTYPE"
	}
	return requested
}

func parseHostLocaleValue(value string) ([]string, valueKind, error) {
	return parseHostLocaleValueForLocale("", value)
}

func parseHostLocaleValueForLocale(locale, value string) ([]string, valueKind, error) {
	if _, err := strconv.ParseInt(value, 10, 64); err == nil {
		return []string{value}, kindNumber, nil
	}
	if value == "" {
		// Git Bash emits empty LC_TIME era fields as era= and alt_digits=.
		return []string{""}, kindString, nil
	}
	cp932 := hostLocaleUsesCP932(locale)
	parts, err := splitHostLocaleValueForCP932(value, cp932)
	if err != nil {
		return nil, kindString, err
	}
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) < 2 || part[0] != '"' || part[len(part)-1] != '"' {
			return nil, kindString, fmt.Errorf("malformed value %q", value)
		}
		unquoted, err := unquoteHostLocaleStringForCP932(part, cp932)
		if err != nil {
			return nil, kindString, err
		}
		values = append(values, unquoted)
	}
	if len(values) > 1 {
		return values, kindStringList, nil
	}
	return values, kindString, nil
}

// splitHostLocaleValue separates the quoted list elements emitted by
// locale(1). A semicolon is a list separator only outside a quoted value;
// locale data itself may contain semicolons (for example in abday).
func splitHostLocaleValue(value string) ([]string, error) {
	return splitHostLocaleValueForCP932(value, false)
}

func splitHostLocaleValueForCP932(value string, cp932 bool) ([]string, error) {
	var parts []string
	start := 0
	inQuotes := false
	escaped := false
	for i := 0; i < len(value); i++ {
		if cp932 && cp932LeadByte(value[i]) && i+1 < len(value) && cp932TrailByte(value[i+1]) {
			i++
			continue
		}
		switch {
		case escaped:
			escaped = false
		case value[i] == '\\':
			escaped = true
		case value[i] == '"':
			inQuotes = !inQuotes
		case value[i] == ';' && !inQuotes:
			parts = append(parts, value[start:i])
			start = i + 1
		}
	}
	if escaped || inQuotes {
		return nil, fmt.Errorf("malformed quoted value %q", value)
	}
	return append(parts, value[start:]), nil
}

// unquoteHostLocaleString is byte-oriented. Host locale(1) output for a
// single-byte codeset can contain bytes which are not valid UTF-8; strconv's
// Go-string decoder replaces those bytes with U+FFFD. Keep them intact so the
// delegated locale -k result remains usable in its advertised codeset.
func unquoteHostLocaleString(value string) (string, error) {
	return unquoteHostLocaleStringForCP932(value, false)
}

func unquoteHostLocaleStringForCP932(value string, cp932 bool) (string, error) {
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return "", fmt.Errorf("malformed quoted value %q", value)
	}
	var out []byte
	for i := 1; i < len(value)-1; i++ {
		c := value[i]
		if cp932 && cp932LeadByte(c) && i+1 < len(value)-1 && cp932TrailByte(value[i+1]) {
			out = append(out, c, value[i+1])
			i++
			continue
		}
		if c != '\\' {
			out = append(out, c)
			continue
		}
		i++
		if i >= len(value)-1 {
			return "", fmt.Errorf("unfinished escape in %q", value)
		}
		switch value[i] {
		case 'a':
			out = append(out, '\a')
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'v':
			out = append(out, '\v')
		case '\\', '"':
			out = append(out, value[i])
		case 'x':
			if i+2 >= len(value)-1 {
				return "", fmt.Errorf("short hex escape in %q", value)
			}
			n, err := strconv.ParseUint(value[i+1:i+3], 16, 8)
			if err != nil {
				return "", fmt.Errorf("bad hex escape in %q: %w", value, err)
			}
			out = append(out, byte(n))
			i += 2
		default:
			if value[i] < '0' || value[i] > '7' {
				return "", fmt.Errorf("unsupported escape in %q", value)
			}
			end := i + 1
			for end < len(value)-1 && end < i+3 && value[end] >= '0' && value[end] <= '7' {
				end++
			}
			n, err := strconv.ParseUint(value[i:end], 8, 8)
			if err != nil {
				return "", fmt.Errorf("bad octal escape in %q: %w", value, err)
			}
			out = append(out, byte(n))
			i = end - 1
		}
	}
	return string(out), nil
}

// hostLocaleUsesCP932 is deliberately limited to names whose codeset is one
// of the CP932 aliases we advertise. Other non-UTF-8 locale data remains
// subject to the ordinary byte-oriented quoting rules.
func hostLocaleUsesCP932(locale string) bool {
	_, codeset := splitLocaleName(locale)
	canonical, ok := canonicalHostCodeset(codeset)
	return ok && canonical == "CP932"
}

func cp932LeadByte(b byte) bool {
	return b >= 0x81 && b <= 0x9f || b >= 0xe0 && b <= 0xfc
}

func cp932TrailByte(b byte) bool {
	return b >= 0x40 && b <= 0x7e || b >= 0x80 && b <= 0xfc
}

func hostLocaleNames(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ';' || r == '\n' || r == '\r' })
	sort.Strings(parts)
	return parts
}

func commandHostLocaleRunner(rc *tool.RunContext, path string) hostLocaleRunner {
	return func(overrides []string, args []string) (string, string, error) {
		child := *rc
		child.Env = append(append([]string(nil), rc.Env...), overrides...)
		var out, errOut bytes.Buffer
		cmd, err := child.StartCommand(path, args, nil, &out, &errOut)
		if err != nil {
			return out.String(), errOut.String(), err
		}
		return out.String(), errOut.String(), cmd.Wait()
	}
}
