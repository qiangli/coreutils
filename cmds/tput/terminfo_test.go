package tputcmd

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The three name tables are the format. A capability inserted or dropped
// anywhere shifts every index after it, and the result is a wrong value under
// a right-looking name rather than an error.
func TestCapabilityTableLengths(t *testing.T) {
	if len(boolNames) != boolArrayLen {
		t.Errorf("boolNames has %d entries, want %d", len(boolNames), boolArrayLen)
	}
	if len(numNames) != numArrayLen {
		t.Errorf("numNames has %d entries, want %d", len(numNames), numArrayLen)
	}
	if len(strNames) != strArrayLen {
		t.Errorf("strNames has %d entries, want %d", len(strNames), strArrayLen)
	}
	seen := map[string]string{}
	for _, tbl := range []struct {
		kind  string
		names []string
	}{{"bool", boolNames}, {"num", numNames}, {"str", strNames}} {
		for _, n := range tbl.names {
			if prev, dup := seen[n]; dup {
				t.Errorf("capability %q appears in both the %s and %s tables", n, prev, tbl.kind)
			}
			seen[n] = tbl.kind
		}
	}
}

// Spot-check the index of one capability per region of each table, including
// the boundaries where a transcription slip is most likely.
func TestCapabilityIndexes(t *testing.T) {
	for _, c := range []struct {
		names []string
		name  string
		index int
	}{
		{boolNames, "bw", 0},
		{boolNames, "am", 1},
		{boolNames, "lpix", 36},
		{boolNames, "OTxr", 43},
		{numNames, "cols", 0},
		{numNames, "it", 1},
		{numNames, "lines", 2},
		{numNames, "colors", 13},
		{numNames, "bitype", 32},
		{numNames, "OTkn", 38},
		{strNames, "cbt", 0},
		{strNames, "bel", 1},
		{strNames, "clear", 5},
		{strNames, "cup", 10},
		{strNames, "kf10", 67},
		{strNames, "rfi", 215},
		{strNames, "kf11", 216},
		{strNames, "kf63", 268},
		{strNames, "el1", 269},
		{strNames, "setaf", 359},
		{strNames, "setab", 360},
		{strNames, "slength", 393},
		{strNames, "box1", 413},
	} {
		if got := indexOf(t, c.names, c.name); got != c.index {
			t.Errorf("%q is at index %d, want %d", c.name, got, c.index)
		}
	}
}

