package paxcmd

import (
	"archive/tar"
	"bufio"
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
		Size:     0, // Normal tar symlink headers carry no body size.
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

func makeReverseHardLinkArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	mtime := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	if err := tw.WriteHeader(&tar.Header{
		Name: "link_first", Mode: 0o644, ModTime: mtime, Typeflag: tar.TypeLink,
		Linkname: "target_later", Uname: "user", Gname: "group",
	}); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{
		Name: "target_later", Mode: 0o644, Size: 4, ModTime: mtime,
		Typeflag: tar.TypeReg, Uname: "user", Gname: "group",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("data")); err != nil {
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
	hardFields := strings.Fields(lines[1])
	if len(hardFields) < 9 || hardFields[1] != "2" || hardFields[4] != "0" ||
		!strings.HasSuffix(lines[1], "file2_hard == file1") {
		t.Errorf("line 1 invalid hard link output: %q", lines[1])
	}
	// Line 2: file3_sym -> file1
	symlinkFields := strings.Fields(lines[2])
	if len(symlinkFields) < 9 || symlinkFields[1] != "1" || symlinkFields[4] != "5" ||
		!strings.HasSuffix(lines[2], "file3_sym -> file1") {
		t.Errorf("line 2 invalid symlink output: %q", lines[2])
	}
	// Line 3: dir1/
	if !strings.HasSuffix(lines[3], "dir1/") || strings.Contains(lines[3], "==") {
		t.Errorf("line 3 invalid dir output: %q", lines[3])
	}
}

func TestPAXListVerboseHardLinkReverseOrder(t *testing.T) {
	arc := makeReverseHardLinkArchive(t)

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

func TestPAXListVerboseReverseOrderTransformPlan(t *testing.T) {
	arc := makeReverseHardLinkArchive(t)
	opener := func(*tool.RunContext, *options) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(arc)), nil
	}

	t.Run("substitution", func(t *testing.T) {
		sub, err := parseSubstitution("/target_later/substituted-target/")
		if err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		rc := &tool.RunContext{Env: []string{"LC_TIME=C", "TZ=UTC"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
		code := listModeWithOpener(rc, &options{verbose: true, subst: []substitution{sub}}, nil, opener)
		if code != 0 || errb.Len() != 0 {
			t.Fatalf("code=%d stderr=%q", code, errb.String())
		}
		if !strings.Contains(out.String(), "link_first == substituted-target\n") ||
			!strings.HasSuffix(out.String(), "substituted-target\n") {
			t.Fatalf("reverse substitution output=%q", out.String())
		}
	})

	t.Run("interactive", func(t *testing.T) {
		tty := newFakeInteractiveTTY("renamed-link\nrenamed-target\n")
		r := &interactiveRenamer{tty: tty, in: bufio.NewReader(tty)}
		var out, errb bytes.Buffer
		rc := &tool.RunContext{Env: []string{"LC_TIME=C", "TZ=UTC"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
		code := listModeWithOpener(rc, &options{verbose: true, interactive: true, renamer: r}, nil, opener)
		if code != 0 || errb.Len() != 0 {
			t.Fatalf("code=%d stderr=%q", code, errb.String())
		}
		if !strings.Contains(out.String(), "renamed-link == renamed-target\n") ||
			!strings.HasSuffix(out.String(), "renamed-target\n") {
			t.Fatalf("reverse interactive output=%q", out.String())
		}
	})

	t.Run("interactive_skip_target", func(t *testing.T) {
		tty := newFakeInteractiveTTY(".\n\n")
		r := &interactiveRenamer{tty: tty, in: bufio.NewReader(tty)}
		var out, errb bytes.Buffer
		rc := &tool.RunContext{Env: []string{"LC_TIME=C", "TZ=UTC"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
		code := listModeWithOpener(rc, &options{verbose: true, interactive: true, renamer: r}, nil, opener)
		if code != 0 || errb.Len() != 0 {
			t.Fatalf("code=%d stderr=%q", code, errb.String())
		}
		if !strings.HasSuffix(out.String(), "link_first == target_later\n") || strings.Contains(out.String(), "\ntarget_later\n") {
			t.Fatalf("interactive skip output=%q", out.String())
		}
	})

	t.Run("interactive_error_is_preflight", func(t *testing.T) {
		tty := newFakeInteractiveTTY(".\n")
		r := &interactiveRenamer{tty: tty, in: bufio.NewReader(tty)}
		var out, errb bytes.Buffer
		rc := &tool.RunContext{Env: []string{"LC_TIME=C", "TZ=UTC"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
		code := listModeWithOpener(rc, &options{verbose: true, interactive: true, renamer: r}, nil, opener)
		if code != 1 || out.Len() != 0 || !strings.Contains(errb.String(), "interactive rename") {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errb.String())
		}
	})

	t.Run("duplicate_raw_name_occurrence", testPAXListVerboseDuplicateTargetKeepsPriorOccurrence)
	t.Run("substitution_collision_occurrence", testPAXListVerboseSubstitutionCollisionKeepsRawOccurrence)
	t.Run("target_occurrence_archive_order", testPAXListTargetOccurrenceUsesArchiveOrder)
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
	if fields := strings.Fields(lines[2]); len(fields) < 5 || fields[4] != "8" {
		t.Errorf("line 2 symlink size does not follow substituted target bytes: %q", lines[2])
	}
}

func TestPAXListVerboseForwardInteractiveTargets(t *testing.T) {
	arc := makeSyntheticHardLinkArchive(t)
	tty := newFakeInteractiveTTY("renamed-target\n.\n.\n.\n")
	r := &interactiveRenamer{tty: tty, in: bufio.NewReader(tty)}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Env: []string{"LC_TIME=C", "TZ=UTC"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	opener := func(*tool.RunContext, *options) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(arc)), nil
	}
	code := listModeWithOpener(rc, &options{verbose: true, interactive: true, renamer: r}, nil, opener)
	if code != 0 || errb.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}
	if !strings.Contains(out.String(), "file2_hard == renamed-target\n") ||
		!strings.Contains(out.String(), "file3_sym -> renamed-target\n") {
		t.Fatalf("forward interactive targets output=%q", out.String())
	}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if strings.Contains(line, "file3_sym ->") {
			if fields := strings.Fields(line); len(fields) < 5 || fields[4] != "14" {
				t.Fatalf("interactive symlink size does not follow target: %q", line)
			}
		}
	}
}

func testPAXListVerboseDuplicateTargetKeepsPriorOccurrence(t *testing.T) {
	var arc bytes.Buffer
	tw := tar.NewWriter(&arc)
	for _, h := range []*tar.Header{
		{Name: "target", Typeflag: tar.TypeReg, Size: 1},
		{Name: "link", Typeflag: tar.TypeLink, Linkname: "target"},
		{Name: "sym", Typeflag: tar.TypeSymlink, Linkname: "target"},
		{Name: "target", Typeflag: tar.TypeReg, Size: 1},
	} {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte("x")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	tty := newFakeInteractiveTTY("first-target\n.\n.\nsecond-target\n")
	r := &interactiveRenamer{tty: tty, in: bufio.NewReader(tty)}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Env: []string{"LC_TIME=C", "TZ=UTC"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	opener := func(*tool.RunContext, *options) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(arc.Bytes())), nil
	}
	code := listModeWithOpener(rc, &options{verbose: true, interactive: true, renamer: r}, nil, opener)
	if code != 0 || errb.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}
	if !strings.Contains(out.String(), "link == first-target\n") ||
		strings.Contains(out.String(), "link == second-target\n") ||
		!strings.Contains(out.String(), "sym -> first-target\n") ||
		strings.Contains(out.String(), "sym -> second-target\n") {
		t.Fatalf("duplicate target occurrence output=%q", out.String())
	}
}

func testPAXListVerboseSubstitutionCollisionKeepsRawOccurrence(t *testing.T) {
	var arc bytes.Buffer
	tw := tar.NewWriter(&arc)
	for _, h := range []*tar.Header{
		{Name: "source", Typeflag: tar.TypeReg, Size: 1},
		{Name: "link", Typeflag: tar.TypeLink, Linkname: "source"},
		{Name: "sym", Typeflag: tar.TypeSymlink, Linkname: "source"},
		{Name: "renamed", Typeflag: tar.TypeReg, Size: 1},
	} {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte("x")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	sub, err := parseSubstitution("/source/renamed/")
	if err != nil {
		t.Fatal(err)
	}
	tty := newFakeInteractiveTTY("source-final\n.\n.\ncollision-final\n")
	r := &interactiveRenamer{tty: tty, in: bufio.NewReader(tty)}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Env: []string{"LC_TIME=C", "TZ=UTC"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	opener := func(*tool.RunContext, *options) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(arc.Bytes())), nil
	}
	code := listModeWithOpener(rc, &options{
		verbose: true, interactive: true, renamer: r, subst: []substitution{sub},
	}, nil, opener)
	if code != 0 || errb.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}
	if !strings.Contains(out.String(), "link == source-final\n") ||
		strings.Contains(out.String(), "link == collision-final\n") ||
		!strings.Contains(out.String(), "sym -> source-final\n") ||
		strings.Contains(out.String(), "sym -> collision-final\n") {
		t.Fatalf("substitution collision output=%q", out.String())
	}
}

func testPAXListTargetOccurrenceUsesArchiveOrder(t *testing.T) {
	for _, tc := range []struct {
		name       string
		linkIndex  int
		original   map[string][]int
		substitute map[string][]int
		want       int
	}{
		{"latest_preceding", 3, map[string][]int{"target": {0, 2, 5}}, nil, 2},
		{"first_later", 0, map[string][]int{"target": {1, 4}}, nil, 1},
		{"raw_before_substitution_collision", 2, map[string][]int{"target": {0}}, map[string][]int{"target": {4}}, 0},
		{"substituted_fallback", 2, nil, map[string][]int{"target": {1, 4}}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := listTargetOccurrence(tc.linkIndex, "target", "target", tc.original, tc.substitute)
			if !ok || got != tc.want {
				t.Fatalf("listTargetOccurrence=(%d, %v), want (%d, true)", got, ok, tc.want)
			}
		})
	}
}

func TestPAXListNlinkMetadataAndFallback(t *testing.T) {
	for _, tc := range []struct {
		name string
		h    *tar.Header
		want uint64
	}{
		{"regular_default", &tar.Header{Typeflag: tar.TypeReg}, 1},
		{"hardlink_fallback", &tar.Header{Typeflag: tar.TypeLink}, 2},
		{"schily", &tar.Header{Typeflag: tar.TypeLink, PAXRecords: map[string]string{"SCHILY.nlink": "7"}}, 7},
		{"cpio", &tar.Header{Typeflag: tar.TypeLink, PAXRecords: map[string]string{"COREUTILS.cpio.c_nlink": "4"}}, 4},
		{"invalid_fallback", &tar.Header{Typeflag: tar.TypeLink, PAXRecords: map[string]string{"SCHILY.nlink": "bad"}}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := listNlink(tc.h); got != tc.want {
				t.Fatalf("listNlink=%d, want %d", got, tc.want)
			}
		})
	}
}

