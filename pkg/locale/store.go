package locale

// The Go-owned locale store. localedef(1) compiles a POSIX locale source into
// one JSON file per locale inside a LOCPATH-style directory, and this file is
// the only reader of that format. Keeping the format explicit and boring (a
// map of category -> keyword -> values, plus the LC_CTYPE classes) is
// deliberate: a compiled locale is data this repository produced, so no
// consumer has to guess at a libc binary layout it cannot parse in pure Go.
//
// Resolution order for every consumer is: a locale WE compiled first, then
// today's host/built-in behaviour for every other name. A locale that is not
// in the store therefore behaves exactly as it did before this store existed.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// StoreEnv is the environment variable that selects the locale store, spelled
// as glibc spells it so an operator who knows LOCPATH needs no new vocabulary.
// Its value is a platform-separated list of directories (filepath.SplitList).
const StoreEnv = "LOCPATH"

// StoreFileExt is the suffix of one compiled locale. One file per locale keeps
// compilation, listing and removal trivially observable with ls(1).
const StoreFileExt = ".json"

// Keyword is one compiled locale keyword. Values holds a single entry for a
// scalar keyword and N entries for a list keyword (abday, mon, am_pm, ...).
// Numeric marks a keyword whose value locale(1) writes unquoted.
type Keyword struct {
	Values  []string `json:"values"`
	Numeric bool     `json:"numeric,omitempty"`
}

// Compiled is one locale as this repository compiled it.
type Compiled struct {
	// Name is the locale name the definition was compiled under.
	Name string `json:"name"`
	// Charmap is the public codeset name (charmap / code_set_name).
	Charmap string `json:"charmap,omitempty"`
	// MbCurMin and MbCurMax come from the charmap used at compile time.
	MbCurMin int `json:"mb_cur_min,omitempty"`
	MbCurMax int `json:"mb_cur_max,omitempty"`
	// Categories maps an LC_* category to its keyword set.
	Categories map[string]map[string]Keyword `json:"categories,omitempty"`
	// Classes maps an LC_CTYPE class name (upper, lower, alpha, ...) to the
	// characters in that class, as a string of the locale's own bytes.
	Classes map[string]string `json:"classes,omitempty"`
	// ToUpper and ToLower are the LC_CTYPE case mappings, source rune to
	// mapped rune, stored as parallel strings so the JSON stays readable.
	ToUpperFrom string `json:"toupper_from,omitempty"`
	ToUpperTo   string `json:"toupper_to,omitempty"`
	ToLowerFrom string `json:"tolower_from,omitempty"`
	ToLowerTo   string `json:"tolower_to,omitempty"`
}

// Set records one keyword under cat, creating the category as needed.
func (c *Compiled) Set(cat, name string, k Keyword) {
	if c.Categories == nil {
		c.Categories = make(map[string]map[string]Keyword)
	}
	if c.Categories[cat] == nil {
		c.Categories[cat] = make(map[string]Keyword)
	}
	c.Categories[cat][name] = k
}

// EnsureCategory records that the locale carries cat even before any keyword
// is set under it. A category present with no keywords is a real outcome (a
// definition may name a category and set nothing this store serves), and it
// must be distinguishable from a category the definition never mentioned.
func (c *Compiled) EnsureCategory(cat string) {
	if c.Categories == nil {
		c.Categories = make(map[string]map[string]Keyword)
	}
	if c.Categories[cat] == nil {
		c.Categories[cat] = make(map[string]Keyword)
	}
}

// Has reports whether the locale carries cat at all. A category absent from a
// compiled locale is not an empty category: consumers must fall back rather
// than answer from data nobody defined.
func (c *Compiled) Has(cat string) bool {
	if c == nil {
		return false
	}
	_, ok := c.Categories[cat]
	return ok
}

