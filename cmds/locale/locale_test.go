package localecmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runCmd runs the command and only THEN reads the buffers. Reading them inside
// the return expression would capture them before run had written anything, and
// every output assertion would pass against an empty string.
func runCmd(t *testing.T, env []string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Dir:   t.TempDir(),
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code := run(rc, args)
	return out.String(), errb.String(), code
}

// --- the no-operand environment listing ---------------------------------------

// The quoting rule IS the information: an unquoted value was set by that
// category's own variable, a quoted one was derived. A listing that quoted
// everything (or nothing) would answer no question at all.
func TestEnvironmentListing(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		want []string
	}{
		{
			"nothing set: every category falls back to the default, quoted",
			nil,
			[]string{
				"LANG=",
				`LC_CTYPE="POSIX"`,
				`LC_NUMERIC="POSIX"`,
				`LC_TIME="POSIX"`,
				`LC_COLLATE="POSIX"`,
				`LC_MONETARY="POSIX"`,
				`LC_MESSAGES="POSIX"`,
				"LC_ALL=",
			},
		},
		{
			"LANG only: every category derives from it, quoted",
			[]string{"LANG=en_US.UTF-8"},
			[]string{
				"LANG=en_US.UTF-8",
				`LC_CTYPE="en_US.UTF-8"`,
				`LC_NUMERIC="en_US.UTF-8"`,
				`LC_TIME="en_US.UTF-8"`,
				`LC_COLLATE="en_US.UTF-8"`,
				`LC_MONETARY="en_US.UTF-8"`,
				`LC_MESSAGES="en_US.UTF-8"`,
				"LC_ALL=",
			},
		},
		{
			"a category variable is written bare; the rest stay derived",
			[]string{"LANG=en_US.UTF-8", "LC_TIME=C"},
			[]string{
				"LANG=en_US.UTF-8",
				`LC_CTYPE="en_US.UTF-8"`,
				`LC_NUMERIC="en_US.UTF-8"`,
				"LC_TIME=C",
				`LC_COLLATE="en_US.UTF-8"`,
				`LC_MONETARY="en_US.UTF-8"`,
				`LC_MESSAGES="en_US.UTF-8"`,
				"LC_ALL=",
			},
		},
		{
			// LC_ALL overrides the per-category variables, so EVERY line goes
			// back to being quoted — that is the visible signal that LC_TIME is
			// set but not in effect.
			"LC_ALL overrides even a category variable that is set",
			[]string{"LANG=en_US.UTF-8", "LC_TIME=C", "LC_ALL=de_DE.UTF-8"},
			[]string{
				"LANG=en_US.UTF-8",
				`LC_CTYPE="de_DE.UTF-8"`,
				`LC_NUMERIC="de_DE.UTF-8"`,
				`LC_TIME="de_DE.UTF-8"`,
				`LC_COLLATE="de_DE.UTF-8"`,
				`LC_MONETARY="de_DE.UTF-8"`,
				`LC_MESSAGES="de_DE.UTF-8"`,
				"LC_ALL=de_DE.UTF-8",
			},
		},
		{
			// An empty assignment is not a setting: it falls through to the
			// next level of precedence rather than selecting an empty locale.
			"an empty category variable falls through to LANG",
			[]string{"LANG=en_US.UTF-8", "LC_TIME="},
			[]string{
				"LANG=en_US.UTF-8",
				`LC_TIME="en_US.UTF-8"`,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runCmd(t, tc.env)
			if code != 0 {
				t.Fatalf("exit %d, stderr %q", code, errOut)
			}
			for _, want := range tc.want {
				if !containsLine(out, want) {
					t.Errorf("output is missing the line %q\n--- got ---\n%s", want, out)
				}
			}
		})
	}
}

// LANG comes first and LC_ALL last; consumers that read the listing positionally
// depend on it.
func TestEnvironmentListingOrder(t *testing.T) {
	out, _, code := runCmd(t, []string{"LANG=C"})
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != len(categories)+2 {
		t.Fatalf("got %d lines, want LANG + %d categories + LC_ALL:\n%s", len(lines), len(categories), out)
	}
	if !strings.HasPrefix(lines[0], "LANG=") {
		t.Errorf("first line = %q, want LANG", lines[0])
	}
	if !strings.HasPrefix(lines[len(lines)-1], "LC_ALL=") {
		t.Errorf("last line = %q, want LC_ALL", lines[len(lines)-1])
	}
	for i, cat := range categories {
		if !strings.HasPrefix(lines[i+1], cat+"=") {
			t.Errorf("line %d = %q, want %s", i+1, lines[i+1], cat)
		}
	}
}

