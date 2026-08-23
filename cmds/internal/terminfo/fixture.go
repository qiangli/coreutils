package terminfo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Fixture builds a compiled terminfo file in memory — the WRITER that mirrors
// the reader in terminfo.go.
//
// Every terminfo test drives one of these rather than the host's database, and
// so do the tests of the commands built on this package (tput, tabs). The
// database a developer happens to have installed is not a fixture: it varies
// by OS and ncurses version, it cannot express the corrupt and edge-case
// inputs the parser has to survive, and a test that reads it passes or fails
// for reasons that have nothing to do with the code.
//
// It lives in a normal source file rather than a _test.go so that the command
// packages can reach it without a second copy of the compiled format; nothing
// in the shipped code path references it, so the linker drops it.
type Fixture struct {
	Names []string
	Bools map[string]bool
	Nums  map[string]int
	Strs  map[string]string

	// Wide selects the extended-number layout (32-bit numeric capabilities).
	Wide bool

	// The ncurses user-defined capability section. It is emitted only when at
	// least one of these is non-empty.
	ExtBools map[string]bool
	ExtNums  map[string]int
	ExtStrs  map[string]string
}

func indexOfCap(list []string, name string) (int, error) {
	for i, n := range list {
		if n == name {
			return i, nil
		}
	}
	return -1, fmt.Errorf("no such capability %q", name)
}

// Build renders the fixture as the bytes of a compiled terminfo file.
func (f Fixture) Build() ([]byte, error) {
	// Array lengths are "highest used index + 1": that is what a real compiler
	// emits, and it is what exercises the reader's reliance on the header
	// counts rather than on the length of its own name tables.
	nBools, nNums, nStrs := 0, 0, 0
	boolIdx := map[int]bool{}
	for name, v := range f.Bools {
		i, err := indexOfCap(boolNames, name)
		if err != nil {
			return nil, err
		}
		boolIdx[i] = v
		if i+1 > nBools {
			nBools = i + 1
		}
	}
	numIdx := map[int]int{}
	for name, v := range f.Nums {
		i, err := indexOfCap(numNames, name)
		if err != nil {
			return nil, err
		}
		numIdx[i] = v
		if i+1 > nNums {
			nNums = i + 1
		}
	}
	strIdx := map[int]string{}
	for name, v := range f.Strs {
		i, err := indexOfCap(strNames, name)
		if err != nil {
			return nil, err
		}
		strIdx[i] = v
		if i+1 > nStrs {
			nStrs = i + 1
		}
	}

	nameBlob := []byte(joinNames(f.Names))
	var table bytes.Buffer
	offsets := make([]int16, nStrs)
	for i := range offsets {
		offsets[i] = valAbsent
	}
	for i := 0; i < nStrs; i++ {
		s, ok := strIdx[i]
		if !ok {
			continue
		}
		offsets[i] = int16(table.Len())
		table.WriteString(s)
		table.WriteByte(0)
	}

	magic := int16(magicLegacy)
	if f.Wide {
		magic = magicExtended
	}
	var out bytes.Buffer
	put16 := func(b *bytes.Buffer, v int) {
		_ = binary.Write(b, binary.LittleEndian, int16(v))
	}
	put32 := func(b *bytes.Buffer, v int) {
		_ = binary.Write(b, binary.LittleEndian, int32(v))
	}
	putNum := func(b *bytes.Buffer, v int) {
		if f.Wide {
			put32(b, v)
		} else {
			put16(b, v)
		}
	}

	put16(&out, int(magic))
	put16(&out, len(nameBlob))
	put16(&out, nBools)
	put16(&out, nNums)
	put16(&out, nStrs)
	put16(&out, table.Len())
	out.Write(nameBlob)
	for i := 0; i < nBools; i++ {
		if boolIdx[i] {
			out.WriteByte(1)
		} else {
			out.WriteByte(0)
		}
	}
	if out.Len()%2 == 1 {
		out.WriteByte(0)
	}
	for i := 0; i < nNums; i++ {
		if v, ok := numIdx[i]; ok {
			putNum(&out, v)
		} else {
			putNum(&out, valAbsent)
		}
	}
	for _, off := range offsets {
		put16(&out, int(off))
	}
	out.Write(table.Bytes())

	if len(f.ExtBools)+len(f.ExtNums)+len(f.ExtStrs) == 0 {
		return out.Bytes(), nil
	}

	if out.Len()%2 == 1 {
		out.WriteByte(0)
	}
	extBoolNames := sortedKeys(f.ExtBools)
	extNumNames := sortedKeys(f.ExtNums)
	extStrNames := sortedKeys(f.ExtStrs)

	var extTable bytes.Buffer
	valOffsets := make([]int16, 0, len(extStrNames))
	for _, n := range extStrNames {
		valOffsets = append(valOffsets, int16(extTable.Len()))
		extTable.WriteString(f.ExtStrs[n])
		extTable.WriteByte(0)
	}
	// Name offsets are relative to the START OF THE NAME AREA, not to the
	// table — the format's sharpest edge, and the reason this fixture writes
	// the two halves with different bases on purpose.
	nameBase := extTable.Len()
	nameOffsets := make([]int16, 0, len(extBoolNames)+len(extNumNames)+len(extStrNames))
	for _, n := range append(append(append([]string{}, extBoolNames...), extNumNames...), extStrNames...) {
		nameOffsets = append(nameOffsets, int16(extTable.Len()-nameBase))
		extTable.WriteString(n)
		extTable.WriteByte(0)
	}

	put16(&out, len(extBoolNames))
	put16(&out, len(extNumNames))
	put16(&out, len(extStrNames))
	put16(&out, len(valOffsets)+len(nameOffsets))
	put16(&out, extTable.Len())
	for _, n := range extBoolNames {
		if f.ExtBools[n] {
			out.WriteByte(1)
		} else {
			out.WriteByte(0)
		}
	}
	if len(extBoolNames)%2 == 1 {
		out.WriteByte(0)
	}
	for _, n := range extNumNames {
		putNum(&out, f.ExtNums[n])
	}
	for _, off := range valOffsets {
		put16(&out, int(off))
	}
	for _, off := range nameOffsets {
		put16(&out, int(off))
	}
	out.Write(extTable.Bytes())
	return out.Bytes(), nil
}

