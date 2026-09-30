package localedefcmd

// End-to-end: a locale definition goes through localedef(1) into a temporary
// LOCPATH, and the consumers read it back from there. The decimal point is a
// character no real locale uses, so nothing here can pass against host data.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"

	// Imported for their init-time registration: these tests drive the
	// applets through the registry exactly as the multicall binary does.
	_ "github.com/qiangli/coreutils/cmds/locale"
	_ "github.com/qiangli/coreutils/cmds/sort"
)

const distinctiveSource = `LC_CTYPE
upper <A>;<B>
lower <a>;<b>
END LC_CTYPE
LC_NUMERIC
decimal_point "<exclamation-mark>"
thousands_sep "<underscore>"
grouping 3
END LC_NUMERIC
LC_MONETARY
currency_symbol "<number-sign>"
frac_digits 2
END LC_MONETARY
LC_TIME
abday "So";"Mo";"Di";"Mi";"Do";"Fr";"Sa"
d_fmt "%d.%m.%Y"
END LC_TIME
LC_MESSAGES
yesexpr "^[oO]"
noexpr "^[nN]"
yesstr "oui"
nostr "non"
END LC_MESSAGES
`

// compileDistinctive compiles the definition above into a private store and
// returns the environment that selects it.
func compileDistinctive(t *testing.T) []string {
	t.Helper()
	store := t.TempDir()
	env := []string{"LOCPATH=" + store}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Dir:   t.TempDir(),
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(distinctiveSource), Out: &out, Err: &errb},
	}
	if code := run(rc, []string{"xx_XX"}); code != 0 {
		t.Fatalf("localedef xx_XX = %d; %s", code, &errb)
	}
	if _, ok := locale.LookupCompiled(env, "xx_XX"); !ok {
		t.Fatal("localedef reported success but wrote no locale into LOCPATH")
	}
	return append(env, "LANG=xx_XX")
}

func runTool(t *testing.T, name string, env []string, stdin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := tool.Lookup(name)
	if cmd == nil {
		t.Fatalf("%s is not registered", name)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Dir:   t.TempDir(),
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestCompiledLocaleReachesLocaleUtility(t *testing.T) {
	env := compileDistinctive(t)
	for _, tc := range []struct{ arg, want string }{
		{"decimal_point", `decimal_point="!"`},
		{"thousands_sep", `thousands_sep="_"`},
		{"grouping", "grouping=3"},
		{"yesexpr", `yesexpr="^[oO]"`},
		{"noexpr", `noexpr="^[nN]"`},
		{"yesstr", `yesstr="oui"`},
		{"d_fmt", `d_fmt="%d.%m.%Y"`},
		{"abday", `abday="So";"Mo";"Di";"Mi";"Do";"Fr";"Sa"`},
		{"currency_symbol", `currency_symbol="#"`},
		{"frac_digits", "frac_digits=2"},
	} {
		t.Run(tc.arg, func(t *testing.T) {
			out, errb, code := runTool(t, "locale", env, "", "-k", tc.arg)
			if code != 0 || strings.TrimSpace(out) != tc.want {
				t.Fatalf("locale -k %s = %q (%d) %q; want %q", tc.arg, out, code, errb, tc.want)
			}
		})
	}
}

// The consuming utility pkg/locale already feeds: sort -n reads LC_NUMERIC.
func TestCompiledLocaleReachesSort(t *testing.T) {
	env := append(compileDistinctive(t), "LC_COLLATE=C")
	out, errb, code := runTool(t, "sort", env, "1!20\n-1!2\n1_000!5\n", "-n")
	if want := "-1!2\n1!20\n1_000!5\n"; code != 0 || out != want {
		t.Fatalf("sort -n = %q (%d) %q; want %q", out, code, errb, want)
	}
}

// The compiled LC_MESSAGES expressions are the ones pkg/locale matches with.
func TestCompiledLocaleReachesMessageMatcher(t *testing.T) {
	env := compileDistinctive(t)
	for _, tc := range []struct {
		response string
		want     bool
	}{{"oui", true}, {"O", true}, {"yes", false}, {"non", false}} {
		got, err := locale.MatchAffirmative(env, tc.response)
		if err != nil {
			t.Fatalf("MatchAffirmative(%q): %v", tc.response, err)
		}
		if got != tc.want {
			t.Fatalf("MatchAffirmative(%q)=%v want %v", tc.response, got, tc.want)
		}
	}
}
