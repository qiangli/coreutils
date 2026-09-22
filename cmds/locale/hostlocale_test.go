package localecmd

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func TestVerifiedHostLocaleCacheReusesOnlyMatchingHostNames(t *testing.T) {
	hostNames := "en_US.UTF-8\n"
	selected := 0
	provider := hostLocaleProvider{run: func(env, args []string) (string, string, error) {
		if len(args) == 1 && args[0] == "-a" {
			return hostNames, "", nil
		}
		name := strings.TrimPrefix(env[0], "LC_ALL=")
		if len(args) == 0 {
			selected++
			return "LC_CTYPE=\"" + name + "\"\n", "", nil
		}
		if len(args) == 2 && args[0] == "-k" {
			if args[1] == "LC_CTYPE" {
				return "charmap=\"UTF-8\"\n", "", nil
			}
			return "yesstr=\"yes\"\n", "", nil
		}
		return "", "", fmt.Errorf("unexpected args %v", args)
	}}
	path := filepath.Join(t.TempDir(), "host-locales.json")
	first := availableLocalesCached(&provider, path, "host-locale.exe")
	if selected != 1 || !slices.Contains(first, "en_US.UTF-8") {
		t.Fatalf("first listing = %v; selected calls = %d", first, selected)
	}
	second := availableLocalesCached(&provider, path, "host-locale.exe")
	if selected != 1 || !reflect.DeepEqual(second, first) {
		t.Fatalf("cached listing = %v; selected calls = %d", second, selected)
	}
	hostNames += "fr_FR.UTF-8\n"
	third := availableLocalesCached(&provider, path, "host-locale.exe")
	if selected != 3 || !slices.Contains(third, "fr_FR.UTF-8") {
		t.Fatalf("changed host listing = %v; selected calls = %d", third, selected)
	}
}

func TestHostLocaleServiceabilityRunsConcurrentlyWithoutChangingListing(t *testing.T) {
	var active, maxActive atomic.Int32
	provider := hostLocaleProvider{parallelism: 3, run: func(env, args []string) (string, string, error) {
		if len(args) == 1 && args[0] == "-a" {
			return "ru_RU.UTF-8\nen_US.UTF-8\nfr_FR.UTF-8\n", "", nil
		}
		name := strings.TrimPrefix(env[0], "LC_ALL=")
		if len(args) == 0 {
			n := active.Add(1)
			for prev := maxActive.Load(); n > prev && !maxActive.CompareAndSwap(prev, n); prev = maxActive.Load() {
			}
			time.Sleep(10 * time.Millisecond)
			active.Add(-1)
			return "LC_CTYPE=\"" + name + "\"\n", "", nil
		}
		if len(args) == 2 && args[0] == "-k" {
			if args[1] == "LC_CTYPE" {
				return "charmap=\"UTF-8\"\n", "", nil
			}
			return "yesstr=\"yes\"\n", "", nil
		}
		return "", "", fmt.Errorf("unexpected args %v", args)
	}}
	got := availableLocalesFromProvider(&provider)
	want := []string{"C", "POSIX", "de_DE.ISO-8859-1", "de_DE.UTF-8", "en_US.UTF-8", "fr_FR.UTF-8", "ru_RU.UTF-8"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("locale -a = %v, want %v", got, want)
	}
	if maxActive.Load() < 2 {
		t.Fatalf("host verification was serialized; max active = %d", maxActive.Load())
	}
}

func TestCommandHostLocaleRunnerWaitsForOutput(t *testing.T) {
	const helperEnv = "COREUTILS_LOCALE_RUNNER_HELPER"
	if os.Getenv(helperEnv) == "1" {
		time.Sleep(25 * time.Millisecond)
		fmt.Fprint(os.Stdout, "late stdout")
		fmt.Fprint(os.Stderr, "late stderr")
		return
	}
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	rc := &tool.RunContext{Dir: t.TempDir(), Env: append(os.Environ(), helperEnv+"=1")}
	stdout, stderr, err := commandHostLocaleRunner(rc, path)(nil, []string{"-test.run=^TestCommandHostLocaleRunnerWaitsForOutput$"})
	if err != nil || !strings.Contains(stdout, "late stdout") || !strings.Contains(stderr, "late stderr") {
		t.Fatalf("runner output = (%q, %q, %v), want delayed stdout and stderr", stdout, stderr, err)
	}
}

