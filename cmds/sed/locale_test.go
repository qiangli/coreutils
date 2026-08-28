package sedcmd

// Locale-category regressions for the Profile D sed residuals.
//
// Two distinct defects are pinned here.
//
// The first was a fail-closed locale check: sed resolved LC_CTYPE and
// LC_COLLATE by comparing against the two literal names "C" and "POSIX",
// so a certification shell exporting LC_ALL=C.UTF-8 — a C locale, spelled
// with its codeset — reached pkg/ctype, which rejects the name, and every
// sed invocation exited 2 before parsing its script. That is why the
// residuals were UNRESOLVED rather than FAIL: no test point ever ran.
//
// The second is the repair's own hazard, and is the reason these cases
// enumerate the categories PAIRWISE. It is not enough to notice that a
// name carries a UTF-8 codeset; POSIX.1 Issue 7 XBD 7.3 gives LC_CTYPE
// and LC_COLLATE disjoint jobs, so a UTF-8 LC_COLLATE must not select a
// character model and a UTF-8 LC_CTYPE must not select a collation.
// Deriving one flag from both categories passes a same-locale test suite
// and is still wrong for every mixed environment.

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// TestSedAcceptsCarriedUTF8LocaleAliases is the Profile D residual itself:
// the exact invocation environments a certification shell stages. Each must
// run the script, not refuse the locale.
func TestSedAcceptsCarriedUTF8LocaleAliases(t *testing.T) {
	for _, env := range [][]string{
		{"LC_ALL=C.UTF-8"},
		{"LANG=C.UTF-8"},
		{"LC_CTYPE=POSIX.UTF8", "LC_COLLATE=C.utf8"},
		{"LC_ALL=C.UTF-8@euro"},
		{"LC_ALL=POSIX.utf-8"},
		// LC_ALL outranks both categories, including a name that would
		// otherwise be refused.
		{"LC_ALL=C.UTF-8", "LC_CTYPE=en_US.UTF-8", "LC_COLLATE=en_US.UTF-8"},
	} {
		out, errOut, code := runSedInDirEnv(t, t.TempDir(), env, "abc\n", "s/[[:alpha:]]/X/g")
		if code != 0 || errOut != "" || out != "XXX\n" {
			t.Errorf("env %v: got (%q, %q, %d), want (\"XXX\\n\", no diagnostic, 0)", env, out, errOut, code)
		}
	}
}

// TestSedResolvesEachLocaleCategoryFromItsOwnVariable is the mixed-locale
// regression. A provider is opened only when its OWN category names a
// locale that needs one; the openers panic when the other category's value
// leaks across.
func TestSedResolvesEachLocaleCategoryFromItsOwnVariable(t *testing.T) {
	const german = "de_DE.iso88591"
	tests := []struct {
		name            string
		ctype, collate  string
		wantCTypeOpen   string // "" means the provider must not be opened
		wantCollateOpen string
	}{
		{"both C", "C", "C", "", ""},
		{"both C UTF-8", "C.UTF-8", "C.UTF-8", "", ""},
		// The two cases the rejected repair got wrong: one category
		// carrying a UTF-8 codeset says nothing about the other.
		{"UTF-8 ctype, C collate", "C.UTF-8", "C", "", ""},
		{"C ctype, UTF-8 collate", "C", "C.UTF-8", "", ""},
		// A provider locale in one category must not pull the other onto
		// a provider, and a UTF-8 alias in the other must not suppress it.
		{"provider ctype, UTF-8 collate", german, "C.UTF-8", german, ""},
		{"UTF-8 ctype, provider collate", "C.UTF-8", german, "", german},
		{"provider ctype, C collate", german, "C", german, ""},
		{"C ctype, provider collate", "C", german, "", german},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctypeOpened, collateOpened := "", ""
			var out, errOut bytes.Buffer
			rc := &tool.RunContext{
				Ctx:   context.Background(),
				Dir:   t.TempDir(),
				Env:   []string{"LANG=C", "LC_CTYPE=" + tc.ctype, "LC_COLLATE=" + tc.collate},
				Stdio: tool.Stdio{In: strings.NewReader("abc\n"), Out: &out, Err: &errOut},
			}
			code := runCommandWithLocales(rc, []string{"s/b/B/"},
				func(name string) (ctypeProvider, error) {
					if tc.wantCTypeOpen == "" {
						t.Fatalf("LC_CTYPE=%q LC_COLLATE=%q opened a ctype provider for %q",
							tc.ctype, tc.collate, name)
					}
					ctypeOpened = name
					return &fakeSedCType{}, nil
				},
				func(name string) (collateProvider, error) {
					if tc.wantCollateOpen == "" {
						t.Fatalf("LC_CTYPE=%q LC_COLLATE=%q opened a collate provider for %q",
							tc.ctype, tc.collate, name)
					}
					collateOpened = name
					return &fakeSedCollate{}, nil
				})
			if code != 0 || errOut.String() != "" || out.String() != "aBc\n" {
				t.Fatalf("got (%q, %q, %d), want (\"aBc\\n\", no diagnostic, 0)",
					out.String(), errOut.String(), code)
			}
			if ctypeOpened != tc.wantCTypeOpen {
				t.Errorf("ctype provider opened for %q, want %q", ctypeOpened, tc.wantCTypeOpen)
			}
			if collateOpened != tc.wantCollateOpen {
				t.Errorf("collate provider opened for %q, want %q", collateOpened, tc.wantCollateOpen)
			}
		})
	}
}

