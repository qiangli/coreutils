// Package terminfo is the pure-Go reader for the compiled terminfo database
// and the `%`-directive language a parameterized capability is written in.
//
// It is shared by every command that has to speak to the terminal itself
// rather than to a file — tput reports and emits capabilities, tabs renders a
// tab-stop list with them — and it exists as one package for a blunt reason:
// the three capability NAME TABLES in caps.go ARE the binary format. A
// compiled entry stores three unnamed arrays, so a capability's name is purely
// its index, and a second transcription of those tables would not fail
// loudly — it would report a plausible value under the wrong name.
//
// Two invariants run through the whole package.
//
// ABSENCE LIVES IN THE KEY SET, never in a zero value. terminfo writes -1 for
// "absent" and -2 for "cancelled", and a numeric capability may legitimately
// hold 0, so `Num` returns the comma-ok form and callers must use it. POSIX
// hangs tput's exit status on exactly that distinction.
//
// THE DATABASE IS READ DIRECTLY. Nothing here shells out to tput, tabs or
// infocmp, and nothing links ncurses; the compiled format is parsed in
// terminfo.go and the parameter engine is interpreted in tparm.go, both
// implemented from the published format and the POSIX/terminfo documentation.
package terminfo

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Entry is one decoded terminal description.
//
// Absent is modelled by absence from the map, never by a zero value: terminfo
// stores -1 for "absent" and -2 for "cancelled", and a numeric capability
// legitimately holds 0, so a present-but-zero number must not read as missing.
// POSIX hangs exit status 1 on exactly that distinction.
type Entry struct {
	names []string // aliases, in file order; the last element is the long name
	bools map[string]bool
	nums  map[string]int
	strs  map[string]string

	// source records where the entry came from, for diagnostics: a file path,
	// or "(built-in)" for the compiled-in fallback table.
	source string
}

func newEntry() *Entry {
	return &Entry{
		bools: map[string]bool{},
		nums:  map[string]int{},
		strs:  map[string]string{},
	}
}

// LongName is the human-readable description: the last |-separated field of
// the names section, as `tput longname` reports it.
func (e *Entry) LongName() string {
	if len(e.names) == 0 {
		return ""
	}
	return e.names[len(e.names)-1]
}

// Str, Num and Bool are the read surface every consumer of a decoded entry
// uses. They return the comma-ok form deliberately: the map's KEY SET is where
// presence lives, because "absent" and a legitimate 0 are different answers
// and a zero value cannot tell them apart.
func (e *Entry) Str(name string) (string, bool) {
	s, ok := e.strs[name]
	return s, ok
}

func (e *Entry) Num(name string) (int, bool) {
	v, ok := e.nums[name]
	return v, ok
}

func (e *Entry) Bool(name string) bool { return e.bools[name] }

// Source reports where the entry came from: a database file path, or
// "(built-in)" for the compiled-in fallback table.
func (e *Entry) Source() string { return e.source }

// ExtendedKind classifies a name that no standard array reserves but this
// entry defines as an ncurses user-defined capability. Such a name is a real
// capability of this terminal even though no standard slot names it.
func (e *Entry) ExtendedKind(name string) Kind {
	switch {
	case containsKey(e.bools, name):
		return KindBool
	case containsKey(e.nums, name):
		return KindNum
	case containsKey(e.strs, name):
		return KindStr
	}
	return KindUnknown
}

func containsKey[V any](m map[string]V, k string) bool {
	_, ok := m[k]
	return ok
}

// The two magic numbers of the compiled format. The only difference between
// them is the width of a numeric capability: 16-bit in the legacy layout,
// 32-bit in the extended-number layout ncurses 6 writes when a value does not
// fit in a signed short (max_pairs on 256-colour terminals is the usual
// trigger). String offsets stay 16-bit in BOTH layouts.
const (
	magicLegacy   = 0o432  // 0x011A, numbers are int16
	magicExtended = 0o1036 // 0x021E, numbers are int32
)

// Reserved numeric/string-offset values.
const (
	valAbsent    = -1
	valCancelled = -2
)