func TestKindOf(t *testing.T) {
	for name, want := range map[string]capKind{
		"am":        capBool,
		"cols":      capNum,
		"cup":       capStr,
		"clear":     capStr,
		"nosuchcap": capUnknown,
		"":          capUnknown,
	} {
		if got := kindOf(name); got != want {
			t.Errorf("kindOf(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestParseLegacyEntry(t *testing.T) {
	e, err := parseTerminfo(demoFixture().build(t))
	if err != nil {
		t.Fatalf("parseTerminfo: %v", err)
	}
	if want := []string{"demo", "demo-alias", "a demo terminal"}; strings.Join(e.names, "|") != strings.Join(want, "|") {
		t.Errorf("names = %v, want %v", e.names, want)
	}
	if e.longName() != "a demo terminal" {
		t.Errorf("longName = %q", e.longName())
	}
	if !e.bools["am"] {
		t.Error("am should be true")
	}
	if e.bools["hc"] {
		t.Error("hc was written false and must not read as true")
	}
	if got := e.nums["cols"]; got != 132 {
		t.Errorf("cols = %d, want 132", got)
	}
	if got := e.strs["cup"]; got != "\x1b[%i%p1%d;%p2%dH" {
		t.Errorf("cup = %q", got)
	}
}

// A numeric capability may legitimately be zero. Absence therefore has to live
// in the map's key set, not in the value — otherwise `tput xmc` reports
// "no such capability" for a terminal whose entry says 0.
func TestPresentZeroIsNotAbsent(t *testing.T) {
	e, err := parseTerminfo(demoFixture().build(t))
	if err != nil {
		t.Fatal(err)
	}
	v, ok := e.nums["xmc"]
	if !ok || v != 0 {
		t.Errorf("xmc = (%d, %v), want (0, true)", v, ok)
	}
	if _, ok := e.nums["lm"]; ok {
		t.Error("lm was never written and must be absent")
	}
	if _, ok := e.strs["cuu1"]; ok {
		t.Error("cuu1 was never written and must be absent")
	}
}

// The 32-bit layout differs from the legacy one ONLY in the width of a numeric
// capability; string offsets stay 16-bit in both.
func TestParseExtendedNumberEntry(t *testing.T) {
	f := demoFixture()
	f.wide = true
	f.nums["pairs"] = 32767
	f.nums["colors"] = 256
	data := f.build(t)
	if magic := int16(binary.LittleEndian.Uint16(data)); magic != magicExtended {
		t.Fatalf("fixture wrote magic %o, want %o", magic, magicExtended)
	}
	e, err := parseTerminfo(data)
	if err != nil {
		t.Fatalf("parseTerminfo: %v", err)
	}
	if got := e.nums["pairs"]; got != 32767 {
		t.Errorf("pairs = %d, want 32767", got)
	}
	if got := e.nums["colors"]; got != 256 {
		t.Errorf("colors = %d, want 256", got)
	}
	if got := e.strs["clear"]; got != "\x1b[H\x1b[2J" {
		t.Errorf("clear = %q — a 32-bit numeric section must not shift the string table", got)
	}
}

// The alignment pad between the boolean and numeric sections is inserted only
// when the offset so far is odd, so an entry has to parse either way.
func TestAlignmentPadding(t *testing.T) {
	for _, name := range []string{"demo", "demoX"} { // even and odd names length
		for _, nBools := range []string{"am", "bw"} {
			f := fixture{
				names: []string{name},
				bools: map[string]bool{nBools: true},
				nums:  map[string]int{"cols": 80},
				strs:  map[string]string{"bel": "\a"},
			}
			e, err := parseTerminfo(f.build(t))
			if err != nil {
				t.Fatalf("%s/%s: %v", name, nBools, err)
			}
			if e.nums["cols"] != 80 || e.strs["bel"] != "\a" {
				t.Errorf("%s/%s: misaligned read: nums=%v strs=%v", name, nBools, e.nums, e.strs)
			}
		}
	}
}

// The extended section addresses its two halves differently: a string VALUE
// offset is relative to the table, a NAME offset to the start of the name
// area. Reading names from the table base decodes each one as whatever value
// happens to sit there, which parses cleanly and reports nonsense.
func TestParseUserDefinedCapabilities(t *testing.T) {
	f := demoFixture()
	f.extBools = map[string]bool{"AX": true, "XT": false}
	f.extNums = map[string]int{"U8": 1}
	f.extStrs = map[string]string{"E3": "\x1b[3J", "Smulx": "\x1b[4:%p1%dm"}
	e, err := parseTerminfo(f.build(t))
	if err != nil {
		t.Fatalf("parseTerminfo: %v", err)
	}
	if !e.bools["AX"] {
		t.Error("AX should be a true boolean")
	}
	if e.bools["XT"] {
		t.Error("XT was written false")
	}
	if got := e.nums["U8"]; got != 1 {
		t.Errorf("U8 = %d, want 1", got)
	}
	if got := e.strs["E3"]; got != "\x1b[3J" {
		t.Errorf("E3 = %q, want the clear-scrollback sequence", got)
	}
	if got := e.strs["Smulx"]; got != "\x1b[4:%p1%dm" {
		t.Errorf("Smulx = %q", got)
	}
	if _, ok := e.strs["AX"]; ok {
		t.Error("AX is a boolean and must not also appear as a string")
	}
	// The standard capabilities must survive alongside the extension.
	if got := e.strs["cup"]; got != "\x1b[%i%p1%d;%p2%dH" {
		t.Errorf("cup = %q", got)
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	good := demoFixture().build(t)
	for _, c := range []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"short header", good[:6]},
		{"wrong magic", append([]byte{0x00, 0x00}, good[2:]...)},
		{"truncated body", good[:len(good)-len(good)/2]},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parseTerminfo(c.data); err == nil {
				t.Error("expected an error")
			} else if !errors.Is(err, errBadFormat) {
				t.Errorf("error %v does not wrap errBadFormat", err)
			}
		})
	}
}

