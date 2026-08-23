package gosed

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/qiangli/coreutils/pkg/bre"
)

type sedTestCtype struct{}

func sedAlpha(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b == 0xc9 || b == 0xe9
}
func (sedTestCtype) IsAlpha(b byte) (bool, error) { return sedAlpha(b), nil }
func (sedTestCtype) IsAlnum(b byte) (bool, error) { return sedAlpha(b) || b >= '0' && b <= '9', nil }
func (sedTestCtype) IsBlank(b byte) (bool, error) { return b == ' ' || b == '\t', nil }
func (sedTestCtype) IsCntrl(b byte) (bool, error) { return b < 0x20 || b == 0x7f, nil }
func (sedTestCtype) IsDigit(b byte) (bool, error) { return b >= '0' && b <= '9', nil }
func (sedTestCtype) IsGraph(b byte) (bool, error) { return b > 0x20 && b != 0x7f, nil }
func (sedTestCtype) IsLower(b byte) (bool, error) { return b >= 'a' && b <= 'z' || b == 0xe9, nil }
func (sedTestCtype) IsPrint(b byte) (bool, error) { return b >= 0x20 && b != 0x7f, nil }
func (sedTestCtype) IsPunct(b byte) (bool, error) { return b == '!' || b == '.', nil }
func (sedTestCtype) IsSpace(b byte) (bool, error) {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r', nil
}
func (sedTestCtype) IsUpper(b byte) (bool, error) { return b >= 'A' && b <= 'Z' || b == 0xc9, nil }
func (sedTestCtype) IsXDigit(b byte) (bool, error) {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F', nil
}
func (sedTestCtype) ToLower(in []byte) ([]byte, error) {
	out := append([]byte(nil), in...)
	for i, b := range out {
		if b >= 'A' && b <= 'Z' {
			out[i] = b + ('a' - 'A')
		} else if b == 0xc9 {
			out[i] = 0xe9
		}
	}
	return out, nil
}

func sedLocaleTables(t *testing.T) *bre.LocaleByteTables {
	t.Helper()
	tables, err := bre.SnapshotLocaleByteTables(sedTestCtype{})
	if err != nil {
		t.Fatal(err)
	}
	return tables
}

func runLocaleSed(t *testing.T, program string, input []byte, opts Options) []byte {
	t.Helper()
	engine, err := NewWithOptions(strings.NewReader(program), opts)
	if err != nil {
		t.Fatalf("NewWithOptions(%q): %v", program, err)
	}
	out, err := engine.RunString(string(input))
	if err != nil {
		t.Fatalf("RunString(%q): %v", program, err)
	}
	return []byte(out)
}

func TestLocaleCompileREFlagsAndSyntax(t *testing.T) {
	tables := sedLocaleTables(t)
	for _, tc := range []struct {
		name, pattern, flags, input string
		extended                    bool
		match                       bool
	}{
		{"BRE", `^\(a\|b\)\+$`, "", "ab", false, true},
		{"ERE", `^(a|b)+$`, "", "ab", true, true},
		{"fold high", string([]byte{0xc9}), "(?i)", string([]byte{0xe9}), false, true},
		{"dot default newline", `.`, "", "\n", false, true},
		{"dot multiline newline", `.`, "(?m)", "\n", false, false},
		{"multiline anchors", `^a$`, "(?m)", "x\na\ny", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			re, err := (Options{ExtendedRegex: tc.extended, LocaleTables: tables}).compileRE(tc.pattern, tc.flags)
			if err != nil {
				t.Fatal(err)
			}
			got, err := re.MatchString(tc.input)
			if err != nil || got != tc.match {
				t.Fatalf("match=%v err=%v, want %v", got, err, tc.match)
			}
		})
	}
}

