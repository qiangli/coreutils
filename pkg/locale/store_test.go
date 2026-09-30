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
