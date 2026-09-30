package collate_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/qiangli/coreutils/cmds/comm"
	_ "github.com/qiangli/coreutils/cmds/grep"
	_ "github.com/qiangli/coreutils/cmds/join"
	_ "github.com/qiangli/coreutils/cmds/localedef"
	_ "github.com/qiangli/coreutils/cmds/ls"
	_ "github.com/qiangli/coreutils/cmds/sed"
	_ "github.com/qiangli/coreutils/cmds/sort"
	"github.com/qiangli/coreutils/pkg/collate"
	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/pkg/localedef"
	"github.com/qiangli/coreutils/tool"
)

func invoke(t *testing.T, dir string, env []string, name, input string, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	rc := &tool.RunContext{Env: env, Dir: dir, Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &err}}
	code := tool.Lookup(name).Run(rc, args)
	return out.String(), err.String(), code
}
func store(t *testing.T, body string) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	env := []string{"LC_CTYPE=C", "LC_COLLATE=custom.UTF-8", "LOCPATH=store"}
	out, err, code := invoke(t, dir, env, "localedef", "LC_COLLATE\n"+body+"\nEND LC_COLLATE\n", "-c", "custom.UTF-8")
	if code != 0 && !(code == 1 && strings.Contains(err, "omitted coded characters")) {
		t.Fatalf("localedef: %d out=%q err=%q", code, out, err)
	}
	return dir, env
}
func TestCompiledOrderConsumers(t *testing.T) {
	dir, env := store(t, "order_start forward\n<z>\n<a>\n<b>\norder_end")
	for _, f := range []string{"a", "b", "z"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range map[string]string{"one": "z 1\na 2\nb 3\n", "two": "z X\na Y\nb Z\n", "left": "z\na\nb\n", "right": "z\na\nb\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name, input, want string
		args              []string
	}{
		{"grep", "z\na\nb\n", "z\na\n", []string{"[z-a]"}},
		{"sort", "b\na\nz\n", "z\na\nb\n", nil},
		{"ls", "", "z\na\nb\n", []string{"-1", "b", "a", "z"}},
		{"comm", "", "\t\tz\n\t\ta\n\t\tb\n", []string{"--check-order", "left", "right"}},
		{"join", "", "z 1 X\na 2 Y\nb 3 Z\n", []string{"one", "two"}},
		{"sed", "z\na\nb\n", "z\na\n", []string{"-n", "/[z-a]/p"}},
		{"sed", "z\na\nb\n", "z\n", []string{"-n", "/[[.z.]]/p"}},
	}
	for _, tc := range cases {
		t.Run(tc.name+strings.Join(tc.args, "_"), func(t *testing.T) {
			out, err, code := invoke(t, dir, env, tc.name, tc.input, tc.args...)
			if code != 0 || out != tc.want {
				t.Fatalf("code=%d out=%q err=%q want=%q", code, out, err, tc.want)
			}
		})
	}
}
func TestCompiledWeightsAndElements(t *testing.T) {
	dir, env := store(t, "collating-symbol <first>\ncollating-element <ch> from \"ch\"\norder_start forward;backward\n<first>\n<z> <first>;<z>\n<a> <first>;<a>\n<ch> <ch>;<ch>\n<b> <b>;<b>\norder_end")
	p, err := collate.OpenEnv(locale.StoreEnvAt(env, func(s string) string { return filepath.Join(dir, s) }), "custom.UTF-8")
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"z", "a"}, {"az", "za"}, {"ch", "b"}, {"a", "ch"}} {
		n, err := p.Compare(pair[0], pair[1])
		if err != nil || n >= 0 {
			t.Errorf("Compare%q=%d,%v", pair, n, err)
		}
	}
	if _, err := p.CollationWeights(); err == nil {
		t.Fatal("multi-character byte brackets silently approximated")
	}
	if _, err := p.Compare("é", "a"); err == nil {
		t.Fatal("undefined input silently approximated")
	}
	p.Close()
	if _, err := p.Compare("a", "a"); err != collate.ErrClosed {
		t.Fatalf("closed: %v", err)
	}
}
func TestCompiledEquivalenceAndRangeDiffer(t *testing.T) {
	dir, env := store(t, "collating-symbol <same>\norder_start forward\n<same>\n<z> <same>\n<a> <same>\n<b>\norder_end")
	out, err, code := invoke(t, dir, env, "sed", "z\na\nb\n", "-n", "/[[=a=]]/p")
	if code != 0 || out != "z\na\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out, err)
	}
	out, err, code = invoke(t, dir, env, "sed", "z\na\nb\n", "-n", "/[a-b]/p")
	if code != 0 || out != "a\nb\n" {
		t.Fatalf("range used weights instead of order: code=%d out=%q err=%q", code, out, err)
	}
}
func TestCompileUnsupportedCollation(t *testing.T) {
	for _, body := range []string{"order_start forward,backward\n<a>\norder_end", "order_start forward\n<z>\n...\n<a>\norder_end", "order_start forward\n<a> <missing>\norder_end"} {
		src, err := localedef.ParseSource(strings.NewReader("LC_COLLATE\n" + body + "\nEND LC_COLLATE\n"))
		if err != nil {
			continue
		}
		if _, err := localedef.Compile("bad", src, nil); err == nil {
			t.Errorf("accepted unsupported %s", body)
		}
	}
}