var errBadFormat = errors.New("not a compiled terminfo file")

type reader struct {
	b   []byte
	pos int
}

func (r *reader) need(n int) bool { return n >= 0 && r.pos+n <= len(r.b) }

func (r *reader) bytes(n int) ([]byte, bool) {
	if !r.need(n) {
		return nil, false
	}
	out := r.b[r.pos : r.pos+n]
	r.pos += n
	return out, true
}

func (r *reader) int16() (int, bool) {
	b, ok := r.bytes(2)
	if !ok {
		return 0, false
	}
	return int(int16(binary.LittleEndian.Uint16(b))), true
}

func (r *reader) int32() (int, bool) {
	b, ok := r.bytes(4)
	if !ok {
		return 0, false
	}
	return int(int32(binary.LittleEndian.Uint32(b))), true
}

// align skips the single padding byte the format inserts so that the numeric
// array starts on an even offset from the beginning of the file.
func (r *reader) align() {
	if r.pos%2 == 1 {
		r.pos++
	}
}

// parseTerminfo decodes a compiled terminfo entry.
//
// Layout: a six-short header (magic, size of the names blob, count of
// booleans, count of numbers, count of string offsets, size of the string
// table), then the names blob, the boolean bytes, an alignment pad, the
// numbers, the string offsets, and the string table. An optional extended
// section (ncurses user-defined capabilities) may follow.
func parseTerminfo(data []byte) (*Entry, error) {
	r := &reader{b: data}
	magic, ok := r.int16()
	if !ok {
		return nil, errBadFormat
	}
	var numWidth int
	switch magic {
	case magicLegacy:
		numWidth = 2
	case magicExtended:
		numWidth = 4
	default:
		return nil, errBadFormat
	}

	nameSize, ok1 := r.int16()
	nBools, ok2 := r.int16()
	nNums, ok3 := r.int16()
	nStrs, ok4 := r.int16()
	tableSize, ok5 := r.int16()
	if !(ok1 && ok2 && ok3 && ok4 && ok5) {
		return nil, errBadFormat
	}
	if nameSize < 0 || nBools < 0 || nNums < 0 || nStrs < 0 || tableSize < 0 {
		return nil, fmt.Errorf("%w: negative section size", errBadFormat)
	}

	e := newEntry()

	nameBlob, ok := r.bytes(nameSize)
	if !ok {
		return nil, fmt.Errorf("%w: truncated names section", errBadFormat)
	}
	e.names = splitNames(string(nameBlob))

	boolBytes, ok := r.bytes(nBools)
	if !ok {
		return nil, fmt.Errorf("%w: truncated boolean section", errBadFormat)
	}
	for i, v := range boolBytes {
		if i >= len(boolNames) {
			break // a newer database declaring capabilities we do not name
		}
		// 0 is false/absent and 1 is true; -2 (0xFE) means cancelled, which is
		// also "not true".
		if v == 1 {
			e.bools[boolNames[i]] = true
		}
	}

	r.align()

	for i := 0; i < nNums; i++ {
		var v int
		var ok bool
		if numWidth == 4 {
			v, ok = r.int32()
		} else {
			v, ok = r.int16()
		}
		if !ok {
			return nil, fmt.Errorf("%w: truncated numeric section", errBadFormat)
		}
		if v < 0 || i >= len(numNames) {
			continue // absent, cancelled, or unnamed
		}
		e.nums[numNames[i]] = v
	}

	offsets := make([]int, nStrs)
	for i := range offsets {
		v, ok := r.int16()
		if !ok {
			return nil, fmt.Errorf("%w: truncated string-offset section", errBadFormat)
		}
		offsets[i] = v
	}
	table, ok := r.bytes(tableSize)
	if !ok {
		return nil, fmt.Errorf("%w: truncated string table", errBadFormat)
	}
	for i, off := range offsets {
		if off < 0 || i >= len(strNames) {
			continue
		}
		s, ok := cstring(table, off)
		if !ok {
			continue
		}
		e.strs[strNames[i]] = s
	}

	// The extended section is optional and is ncurses' own addition. A file
	// that stops here is complete and valid, and a malformed extension must
	// not invalidate the standard capabilities we already decoded — so every
	// failure below is a silent stop, not an error.
	r.align()
	parseExtended(r, e, numWidth)

	return e, nil
}

