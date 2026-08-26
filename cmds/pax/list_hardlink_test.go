package paxcmd

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func makeSyntheticHardLinkArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	mtime := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// 1. Regular file
	h1 := &tar.Header{
		Name:     "file1",
		Mode:     0o644,
		Size:     5,
		ModTime:  mtime,
		Typeflag: tar.TypeReg,
		Uname:    "user",
		Gname:    "group",
	}
	if err := tw.WriteHeader(h1); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}

	// 2. Hard link to file1
	h2 := &tar.Header{
		Name:     "file2_hard",
		Mode:     0o644,
		Size:     0,
		ModTime:  mtime,
		Typeflag: tar.TypeLink,
		Linkname: "file1",
		Uname:    "user",
		Gname:    "group",
	}
	if err := tw.WriteHeader(h2); err != nil {
		t.Fatal(err)
	}

	// 3. Symlink to file1
	h3 := &tar.Header{
		Name:     "file3_sym",
		Mode:     0o777,
		Size:     5,
		ModTime:  mtime,
		Typeflag: tar.TypeSymlink,
		Linkname: "file1",
		Uname:    "user",
		Gname:    "group",
	}
	if err := tw.WriteHeader(h3); err != nil {
		t.Fatal(err)
	}

	// 4. Directory
	h4 := &tar.Header{
		Name:     "dir1/",
		Mode:     0o755,
		Size:     0,
		ModTime:  mtime,
		Typeflag: tar.TypeDir,
		Uname:    "user",
		Gname:    "group",
	}
	if err := tw.WriteHeader(h4); err != nil {
		t.Fatal(err)
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPAXListVerboseHardLinkOutput(t *testing.T) {
	arc := makeSyntheticHardLinkArchive(t)
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Env:   []string{"LC_TIME=C", "TZ=UTC"},
		Stdio: tool.Stdio{Out: &out, Err: &errb},
	}
	opener := func(*tool.RunContext, *options) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(arc)), nil
	}

	code := listModeWithOpener(rc, &options{verbose: true}, nil, opener)
	if code != 0 || errb.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4:\n%s", len(lines), out.String())
	}

	// Line 0: file1
	if !strings.HasSuffix(lines[0], "file1") || strings.Contains(lines[0], "==") || strings.Contains(lines[0], "->") {
		t.Errorf("line 0 invalid: %q", lines[0])
	}
	// Line 1: file2_hard == file1
	if !strings.HasSuffix(lines[1], "file2_hard == file1") {
		t.Errorf("line 1 invalid hard link output: %q", lines[1])
	}
	// Line 2: file3_sym -> file1
	if !strings.HasSuffix(lines[2], "file3_sym -> file1") {
		t.Errorf("line 2 invalid symlink output: %q", lines[2])
	}
	// Line 3: dir1/
	if !strings.HasSuffix(lines[3], "dir1/") || strings.Contains(lines[3], "==") {
		t.Errorf("line 3 invalid dir output: %q", lines[3])
	}
}

func TestPAXListVerboseHardLinkReverseOrder(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	mtime := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Hard link before its target
	h1 := &tar.Header{
		Name:     "link_first",
		Mode:     0o644,
		ModTime:  mtime,
		Typeflag: tar.TypeLink,
		Linkname: "target_later",
		Uname:    "user",
		Gname:    "group",
	}
	if err := tw.WriteHeader(h1); err != nil {
		t.Fatal(err)
	}

	h2 := &tar.Header{
		Name:     "target_later",
		Mode:     0o644,
		Size:     4,
		ModTime:  mtime,
		Typeflag: tar.TypeReg,
		Uname:    "user",
		Gname:    "group",
	}
	if err := tw.WriteHeader(h2); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("data")); err != nil {
		t.Fatal(err)
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Env:   []string{"LC_TIME=C", "TZ=UTC"},
		Stdio: tool.Stdio{Out: &out, Err: &errb},
	}
	opener := func(*tool.RunContext, *options) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(buf.Bytes())), nil
	}

	code := listModeWithOpener(rc, &options{verbose: true}, nil, opener)
	if code != 0 || errb.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2:\n%s", len(lines), out.String())
	}
	if !strings.HasSuffix(lines[0], "link_first == target_later") {
		t.Errorf("line 0 hard link reverse order failed: %q", lines[0])
	}
	if !strings.HasSuffix(lines[1], "target_later") || strings.Contains(lines[1], "==") {
		t.Errorf("line 1 target output failed: %q", lines[1])
	}
}