func TestCompiledIgnoreExpansionAndCopy(t *testing.T) {
	dir, env := store(t, "order_start forward\n<a>\n<b>\n<x> \"<a><b>\"\n<z> IGNORE\norder_end")
	out, err, code := invoke(t, dir, env, "localedef", "LC_COLLATE\ncopy \"custom.UTF-8\"\nEND LC_COLLATE\n", "copied")
	if code != 0 {
		t.Fatalf("copy: %d %q %q", code, out, err)
	}
	env = locale.StoreEnvAt(env, func(s string) string { return filepath.Join(dir, s) })
	p, e := collate.OpenEnv(env, "copied")
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	for _, pair := range [][2]string{{"x", "ab"}, {"zaz", "a"}, {"zz", ""}} {
		n, e := p.Compare(pair[0], pair[1])
		if e != nil || n != 0 {
			t.Errorf("Compare%q=%d,%v", pair, n, e)
		}
	}
}

func TestCompiledMalformedDoesNotFallBack(t *testing.T) {
	dir, env := store(t, "order_start forward\n<z>\n<a>\norder_end")
	if err := os.WriteFile(filepath.Join(dir, "store", "custom.UTF-8.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sort", "ls", "comm", "join", "grep", "sed"} {
		args := map[string][]string{"comm": {"missing", "missing"}, "join": {"missing", "missing"}, "grep": {"[z-a]"}, "sed": {"p"}}
		out, err, code := invoke(t, dir, env, name, "a\nz\n", args[name]...)
		if code == 0 || out != "" || !strings.Contains(err, "invalid compiled locale") {
			t.Errorf("%s: %d %q %q", name, code, out, err)
		}
	}
}

func TestCompiledUndefined(t *testing.T) {
	dir, env := store(t, "order_start forward;forward\n<z>\nUNDEFINED IGNORE;...\n<a>\norder_end")
	p, err := collate.OpenEnv(locale.StoreEnvAt(env, func(s string) string { return filepath.Join(dir, s) }), "custom.UTF-8")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if n, err := p.Compare("bz", "a"); err != nil || n >= 0 {
		t.Fatalf("UNDEFINED IGNORE: %d %v", n, err)
	}
	if n, err := p.Compare("b", "c"); err != nil || n >= 0 {
		t.Fatalf("UNDEFINED encoding order: %d %v", n, err)
	}
}

func TestCompiledPositionAndIgnore(t *testing.T) {
	for _, direction := range []string{"forward", "backward"} {
		t.Run(direction, func(t *testing.T) {
			dir, env := store(t, "order_start "+direction+",position\n<a>\n<b>\n<x> IGNORE\n<z> \"<a><b>\"\nUNDEFINED IGNORE\norder_end")
			p, err := collate.OpenEnv(locale.StoreEnvAt(env, func(s string) string { return filepath.Join(dir, s) }), "custom.UTF-8")
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			pairs := [][2]string{{"b", "xa"}, {"axxb", "axxxa"}, {"ax", "a"}, {"z", "ab"}, {"xx", ""}}
			wants := []int{-1, -1, 0, 0, 0}
			if direction == "backward" {
				pairs = [][2]string{{"b", "ax"}, {"bxxa", "axxxa"}, {"xa", "a"}, {"z", "ab"}, {"xx", ""}}
			}
			for i, pair := range pairs {
				n, err := p.Compare(pair[0], pair[1])
				if err != nil || (wants[i] == 0 && n != 0) || (wants[i] < 0 && n >= 0) {
					t.Errorf("%q=%d,%v", pair, n, err)
				}
			}
		})
	}
}

func TestCompiledOmissionForce(t *testing.T) {
	dir := t.TempDir()
	env := []string{"LOCPATH=store"}
	source := "LC_COLLATE\norder_start forward\n<z>\n<a>\norder_end\nEND LC_COLLATE\n"
	_, diagnostic, code := invoke(t, dir, env, "localedef", source, "omitted")
	if code != 4 || !strings.Contains(diagnostic, "warning:") || !strings.Contains(diagnostic, "omitted coded characters") {
		t.Fatalf("%d %s", code, diagnostic)
	}
	if _, err := os.Stat(filepath.Join(dir, "store", "omitted.json")); !os.IsNotExist(err) {
		t.Fatalf("unforced output: %v", err)
	}
	_, diagnostic, code = invoke(t, dir, env, "localedef", source, "-c", "omitted")
	if code != 1 || !strings.Contains(diagnostic, "warning:") {
		t.Fatalf("%d %s", code, diagnostic)
	}
	path := filepath.Join(dir, "store", "omitted.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := collate.OpenEnv(locale.StoreEnvAt(env, func(s string) string { return filepath.Join(dir, s) }), "omitted")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	for _, pair := range [][2]string{{"z", "a"}, {"a", "!"}, {"!", "b"}, {"b", "c"}} {
		n, err := p.Compare(pair[0], pair[1])
		if err != nil || n >= 0 {
			t.Errorf("%q=%d,%v", pair, n, err)
		}
	}
	bad := strings.Replace(source, "<z>", "<z> ...", 1)
	if _, _, code = invoke(t, dir, env, "localedef", bad, "-c", "omitted"); code != 4 {
		t.Fatalf("forced error=%d", code)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed compilation changed output")
	}
}

func TestCompiledRangeAndEmptyWeightOrdering(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		pairs      [][2]string
	}{
		{"range", "order_start forward\n<z>\n<a>\n...\n<d>\nUNDEFINED IGNORE\norder_end", [][2]string{{"z", "a"}, {"a", "b"}, {"b", "c"}, {"c", "d"}}},
		{"empty", "order_start forward;backward\n<a> <a>;\n<b> <a>;\n<c> ;<a>\nUNDEFINED IGNORE;IGNORE\norder_end", [][2]string{{"a", "b"}, {"ba", "ab"}, {"b", "c"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, env := store(t, tc.body)
			p, err := collate.OpenEnv(locale.StoreEnvAt(env, func(s string) string { return filepath.Join(dir, s) }), "custom.UTF-8")
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			for _, pair := range tc.pairs {
				n, err := p.Compare(pair[0], pair[1])
				if err != nil || n >= 0 {
					t.Errorf("%q=%d,%v", pair, n, err)
				}
			}
		})
	}
}