// A truncated or malformed EXTENSION must not discard the standard
// capabilities that already parsed: the extension is ncurses' own addition and
// an entry that stops before it is complete and valid.
func TestMalformedExtensionIsIgnored(t *testing.T) {
	f := demoFixture()
	f.extStrs = map[string]string{"E3": "\x1b[3J"}
	data := f.build(t)
	e, err := parseTerminfo(data[:len(data)-4])
	if err != nil {
		t.Fatalf("parseTerminfo: %v", err)
	}
	if got := e.strs["clear"]; got != "\x1b[H\x1b[2J" {
		t.Errorf("clear = %q, want the standard capabilities to survive", got)
	}
}

// --- database search -------------------------------------------------------

func writeEntry(t *testing.T, dir, name string, f fixture, hexBucket bool) {
	t.Helper()
	bucket := string(name[0])
	if hexBucket {
		bucket = strings.ToLower(strings.TrimPrefix(strings.ToUpper(hexOf(name[0])), "0X"))
	}
	full := filepath.Join(dir, bucket)
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(full, name), f.build(t), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hexOf(c byte) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[c>>4], digits[c&0xf]})
}

func envFunc(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

func TestLookupHonoursSearchOrder(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()

	early := demoFixture()
	early.names = []string{"demo", "from TERMINFO"}
	writeEntry(t, first, "demo", early, false)

	late := demoFixture()
	late.names = []string{"demo", "from TERMINFO_DIRS"}
	writeEntry(t, second, "demo", late, false)

	e, err := loadEntry(envFunc(map[string]string{
		"TERMINFO":      first,
		"TERMINFO_DIRS": second,
	}), "demo")
	if err != nil {
		t.Fatalf("loadEntry: %v", err)
	}
	if e.longName() != "from TERMINFO" {
		t.Errorf("resolved %q; $TERMINFO must be searched before $TERMINFO_DIRS", e.longName())
	}

	e, err = loadEntry(envFunc(map[string]string{"TERMINFO_DIRS": second}), "demo")
	if err != nil {
		t.Fatalf("loadEntry: %v", err)
	}
	if e.longName() != "from TERMINFO_DIRS" {
		t.Errorf("resolved %q, want the TERMINFO_DIRS entry", e.longName())
	}
}

// A case-insensitive filesystem cannot keep the "A" and "a" buckets apart, so
// those installations name the bucket with the character's hex value. Missing
// that spelling means silently ignoring the whole system database on macOS.
func TestLookupFindsHexBucketDirectory(t *testing.T) {
	dir := t.TempDir()
	f := demoFixture()
	f.names = []string{"xdemo", "hex bucket"}
	writeEntry(t, dir, "xdemo", f, true)

	if _, err := os.Stat(filepath.Join(dir, "78", "xdemo")); err != nil {
		t.Fatalf("fixture did not write the hex bucket: %v", err)
	}
	e, err := loadEntry(envFunc(map[string]string{"TERMINFO": dir}), "xdemo")
	if err != nil {
		t.Fatalf("loadEntry: %v", err)
	}
	if e.longName() != "hex bucket" {
		t.Errorf("longName = %q", e.longName())
	}
}

func TestLookupSearchesHomeTerminfo(t *testing.T) {
	home := t.TempDir()
	f := demoFixture()
	f.names = []string{"hdemo", "from home"}
	writeEntry(t, filepath.Join(home, ".terminfo"), "hdemo", f, false)

	env := map[string]string{"HOME": home}
	if runtime.GOOS == "windows" {
		env["USERPROFILE"] = home
	}
	e, err := loadEntry(envFunc(env), "hdemo")
	if err != nil {
		t.Fatalf("loadEntry: %v", err)
	}
	if e.longName() != "from home" {
		t.Errorf("longName = %q", e.longName())
	}
}

// An empty element of TERMINFO_DIRS means "the compiled-in default list".
// Dropping it would quietly change which entry wins.
func TestTerminfoDirsEmptyElementMeansTheDefaults(t *testing.T) {
	dirs := terminfoDirs(envFunc(map[string]string{
		"TERMINFO_DIRS": "/one" + string(os.PathListSeparator) + string(os.PathListSeparator) + "/two",
	}))
	var got []string
	for _, d := range dirs {
		if d == "/one" || d == "/two" || d == systemDirs[0] {
			got = append(got, d)
		}
	}
	want := []string{"/one", systemDirs[0], "/two"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", got, want)
	}
}

// A terminal name becomes a path component, so anything that could escape the
// database directory is refused rather than opened.
func TestLookupRefusesPathTraversal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "secret"), demoFixture().build(t), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../secret", "..", ".", "a/b", `a\b`} {
		if _, err := loadEntry(envFunc(map[string]string{"TERMINFO": dir}), name); !errors.Is(err, errUnknownTerm) {
			t.Errorf("loadEntry(%q) error = %v, want errUnknownTerm", name, err)
		}
	}
}

