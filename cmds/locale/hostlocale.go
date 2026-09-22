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
	for _, category := range categories {
		if _, err := p.query(name, category); err != nil {
			return false
		}
	}
	keywords, err := p.query(name, "charmap")
	return err == nil && len(keywords) == 1 && keywords[0].Values[0] != ""
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
	return parseHostLocaleKeywords(category, out)
}

func parseHostLocaleKeywords(category, out string) ([]keyword, error) {
	var result []keyword
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if line == "" {
			continue // LC_COLLATE legitimately has no scalar keywords.
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || name == "" {
			return nil, fmt.Errorf("malformed host locale output %q", line)
		}
		values, kind, err := parseHostLocaleValue(value)
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
	if _, err := strconv.ParseInt(value, 10, 64); err == nil {
		return []string{value}, kindNumber, nil
	}
	parts := strings.Split(value, ";")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) < 2 || part[0] != '"' || part[len(part)-1] != '"' {
			return nil, kindString, fmt.Errorf("malformed value %q", value)
		}
		unquoted, err := strconv.Unquote(part)
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
