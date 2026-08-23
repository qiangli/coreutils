package tputcmd

import (
	"bytes"
	"encoding/binary"
	"sort"
	"testing"
)

// fixture builds a compiled terminfo file in memory.
//
// Every terminfo test in this package drives one of these rather than the
// host's database. The database a developer happens to have installed is not
// a fixture: it varies by OS and ncurses version, it cannot express the
// corrupt and edge-case inputs the parser has to survive, and a test that
// reads it passes or fails for reasons that have nothing to do with the code.
type fixture struct {
	names []string
	bools map[string]bool
	nums  map[string]int
	strs  map[string]string

	// wide selects the extended-number layout (32-bit numeric capabilities).
	wide bool

	// The ncurses user-defined capability section. It is emitted only when at
	// least one of these is non-empty.
	extBools map[string]bool
	extNums  map[string]int
	extStrs  map[string]string
}

func indexOf(t *testing.T, list []string, name string) int {
	t.Helper()
	for i, n := range list {
		if n == name {
			return i
		}
	}
	t.Fatalf("no such capability %q", name)
	return -1
}

func (f fixture) build(t *testing.T) []byte {
	t.Helper()

	// Array lengths are "highest used index + 1": that is what a real compiler
	// emits, and it is what exercises the reader's reliance on the header
	// counts rather than on the length of its own name tables.
	nBools, nNums, nStrs := 0, 0, 0
	boolIdx := map[int]bool{}
	for name, v := range f.bools {
		i := indexOf(t, boolNames, name)
		boolIdx[i] = v
		if i+1 > nBools {
			nBools = i + 1
		}
	}
	numIdx := map[int]int{}
	for name, v := range f.nums {
		i := indexOf(t, numNames, name)
		numIdx[i] = v
		if i+1 > nNums {
			nNums = i + 1
		}
	}
	strIdx := map[int]string{}
	for name, v := range f.strs {
		i := indexOf(t, strNames, name)
		strIdx[i] = v
		if i+1 > nStrs {
			nStrs = i + 1
		}
	}

	nameBlob := []byte(joinNames(f.names))
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
	if f.wide {
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
		if f.wide {
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

	if len(f.extBools)+len(f.extNums)+len(f.extStrs) == 0 {
		return out.Bytes()
	}

	if out.Len()%2 == 1 {
		out.WriteByte(0)
	}
	extBoolNames := sortedKeys(f.extBools)
	extNumNames := sortedKeys(f.extNums)
	extStrNames := sortedKeys(f.extStrs)

	var extTable bytes.Buffer
	valOffsets := make([]int16, 0, len(extStrNames))
	for _, n := range extStrNames {
		valOffsets = append(valOffsets, int16(extTable.Len()))
		extTable.WriteString(f.extStrs[n])
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
		if f.extBools[n] {
			out.WriteByte(1)
		} else {
			out.WriteByte(0)
		}
	}
	if len(extBoolNames)%2 == 1 {
		out.WriteByte(0)
	}
	for _, n := range extNumNames {
		putNum(&out, f.extNums[n])
	}
	for _, off := range valOffsets {
		put16(&out, int(off))
	}
	for _, off := range nameOffsets {
		put16(&out, int(off))
	}
	out.Write(extTable.Bytes())
	return out.Bytes()
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

// demoFixture is a small but realistic entry used by several tests.
func demoFixture() fixture {
	return fixture{
		names: []string{"demo", "demo-alias", "a demo terminal"},
		bools: map[string]bool{"am": true, "bce": true, "hc": false},
		nums:  map[string]int{"cols": 132, "lines": 50, "xmc": 0},
		strs: map[string]string{
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