// parseExtended decodes ncurses user-defined capabilities. Layout mirrors the
// main body: a five-short header (counts of extended booleans, numbers and
// strings, the total number of string offsets, and the extended table size),
// then the values, then — at the tail of the same string table — the NAMES of
// every extended capability, booleans first, then numbers, then strings.
func parseExtended(r *reader, e *Entry, numWidth int) {
	nBools, ok1 := r.int16()
	nNums, ok2 := r.int16()
	nStrs, ok3 := r.int16()
	nOffsets, ok4 := r.int16()
	tableSize, ok5 := r.int16()
	if !(ok1 && ok2 && ok3 && ok4 && ok5) {
		return
	}
	if nBools < 0 || nNums < 0 || nStrs < 0 || nOffsets < 0 || tableSize < 0 {
		return
	}
	// Offsets cover the string VALUES plus one name per capability of any kind.
	if nOffsets != nStrs+nBools+nNums+nStrs {
		return
	}

	boolVals, ok := r.bytes(nBools)
	if !ok {
		return
	}
	r.align()

	numVals := make([]int, nNums)
	for i := range numVals {
		var v int
		if numWidth == 4 {
			v, ok = r.int32()
		} else {
			v, ok = r.int16()
		}
		if !ok {
			return
		}
		numVals[i] = v
	}

	offsets := make([]int, nOffsets)
	for i := range offsets {
		v, ok := r.int16()
		if !ok {
			return
		}
		offsets[i] = v
	}
	table, ok := r.bytes(tableSize)
	if !ok {
		return
	}

	// The extended table holds the string VALUES first and then the NAMES of
	// every extended capability of any kind. The two halves are addressed
	// differently, and this is the trap: a value offset is relative to the
	// start of the table, but a NAME offset is relative to the start of the
	// name half. Reading names from the table base instead decodes each name
	// as whatever value string happens to sit there — producing an entry that
	// parses cleanly and reports a boolean's value under a string's name.
	strVals := make([]string, nStrs)
	nameBase := 0
	for i := 0; i < nStrs; i++ {
		if offsets[i] < 0 {
			continue
		}
		s, ok := cstring(table, offsets[i])
		if !ok {
			continue
		}
		strVals[i] = s
		if end := offsets[i] + len(s) + 1; end > nameBase {
			nameBase = end
		}
	}
	names := make([]string, 0, nBools+nNums+nStrs)
	for i := nStrs; i < nOffsets; i++ {
		if offsets[i] < 0 {
			names = append(names, "")
			continue
		}
		s, ok := cstring(table, nameBase+offsets[i])
		if !ok {
			s = ""
		}
		names = append(names, s)
	}
	if len(names) != nBools+nNums+nStrs {
		return
	}

	for i := 0; i < nBools; i++ {
		if names[i] != "" && boolVals[i] == 1 {
			e.bools[names[i]] = true
		}
	}
	for i := 0; i < nNums; i++ {
		name := names[nBools+i]
		if name != "" && numVals[i] >= 0 {
			e.nums[name] = numVals[i]
		}
	}
	for i := 0; i < nStrs; i++ {
		name := names[nBools+nNums+i]
		if name != "" && offsets[i] >= 0 {
			e.strs[name] = strVals[i]
		}
	}
}

// cstring reads the NUL-terminated string at off. An unterminated tail is
// rejected rather than silently returning the rest of the table.
func cstring(table []byte, off int) (string, bool) {
	if off < 0 || off >= len(table) {
		return "", false
	}
	rest := table[off:]
	for i, c := range rest {
		if c == 0 {
			return string(rest[:i]), true
		}
	}
	return "", false
}