func TestLocaleEverySedCompileSeam(t *testing.T) {
	opts := Options{LocaleTables: sedLocaleTables(t)}
	high := string([]byte{0xe9})
	if got := runLocaleSed(t, `s/[[:alpha:]]/X/g`, append([]byte{0xe9}, '\n'), opts); string(got) != "X\n" {
		t.Fatalf("substitution=%q", got)
	}
	if got := runLocaleSed(t, `/[[:alpha:]]/s/.*/X/`, append([]byte{0xe9}, '\n'), opts); string(got) != "X\n" {
		t.Fatalf("address=%q", got)
	}
	highRE, err := opts.compileRE("\\("+high+"\\)", "")
	if err != nil {
		t.Fatal(err)
	}
	highMatches, err := highRE.FindAllSubmatchIndex([]byte{0xe9}, -1)
	if err != nil || len(highMatches) != 1 {
		t.Fatalf("raw expansion matches=%v err=%v", highMatches, err)
	}
	if got, err := highRE.Expand(nil, []byte(`<${1}>`), []byte{0xe9}, highMatches[0]); err != nil || string(got) != "<"+high+">" {
		t.Fatalf("raw expansion=%q err=%v", got, err)
	}
	rx, replacement, err := CompileSimpleSubstitution(`[[:alpha:]]`, `<&>`, opts)
	if err != nil {
		t.Fatal(err)
	}
	matches, err := rx.FindAllSubmatchIndex([]byte{0xe9}, -1)
	if err != nil || len(matches) != 1 {
		t.Fatalf("fast matches=%v err=%v", matches, err)
	}
	if got, err := rx.Expand(nil, replacement, []byte{0xe9}, matches[0]); err != nil || string(got) != "<"+high+">" {
		t.Fatalf("fast expansion=%q err=%v", got, err)
	}
	if got := runLocaleSed(t, "s/a/X/;s//Y/", []byte("a\n"), opts); string(got) != "X\n" {
		t.Fatalf("null reuse=%q", got)
	}
	nullInstruction, err := newSubstitution(opts, "", "X", "", "", "a", func(string, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	fallbackVM := &vm{pat: "a"}
	if err := nullInstruction(fallbackVM); err != nil || fallbackVM.pat != "X" {
		t.Fatalf("null fallback pat=%q err=%v", fallbackVM.pat, err)
	}
	reused, err := opts.compileRE("b", "")
	if err != nil {
		t.Fatal(err)
	}
	reuseVM := &vm{pat: "b", lastRE: reused}
	if err := nullInstruction(reuseVM); err != nil || reuseVM.pat != "X" {
		t.Fatalf("null dynamic reuse pat=%q err=%v", reuseVM.pat, err)
	}
	if got := runLocaleSed(t, "s/A/X/I", []byte("a\n"), opts); string(got) != "X\n" {
		t.Fatalf("uppercase I=%q", got)
	}
	rawFoldProgram := string([]byte{'s', '/', 0xc9, '/', 'X', '/', 'I', 'g'})
	if got := runLocaleSed(t, rawFoldProgram, []byte{0xc9, 0xe9, '\n'}, opts); string(got) != "XX\n" {
		t.Fatalf("raw single-byte pattern and fold=%x", got)
	}
	rawBracketProgram := string([]byte{'s', '/', '[', 0xc9, ']', '/', 'Y', '/'})
	if got := runLocaleSed(t, rawBracketProgram, []byte{0xc9, '\n'}, opts); string(got) != "Y\n" {
		t.Fatalf("raw single-byte bracket=%x", got)
	}
	rawAddressProgram := string([]byte{'/', 0xc9, '/', 's', '/', '.', '/', 'Z', '/'})
	if got := runLocaleSed(t, rawAddressProgram, []byte{0xc9, '\n'}, opts); string(got) != "Z\n" {
		t.Fatalf("raw single-byte address=%x", got)
	}
	rawReplacementProgram := string([]byte{'s', '/', 'A', '/', 0xe9, '/'})
	if got := runLocaleSed(t, rawReplacementProgram, []byte("A\n"), opts); !bytes.Equal(got, []byte{0xe9, '\n'}) {
		t.Fatalf("raw single-byte replacement=%x", got)
	}
	rawDelimiterProgram := string([]byte{'s', 0xc9, '\\', 0xc9, 0xc9, 'D', 0xc9})
	if got := runLocaleSed(t, rawDelimiterProgram, []byte{0xc9, '\n'}, opts); string(got) != "D\n" {
		t.Fatalf("raw single-byte escaped delimiter=%x", got)
	}
	rawAddressDelimiterProgram := string([]byte{'\\', 0xc9, 'A', 0xc9, 's', '/', 'A', '/', 'Z', '/'})
	if got := runLocaleSed(t, rawAddressDelimiterProgram, []byte("A\n"), opts); string(got) != "Z\n" {
		t.Fatalf("raw single-byte address delimiter=%x", got)
	}
	rawTranslationDelimiterProgram := string([]byte{'y', 0xc9, 'A', 0xc9, 'B', 0xc9})
	if got := runLocaleSed(t, rawTranslationDelimiterProgram, []byte("A\n"), opts); string(got) != "B\n" {
		t.Fatalf("raw single-byte translation delimiter=%x", got)
	}
	if got := runLocaleSed(t, "séAéBé", []byte("A\n"), opts); string(got) != "B\n" {
		t.Fatalf("valid UTF-8 delimiter=%x", got)
	}
	if got := runLocaleSed(t, "N;s/./X/g", []byte("a\nb\n"), opts); string(got) != "XXX\n" {
		t.Fatalf("default dot N=%q", got)
	}
	if got := runLocaleSed(t, "N;s/./X/gm", []byte("a\nb\n"), opts); string(got) != "X\nX\n" {
		t.Fatalf("multiline dot N=%q", got)
	}
	if got := runLocaleSed(t, "N;s/^b$/X/M", []byte("a\nb\n"), opts); string(got) != "a\nX\n" {
		t.Fatalf("uppercase M=%q", got)
	}
}

func TestLocaleCompileSeamsRejectUnsupported(t *testing.T) {
	opts := Options{LocaleTables: sedLocaleTables(t)}
	for _, pattern := range []string{`\b`, `\1`, `[a-z]`, `[[=ab=]]`, `[[.ab.]]`, `a\{1001\}`} {
		if _, err := opts.compileRE(pattern, ""); err == nil {
			t.Errorf("compileRE(%q) succeeded", pattern)
		}
		if _, _, err := CompileSimpleSubstitution(pattern, "x", opts); err == nil {
			t.Errorf("fast compile(%q) succeeded", pattern)
		}
		if _, err := newRECondition(opts, pattern, "", &location{}); err == nil {
			t.Errorf("address compile(%q) succeeded", pattern)
		}
		if _, err := newSubstitution(opts, pattern, "x", "", "", "", nil); err == nil {
			t.Errorf("substitution compile(%q) succeeded", pattern)
		}
	}
}

func TestLocaleOptionsConcurrentNoGlobals(t *testing.T) {
	tables := sedLocaleTables(t)
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for worker := 0; worker < 12; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			opts := Options{LocaleTables: tables, ExtendedRegex: worker%2 != 0}
			pattern := `^\(a\|b\)\+$`
			if opts.ExtendedRegex {
				pattern = `^(a|b)+$`
			}
			for i := 0; i < 10; i++ {
				re, err := opts.compileRE(pattern, "")
				if err != nil {
					errs <- err
					return
				}
				matched, err := re.MatchString("ab")
				if err != nil || !matched {
					errs <- fmt.Errorf("matched=%v err=%v", matched, err)
					return
				}
			}
		}(worker)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