// --- keyword queries -----------------------------------------------------------

var posixEnv = []string{"LC_ALL=POSIX"}

func TestKeywordValues(t *testing.T) {
	for _, tc := range []struct {
		keyword string
		plain   string
		keyed   string
	}{
		{"decimal_point", ".", `decimal_point="."`},
		{"thousands_sep", "", `thousands_sep=""`},
		{"d_fmt", "%m/%d/%y", `d_fmt="%m/%d/%y"`},
		{"t_fmt", "%H:%M:%S", `t_fmt="%H:%M:%S"`},
		{"yesexpr", "^[yY]", `yesexpr="^[yY]"`},
		// A numeric keyword is NOT quoted under -k; a consumer parsing the
		// pair would otherwise have to guess whether the quotes are data.
		{"frac_digits", "-1", "frac_digits=-1"},
		{"int_frac_digits", "-1", "int_frac_digits=-1"},
		{"currency_symbol", "", `currency_symbol=""`},
		// A list is semicolon-separated, and under -k every element is quoted
		// individually.
		{"am_pm", "AM;PM", `am_pm="AM";"PM"`},
		{"abday", "Sun;Mon;Tue;Wed;Thu;Fri;Sat",
			`abday="Sun";"Mon";"Tue";"Wed";"Thu";"Fri";"Sat"`},
	} {
		t.Run(tc.keyword, func(t *testing.T) {
			out, errOut, code := runCmd(t, posixEnv, tc.keyword)
			if code != 0 {
				t.Fatalf("exit %d, stderr %q", code, errOut)
			}
			if out != tc.plain+"\n" {
				t.Errorf("locale %s = %q, want %q", tc.keyword, out, tc.plain+"\n")
			}

			out, errOut, code = runCmd(t, posixEnv, "-k", tc.keyword)
			if code != 0 {
				t.Fatalf("-k exit %d, stderr %q", code, errOut)
			}
			if out != tc.keyed+"\n" {
				t.Errorf("locale -k %s = %q, want %q", tc.keyword, out, tc.keyed+"\n")
			}
		})
	}
}

func TestMonthNames(t *testing.T) {
	out, _, code := runCmd(t, posixEnv, "mon")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := "January;February;March;April;May;June;July;August;September;October;November;December\n"
	if out != want {
		t.Errorf("mon = %q, want %q", out, want)
	}
}

func TestCharmapOperand(t *testing.T) {
	out, _, code := runCmd(t, posixEnv, "charmap")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if out != posixCharmap+"\n" {
		t.Errorf("charmap = %q, want %q", out, posixCharmap+"\n")
	}

	// A codeset in the locale name IS the charmap: C.UTF-8 is still the POSIX
	// locale, but its characters are multibyte and mb_cur_max must say so.
	out, _, code = runCmd(t, []string{"LC_ALL=C.UTF-8"}, "-k", "charmap", "mb_cur_max")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !containsLine(out, `charmap="UTF-8"`) || !containsLine(out, "mb_cur_max=4") {
		t.Errorf("C.UTF-8 gave %q, want charmap UTF-8 and mb_cur_max 4", out)
	}
}

func TestCategoryOperandWritesEveryKeyword(t *testing.T) {
	out, errOut, code := runCmd(t, posixEnv, "-k", "LC_NUMERIC")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	for _, want := range []string{`decimal_point="."`, `thousands_sep=""`, `grouping=""`} {
		if !containsLine(out, want) {
			t.Errorf("LC_NUMERIC output missing %q:\n%s", want, out)
		}
	}
	// No other category's keywords may leak in.
	if strings.Contains(out, "d_fmt") || strings.Contains(out, "frac_digits") {
		t.Errorf("LC_NUMERIC output contains another category's keywords:\n%s", out)
	}
}

func TestCategoryHeaderFlag(t *testing.T) {
	out, _, code := runCmd(t, posixEnv, "-ck", "LC_NUMERIC")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.HasPrefix(out, "LC_NUMERIC\n") {
		t.Errorf("-c must write the category name first, got:\n%s", out)
	}

	// -c also applies to a bare keyword: the header names the category the
	// keyword belongs to.
	out, _, code = runCmd(t, posixEnv, "-c", "decimal_point")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if out != "LC_NUMERIC\n.\n" {
		t.Errorf("-c decimal_point = %q, want the category header then the value", out)
	}
}

// LC_COLLATE's locale definition is collation rules, not name=value settings,
// so the category exists and is queryable but has no keywords. That must be an
// empty result, not an error: the category IS valid.
func TestCollateHasNoKeywordsButIsNotAnError(t *testing.T) {
	out, errOut, code := runCmd(t, posixEnv, "-k", "LC_COLLATE")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if out != "" {
		t.Errorf("LC_COLLATE keywords = %q, want none", out)
	}
}