func splitNames(blob string) []string {
	blob = strings.TrimRight(blob, "\x00")
	if blob == "" {
		return nil
	}
	return strings.Split(blob, "|")
}

// --- database lookup -------------------------------------------------------

// systemDirs is the conventional search path for a compiled database. It is
// deliberately a superset across platforms: a directory that does not exist is
// simply skipped, and being generous here is what lets one binary work on
// glibc Linux, musl Linux, macOS and the BSDs without a build tag.
var systemDirs = []string{
	"/usr/share/terminfo",
	"/lib/terminfo",
	"/usr/lib/terminfo",
	"/usr/share/lib/terminfo",
	"/etc/terminfo",
	"/usr/local/share/terminfo",
	"/usr/local/lib/terminfo",
	"/usr/share/misc/terminfo",
}

// terminfoDirs is the ordered directory list to search for term.
func terminfoDirs(getenv func(string) string) []string {
	var dirs []string
	add := func(d string) {
		if d == "" {
			return
		}
		for _, seen := range dirs {
			if seen == d {
				return
			}
		}
		dirs = append(dirs, d)
	}

	if d := getenv("TERMINFO"); d != "" {
		add(d)
	}
	// An empty element of TERMINFO_DIRS means "the compiled-in default list",
	// which is the same convention the reference implementation uses; dropping
	// it silently would quietly change which entry wins.
	if list := getenv("TERMINFO_DIRS"); list != "" {
		for _, d := range strings.Split(list, string(os.PathListSeparator)) {
			if d == "" {
				for _, sd := range systemDirs {
					add(sd)
				}
				continue
			}
			add(d)
		}
	}
	for _, d := range systemDirs {
		add(d)
	}
	if home := homeDir(getenv); home != "" {
		add(filepath.Join(home, ".terminfo"))
	}
	return dirs
}

func homeDir(getenv func(string) string) string {
	if h := getenv("HOME"); h != "" {
		return h
	}
	if runtime.GOOS == "windows" {
		return getenv("USERPROFILE")
	}
	return ""
}

// candidatePaths returns the file names an entry for term may be stored under
// inside one database directory.
//
// Two spellings exist for the bucket directory. The portable one is the first
// character of the terminal name ("x/xterm"). Case-insensitive filesystems
// cannot keep "A" and "a" apart, so those installations name the bucket with
// the character's hexadecimal value instead ("78/xterm") — that is what macOS
// ships. Trying both costs one failed stat and is the difference between
// finding the system database and silently falling back.
func candidatePaths(dir, term string) []string {
	if term == "" {
		return nil
	}
	c := term[0]
	return []string{
		filepath.Join(dir, string(rune(c)), term),
		filepath.Join(dir, fmt.Sprintf("%02x", c), term),
	}
}

// ErrUnknownTerm is the "no information about this terminal" condition, which
// POSIX maps to exit status 3.
var ErrUnknownTerm = errors.New("unknown terminal type")

// Load finds and decodes the description for term.
//
// The on-disk database always wins over the compiled-in table: the built-ins
// exist so the tool still works where no database is installed (a scratch
// container, Windows), not to override an administrator's entry.
func Load(getenv func(string) string, term string) (*Entry, error) {
	if term == "" {
		return nil, ErrUnknownTerm
	}
	// A terminal name is used to build a path, so refuse anything that could
	// escape the database directory.
	if strings.ContainsAny(term, "/\\") || term == "." || term == ".." {
		return nil, ErrUnknownTerm
	}
	var firstErr error
	for _, dir := range terminfoDirs(getenv) {
		for _, path := range candidatePaths(dir, term) {
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			e, err := parseTerminfo(data)
			if err != nil {
				// A corrupt file is worth reporting, but keep looking: a later
				// directory may hold a good copy.
				if firstErr == nil {
					firstErr = fmt.Errorf("%s: %w", path, err)
				}
				continue
			}
			e.source = path
			return e, nil
		}
	}
	if e := builtinEntry(term); e != nil {
		return e, nil
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, ErrUnknownTerm
}
