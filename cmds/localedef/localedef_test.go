package localedefcmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/qiangli/coreutils/cmds/locale"
	_ "github.com/qiangli/coreutils/cmds/sort"
	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/pkg/localedef"
	"github.com/qiangli/coreutils/pkg/posixprovider"
	"github.com/qiangli/coreutils/tool"
)

// Every invocation gets its own LOCPATH so a test never reads or writes the
// developer's real locale store.
func storeEnv(dir string) []string { return []string{"LOCPATH=" + filepath.Join(dir, "store")} }

func TestCommand(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		input string
		code  int
		diag  string
	}{
		{"stdin", []string{"test"}, "LC_NUMERIC\ndecimal_point \".\"\nEND LC_NUMERIC\n", 0, ""},
		{"copy", []string{"-c", "test"}, "LC_TIME\ncopy \"base\"\nEND LC_TIME\n", 0, ""},
		{"files", []string{"-f", "map", "-i", "source", "test"}, "", 0, ""},
		{"attached", []string{"-fmap", "-isource", "test"}, "", 0, ""},
		{"warning", []string{"-f", "map", "test"}, "LC_CTYPE\nupper <absent>\nEND LC_CTYPE\n", 4, "line 2"},
		{"forced warning", []string{"-cfmap", "test"}, "LC_CTYPE\nupper <absent>\nEND LC_CTYPE\n", 1, "warning"},
		{"reference error", []string{"-c", "-f", "map", "test"}, "LC_NUMERIC\ndecimal_point \"<absent>\"\nEND LC_NUMERIC\n", 4, "line 2"},
		{"syntax", []string{"test"}, "LC_NUMERIC\nwrong 2\nEND LC_NUMERIC\n", 4, "line 2"},
		{"missing input", []string{"-i", "absent", "test"}, "", 4, "absent"},
		{"missing map", []string{"-f", "absent", "test"}, "", 4, "absent"},
		{"unknown option", []string{"-z", "test"}, "", 4, "option"},
		{"missing option value", []string{"-i"}, "", 4, "argument"},
		{"missing name", nil, "", 4, "name"},
		{"extra name", []string{"a", "b"}, "", 4, "operand"},
		{"help", []string{"--help"}, "", 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "map"), []byte("<code_set_name> TEST\nCHARMAP\n<period> \\x2e\nEND CHARMAP\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "source"), []byte("LC_NUMERIC\ndecimal_point \"<period>\"\nEND LC_NUMERIC\n"), 0600); err != nil {
				t.Fatal(err)
			}
			// A compiled "base" locale so a copy directive has something real
			// to resolve against: only a locale we compiled can be copied.
			base := &locale.Compiled{Name: "base", Charmap: "ASCII", MbCurMin: 1, MbCurMax: 1}
			base.Set("LC_TIME", "d_fmt", locale.Keyword{Values: []string{"%d.%m.%Y"}})
			env := storeEnv(dir)
			if err := locale.Save(filepath.Join(dir, "store"), base); err != nil {
				t.Fatal(err)
			}
			var out, errout bytes.Buffer
			rc := &tool.RunContext{Dir: dir, Env: env, Stdio: tool.Stdio{In: strings.NewReader(tc.input), Out: &out, Err: &errout}}
			code := run(rc, tc.args)
			if code != tc.code || !strings.Contains(errout.String(), tc.diag) {
				t.Fatalf("code=%d stderr=%q want %d / %q", code, errout.String(), tc.code, tc.diag)
			}
			if tc.code == 0 && errout.Len() != 0 {
				t.Fatalf("unexpected stderr: %s", &errout)
			}
			if tc.name != "help" && out.Len() != 0 {
				t.Fatalf("unexpected stdout: %s", &out)
			}
			// The locale goes into the store, never into the working directory.
			if _, err := os.Stat(filepath.Join(dir, "test")); !os.IsNotExist(err) {
				t.Fatalf("command wrote into the working directory: %v", err)
			}
			// --help writes usage and compiles nothing; every other successful
			// invocation (forced warnings included) publishes the locale.
			_, compiled := locale.LookupCompiled(env, "test")
			if want := tc.code < 4 && tc.name != "help"; compiled != want {
				t.Fatalf("compiled=%v want %v (code %d)", compiled, want, code)
			}
		})
	}
}

// Only the test invokes the existing, provenance-checked external provider.
// Invalid input compares rejection, since this story does not compile output.
func TestProviderRejectsInvalidInput(t *testing.T) {
	provider, err := posixprovider.Resolve("localedef")
	if err != nil {
		t.Skipf("external localedef provider unavailable: %v", err)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "source")
	// Supply our own ASCII map so failure cannot be caused by absent host
	// charmaps. These are generated from the Go model, not upstream data.
	cm := localedef.DefaultCharmap()
	names := make([]string, 0, len(cm.Symbols))
	for name := range cm.Symbols {
		names = append(names, name)
	}
	sort.Strings(names)
	var charmap strings.Builder
	charmap.WriteString("<code_set_name> ASCII\nCHARMAP\n")
	for _, name := range names {
		fmt.Fprintf(&charmap, "<%s> \\x%02x\n", name, cm.Symbols[name][0])
	}
	charmap.WriteString("END CHARMAP\n")
	mapfile := filepath.Join(dir, "map")
	if err := os.WriteFile(mapfile, []byte(charmap.String()), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("LC_NUMERIC\ndecimal_point \"unterminated\nEND LC_NUMERIC\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p := exec.CommandContext(ctx, provider, "-f", mapfile, "-i", src, filepath.Join(dir, "reference"))
	output, err := p.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatal(ctx.Err())
	}
	if err == nil || !strings.Contains(string(output), "source:2:") {
		t.Fatalf("provider did not diagnose malformed source line 2: %v %s", err, output)
	}
	var out, diag bytes.Buffer
	rc := &tool.RunContext{Dir: dir, Env: storeEnv(dir), Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &diag}}
	if code := run(rc, []string{"-f", "map", "-i", "source", "test"}); code <= 3 || !strings.Contains(diag.String(), "line 2") {
		t.Fatalf("Go command accepted malformed source: %d %s", code, &diag)
	}
}