// TestSedCollateCategoryOwnsEquivalenceClasses proves the split is
// semantic and not merely bookkeeping: with a UTF-8 LC_CTYPE, whether
// [[=e=]] has members is decided by LC_COLLATE alone.
func TestSedCollateCategoryOwnsEquivalenceClasses(t *testing.T) {
	tests := []struct {
		name    string
		collate string
		want    string
	}{
		{"C collation has no equivalents", "C.UTF-8", "Eé\n"},
		{"provider collation supplies them", "de_DE.iso88591", "EE\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			rc := &tool.RunContext{
				Ctx:   context.Background(),
				Dir:   t.TempDir(),
				Env:   []string{"LANG=C", "LC_CTYPE=C.UTF-8", "LC_COLLATE=" + tc.collate},
				Stdio: tool.Stdio{In: strings.NewReader("eé\n"), Out: &out, Err: &errOut},
			}
			code := runCommandWithLocales(rc, []string{"s/[[=e=]]/E/g"},
				func(string) (ctypeProvider, error) { return &fakeSedCType{}, nil },
				func(string) (collateProvider, error) { return &fakeSedCollate{}, nil })
			if code != 0 || errOut.String() != "" || out.String() != tc.want {
				t.Fatalf("LC_COLLATE=%s: got (%q, %q, %d), want %q",
					tc.collate, out.String(), errOut.String(), code, tc.want)
			}
		})
	}
}

// TestSedCTypeAloneSelectsByteOrCharacterMatching is the mixed-category
// regression at the matcher seam. A UTF-8 collation cannot make C LC_CTYPE's
// dot consume a whole UTF-8 character, and a C collation cannot make a UTF-8
// LC_CTYPE dot consume only its first byte.
func TestSedCTypeAloneSelectsByteOrCharacterMatching(t *testing.T) {
	tests := []struct {
		name string
		env  []string
		want string
	}{
		{"C ctype, UTF-8 collate", []string{"LC_CTYPE=C", "LC_COLLATE=C.UTF-8"}, "XX\n"},
		{"UTF-8 ctype, C collate", []string{"LC_CTYPE=C.UTF-8", "LC_COLLATE=C"}, "X\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runSedInDirEnv(t, t.TempDir(), tc.env, "é\n", "s/./X/g")
			if code != 0 || errOut != "" || out != tc.want {
				t.Fatalf("got (%q, %q, %d), want %q", out, errOut, code, tc.want)
			}
		})
	}
}

// TestSedCUTF8MatchesGlibcClasses pins values where Go's Unicode classes are
// not a substitute for glibc C.UTF-8.
func TestSedCUTF8MatchesGlibcClasses(t *testing.T) {
	input := "é٣\u00a0\n"
	tests := []struct {
		class, want string
	}{
		{"alpha", "XX\u00a0\n"},
		{"alnum", "XX\u00a0\n"},
		{"digit", input},
		{"graph", "XXX\n"},
		{"print", "XXX\n"},
		{"punct", "é٣X\n"},
		{"space", input},
	}
	for _, tc := range tests {
		out, errOut, code := runSedInDirEnv(t, t.TempDir(), []string{"LC_ALL=C.UTF-8"}, input,
			"s/[[:"+tc.class+":]]/X/g")
		if code != 0 || errOut != "" || out != tc.want {
			t.Errorf("class %s = (%q, %q, %d), want %q", tc.class, out, errOut, code, tc.want)
		}
	}
}

// TestSedRejectsUncarriedLocalesByCategory keeps the repair fail-closed:
// widening the C/POSIX test to cover the UTF-8 aliases must not turn an
// unsupported locale into a silent approximation. The diagnostic names the
// category that carried the bad value, so a mixed environment is
// diagnosable.
func TestSedRejectsUncarriedLocalesByCategory(t *testing.T) {
	tests := []struct {
		name, env, want string
	}{
		{"ctype", "LC_CTYPE=en_US.UTF-8", `sed: LC_CTYPE "en_US.UTF-8":`},
		{"collate", "LC_COLLATE=en_US.UTF-8", `sed: LC_COLLATE "en_US.UTF-8":`},
		{"ctype codeset", "LC_CTYPE=C.ISO-8859-15", `sed: LC_CTYPE "C.ISO-8859-15":`},
		{"collate codeset", "LC_COLLATE=C.ISO-8859-15", `sed: LC_COLLATE "C.ISO-8859-15":`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := []string{"LC_CTYPE=C", "LC_COLLATE=C", tc.env}
			out, errOut, code := runSedInDirEnv(t, t.TempDir(), env, "abc\n", "s/a/X/")
			if code != 2 || out != "" || !strings.Contains(errOut, tc.want) {
				t.Errorf("env %v: got (%q, %q, %d), want exit 2 naming %s", env, out, errOut, code, tc.want)
			}
		})
	}
}