func TestPAXListVerboseHardLinkWithSubstitution(t *testing.T) {
	arc := makeSyntheticHardLinkArchive(t)
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Env:   []string{"LC_TIME=C", "TZ=UTC"},
		Stdio: tool.Stdio{Out: &out, Err: &errb},
	}
	opener := func(*tool.RunContext, *options) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(arc)), nil
	}

	sub, err := parseSubstitution("/file1/renamed1/")
	if err != nil {
		t.Fatal(err)
	}

	code := listModeWithOpener(rc, &options{verbose: true, subst: []substitution{sub}}, nil, opener)
	if code != 0 || errb.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4:\n%s", len(lines), out.String())
	}
	if !strings.HasSuffix(lines[0], "renamed1") {
		t.Errorf("line 0 substitution failed: %q", lines[0])
	}
	if !strings.HasSuffix(lines[1], "file2_hard == renamed1") {
		t.Errorf("line 1 hard link target substitution failed: %q", lines[1])
	}
	if !strings.HasSuffix(lines[2], "file3_sym -> renamed1") {
		t.Errorf("line 2 symlink target substitution failed: %q", lines[2])
	}
}

func TestPAXListVerboseCustomListOptInteractions(t *testing.T) {
	arc := makeSyntheticHardLinkArchive(t)
	opener := func(*tool.RunContext, *options) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(arc)), nil
	}

	t.Run("listopt_linkpath", func(t *testing.T) {
		var out, errb bytes.Buffer
		rc := &tool.RunContext{Stdio: tool.Stdio{Out: &out, Err: &errb}}
		paxOpts := paxOptions{listSet: true, listFormat: "%(path)s|%(linkname)s"}
		code := listModeWithOpener(rc, &options{verbose: true, paxOptions: paxOpts}, nil, opener)
		if code != 0 || errb.Len() != 0 {
			t.Fatalf("code=%d stderr=%q", code, errb.String())
		}
		expected := "file1|\nfile2_hard|file1\nfile3_sym|file1\ndir1/|\n"
		if out.String() != expected {
			t.Fatalf("got %q, want %q", out.String(), expected)
		}
	})

	t.Run("listopt_L_specifier", func(t *testing.T) {
		var out, errb bytes.Buffer
		rc := &tool.RunContext{Stdio: tool.Stdio{Out: &out, Err: &errb}}
		paxOpts := paxOptions{listSet: true, listFormat: "%L"}
		code := listModeWithOpener(rc, &options{verbose: true, paxOptions: paxOpts}, nil, opener)
		if code != 0 || errb.Len() != 0 {
			t.Fatalf("code=%d stderr=%q", code, errb.String())
		}
		// %L expands to "path -> linkname" ONLY for symlinks per POSIX spec
		expected := "file1\nfile2_hard\nfile3_sym -> file1\ndir1/\n"
		if out.String() != expected {
			t.Fatalf("got %q, want %q", out.String(), expected)
		}
	})
}

func TestPAXListHardLinkWriteErrorStatus(t *testing.T) {
	arc := makeSyntheticHardLinkArchive(t)
	opener := func(*tool.RunContext, *options) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(arc)), nil
	}

	var errb bytes.Buffer
	rc := &tool.RunContext{
		Env:   []string{"LC_TIME=C", "TZ=UTC"},
		Stdio: tool.Stdio{Out: paxListFailWriter{errors.New("disk full")}, Err: &errb},
	}

	code := listModeWithOpener(rc, &options{verbose: true}, nil, opener)
	if code != 1 {
		t.Fatalf("exit code=%d, want 1", code)
	}
	if !strings.Contains(errb.String(), "pax: write error: disk full") {
		t.Fatalf("stderr %q lacks write error", errb.String())
	}
}
