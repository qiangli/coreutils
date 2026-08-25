package locale

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name string
		env  []string
		cat  Category
		want string
	}{
		// --- empty / nil environment ---------------------------------
		{"nil env → POSIX", nil, Collate, "POSIX"},
		{"empty env → POSIX", []string{}, Collate, "POSIX"},

		// --- only LANG set -------------------------------------------
		{"LANG only", []string{"LANG=en_US.UTF-8"}, Collate, "en_US.UTF-8"},
		{"LANG only for ctype", []string{"LANG=fr_FR"}, CType, "fr_FR"},
		{"LANG only for numeric", []string{"LANG=de_DE"}, Numeric, "de_DE"},

		// --- only category set (no LANG) -----------------------------
		{"LC_COLLATE only, no LANG", []string{"LC_COLLATE=de_DE"}, Collate, "de_DE"},
		{"LC_CTYPE only, no LANG → different cat falls to POSIX",
			[]string{"LC_CTYPE=de_DE"}, Collate, "POSIX"},
		{"LC_NUMERIC only, no LANG", []string{"LC_NUMERIC=de_DE.UTF-8"}, Numeric, "de_DE.UTF-8"},
		{"LC_TIME only, no LANG", []string{"LC_TIME=ja_JP"}, Time, "ja_JP"},
		{"LC_MESSAGES only, no LANG", []string{"LC_MESSAGES=es_ES"}, Messages, "es_ES"},

		// --- category overrides LANG ---------------------------------
		{"LC_COLLATE overrides LANG", []string{"LANG=en_US", "LC_COLLATE=de_DE"}, Collate, "de_DE"},
		{"unrelated category falls to LANG",
			[]string{"LANG=en_US", "LC_COLLATE=de_DE"}, CType, "en_US"},

		// --- LC_ALL overrides everything -----------------------------
		{"LC_ALL overrides category and LANG",
			[]string{"LANG=en_US", "LC_COLLATE=de_DE", "LC_ALL=C"}, Collate, "C"},
		{"LC_ALL overrides for all categories",
			[]string{"LANG=en_US", "LC_COLLATE=de_DE", "LC_CTYPE=fr_FR", "LC_NUMERIC=es_ES",
				"LC_ALL=ja_JP.UTF-8"},
			Numeric, "ja_JP.UTF-8"},

		// --- empty values fall through -------------------------------
		{"empty LC_ALL falls through to category",
			[]string{"LANG=en_US", "LC_COLLATE=de_DE", "LC_ALL="}, Collate, "de_DE"},
		{"empty LC_ALL falls through to LANG",
			[]string{"LANG=en_US", "LC_ALL="}, Collate, "en_US"},
		{"empty category falls through to LANG",
			[]string{"LANG=en_US", "LC_COLLATE="}, Collate, "en_US"},
		{"empty LANG falls through to Default",
			[]string{"LANG="}, Collate, "POSIX"},
		{"empty LC_ALL and empty category falls to LANG",
			[]string{"LANG=en_US", "LC_ALL=", "LC_COLLATE="}, Collate, "en_US"},
		{"all empty → POSIX",
			[]string{"LANG=", "LC_ALL=", "LC_COLLATE="}, Collate, "POSIX"},

		// --- last duplicate wins -------------------------------------
		{"duplicate LANG, last wins",
			[]string{"LANG=en_US", "LANG=de_DE"}, Collate, "de_DE"},
		{"duplicate LC_ALL, last wins",
			[]string{"LC_ALL=en_US", "LC_ALL=C"}, Collate, "C"},
		{"duplicate category, last wins",
			[]string{"LC_COLLATE=en_US", "LC_COLLATE=de_DE"}, Collate, "de_DE"},
		{"three copies of LANG, last wins",
			[]string{"LANG=A", "LANG=B", "LANG=C"}, Collate, "C"},

		// --- codeset and modifier preserved (no normalization) -------
		{"codeset preserved", []string{"LANG=en_US.UTF-8"}, Collate, "en_US.UTF-8"},
		{"ISO codeset preserved", []string{"LANG=de_DE.ISO-8859-1"}, Collate, "de_DE.ISO-8859-1"},
		{"modifier preserved", []string{"LANG=de_DE@euro"}, Collate, "de_DE@euro"},
		{"codeset and modifier preserved",
			[]string{"LANG=de_DE.ISO-8859-1@euro"}, Collate, "de_DE.ISO-8859-1@euro"},
		{"complex locale preserved through LC_ALL",
			[]string{"LC_ALL=zh_TW.Big5@latin"}, Numeric, "zh_TW.Big5@latin"},

		// --- case not folded (raw value returned as-is) -------------
		{"uppercase locale preserved", []string{"LANG=EN_US.UTF-8"}, Collate, "EN_US.UTF-8"},
		{"mixed case preserved", []string{"LANG=En_Us"}, Collate, "En_Us"},

		// --- unrelated env entries ignored --------------------------
		{"unrelated entries ignored",
			[]string{"PATH=/bin", "HOME=/root", "LANG=en_US"}, Collate, "en_US"},
		{"near-miss key prefix not matched",
			[]string{"LC_COLLATEX=de_DE", "LANG=en_US"}, Collate, "en_US"},

		// --- zero-value Category -------------------------------------
		{"zero-value category skips category level",
			[]string{"LC_ALL=C", "LANG=en_US"}, "", "C"},
		{"zero-value category with only LANG",
			[]string{"LANG=en_US"}, "", "en_US"},
		{"zero-value category, nothing set", nil, "", "POSIX"},

		// --- POSIX and C as explicit values --------------------------
		{"explicit POSIX via LC_ALL", []string{"LC_ALL=POSIX"}, Collate, "POSIX"},
		{"explicit C via LC_ALL", []string{"LC_ALL=C"}, Collate, "C"},
		{"explicit POSIX via LANG", []string{"LANG=POSIX"}, Collate, "POSIX"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Resolve(tc.env, tc.cat)
			if got != tc.want {
				t.Errorf("Resolve(%v, %q) = %q; want %q", tc.env, tc.cat, got, tc.want)
			}
		})
	}
}

