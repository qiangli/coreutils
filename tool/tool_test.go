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

	got := rc.ResolveExecutable("bin\\tool")
	want := filepath.Join(dir, "bin", "tool.exe")
	if !strings.EqualFold(got, want) {
		t.Errorf("ResolveExecutable(%q) = %q, want %q", "bin\\tool", got, want)
	}

	got2 := rc.ResolveExecutable("bin/tool")
	if !strings.EqualFold(got2, want) {
		t.Errorf("ResolveExecutable(%q) = %q, want %q", "bin/tool", got2, want)
	}
}

func TestSystemDrive(t *testing.T) {
	got := systemDrive()
	if runtime.GOOS == "windows" {
		// Must end with \, e.g. C:\
		if len(got) < 3 || got[len(got)-1] != '\\' {
			t.Errorf("systemDrive() = %q, want a drive root ending with \\", got)
		}
	} else {
		if got != "/" {
			t.Errorf("systemDrive() = %q, want /", got)
		}
	}
}

func TestToOSPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		sd := systemDrive()
		tests := []struct {
			path string
			want string
		}{
			{"/foo/bar", sd + "foo\\bar"},
			{"/", sd},
			{"/usr/bin", sd + "usr\\bin"},
			{`C:\foo`, `C:\foo`},
			{`C:/foo`, `C:\foo`},
			{`\\server\share\path`, `\\server\share\path`},
			{`//server/share/path`, `\\server\share\path`},
			{`foo/bar/baz`, `foo\bar\baz`},
			{``, ``},
		}
		for _, tt := range tests {
			got := toOSPath(tt.path)
			if got != tt.want {
				t.Errorf("toOSPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		}
	} else {
		tests := []string{"/foo/bar", "foo/bar", "/", "/usr/bin", "", "a/b/c"}
		for _, p := range tests {
			if got := toOSPath(p); got != p {
				t.Errorf("toOSPath(%q) = %q, want %q (identity on Unix)", p, got, p)
			}
		}
	}
}

func TestFromOSPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		sd := systemDrive()
		tests := []struct {
			path string
			want string
		}{
			{sd + "foo\\bar", "/foo/bar"},
			{sd, "/"},
			{sd + "Users\\Alice", "/Users/Alice"},
			{`\foo\bar`, `/foo/bar`},
			{`\`, `/`},
			{`C:\foo\bar`, `/foo/bar`},
			{`c:\Foo\BAR`, `/Foo/BAR`}, // case-insensitive drive match
			{`\\server\share\path`, `//server/share/path`},
			{`foo\bar\baz`, `foo/bar/baz`},
			{``, ``},
		}
		for _, tt := range tests {
			got := fromOSPath(tt.path)
			if got != tt.want {
				t.Errorf("fromOSPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		}
	} else {
		tests := []string{"/foo/bar", "foo/bar", "/", "/usr/bin", "", "a/b/c"}
		for _, p := range tests {
			if got := fromOSPath(p); got != p {
				t.Errorf("fromOSPath(%q) = %q, want %q (identity on Unix)", p, got, p)
			}
		}
	}
}

func TestLocalFSToOSFromOSRoundtrip(t *testing.T) {
	fs := NewLocalFS()
	paths := []string{
		"/",
		"/foo",
		"/foo/bar",
		"/Users/Alice",
		"relative/path",
		"",
	}
	for _, p := range paths {
		osPath := fs.ToOS(p)
		back := fs.FromOS(osPath)
		if back != p {
			t.Errorf("roundtrip %q: ToOS=%q FromOS=%q, want %q", p, osPath, back, p)
		}
	}
}

func TestLocalFSSysDrive(t *testing.T) {
	fs := NewLocalFS()
	sd := fs.SysDrive()
	if sd == "" {
		t.Error("SysDrive() returned empty string")
	}
	if runtime.GOOS == "windows" {
		if len(sd) < 3 || sd[len(sd)-1] != '\\' {
			t.Errorf("SysDrive() = %q, want a drive root ending with \\", sd)
		}
	} else {
		if sd != "/" {
			t.Errorf("SysDrive() = %q, want /", sd)
		}
	}
}

func TestLocalFSPassthrough(t *testing.T) {
	dir := t.TempDir()
	fs := NewLocalFS()

	err := fs.MkdirAll(dir, 0o755)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello world"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	// Stat through VFS.
	fi, err := fs.Stat(dir + "/test.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if fi.Name() != "test.txt" {
		t.Errorf("Stat name = %q, want test.txt", fi.Name())
	}
	if fi.Size() != 11 {
		t.Errorf("Stat size = %d, want 11", fi.Size())
	}

	// ReadFile through VFS.
	data, err := fs.ReadFile(dir + "/test.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("ReadFile = %q, want hello world", string(data))
	}

	// Open through VFS.
	f, err := fs.Open(dir + "/test.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	buf := make([]byte, 100)
	n, err := f.Read(buf)
	if err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	if string(buf[:n]) != "hello world" {
		t.Errorf("Open/Read = %q, want hello world", string(buf[:n]))
	}

	// ReadDir through VFS.
	ents, err := fs.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("ReadDir count = %d, want 1", len(ents))
	}
	if ents[0].Name() != "test.txt" {
		t.Errorf("ReadDir[0].Name() = %q, want test.txt", ents[0].Name())
	}

	// Remove through VFS.
	sub := filepath.Join(dir, "subdir")
	err = fs.Mkdir(sub, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	err = fs.Remove(sub)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Errorf("subdir still exists after Remove")
	}

	// RemoveAll through VFS.
	sub2 := filepath.Join(dir, "sub2")
	if err := os.Mkdir(sub2, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub2, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = fs.RemoveAll(sub2)
	if err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	if _, err := os.Stat(sub2); !os.IsNotExist(err) {
		t.Errorf("sub2 still exists after RemoveAll")
	}

	// Rename through VFS.
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	if err := os.WriteFile(src, []byte("renamed"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = fs.Rename(src, dst)
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("src still exists after Rename")
	}
	data, err = os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "renamed" {
		t.Errorf("dst content = %q, want renamed", string(data))
	}
}

func TestLocalFSCreateAndOpenFile(t *testing.T) {
	dir := t.TempDir()
	fs := NewLocalFS()

	// Create a file through VFS.
	fp := filepath.Join(dir, "created.txt")
	f, err := fs.Create(fp)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	n, err := f.Write([]byte("created content"))
	if err != nil {
		f.Close()
		t.Fatal(err)
	}
	if n != 15 {
		t.Errorf("Write wrote %d bytes, want 15", n)
	}
	f.Close()

	// Verify it exists via OS.
	data, err := os.ReadFile(fp)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "created content" {
		t.Errorf("content = %q, want created content", string(data))
	}

	// OpenFile through VFS for append.
	f2, err := fs.OpenFile(fp, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	_, err = f2.Write([]byte(" more"))
	if err != nil {
		f2.Close()
		t.Fatal(err)
	}
	f2.Close()

	data, err = os.ReadFile(fp)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "created content more" {
		t.Errorf("content = %q, want created content more", string(data))
	}
}

func TestRunContextHasFS(t *testing.T) {
	rc := &RunContext{Ctx: context.Background(), FS: NewLocalFS()}
	if rc.FS == nil {
		t.Fatal("FS is nil, want initialized LocalFS")
	}
	if rc.FS.SysDrive() == "" {
		t.Error("FS.SysDrive() is empty")
	}
}