func TestMultipleOperands(t *testing.T) {
	out, _, code := runCmd(t, posixEnv, "decimal_point", "d_fmt")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if out != ".\n%m/%d/%y\n" {
		t.Errorf("got %q, want one line per operand in order", out)
	}
}

// An unknown name is an error, and the remaining operands are still processed —
// one typo must not silently swallow the rest of the query.
func TestUnknownNameIsAnErrorButLaterOperandsStillRun(t *testing.T) {
	out, errOut, code := runCmd(t, posixEnv, "no_such_keyword", "decimal_point")
	if code == 0 {
		t.Fatal("an unknown name must not exit 0")
	}
	if !strings.Contains(errOut, "no_such_keyword") {
		t.Errorf("stderr = %q, want the offending name", errOut)
	}
	if out != ".\n" {
		t.Errorf("stdout = %q, want the second operand still answered", out)
	}
}

// The refusal that keeps this honest: a locale whose data this build does not
// carry is named in the diagnostic, never answered with C's values.
func TestUnavailableLocaleIsRefusedByName(t *testing.T) {
	out, errOut, code := runCmd(t, []string{"LC_ALL=de_DE.UTF-8"}, "decimal_point")
	if code == 0 {
		t.Fatal("a locale with no data must not be answered")
	}
	if out != "" {
		t.Errorf("stdout = %q, want nothing rather than C's decimal point", out)
	}
	if !strings.Contains(errOut, "de_DE.UTF-8") {
		t.Errorf("stderr = %q, want the locale named", errOut)
	}
}

func TestGermanISO88591TimeData(t *testing.T) {
	env := []string{"LC_ALL=de_DE.iso88591"}
	out, errOut, code := runCmd(t, env, "-k", "mon", "day")
	if code != 0 || errOut != "" {
		t.Fatalf("German LC_TIME query = (%q, %d)", errOut, code)
	}
	want := "mon=\"Januar\";\"Februar\";\"M\xe4rz\";\"April\";\"Mai\";\"Juni\";\"Juli\";\"August\";\"September\";\"Oktober\";\"November\";\"Dezember\"\n" +
		"day=\"Sonntag\";\"Montag\";\"Dienstag\";\"Mittwoch\";\"Donnerstag\";\"Freitag\";\"Samstag\"\n"
	if out != want {
		t.Fatalf("German LC_TIME bytes = %q, want %q", out, want)
	}

	_, errOut, code = runCmd(t, env, "decimal_point")
	if code == 0 || !strings.Contains(errOut, "LC_NUMERIC") {
		t.Fatalf("uncarried German LC_NUMERIC was not refused: (%q, %d)", errOut, code)
	}
}

func TestGermanMessagesDataUsesAuthoritativeYesexpr(t *testing.T) {
	out, errOut, code := runCmd(t, []string{"LC_MESSAGES=de_DE.UTF-8"}, "-k", "yesexpr", "yesstr", "noexpr", "nostr")
	want := "yesexpr=\"^[+1jJyY]\"\nyesstr=\"ja\"\nnoexpr=\"^[-0nN]\"\nnostr=\"nein\"\n"
	if code != 0 || errOut != "" || out != want {
		t.Fatalf("German LC_MESSAGES = (%q, %q, %d), want %q", out, errOut, code, want)
	}
}

// The refusal is per CATEGORY, because each category resolves its own locale.
// A German LC_MONETARY must not poison an LC_TIME query that is still POSIX.
func TestRefusalIsPerCategory(t *testing.T) {
	env := []string{"LANG=C", "LC_MONETARY=de_DE.UTF-8"}

	out, errOut, code := runCmd(t, env, "d_fmt")
	if code != 0 {
		t.Fatalf("an LC_TIME query must be unaffected: exit %d, stderr %q", code, errOut)
	}
	if out != "%m/%d/%y\n" {
		t.Errorf("d_fmt = %q", out)
	}

	_, errOut, code = runCmd(t, env, "currency_symbol")
	if code == 0 {
		t.Fatal("the LC_MONETARY query must be refused")
	}
	if !strings.Contains(errOut, "LC_MONETARY") {
		t.Errorf("stderr = %q, want the category named", errOut)
	}
}

// --- -a and -m -------------------------------------------------------------------