func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += "|"
		}
		out += n
	}
	return out + "\x00"
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// WriteEntry compiles f and files it in dir under the bucket a database
// lookup will look in.
//
// hexBucket selects the spelling case-insensitive filesystems use — the
// character's hexadecimal value ("78/xterm") instead of the character itself
// ("x/xterm") — so a test can exercise both halves of candidatePaths.
func WriteEntry(dir, name string, f Fixture, hexBucket bool) error {
	if name == "" {
		return fmt.Errorf("terminfo: empty entry name")
	}
	data, err := f.Build()
	if err != nil {
		return err
	}
	bucket := string(name[0])
	if hexBucket {
		const digits = "0123456789abcdef"
		bucket = string([]byte{digits[name[0]>>4], digits[name[0]&0xf]})
	}
	full := filepath.Join(dir, bucket)
	if err := os.MkdirAll(full, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(full, name), data, 0o644)
}

// DemoFixture is a small but realistic entry used by several tests.
//
// Its oddities are all deliberate: `hc` is written FALSE (so a false boolean
// must not read as true), `xmc` is written ZERO (so a present zero must not
// read as absent), and `el` carries a padding delay (so the delay must not
// reach stdout).
func DemoFixture() Fixture {
	return Fixture{
		Names: []string{"demo", "demo-alias", "a demo terminal"},
		Bools: map[string]bool{"am": true, "bce": true, "hc": false},
		Nums:  map[string]int{"cols": 132, "lines": 50, "xmc": 0},
		Strs: map[string]string{
			"bel":   "\a",
			"clear": "\x1b[H\x1b[2J",
			"cup":   "\x1b[%i%p1%d;%p2%dH",
			"is1":   "<is1>",
			"is2":   "<is2>",
			"is3":   "<is3>",
			"rs2":   "<rs2>",
			"el":    "\x1b[K$<5>",
		},
	}
}