// CategoryNames lists the compiled categories in a stable order.
func (c *Compiled) CategoryNames() []string {
	if c == nil {
		return nil
	}
	names := make([]string, 0, len(c.Categories))
	for name := range c.Categories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Keyword returns the compiled keyword under cat.
func (c *Compiled) Keyword(cat, name string) (Keyword, bool) {
	if c == nil {
		return Keyword{}, false
	}
	k, ok := c.Categories[cat][name]
	return k, ok
}

// KeywordString returns a scalar keyword's single value.
func (c *Compiled) KeywordString(cat, name string) (string, bool) {
	k, ok := c.Keyword(cat, name)
	if !ok || len(k.Values) == 0 {
		return "", ok
	}
	return k.Values[0], true
}

// KeywordValues returns a list keyword's values.
func (c *Compiled) KeywordValues(cat, name string) ([]string, bool) {
	k, ok := c.Keyword(cat, name)
	if !ok {
		return nil, false
	}
	return k.Values, true
}

// MessagesData returns the compiled LC_MESSAGES data. It reports false when
// the locale carries no yesexpr, since an affirmative matcher with no
// expression would accept nothing while claiming to be authoritative.
func (c *Compiled) MessagesData() (MessagesData, bool) {
	yes, ok := c.KeywordString("LC_MESSAGES", "yesexpr")
	if !ok || yes == "" {
		return MessagesData{}, false
	}
	no, _ := c.KeywordString("LC_MESSAGES", "noexpr")
	yesStr, _ := c.KeywordString("LC_MESSAGES", "yesstr")
	noStr, _ := c.KeywordString("LC_MESSAGES", "nostr")
	return MessagesData{YesExpr: yes, NoExpr: no, YesStr: yesStr, NoStr: noStr}, true
}

// Class returns the characters of an LC_CTYPE class.
func (c *Compiled) Class(name string) (string, bool) {
	if c == nil {
		return "", false
	}
	s, ok := c.Classes[name]
	return s, ok
}

// StoreDirs returns the locale-store directories an invocation reads, most
// significant first. $LOCPATH wins when it is set and nonempty; otherwise the
// documented default is $HOME/.bashy/locale, which is where every other
// bashy-owned state directory in this module lives. A process with neither is
// answered with no directories at all rather than a guess at a system path:
// writing compiled locales into /usr/lib is not this program's business.
func StoreDirs(env []string) []string {
	if v, ok := getEnv(env, StoreEnv); ok && strings.TrimSpace(v) != "" {
		var dirs []string
		for _, d := range filepath.SplitList(v) {
			if d = strings.TrimSpace(d); d != "" {
				dirs = append(dirs, filepath.Clean(d))
			}
		}
		if len(dirs) > 0 {
			return dirs
		}
	}
	for _, key := range []string{"HOME", "USERPROFILE"} {
		if v, ok := getEnv(env, key); ok && strings.TrimSpace(v) != "" {
			return []string{filepath.Join(filepath.Clean(strings.TrimSpace(v)), ".bashy", "locale")}
		}
	}
	return nil
}

// DefaultStoreDir is the directory localedef writes to: the first entry of
// StoreDirs. It reports an error when the invocation names none, so the
// failure is a diagnostic rather than a file in an unexpected place.
func DefaultStoreDir(env []string) (string, error) {
	dirs := StoreDirs(env)
	if len(dirs) == 0 {
		return "", fmt.Errorf("no locale store directory: set %s or HOME", StoreEnv)
	}
	return dirs[0], nil
}

// ValidStoreName reports whether name may address a file in the store. A
// locale name is one path component: anything with a separator, a parent
// reference or a leading dot is refused so a locale name from the environment
// can never reach outside the store.
func ValidStoreName(name string) bool {
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, ".") {
		return false
	}
	if strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, filepath.Separator) {
		return false
	}
	if strings.ContainsRune(name, 0) || strings.ContainsAny(name, ":;") {
		return false
	}
	return name == filepath.Clean(name)
}

// StorePath is the file one compiled locale occupies inside dir.
func StorePath(dir, name string) (string, error) {
	if !ValidStoreName(name) {
		return "", fmt.Errorf("invalid locale name %q", name)
	}
	return filepath.Join(dir, name+StoreFileExt), nil
}

