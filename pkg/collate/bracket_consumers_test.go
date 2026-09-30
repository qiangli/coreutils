package collate_test

import (
	_ "github.com/qiangli/coreutils/cmds/awk"
	_ "github.com/qiangli/coreutils/cmds/ed"
	_ "github.com/qiangli/coreutils/cmds/expr"
	"strings"
	"testing"
)

func TestCompiledFullElementBrackets(t *testing.T) {
	dir, env := store(t, "collating-symbol <same>\ncollating-element <digraph> from \"ch\"\norder_start forward\n<same>\n<z>\n<c>\n<digraph> <same>\n<x> <same>\n<h>\n<a>\norder_end")
	input := "c\nh\nch\nx\nchch\ncch\nchh\nz\na\n"
	for _, tc := range []struct{ pattern, want string }{
		{`^[[.ch.]]$`, "ch\n"},
		{`^[[.ch.]]*$`, "ch\nchch\n"},
		{`^[[=ch=]]$`, "ch\nx\n"},
		{`^[[=x=]]$`, "ch\nx\n"},
		{`^[c-[.ch.]]$`, "c\nch\n"},
		{`^[[.ch.]-h]$`, "h\nch\nx\n"},
		{`^[ch]$`, "c\nh\n"},
		{`^[c[.ch.]h]$`, "c\nh\nch\n"},
		{`^[c[.ch.]]h$`, "ch\nchh\n"},
		{`^[^ch]$`, "x\nz\na\n"},
		{`^c[[.ch.]]$`, "cch\n"},
		{`^[[.ch.]]h$`, "chh\n"},
		{`^[z-c]$`, "c\nz\n"},
	} {
		for _, extended := range []bool{false, true} {
			for _, command := range []string{"grep", "sed"} {
				t.Run(command+tc.pattern+map[bool]string{false: "BRE", true: "ERE"}[extended], func(t *testing.T) {
					args := []string{}
					if extended {
						args = append(args, "-E")
					}
					if command == "grep" {
						args = append(args, tc.pattern)
					} else {
						args = append(args, "-n", "/"+tc.pattern+"/p")
					}
					out, stderr, code := invoke(t, dir, env, command, input, args...)
					if code != 0 || out != tc.want || stderr != "" {
						t.Fatalf("code=%d out=%q err=%q want=%q", code, out, stderr, tc.want)
					}
				})
			}
		}
	}
	for _, tc := range []struct {
		command, input, want string
		args                 []string
	}{
		{"grep", "chch\ncch\n", "chch\nch\n", []string{"-o", `[[.ch.]]\{1,2\}`}},
		{"sed", "chch\nx\n", "<chch>\nx\n", []string{`s/^\([[.ch.]]*\)$/<\1>/`}},
		{"sed", "chch\nch\n", "Y\nch\n", []string{`s/^\([[.ch.]]\)\1$/Y/`}},
		{"grep", "CH\nCh\ncH\nch\n", "CH\nCh\ncH\nch\n", []string{"-i", `^[[.ch.]]$`}},
	} {
		out, stderr, code := invoke(t, dir, env, tc.command, tc.input, tc.args...)
		if code != 0 || out != tc.want || stderr != "" {
			t.Errorf("%s %v: code=%d out=%q err=%q want=%q", tc.command, tc.args, code, out, stderr, tc.want)
		}
	}
	for _, tc := range []struct {
		command, input, want string
		args                 []string
	}{
		{"awk", "ch\nc\nchch\n", "ch\nchch\n", []string{`/^[c-[.ch.]]+$/ && length($0)>1 {print}`}},
		{"expr", "", "chch\n", []string{"chch", ":", `\([[.ch.]]*\)`}},
		{"ed", "a\nch\nc\nchch\n.\ng/^[[.ch.]]*$/p\nQ\n", "ch\nchch\n", []string{"-s"}},
	} {
		out, stderr, code := invoke(t, dir, env, tc.command, tc.input, tc.args...)
		if code != 0 || out != tc.want || stderr != "" {
			t.Errorf("%s: %d %q %q want %q", tc.command, code, out, stderr, tc.want)
		}
	}
	// A compiled LC_COLLATE also combines with sed's Unicode character model.
	utfEnv := append(append([]string{}, env...), "LC_CTYPE=C.UTF-8")
	for _, pattern := range []string{`^[[.ch.]]*$`, `^\([[.ch.]]\)\1$`} {
		out, stderr, code := invoke(t, dir, utfEnv, "sed", "chch\nc\n", "-n", "/"+pattern+"/p")
		if code != 0 || out != "chch\n" {
			t.Errorf("UTF8 %s: %d %q %q", pattern, code, out, stderr)
		}
	}
	for _, pattern := range []string{`[[.missing.]]`, `[[=missing=]]`, `[h-[.ch.]]`, `[[:alpha:]-h]`} {
		_, stderr, code := invoke(t, dir, env, "grep", "ch\n", pattern)
		if code != 2 || strings.TrimSpace(stderr) == "" {
			t.Errorf("accepted invalid %s: %d %q", pattern, code, stderr)
		}
	}
}

func TestCompiledMultibyteBrackets(t *testing.T) {
	dir, env := store(t, "collating-symbol <same>\norder_start forward\n<same>\n<a> <same>\n<U00E9> <same>\n<z>\norder_end")
	for _, ctype := range []string{"C", "C.UTF-8"} {
		selected := append(append([]string{}, env...), "LC_CTYPE="+ctype)
		for _, tc := range []struct{ pattern, want string }{
			{`^[[.é.]]*$`, "é\néé\n"},
			{`^[[=é=]]$`, "a\né\n"},
			{`^[a-[.é.]]$`, "a\né\n"},
			{`^\([[.é.]]\)\1$`, "éé\n"},
		} {
			out, stderr, code := invoke(t, dir, selected, "sed", "a\né\néé\nz\n", "-n", "/"+tc.pattern+"/p")
			if code != 0 || out != tc.want || stderr != "" {
				t.Errorf("%s %s: %d %q %q want %q", ctype, tc.pattern, code, out, stderr, tc.want)
			}
		}
	}
	out, stderr, code := invoke(t, dir, env, "grep", "éé\na\n", "-o", `[[.é.]]*`)
	if code != 0 || out != "éé\n" || stderr != "" {
		t.Errorf("grep offsets: %d %q %q", code, out, stderr)
	}
}