func TestResolveAll(t *testing.T) {
	t.Run("nil env", func(t *testing.T) {
		c := ResolveAll(nil)
		if c.All != "POSIX" || c.Collate != "POSIX" || c.CType != "POSIX" ||
			c.Numeric != "POSIX" || c.Messages != "POSIX" || c.Time != "POSIX" {
			t.Errorf("ResolveAll(nil) = %+v; want all POSIX", c)
		}
	})

	t.Run("only LANG", func(t *testing.T) {
		c := ResolveAll([]string{"LANG=en_US.UTF-8"})
		want := Categories{
			All: "POSIX", Collate: "en_US.UTF-8", CType: "en_US.UTF-8",
			Numeric: "en_US.UTF-8", Messages: "en_US.UTF-8", Time: "en_US.UTF-8",
		}
		if c != want {
			t.Errorf("ResolveAll = %+v; want %+v", c, want)
		}
	})

	t.Run("LC_ALL overrides all", func(t *testing.T) {
		c := ResolveAll([]string{
			"LANG=en_US", "LC_COLLATE=de_DE", "LC_CTYPE=fr_FR",
			"LC_NUMERIC=es_ES", "LC_ALL=C",
		})
		if c.All != "C" {
			t.Errorf("All = %q; want C", c.All)
		}
		for _, v := range []string{c.Collate, c.CType, c.Numeric, c.Messages, c.Time} {
			if v != "C" {
				t.Errorf("category = %q; want C (LC_ALL override)", v)
			}
		}
	})

	t.Run("individual categories with LANG fallback", func(t *testing.T) {
		c := ResolveAll([]string{
			"LANG=en_US.UTF-8",
			"LC_COLLATE=de_DE",
			"LC_NUMERIC=de_DE.UTF-8",
		})
		if c.All != "POSIX" {
			t.Errorf("All = %q; want POSIX", c.All)
		}
		if c.Collate != "de_DE" {
			t.Errorf("Collate = %q; want de_DE", c.Collate)
		}
		if c.CType != "en_US.UTF-8" {
			t.Errorf("CType = %q; want en_US.UTF-8 (LANG fallback)", c.CType)
		}
		if c.Numeric != "de_DE.UTF-8" {
			t.Errorf("Numeric = %q; want de_DE.UTF-8", c.Numeric)
		}
		if c.Messages != "en_US.UTF-8" {
			t.Errorf("Messages = %q; want en_US.UTF-8 (LANG fallback)", c.Messages)
		}
		if c.Time != "en_US.UTF-8" {
			t.Errorf("Time = %q; want en_US.UTF-8 (LANG fallback)", c.Time)
		}
	})

	t.Run("empty LC_ALL does not override", func(t *testing.T) {
		c := ResolveAll([]string{"LC_ALL=", "LC_COLLATE=de_DE", "LANG=en_US"})
		if c.All != "POSIX" {
			t.Errorf("All = %q; want POSIX (empty LC_ALL)", c.All)
		}
		if c.Collate != "de_DE" {
			t.Errorf("Collate = %q; want de_DE", c.Collate)
		}
	})
}

