package paxcmd

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPAXWriteSizeOverrideTruncatesAndZeroExtends(t *testing.T) {
	tests := []struct {
		name    string
		option  string
		source  string
		want    string
		wantLen int64
	}{
		{name: "global truncates", option: "size=3", source: "abcdef", want: "abc", wantLen: 3},
		{name: "local extends", option: "size:=5", source: "abc", want: "abc\x00\x00", wantLen: 5},
		{name: "local overrides global", option: "size=6,size:=2", source: "abcdef", want: "ab", wantLen: 2},
		{name: "empty local removes global", option: "size=3,size:=", source: "abcdef", want: "abcdef", wantLen: 6},
		{name: "delete removes global", option: "size=3,delete=size", source: "abcdef", want: "abcdef", wantLen: 6},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "file"), []byte(tc.source), 0o600); err != nil {
				t.Fatal(err)
			}
			out, errOut, code := exec(t, dir, "", "-w", "-x", "pax", "-o", tc.option, "file")
			if code != 0 || errOut != "" {
				t.Fatalf("pax -w: code=%d stderr=%q", code, errOut)
			}
			tr := tar.NewReader(strings.NewReader(out))
			var h *tar.Header
			for {
				var err error
				h, err = tr.Next()
				if err != nil {
					t.Fatal(err)
				}
				if h.Typeflag == tar.TypeReg {
					break
				}
			}
			body, err := io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
			if h.Size != tc.wantLen || string(body) != tc.want {
				t.Fatalf("member size/body = %d/%q, want %d/%q", h.Size, body, tc.wantLen, tc.want)
			}
		})
	}

	for _, option := range []string{"linkdata,size=3", "linkdata,size:=3"} {
		t.Run("hardlink "+option, func(t *testing.T) {
			dir := t.TempDir()
			first := filepath.Join(dir, "one")
			if err := os.WriteFile(first, []byte("abcdef"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Link(first, filepath.Join(dir, "two")); err != nil {
				t.Fatal(err)
			}
			out, errOut, code := exec(t, dir, "", "-w", "-x", "pax", "-o", option, "one", "two")
			if code != 0 || errOut != "" {
				t.Fatalf("pax -w: code=%d stderr=%q", code, errOut)
			}
			members := rawRealMembers(t, []byte(out))
			if len(members) != 2 {
				t.Fatalf("members = %#v", members)
			}
			for i, member := range members {
				if len(member.data) != 3 || string(member.data) != "abc" {
					t.Fatalf("member %d size/body = %d/%q, want 3/%q", i, len(member.data), member.data, "abc")
				}
			}
			if members[1].kind != tar.TypeLink || members[1].link != "one" {
				t.Fatalf("second link metadata = %#v", members[1])
			}
		})
	}

	for _, option := range []string{"linkdata,size=3,size:=", "linkdata,size=3,delete=size"} {
		t.Run("hardlink removed override "+option, func(t *testing.T) {
			dir := t.TempDir()
			first := filepath.Join(dir, "one")
			if err := os.WriteFile(first, []byte("abcdef"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Link(first, filepath.Join(dir, "two")); err != nil {
				t.Fatal(err)
			}
			out, errOut, code := exec(t, dir, "", "-w", "-x", "pax", "-o", option, "one", "two")
			if code != 0 || errOut != "" {
				t.Fatalf("pax -w: code=%d stderr=%q", code, errOut)
			}
			members := rawRealMembers(t, []byte(out))
			if len(members) != 2 {
				t.Fatalf("members = %#v", members)
			}
			for i, member := range members {
				if len(member.data) != 6 || string(member.data) != "abcdef" {
					t.Fatalf("member %d size/body = %d/%q, want 6/%q", i, len(member.data), member.data, "abcdef")
				}
			}
			if members[1].kind != tar.TypeLink || members[1].link != "one" {
				t.Fatalf("second link metadata = %#v", members[1])
			}
		})
	}
}

func TestPAXCopySizeOverrideMaterializesAdjustedData(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "file"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	_, errOut, code := exec(t, source, "", "-r", "-w", "-x", "pax", "-o", "size:=5", "file", destination)
	if code != 0 || errOut != "" {
		t.Fatalf("pax copy: code=%d stderr=%q", code, errOut)
	}
	body, err := os.ReadFile(filepath.Join(destination, "file"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "abc\x00\x00" {
		t.Fatalf("copied body = %q, want zero-extended data", body)
	}
}
