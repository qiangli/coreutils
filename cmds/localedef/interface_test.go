package localedefcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
)

func TestTargetCodesetMapping(t *testing.T) {
	for _, tc := range []struct {
		name, target, encoding, want string
		code                         int
	}{
		{"ascii", "ASCII", "<U0041>", "A", 0},
		{"ascii alias", "us-ascii", "<U0041>", "A", 0},
		{"utf8 ascii", "UTF-8", "<U0041>", "A", 0},
		{"utf8 nonascii", "utf8", "<U00E9>", "é", 0},
		{"utf8 supplementary", "UTF-8", "<U0001F642>", "🙂", 0},
		{"ascii nonascii", "ASCII", "<U00E9>", "", 2},
		{"surrogate", "UTF-8", "<UD800>", "", 2},
		{"out of range", "UTF-8", "<U00110000>", "", 2},
		{"unsupported", "UTF-16", "<U0041>", "", 2},
		{"hex bytes", "UTF-8", `\xc3\xa9`, "é", 0},
		{"decimal bytes", "ASCII", `\d065`, "A", 0},
		{"octal bytes", "ASCII", `\101`, "A", 0},
		{"invalid target bytes", "ASCII", `\xc3\xa9`, "", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			mapText := "<mb_cur_max> 4\nCHARMAP\n<letter> " + tc.encoding + "\nEND CHARMAP\n"
			if err := os.WriteFile(filepath.Join(dir, "map"), []byte(mapText), 0600); err != nil {
				t.Fatal(err)
			}
			var out, diag bytes.Buffer
			rc := &tool.RunContext{Dir: dir, Env: storeEnv(dir), Stdio: tool.Stdio{In: strings.NewReader("LC_MESSAGES\nyesstr \"<letter>\"\nEND LC_MESSAGES\n"), Out: &out, Err: &diag}}
			code := run(rc, []string{"-c", "-u", tc.target, "-f", "map", "target"})
			if code != tc.code {
				t.Fatalf("code=%d want %d: %s", code, tc.code, &diag)
			}
			c, ok := locale.LookupCompiled(storeEnv(dir), "target")
			if tc.code != 0 {
				if ok || out.Len() != 0 {
					t.Fatal("failed conversion published output")
				}
				return
			}
			if !ok {
				t.Fatal("compiled locale missing")
			}
			if got, _ := c.KeywordString("LC_MESSAGES", "yesstr"); got != tc.want {
				t.Errorf("mapped=%q want %q", got, tc.want)
			}
			if out.String() != "LC_MESSAGES\n" {
				t.Errorf("categories=%q", &out)
			}
		})
	}
}

func TestPathnameCompilerAndConsumers(t *testing.T) {
	for _, absolute := range []bool{false, true} {
		t.Run(map[bool]string{false: "relative", true: "absolute"}[absolute], func(t *testing.T) {
			dir := t.TempDir()
			name := "private/custom"
			if absolute {
				name = filepath.Join(dir, "private", "custom")
			}
			env := []string{"LC_NUMERIC=" + name, "LC_MESSAGES=" + name, "LC_COLLATE=C"} // no store or HOME required
			invoke := func(command, input string, args ...string) string {
				t.Helper()
				var out, diag bytes.Buffer
				rc := &tool.RunContext{Dir: dir, Env: env, Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &diag}}
				if code := tool.Lookup(command).Run(rc, args); code != 0 {
					t.Fatalf("%s code=%d: %s", command, code, &diag)
				}
				return out.String()
			}
			out := invoke("localedef", "LC_NUMERIC\ndecimal_point \"!\"\nthousands_sep \"_\"\nEND LC_NUMERIC\nLC_MESSAGES\nyesexpr \"^[oO]\"\nEND LC_MESSAGES\n", name)
			if out != "LC_MESSAGES\nLC_NUMERIC\n" {
				t.Errorf("categories=%q", out)
			}
			if _, err := os.Stat(filepath.Join(dir, "private", "custom")); err != nil {
				t.Fatalf("exact output path: %v", err)
			}
			if got := invoke("locale", "", "-k", "decimal_point"); got != "decimal_point=\"!\"\n" {
				t.Errorf("locale readback=%q", got)
			}
			if got := invoke("sort", "1_000!5\n900\n", "-n"); got != "900\n1_000!5\n" {
				t.Errorf("numeric sort=%q", got)
			}
			invoke("localedef", "LC_NUMERIC\ncopy \""+filepath.ToSlash(name)+"\"\nEND LC_NUMERIC\n", "./copied")
			// Collation takes a separate reader path; it must resolve rc.Dir too.
			env = []string{"LC_COLLATE=" + name, "LC_CTYPE=C", "LC_NUMERIC=C"}
			invoke("localedef", "LC_COLLATE\norder_start forward\n<b>\n<a>\norder_end\nEND LC_COLLATE\n", name)
			if got := invoke("sort", "a\nb\n"); got != "b\na\n" {
				t.Errorf("collation sort=%q", got)
			}
		})
	}
}