func TestPAXListOptUsesEffectivePAXPathAndLinkpath(t *testing.T) {
	originalName := "old/" + strings.Repeat("n", 120)
	originalTarget := "old/" + strings.Repeat("t", 120)
	var arc bytes.Buffer
	tw := tar.NewWriter(&arc)
	if err := tw.WriteHeader(&tar.Header{
		Name: originalName, Linkname: originalTarget, Typeflag: tar.TypeSymlink,
		Mode: 0o777, Format: tar.FormatPAX,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	sub, err := parseSubstitution(",old/,new/,")
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Stdio: tool.Stdio{Out: &out, Err: &errb}}
	opener := func(*tool.RunContext, *options) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(arc.Bytes())), nil
	}
	format := "%F|%L|%(path)s|%(linkpath)s|%(linkname)s"
	code := listModeWithOpener(rc, &options{
		verbose: true, subst: []substitution{sub},
		paxOptions: paxOptions{listSet: true, listFormat: format},
	}, nil, opener)
	if code != 0 || errb.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}
	name := strings.Replace(originalName, "old/", "new/", 1)
	target := strings.Replace(originalTarget, "old/", "new/", 1)
	want := name + "|" + name + " -> " + target + "|" + name + "|" + target + "|" + target + "\n"
	if out.String() != want {
		t.Fatalf("listopt output=%q, want %q", out.String(), want)
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

type failAfterListWriter struct {
	remaining int
	out       bytes.Buffer
	err       error
}

func (w *failAfterListWriter) Write(p []byte) (int, error) {
	if w.remaining == 0 {
		return 0, w.err
	}
	w.remaining--
	return w.out.Write(p)
}

func TestPAXListHardLinkAndCustomOutputErrors(t *testing.T) {
	arc := makeSyntheticHardLinkArchive(t)
	opener := func(*tool.RunContext, *options) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(arc)), nil
	}
	for _, tc := range []struct {
		name      string
		remaining int
		opts      options
	}{
		{"hardlink_after_regular", 1, options{verbose: true}},
		{"custom_listopt", 0, options{verbose: true, paxOptions: paxOptions{listSet: true, listFormat: "%F"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer := &failAfterListWriter{remaining: tc.remaining, err: errors.New("injected branch failure")}
			var errb bytes.Buffer
			rc := &tool.RunContext{Env: []string{"LC_TIME=C", "TZ=UTC"}, Stdio: tool.Stdio{Out: writer, Err: &errb}}
			if code := listModeWithOpener(rc, &tc.opts, nil, opener); code != 1 {
				t.Fatalf("code=%d, want 1", code)
			}
			if !strings.Contains(errb.String(), "pax: write error: injected branch failure") {
				t.Fatalf("stderr=%q", errb.String())
			}
			if tc.remaining == 1 && !strings.Contains(writer.out.String(), "file1") {
				t.Fatalf("hardlink branch was not reached after regular output: %q", writer.out.String())
			}
		})
	}
}
