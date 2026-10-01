package locale

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreDirsLocPath(t *testing.T) {
	dir := t.TempDir()
	dirs := StoreDirs([]string{"HOME=/home/agent", "LOCPATH=" + dir})
	if len(dirs) != 1 || dirs[0] != dir {
		t.Fatalf("StoreDirs=%v want [%s]", dirs, dir)
	}
}

func TestStoreDirsHomeDefault(t *testing.T) {
	dirs := StoreDirs([]string{"HOME=/home/agent"})
	want := filepath.Join("/home/agent", ".bashy", "locale")
	if len(dirs) != 1 || dirs[0] != want {
		t.Fatalf("StoreDirs=%v want [%s]", dirs, want)
	}
}

func TestSaveAndLookupCompiled(t *testing.T) {
	dir := t.TempDir()
	env := []string{"LOCPATH=" + dir}
	c := &Compiled{Name: "xx_XX", Charmap: "ASCII", MbCurMin: 1, MbCurMax: 1}
	c.Set("LC_NUMERIC", "decimal_point", Keyword{Values: []string{"!"}})
	c.Set("LC_MESSAGES", "yesexpr", Keyword{Values: []string{"^[oO]"}})
	if err := Save(dir, c); err != nil {
		t.Fatal(err)
	}
	got, ok := LookupCompiled(env, "xx_XX")
	if !ok {
		t.Fatal("LookupCompiled did not find the compiled locale")
	}
	if v, _ := got.KeywordString("LC_NUMERIC", "decimal_point"); v != "!" {
		t.Fatalf("decimal_point=%q want !", v)
	}
	if _, ok := LookupCompiled(env, "absent"); ok {
		t.Fatal("LookupCompiled invented an absent locale")
	}
	if _, ok := LookupCompiled(env, "../escape"); ok {
		t.Fatal("LookupCompiled accepted a path operand as a locale name")
	}
	if names := CompiledNames(env); len(names) != 1 || names[0] != "xx_XX" {
		t.Fatalf("CompiledNames=%v want [xx_XX]", names)
	}
	if c2, ok := CompiledFor([]string{"LOCPATH=" + dir, "LC_ALL=xx_XX"}, Numeric); !ok || c2.Name != "xx_XX" {
		t.Fatalf("CompiledFor=%v %v", c2, ok)
	}
}

func TestCompiledMessagesData(t *testing.T) {
	c := &Compiled{Name: "xx_XX"}
	c.Set("LC_MESSAGES", "yesexpr", Keyword{Values: []string{"^[oO]"}})
	c.Set("LC_MESSAGES", "noexpr", Keyword{Values: []string{"^[nN]"}})
	d, ok := c.MessagesData()
	if !ok || d.YesExpr != "^[oO]" || d.NoExpr != "^[nN]" {
		t.Fatalf("MessagesData=%+v ok=%v", d, ok)
	}
}

func TestMalformedStoreFallsBack(t *testing.T) {
	dir := t.TempDir()
	env := []string{"LOCPATH=" + dir, "LC_MESSAGES=POSIX"}
	path := filepath.Join(dir, "POSIX.json")
	for _, contents := range []string{
		`{`,
		`{"name":"wrong","categories":{"LC_MESSAGES":{"yesexpr":{"values":["^[oO]"]}}}}`,
		`{"name":"POSIX","categories":{"LC_MESSAGES":{"yesexpr":{"values":[]}}}}`,
		`{"name":"POSIX","categories":{"LC_COLLATE":{}}}`,
		`{"name":"POSIX","categories":{"LC_NUMERIC":null}}`,
		`{"name":"POSIX"} trailing`,
	} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, ok := LookupCompiled(env, "POSIX"); ok {
			t.Fatalf("accepted malformed store %q", contents)
		}
		if names := CompiledNames(env); len(names) != 0 {
			t.Fatalf("listed malformed store %q: %v", contents, names)
		}
		match, err := MatchAffirmative(env, "yes")
		if err != nil || !match {
			t.Fatalf("malformed store did not retain POSIX fallback: %v, %v", match, err)
		}
	}
}

func TestCompiledMessagesMatcher(t *testing.T) {
	dir := t.TempDir()
	c := &Compiled{Name: "xx_XX"}
	c.Set("LC_MESSAGES", "yesexpr", Keyword{Values: []string{"^[oO]"}})
	if err := Save(dir, c); err != nil {
		t.Fatal(err)
	}
	env := []string{"LOCPATH=" + dir, "LANG=xx_XX"}
	for _, tc := range []struct {
		response string
		want     bool
	}{{"oui", true}, {"O", true}, {"yes", false}} {
		got, err := MatchAffirmative(env, tc.response)
		if err != nil || got != tc.want {
			t.Fatalf("MatchAffirmative(%q) = %v, %v; want %v", tc.response, got, err, tc.want)
		}
	}
}

func TestPrivateStorePaths(t *testing.T) {
	dir := t.TempDir()
	c := &Compiled{Name: "./original"}
	c.Set("LC_MESSAGES", "yesexpr", Keyword{Values: []string{"^[oO]"}})
	path := filepath.Join(dir, "renamed")
	if err := SavePath(path, c); err != nil {
		t.Fatal(err)
	}
	original := []string{"LANG=./renamed"}
	env := StoreEnvAt(original, func(p string) string { return filepath.Join(dir, p) })
	if len(original) != 1 {
		t.Fatal("mutated environment")
	}
	if got, ok := CompiledFor(env, Messages); !ok || got.Name != c.Name {
		t.Fatalf("private lookup: %+v %v", got, ok)
	}
	if !HasCompiledFile(env, "./renamed") {
		t.Fatal("private file not detected")
	}
	if ok, err := MatchAffirmative(env, "oui"); err != nil || !ok {
		t.Fatalf("private messages: %v %v", ok, err)
	}
	if err := os.WriteFile(path, []byte("malformed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := LookupCompiled(env, "./renamed"); ok {
		t.Fatal("accepted malformed private file")
	}
	if !HasCompiledFile(env, "./renamed") {
		t.Fatal("malformed private file not detected")
	}
	if _, ok := LookupCompiled(env, "./absent"); ok {
		t.Fatal("invented missing private file")
	}
	if err := Save(dir, c); err == nil {
		t.Fatal("public save accepted a path name")
	}
}

func TestSavePathPreservesUnwritableExistingFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can open a read-only file for writing")
	}
	path := filepath.Join(t.TempDir(), "locale")
	const original = "existing locale data\n"
	if err := os.WriteFile(path, []byte(original), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := SavePath(path, &Compiled{Name: "replacement"}); err == nil {
		t.Fatal("SavePath replaced an unwritable existing output")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != original {
		t.Fatalf("existing output = %q, %v; want unchanged %q", got, err, original)
	}
}

func TestCollationPositionStore(t *testing.T) {
	for _, position := range [][]bool{nil, {true}, {false, true}} {
		c := &Compiled{Name: "position", Collation: &Collation{Backward: []bool{false}, Position: position, Elements: []CollatingElement{{Text: "a", Order: 1, Weights: [][]int{{1}}}}}}
		c.EnsureCategory("LC_COLLATE")
		dir := t.TempDir()
		err := Save(dir, c)
		if len(position) > 1 {
			if err == nil {
				t.Fatal("accepted malformed position levels")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		got, ok := LookupCompiled([]string{"LOCPATH=" + dir}, "position")
		if !ok || len(got.Collation.Position) != len(position) {
			t.Fatal("position store round trip failed")
		}
	}
}