func TestTargetRejectionPreservesPrivateOutput(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		args         []string
		code         int
	}{
		{"literal outside target", "LC_MESSAGES\nyesstr \"é\"\nEND LC_MESSAGES\n", []string{"-u", "ASCII"}, 2},
		{"unsupported target", "LC_MESSAGES\nyesstr \"yes\"\nEND LC_MESSAGES\n", []string{"-u", "UTF-16"}, 2},
		{"unforced warning", "LC_CTYPE\nupper <absent>\nEND LC_CTYPE\n", nil, 4},
		{"forced error", "LC_MESSAGES\nyesstr \"<absent>\"\nEND LC_MESSAGES\n", []string{"-c"}, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, existing := range []bool{false, true} {
				dir := t.TempDir()
				path := filepath.Join(dir, "private")
				before := []byte("previous artifact\n")
				if existing {
					if err := os.WriteFile(path, before, 0600); err != nil {
						t.Fatal(err)
					}
				}
				var out, diag bytes.Buffer
				rc := &tool.RunContext{Dir: dir, Stdio: tool.Stdio{In: strings.NewReader(tc.source), Out: &out, Err: &diag}}
				args := append(append([]string(nil), tc.args...), "./private")
				if code := run(rc, args); code != tc.code {
					t.Fatalf("code=%d want %d: %s", code, tc.code, &diag)
				}
				if out.Len() != 0 {
					t.Errorf("failure stdout=%q", &out)
				}
				after, err := os.ReadFile(path)
				if existing {
					if err != nil || !bytes.Equal(before, after) {
						t.Errorf("existing output changed: %v", err)
					}
				} else if !os.IsNotExist(err) {
					t.Errorf("created rejected output: %v", err)
				}
			}
		})
	}
}

func TestCopiedDataMustFitTarget(t *testing.T) {
	dir := t.TempDir()
	env := storeEnv(dir)
	base := &locale.Compiled{Name: "base", Charmap: "UTF-8", MbCurMin: 1, MbCurMax: 4}
	base.Set("LC_MESSAGES", "yesstr", locale.Keyword{Values: []string{"é"}})
	if err := locale.Save(filepath.Join(dir, "store"), base); err != nil {
		t.Fatal(err)
	}
	var out, diag bytes.Buffer
	rc := &tool.RunContext{Dir: dir, Env: env, Stdio: tool.Stdio{In: strings.NewReader("LC_MESSAGES\ncopy \"base\"\nEND LC_MESSAGES\n"), Out: &out, Err: &diag}}
	if code := run(rc, []string{"-c", "-u", "ASCII", "target"}); code != 2 {
		t.Fatalf("code=%d: %s", code, &diag)
	}
	if _, ok := locale.LookupCompiled(env, "target"); ok || out.Len() != 0 {
		t.Fatal("published non-ASCII copy as ASCII")
	}
}