// Save writes c into dir, creating the directory when absent. The write goes
// to a temporary file in the same directory and is renamed into place, so a
// concurrent reader never observes a half-written locale.
func Save(dir string, c *Compiled) error {
	if err := validateCompiled(c); err != nil {
		return err
	}
	path, err := StorePath(dir, c.Name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(dir, "."+c.Name+"-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// Load reads one compiled locale from an explicit path.
func Load(path string) (*Compiled, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Compiled
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("%s: trailing data in compiled locale", path)
	}
	if err := validateCompiled(&c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if filepath.Base(path) != c.Name+StoreFileExt {
		return nil, fmt.Errorf("%s: compiled locale name does not match file", path)
	}
	return &c, nil
}

func validateCompiled(c *Compiled) error {
	if c == nil || !ValidStoreName(c.Name) {
		return fmt.Errorf("invalid compiled locale name")
	}
	if c.MbCurMin < 0 || c.MbCurMax < 0 || (c.MbCurMax > 0 && c.MbCurMin > c.MbCurMax) {
		return fmt.Errorf("invalid compiled charmap width")
	}
	for cat, keywords := range c.Categories {
		switch cat {
		case "LC_CTYPE", "LC_NUMERIC", "LC_MONETARY", "LC_TIME", "LC_MESSAGES":
		default:
			return fmt.Errorf("invalid compiled category %q", cat)
		}
		for key, value := range keywords {
			if key == "" || len(value.Values) == 0 {
				return fmt.Errorf("invalid compiled keyword %q", key)
			}
		}
	}
	if utf8.RuneCountInString(c.ToUpperFrom) != utf8.RuneCountInString(c.ToUpperTo) ||
		utf8.RuneCountInString(c.ToLowerFrom) != utf8.RuneCountInString(c.ToLowerTo) {
		return fmt.Errorf("invalid compiled case mapping")
	}
	return nil
}

// LookupCompiled returns the locale compiled under name, searching the store
// directories in order. An absent or malformed file is simply not found: the
// caller then keeps its existing host behaviour.
func LookupCompiled(env []string, name string) (*Compiled, bool) {
	if !ValidStoreName(name) {
		return nil, false
	}
	for _, dir := range StoreDirs(env) {
		path, err := StorePath(dir, name)
		if err != nil {
			continue
		}
		c, err := Load(path)
		if err != nil {
			continue
		}
		return c, true
	}
	return nil, false
}

// CompiledFor resolves cat from env (POSIX precedence) and returns the locale
// we compiled for that name, when there is one.
func CompiledFor(env []string, cat Category) (*Compiled, bool) {
	return LookupCompiled(env, Resolve(env, cat))
}

// CompiledCategory is CompiledFor restricted to locales that actually carry
// cat. It is the predicate a consuming utility wants: "is this invocation's
// category answered by data we compiled?"
func CompiledCategory(env []string, cat Category) (*Compiled, bool) {
	c, ok := CompiledFor(env, cat)
	if !ok || !c.Has(string(cat)) {
		return nil, false
	}
	return c, true
}

// CompiledNames lists every locale name in the store, deduplicated and
// sorted. Earlier store directories shadow later ones, as in lookup.
func CompiledNames(env []string) []string {
	seen := map[string]bool{}
	var names []string
	for _, dir := range StoreDirs(env) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), StoreFileExt) {
				continue
			}
			name := strings.TrimSuffix(e.Name(), StoreFileExt)
			if !ValidStoreName(name) || seen[name] {
				continue
			}
			if _, err := Load(filepath.Join(dir, e.Name())); err == nil {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names
}

// NumericSeparators returns the LC_NUMERIC radix and thousands separator of a
// locale we compiled, for the utilities that parse and format numbers. Only
// single-byte separators are reported: a consumer that scans bytes cannot
// honour a multibyte separator, and pretending otherwise would misparse
// silently. ok is false when the invocation's LC_NUMERIC is not ours.
func NumericSeparators(env []string) (decPt, thousSep byte, ok bool) {
	c, found := CompiledCategory(env, Numeric)
	if !found {
		return 0, 0, false
	}
	point, has := c.KeywordString("LC_NUMERIC", "decimal_point")
	if !has || len(point) != 1 {
		return 0, 0, false
	}
	sep, _ := c.KeywordString("LC_NUMERIC", "thousands_sep")
	if len(sep) > 1 {
		return 0, 0, false
	}
	var thousand byte
	if len(sep) == 1 {
		thousand = sep[0]
	}
	return point[0], thousand, true
}