func TestResolveNoMutation(t *testing.T) {
	// Resolve must not mutate its env argument.
	env := []string{"LANG=en_US.UTF-8", "LC_COLLATE=de_DE"}
	original := make([]string, len(env))
	copy(original, env)

	_ = Resolve(env, Collate)
	_ = ResolveAll(env)

	for i := range env {
		if env[i] != original[i] {
			t.Fatalf("env mutated: got %q; want %q at index %d", env[i], original[i], i)
		}
	}
	if len(env) != len(original) {
		t.Fatalf("env length changed: got %d; want %d", len(env), len(original))
	}
}

func TestResolveRace(t *testing.T) {
	// Resolve is a pure function — concurrent calls with the same env must
	// be race-free.
	env := []string{
		"LANG=en_US.UTF-8",
		"LC_ALL=de_DE.ISO-8859-1@euro",
		"LC_COLLATE=fr_FR",
		"LC_CTYPE=ja_JP.UTF-8",
		"LC_NUMERIC=es_ES",
		"LC_MESSAGES=zh_CN",
		"LC_TIME=ru_RU",
	}
	cats := []Category{Collate, CType, Numeric, Messages, Time}

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, cat := range cats {
				got := Resolve(env, cat)
				// With LC_ALL set, every category must resolve to LC_ALL.
				if got != "de_DE.ISO-8859-1@euro" {
					t.Errorf("Resolve(env, %s) = %q; want de_DE.ISO-8859-1@euro", cat, got)
				}
			}
			_ = ResolveAll(env)
		}()
	}
	wg.Wait()
}

func TestResolveConsistency(t *testing.T) {
	// ResolveAll must produce values consistent with individual Resolve calls.
	env := []string{
		"LANG=en_US.UTF-8",
		"LC_COLLATE=de_DE",
		"LC_CTYPE=fr_FR.UTF-8",
		"LC_NUMERIC=es_ES",
	}
	c := ResolveAll(env)

	checks := []struct {
		got  string
		want string
		desc string
	}{
		{c.Collate, Resolve(env, Collate), "Collate"},
		{c.CType, Resolve(env, CType), "CType"},
		{c.Numeric, Resolve(env, Numeric), "Numeric"},
		{c.Messages, Resolve(env, Messages), "Messages"},
		{c.Time, Resolve(env, Time), "Time"},
	}
	for _, ch := range checks {
		if ch.got != ch.want {
			t.Errorf("ResolveAll.%s = %q but Resolve(env, %s) = %q",
				ch.desc, ch.got, ch.desc, ch.want)
		}
	}
}

func TestMessageMatcherUsesAnchoredYesExpression(t *testing.T) {
	for _, tc := range []struct {
		name     string
		env      []string
		response string
		want     bool
	}{
		{"C yes", nil, "yes\n", true},
		{"C uppercase", []string{"LC_MESSAGES=C"}, "Y\n", true},
		{"leading space stays negative", nil, " yes\n", false},
		{"leading tab stays negative", nil, "\tyes\n", false},
		{"empty line", nil, "\n", false},
		{"German ja", []string{"LC_MESSAGES=de_DE.UTF-8"}, "ja\n", true},
		{"German plus", []string{"LC_MESSAGES=de_DE"}, "+\n", true},
		{"German one", []string{"LC_MESSAGES=de_DE.ISO-8859-1"}, "1\n", true},
		{"German Latin-9 euro", []string{"LC_MESSAGES=de_DE.ISO-8859-15@euro"}, "ja\n", true},
		{"German yes", []string{"LC_MESSAGES=de_DE.UTF-8"}, "yes\n", true},
		{"German leading space", []string{"LC_MESSAGES=de_DE.UTF-8"}, " ja\n", false},
		{"LC_ALL precedence", []string{"LC_MESSAGES=de_DE", "LC_ALL=C"}, "ja\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			matcher, err := MessagesMatcher(tc.env)
			if err != nil {
				t.Fatal(err)
			}
			if got := matcher.MatchAffirmative(tc.response); got != tc.want {
				t.Fatalf("MatchAffirmative(%q) = %v, want %v", tc.response, got, tc.want)
			}
			if got, err := MatchAffirmative(tc.env, tc.response); err != nil || got != tc.want {
				t.Fatalf("shared MatchAffirmative(%q) = (%v, %v), want %v", tc.response, got, err, tc.want)
			}
		})
	}
}