func withFixtureDirs(t *testing.T) (localeDir, charmapDir string) {
	t.Helper()
	root := t.TempDir()
	localeDir = filepath.Join(root, "locales")
	charmapDir = filepath.Join(root, "charmaps")
	for _, d := range []string{localeDir, charmapDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	oldL, oldC := localeDirs, charmapDirs
	localeDirs, charmapDirs = []string{localeDir}, []string{charmapDir}
	t.Cleanup(func() { localeDirs, charmapDirs = oldL, oldC })
	return localeDir, charmapDir
}

func touch(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// The process umask can strip bits from the requested mode, so set it
	// explicitly wherever the mode is load-bearing.
	if err := os.Chmod(filepath.Join(dir, name), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAllLocales(t *testing.T) {
	localeDir, _ := withFixtureDirs(t)
	for _, n := range []string{"en_US", "de_DE", "locale-archive", ".hidden"} {
		touch(t, localeDir, n)
	}

	out, errOut, code := runCmd(t, nil, "-a")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	got := lines(out)
	want := []string{"C", "POSIX", "de_DE", "en_US"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("locale -a = %v, want %v (sorted, C and POSIX always present,\n"+
			"locale-archive and dotfiles excluded)", got, want)
	}
}

// C and POSIX exist whether or not any locale directory does: they are required
// by the standard and are not something a host installs.
func TestAllLocalesWithNoDirectories(t *testing.T) {
	oldL := localeDirs
	localeDirs = []string{filepath.Join(t.TempDir(), "absent")}
	t.Cleanup(func() { localeDirs = oldL })

	out, _, code := runCmd(t, nil, "-a")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Join(lines(out), ",") != "C,POSIX" {
		t.Errorf("locale -a = %v, want just C and POSIX", lines(out))
	}
}

func TestCharmaps(t *testing.T) {
	_, charmapDir := withFixtureDirs(t)
	// Charmap files are conventionally gzipped; the compression is not part of
	// the name a caller can pass to localedef.
	for _, n := range []string{"UTF-8.gz", "ANSI_X3.4-1968.gz", "ISO-8859-1"} {
		touch(t, charmapDir, n)
	}

	out, errOut, code := runCmd(t, nil, "-m")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	want := []string{"ANSI_X3.4-1968", "ISO-8859-1", "UTF-8"}
	if strings.Join(lines(out), ",") != strings.Join(want, ",") {
		t.Errorf("locale -m = %v, want %v with .gz stripped", lines(out), want)
	}
}

func TestCharmapsWithNoDirectory(t *testing.T) {
	oldC := charmapDirs
	charmapDirs = []string{filepath.Join(t.TempDir(), "absent")}
	t.Cleanup(func() { charmapDirs = oldC })

	out, _, code := runCmd(t, nil, "-m")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	// The POSIX charmap is built in, so an empty list would deny a charmap the
	// implementation demonstrably supports.
	if strings.Join(lines(out), ",") != posixCharmap {
		t.Errorf("locale -m = %v, want the built-in POSIX charmap", lines(out))
	}
}

// --- option validation -------------------------------------------------------------

func TestMutuallyExclusiveOptions(t *testing.T) {
	for _, args := range [][]string{
		{"-a", "-m"},
		{"-a", "-k"},
		{"-m", "-c"},
		{"-a", "decimal_point"},
		{"-m", "LC_TIME"},
		{"-k"}, // -k with no operand has nothing to name
		{"-c"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out, errOut, code := runCmd(t, posixEnv, args...)
			if code == 0 {
				t.Fatalf("%v must be a usage error", args)
			}
			if out != "" {
				t.Errorf("a usage error must write nothing to stdout, got %q", out)
			}
			if !strings.Contains(errOut, "locale") {
				t.Errorf("stderr = %q, want a diagnostic naming the command", errOut)
			}
		})
	}
}

func TestUnsupportedFlagFailsLoudly(t *testing.T) {
	_, errOut, code := runCmd(t, posixEnv, "-z")
	if code == 0 {
		t.Fatal("an unimplemented flag must not be accepted")
	}
	if !strings.Contains(errOut, "locale") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestHelpAndVersion(t *testing.T) {
	for _, arg := range []string{"--help", "--version", "-h", "-V"} {
		t.Run(arg, func(t *testing.T) {
			out, errOut, code := runCmd(t, posixEnv, arg)
			if code != 0 {
				t.Fatalf("%s exit %d, stderr %q", arg, code, errOut)
			}
			if !strings.Contains(out, "locale") {
				t.Errorf("%s output = %q", arg, out)
			}
		})
	}
}

// --- helpers ------------------------------------------------------------------------

func lines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func containsLine(out, want string) bool {
	for _, l := range lines(out) {
		if l == want {
			return true
		}
	}
	return false
}