func TestOptions(t *testing.T) {
	o, err := parseOptions([]string{"-cfmap", "-isource", "-uUTF-8", "--", "-name"})
	if err != nil || !o.Force || o.Charmap != "map" || o.Source != "source" || o.CodeSet != "UTF-8" || o.Name != "-name" {
		t.Fatalf("options=%+v err=%v", o, err)
	}
}

func TestDefaultCharmap(t *testing.T) {
	for _, tc := range []struct {
		symbol string
		code   int
	}{{"period", 0}, {"A", 0}, {"newline", 0}, {"missing", 4}} {
		t.Run(tc.symbol, func(t *testing.T) {
			var out, diag bytes.Buffer
			rc := &tool.RunContext{Env: storeEnv(t.TempDir()), Stdio: tool.Stdio{In: strings.NewReader("LC_NUMERIC\ndecimal_point \"<" + tc.symbol + ">\"\nEND LC_NUMERIC\n"), Out: &out, Err: &diag}}
			if code := run(rc, []string{"test"}); code != tc.code {
				t.Fatalf("code=%d want %d stderr=%s", code, tc.code, &diag)
			}
		})
	}
}

// Rejection must preserve an existing artifact as well as avoid a new one.
func TestRejectedDefinitionPreservesStore(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   []string
		source string
		code   int
	}{
		{"forced validation error", []string{"-c"}, "LC_COLLATE\ncollating-symbol <A>\nEND LC_COLLATE\n", 4},
		{"warning without force", nil, "LC_CTYPE\nupper <absent>\nEND LC_CTYPE\n", 4},
		{"unsupported conversion", []string{"-u", "UTF-16"}, "LC_NUMERIC\ndecimal_point \".\"\nEND LC_NUMERIC\n", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, existing := range []bool{false, true} {
				dir := t.TempDir()
				env := storeEnv(dir)
				c := &locale.Compiled{Name: "target", Charmap: "ASCII", MbCurMin: 1, MbCurMax: 1}
				c.Set("LC_NUMERIC", "decimal_point", locale.Keyword{Values: []string{"!"}})
				path, _ := locale.StorePath(filepath.Join(dir, "store"), "target")
				var before []byte
				if existing {
					if err := locale.Save(filepath.Join(dir, "store"), c); err != nil {
						t.Fatal(err)
					}
					before, _ = os.ReadFile(path)
				}
				var out, diag bytes.Buffer
				rc := &tool.RunContext{Dir: dir, Env: env, Stdio: tool.Stdio{In: strings.NewReader(tc.source), Out: &out, Err: &diag}}
				args := append(append([]string{}, tc.args...), "target")
				code := run(rc, args)
				if code != tc.code {
					t.Errorf("code=%d want %d; %s", code, tc.code, &diag)
				}
				after, err := os.ReadFile(path)
				if existing {
					if err != nil || !bytes.Equal(before, after) {
						t.Errorf("existing artifact changed: %v", err)
					}
				} else if !os.IsNotExist(err) {
					t.Errorf("rejected definition created output: %v", err)
				}
			}
		})
	}
}

func TestRelativeStoreCompilerAndConsumers(t *testing.T) {
	dir := t.TempDir()
	env := []string{"LOCPATH=store", "LC_NUMERIC=relative", "LC_MESSAGES=relative", "LC_COLLATE=C"}
	invoke := func(name, input string, args ...string) (string, int) {
		var out, diag bytes.Buffer
		rc := &tool.RunContext{Dir: dir, Env: env, Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &diag}}
		code := tool.Lookup(name).Run(rc, args)
		if code != 0 {
			t.Errorf("%s code=%d: %s", name, code, &diag)
		}
		return out.String(), code
	}
	invoke("localedef", "LC_NUMERIC\ndecimal_point \"!\"\nthousands_sep \"_\"\ngrouping 3\nEND LC_NUMERIC\nLC_MESSAGES\nyesexpr \"^[oO]\"\nnoexpr \"^[nN]\"\nEND LC_MESSAGES\n", "relative")
	path, _ := locale.StorePath(filepath.Join(dir, "store"), "relative")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("store not relative to invocation: %v", err)
	}
	for _, tc := range []struct{ key, want string }{{"decimal_point", `decimal_point="!"`}, {"yesexpr", `yesexpr="^[oO]"`}} {
		out, _ := invoke("locale", "", "-k", tc.key)
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s got %q want %q", tc.key, out, tc.want)
		}
	}
	out, _ := invoke("sort", "1_000!5\n900\n", "-n")
	if out != "900\n1_000!5\n" {
		t.Errorf("numeric sort got %q", out)
	}
	invoke("localedef", "LC_NUMERIC\ncopy \"relative\"\nEND LC_NUMERIC\n", "copy")
	copied, ok := locale.LookupCompiled([]string{"LOCPATH=" + filepath.Join(dir, "store")}, "copy")
	if !ok {
		t.Error("copy did not resolve invocation store")
	} else if k, _ := copied.Keyword("LC_NUMERIC", "decimal_point"); len(k.Values) != 1 || k.Values[0] != "!" {
		t.Errorf("copy lost radix: %+v", k)
	}
	out, _ = invoke("locale", "", "-a")
	if !strings.Contains(out, "relative\n") {
		t.Errorf("locale -a missing compiled locale: %q", out)
	}
}