func TestLookupUnknownTerminal(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"", "definitely-not-a-terminal"} {
		if _, err := loadEntry(envFunc(map[string]string{"TERMINFO": dir}), name); !errors.Is(err, errUnknownTerm) {
			t.Errorf("loadEntry(%q) error = %v, want errUnknownTerm", name, err)
		}
	}
}

// The compiled-in table is a floor for hosts with no database — never an
// override of one that exists.
func TestOnDiskEntryBeatsTheBuiltIn(t *testing.T) {
	dir := t.TempDir()
	f := demoFixture()
	f.names = []string{"vt100", "the administrator's vt100"}
	f.strs["clear"] = "<local>"
	writeEntry(t, dir, "vt100", f, false)

	e, err := loadEntry(envFunc(map[string]string{"TERMINFO": dir}), "vt100")
	if err != nil {
		t.Fatalf("loadEntry: %v", err)
	}
	if e.strs["clear"] != "<local>" {
		t.Errorf("clear = %q, want the on-disk entry to win", e.strs["clear"])
	}

	// The other half of the rule — falling back when nothing is on disk — can
	// only be asserted on a host that actually has no entry for the type. That
	// is the situation the fallback exists for (Windows, a scratch container);
	// where a system database does have one, finding it IS the correct result.
	env := envFunc(map[string]string{"TERMINFO": t.TempDir()})
	e, err = loadEntry(env, "vt100")
	if err != nil {
		t.Fatalf("loadEntry: %v", err)
	}
	if onDisk := entryIsOnDisk(env, "vt100"); onDisk != (e.source != "(built-in)") {
		t.Errorf("source = %q with on-disk=%v", e.source, onDisk)
	}
}

func entryIsOnDisk(getenv func(string) string, term string) bool {
	for _, dir := range terminfoDirs(getenv) {
		for _, p := range candidatePaths(dir, term) {
			if _, err := os.Stat(p); err == nil {
				return true
			}
		}
	}
	return false
}

func TestBuiltinEntries(t *testing.T) {
	for _, name := range []string{"dumb", "vt100", "vt100-am", "ansi", "xterm", "xterm-color", "xterm-256color"} {
		e := builtinEntry(name)
		if e == nil {
			t.Errorf("%s: no built-in entry", name)
			continue
		}
		if e.longName() == "" {
			t.Errorf("%s: no description", name)
		}
		if _, ok := e.strs["bel"]; !ok {
			t.Errorf("%s: no bel", name)
		}
	}
	if builtinEntry("no-such-builtin") != nil {
		t.Error("builtinEntry answered for a type it does not carry")
	}
	// The description is not an alias: `tput -T "a demo terminal"` must fail.
	if builtinEntry("80-column dumb tty") != nil {
		t.Error("the long description must not match as an alias")
	}
	if !strings.Contains(builtinNames(), "xterm-256color") {
		t.Errorf("builtinNames() = %q", builtinNames())
	}
	if strings.Contains(builtinNames(), "dumb tty") {
		t.Errorf("builtinNames() should list aliases, not descriptions: %q", builtinNames())
	}
	// Every parameterized built-in string must be a valid capability string.
	for _, def := range builtins {
		for name, s := range def.strs {
			if _, err := tparm(s, nums(1, 1, 1, 1, 1, 1, 1, 1, 1)); err != nil {
				t.Errorf("built-in %s %s = %q: %v", def.names[0], name, s, err)
			}
		}
	}
}