func TestMessagesMatcherFailsClosedForUnsupportedLocale(t *testing.T) {
	matcher, err := MessagesMatcher([]string{"LC_MESSAGES=en_US.UTF-8"})
	if err == nil || !errors.Is(err, ErrUnsupportedMessagesLocale) {
		t.Fatalf("MessagesMatcher unsupported error = %v", err)
	}
	if matcher.MatchAffirmative("yes\n") {
		t.Fatal("unsupported locale silently used the C yesexpr")
	}
}

func TestMessagesProviderExplicitAliases(t *testing.T) {
	for _, name := range []string{"C", "POSIX", "C.UTF-8", "C.utf8", "de_DE", "de_DE.UTF-8", "de_DE.iso88591", "de_DE.ISO-8859-15@euro"} {
		if _, ok := LookupMessages(name); !ok {
			t.Errorf("explicit LC_MESSAGES alias %q was rejected", name)
		}
	}
	for _, name := range []string{"c", "en_US.UTF-8", "de_DE.ISO-8859-2", "de_DE.UTF-8x"} {
		if _, ok := LookupMessages(name); ok {
			t.Errorf("unsupported LC_MESSAGES locale %q was accepted", name)
		}
	}
}

func TestCompileMessageMatcherSupportsGeneralMultibyteERE(t *testing.T) {
	matcher, err := CompileMessageMatcher(`^(sí|はい)$`)
	if err != nil {
		t.Fatal(err)
	}
	for _, response := range []string{"sí\n", "はい\n"} {
		if !matcher.MatchAffirmative(response) {
			t.Errorf("multibyte yesexpr rejected %q", response)
		}
	}
	if matcher.MatchAffirmative(" はい\n") {
		t.Fatal("multibyte yesexpr ignored its anchor")
	}
}

func TestGetEnvPrefixSafety(t *testing.T) {
	// Verify that getEnv's KEY= prefix matching doesn't match substrings.
	// E.g. looking up "LANG" must not match "LANGUAGE=en".
	env := []string{
		"LANGUAGE=en_US",
		"LC_COLLATE_TEST=evil",
	}
	// LANGUAGE should not match LANG.
	if v, ok := getEnv(env, "LANG"); ok || v != "" {
		t.Errorf(`getEnv(env, "LANG") = %q, %v; want "", false (LANGUAGE must not match)`, v, ok)
	}
	// LC_COLLATE_TEST should not match LC_COLLATE.
	if v, ok := getEnv(env, "LC_COLLATE"); ok || v != "" {
		t.Errorf(`getEnv(env, "LC_COLLATE") = %q, %v; want "", false`, v, ok)
	}
}

func TestBenchResolveManyEnvEntries(t *testing.T) {
	// Stress-test: a large env with many entries, ensure correctness and
	// that the last duplicate wins even with many preceding entries.
	var env []string
	for i := 0; i < 500; i++ {
		env = append(env, "VAR"+strings.Repeat("X", i%20)+"=noise")
	}
	env = append(env, "LANG=final_LANG")
	env = append(env, "LC_COLLATE=final_COLLATE")
	env = append(env, "LC_ALL=final_ALL")

	// LC_ALL must win.
	if got := Resolve(env, Collate); got != "final_ALL" {
		t.Errorf("Resolve with large env = %q; want final_ALL", got)
	}
	if got := Resolve(env, Numeric); got != "final_ALL" {
		t.Errorf("Resolve with large env = %q; want final_ALL", got)
	}
}
