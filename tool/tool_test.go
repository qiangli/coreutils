package tool

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPath(t *testing.T) {
	tests := []struct {
		name    string
		dir     string
		operand string
		want    string
	}{
		{
			name:    "simple relative",
			dir:     "/work",
			operand: "foo",
			want:    "/work/foo",
		},
		{
			name:    "nested relative",
			dir:     "/work",
			operand: "a/b/c",
			want:    "/work/a/b/c",
		},
		{
			name:    "dot relative",
			dir:     "/work",
			operand: "./foo",
			want:    "/work/foo",
		},
		{
			name:    "parent relative",
			dir:     "/work/sub",
			operand: "../other",
			want:    "/work/other",
		},
		{
			name:    "absolute unix",
			dir:     "/work",
			operand: "/usr/bin",
			want:    "/usr/bin",
		},
		{
			name:    "empty dir relative",
			dir:     "",
			operand: "foo",
			want:    "foo",
		},
		{
			name:    "empty dir absolute",
			dir:     "",
			operand: "/usr/bin",
			want:    "/usr/bin",
		},
		{
			name:    "empty operand",
			dir:     "/work",
			operand: "",
			want:    "/work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := &RunContext{Ctx: context.Background(), Dir: tt.dir}
			got := rc.Path(tt.operand)
			if runtime.GOOS == "windows" {
				// On Windows the test expectations with / need adjustment.
				// We test the structural property: Path() produces platform-separated paths.
				want := filepath.FromSlash(tt.want)
				if got != want {
					t.Errorf("Path(%q) with dir=%q = %q, want %q", tt.operand, tt.dir, got, want)
				}
				return
			}
			if got != tt.want {
				t.Errorf("Path(%q) with dir=%q = %q, want %q", tt.operand, tt.dir, got, tt.want)
			}
		})
	}
}

func TestIsAbsPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/", true},
		{"/usr/bin", true},
		{"foo", false},
		{"foo/bar", false},
		{"./foo", false},
	}

	if runtime.GOOS == "windows" {
		tests = append(tests,
			struct {
				path string
				want bool
			}{`C:\foo`, true},
			struct {
				path string
				want bool
			}{`C:/foo`, true},
			struct {
				path string
				want bool
			}{`\foo`, true},
			struct {
				path string
				want bool
			}{`/foo`, true},
			struct {
				path string
				want bool
			}{`\\server\share\foo`, true},
			struct {
				path string
				want bool
			}{`//server/share/foo`, true},
			struct {
				path string
				want bool
			}{`foo\bar`, false},
		)
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isAbsPath(tt.path); got != tt.want {
				t.Errorf("isAbsPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestPathextFromEnv(t *testing.T) {
	if runtime.GOOS != "windows" {
		if got := pathextFromEnv([]string{"PATHEXT=.COM;.EXE"}); got != nil {
			t.Errorf("pathextFromEnv on non-windows = %v, want nil", got)
		}
		return
	}

	tests := []struct {
		name string
		env  []string
		want []string
	}{
		{
			name: "explicit PATHEXT",
			env:  []string{"PATHEXT=.COM;.EXE"},
			want: []string{".COM", ".EXE"},
		},
		{
			name: "PATHEXT without dots",
			env:  []string{"PATHEXT=EXE;CMD"},
			want: []string{".EXE", ".CMD"},
		},
		{
			name: "empty PATHEXT falls back to default",
			env:  []string{"PATHEXT="},
			want: []string{".COM", ".EXE", ".BAT", ".CMD"},
		},
		{
			name: "missing PATHEXT falls back to default",
			env:  nil,
			want: []string{".COM", ".EXE", ".BAT", ".CMD"},
		},
		{
			name: "case-insensitive lookup (lowercase)",
			env:  []string{"pathext=.py;.sh"},
			want: []string{".py", ".sh"},
		},
		{
			name: "last assignment wins",
			env:  []string{"PATHEXT=.IGNORE", "PATHEXT=.WSH"},
			want: []string{".WSH"},
		},
		{
			name: "skip empty entries",
			env:  []string{"PATHEXT=.COM;;.EXE;"},
			want: []string{".COM", ".EXE"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathextFromEnv(tt.env)
			if len(got) != len(tt.want) {
				t.Errorf("pathextFromEnv = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("pathextFromEnv[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestResolveExecutable(t *testing.T) {
	dir := t.TempDir()
	rc := &RunContext{Ctx: context.Background(), Dir: dir}

	// On Unix, ResolveExecutable just returns rc.Path(name).
	name := "myprog"
	got := rc.ResolveExecutable(name)
	want := rc.Path(name)
	if got != want {
		t.Errorf("ResolveExecutable(%q) = %q, want %q", name, got, want)
	}

	// On Windows, when files actually exist, PATHEXT resolution applies.
	if runtime.GOOS == "windows" {
		// Create a .bat file — ResolveExecutable should find it via PATHEXT.
		batPath := filepath.Join(dir, name+".bat")
		if err := os.WriteFile(batPath, []byte("@echo off"), 0o644); err != nil {
			t.Fatal(err)
		}
		envWithPathext := []string{"PATHEXT=.COM;.EXE;.BAT;.CMD"}
		rc.Env = envWithPathext
		got := rc.ResolveExecutable(name)
		if got != batPath {
			t.Errorf("ResolveExecutable(%q) = %q, want %q (existing .bat)", name, got, batPath)
		}

		// When the name already has an extension, return rc.Path(name) directly.
		exeName := "other.exe"
		exePath := filepath.Join(dir, exeName)
		if err := os.WriteFile(exePath, []byte("fake"), 0o644); err != nil {
			t.Fatal(err)
		}
		got2 := rc.ResolveExecutable(exeName)
		if got2 != exePath {
			t.Errorf("ResolveExecutable(%q) = %q, want %q", exeName, got2, exePath)
		}
	}
}

func TestNormalizePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		tests := []struct {
			in   string
			want string
		}{
			{`C:\foo`, `C:\foo`},
			{`C:/foo`, `C:\foo`},
			{`/foo/bar`, `\foo\bar`},
			{`foo/bar/baz`, `foo\bar\baz`},
			{`foo\bar`, `foo\bar`},
		}
		for _, tt := range tests {
			got := normalizePath(tt.in)
			if got != tt.want {
				t.Errorf("normalizePath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		}
	} else {
		// On Unix, normalizePath is identity.
		for _, p := range []string{"/foo", "foo/bar", "a/b/c"} {
			if got := normalizePath(p); got != p {
				t.Errorf("normalizePath(%q) = %q, want %q", p, got, p)
			}
		}
	}
}

func TestResolveExecutableExistingExt(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PATHEXT resolution is windows-only")
	}

	dir := t.TempDir()
	rc := &RunContext{
		Ctx: context.Background(),
		Dir: dir,
		Env: []string{"PATHEXT=.EXE;.BAT"},
	}

	// Create a file with an explicit extension.
	name := "tool.bat"
	batPath := filepath.Join(dir, name)
	if err := os.WriteFile(batPath, []byte("@echo off"), 0o644); err != nil {
		t.Fatal(err)
	}

	// resolveExecutable should return the path directly when extension present.
	got := resolveExecutable(rc, name)
	if got != batPath {
		t.Errorf("resolveExecutable(%q) = %q, want %q", name, got, batPath)
	}
}

func TestResolveExecutableSubdir(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PATHEXT resolution is windows-only")
	}

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "tool.exe"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	rc := &RunContext{
		Ctx: context.Background(),
		Dir: dir,
		Env: []string{"PATHEXT=.EXE"},
	}

	// Path separator in name should work.
	got := rc.ResolveExecutable("bin\\tool")
	want := filepath.Join(dir, "bin", "tool.exe")
	if !strings.EqualFold(got, want) {
		t.Errorf("ResolveExecutable(%q) = %q, want %q", "bin\\tool", got, want)
	}

	// Forward slash variant.
	got2 := rc.ResolveExecutable("bin/tool")
	if !strings.EqualFold(got2, want) {
		t.Errorf("ResolveExecutable(%q) = %q, want %q", "bin/tool", got2, want)
	}
}