func TestParseHostLocaleValueGitBashTimeValues(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		kind  valueKind
		want  []string
	}{
		{name: "empty era", input: "", kind: kindString, want: []string{""}},
		{name: "quoted semicolons", input: `"Sun;Mon;Tue";"Wed;Thu;Fri;Sat"`, kind: kindStringList, want: []string{"Sun;Mon;Tue", "Wed;Thu;Fri;Sat"}},
		{name: "hex escape", input: `"Sun\x3bMon"`, kind: kindString, want: []string{"Sun;Mon"}},
		{name: "number", input: "7", kind: kindNumber, want: []string{"7"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, kind, err := parseHostLocaleValue(tc.input)
			if err != nil {
				t.Fatalf("parseHostLocaleValue(%q): %v", tc.input, err)
			}
			if kind != tc.kind || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseHostLocaleValue(%q) = %#v, %v; want %#v, %v", tc.input, got, kind, tc.want, tc.kind)
			}
		})
	}
}

func TestParseHostLocaleKeywordsGitBashTimeCategory(t *testing.T) {
	out := "abday=\"Sun;Mon;Tue\";\"Wed;Thu;Fri;Sat\"\n" +
		"era=\n" +
		"alt_digits=\n"
	got, err := parseHostLocaleKeywords("LC_TIME", out)
	if err != nil {
		t.Fatalf("parse Git Bash LC_TIME output: %v", err)
	}
	want := []keyword{
		{Name: "abday", Category: "LC_TIME", Kind: kindStringList, Values: []string{"Sun;Mon;Tue", "Wed;Thu;Fri;Sat"}, HostRaw: true, RawValue: `"Sun;Mon;Tue";"Wed;Thu;Fri;Sat"`},
		{Name: "era", Category: "LC_TIME", Kind: kindString, Values: []string{""}, HostRaw: true},
		{Name: "alt_digits", Category: "LC_TIME", Kind: kindString, Values: []string{""}, HostRaw: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed Git Bash LC_TIME output = %#v, want %#v", got, want)
	}
}

func TestParseHostLocaleValueRejectsMalformedOutput(t *testing.T) {
	for _, input := range []string{`"unterminated`, `"ok"tail`, `"bad\q"`, `"bad\x0"`, `"ok";`} {
		if _, _, err := parseHostLocaleValue(input); err == nil {
			t.Errorf("parseHostLocaleValue(%q) succeeded; want malformed output error", input)
		}
	}
}

func TestHostLocaleProviderCP932TrailBackslash(t *testing.T) {
	// 0x8f 0x5c is one CP932 character. The following quote closes the
	// first list element and the semicolon starts the second one.
	raw := []byte{'"', 0x8f, 0x5c, '"', ';', '"', '2', '"'}
	value := string(raw)
	provider := hostLocaleProvider{run: func(env, args []string) (string, string, error) {
		if len(args) == 0 {
			return "LC_CTYPE=\"ja_JP.SJIS\"\n", "", nil
		}
		switch args[1] {
		case "LC_CTYPE":
			return "charmap=\"CP932\"\ncode_set_name=\"CP932\"\n", "", nil
		case "LC_TIME":
			return "alt_digits=" + value + "\n", "", nil
		default:
			return "yesstr=\"yes\"\n", "", nil
		}
	}}

	keywords, err := provider.query("ja_JP.SJIS", "LC_TIME")
	if err != nil || len(keywords) != 1 {
		t.Fatalf("query CP932 LC_TIME = %#v, %v", keywords, err)
	}
	if got, want := keywords[0].Values, []string{string([]byte{0x8f, 0x5c}), "2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("raw CP932 alt_digits = %#v, want %#v", got, want)
	}
	if got, want := render(keywords[0], true), "alt_digits="+value; got != want {
		t.Errorf("rendered CP932 alt_digits = %q, want %q", got, want)
	}
	if !provider.serves("ja_JP.SJIS") {
		t.Error("serves(ja_JP.SJIS) = false, want true")
	}

	if _, _, err := parseHostLocaleValueForLocale("ja_JP.SJIS", `"bad\q"`); err == nil {
		t.Error("CP932 parser accepted malformed ASCII escape")
	}
	if _, _, err := parseHostLocaleValueForLocale("de_DE.ISO-8859-1", value); err == nil {
		t.Error("non-CP932 parser accepted CP932 trail backslash")
	}
}
