package makecmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This corpus deliberately exercises only behavior specified by POSIX Issue 7.
// GNU Make is an oracle, not an implementation source. CI runners that carry
// the campaign's pinned GNU Make 4.3 execute it automatically.
func TestDifferentialGNUmake43(t *testing.T) {
	gnu := findGNUmake43(t)
	cases := []struct {
		name, makefile string
		files          map[string]string
		args           []string
		env            []string
		result         string
	}{
		{
			name: "macro-substitution",
			makefile: `.POSIX:
SRC = one.c two.c
LATE = before
COPY = $(LATE)
LATE = after
all:
	@printf '%s|%s' '$(COPY)' '$(SRC:.c=.o)' > result
`,
			result: "after|one.o two.o",
		},
		{
			name: "explicit-automatic-macros",
			makefile: `.POSIX:
all: one two
	@printf '%s|%s|%s' '$@' '$<' '$?' > result
`,
			files: map[string]string{"one": "1", "two": "2"}, result: "all|one|one two",
		},
		{
			name: "double-suffix-inference",
			makefile: `.POSIX:
.SUFFIXES:
.SUFFIXES: .o .c
.c.o:
	@printf '%s|%s|%s' '$<' '$*' '$@' > $@
all: unit.o
	@cat unit.o > result
`,
			files: map[string]string{"unit.c": "source"}, result: "unit.c|unit|unit.o",
		},
		{
			name: "command-macro-precedence",
			makefile: `.POSIX:
VALUE = file
all:
	@printf '%s' '$(VALUE)' > result
`,
			args: []string{"VALUE=command"}, result: "command",
		},
		{
			name: "include-inline-and-continuation",
			makefile: `.POSIX:
include values.mk
all: ; @printf '%s' '$(WORDS)' > result
`,
			files: map[string]string{"values.mk": "WORDS = one \\\n  two\n"}, result: "one  two",
		},
		{
			name: "single-suffix-inference",
			makefile: `.POSIX:
.SUFFIXES:
.SUFFIXES: .src
.src:
	@printf '%s|%s|%s' '$<' '$*' '$@' > $@
all: generated
	@cat generated > result
`,
			files: map[string]string{"generated.src": "source"}, result: "generated.src|generated|generated",
		},
		{
			name: "makeflags-macro",
			makefile: `.POSIX:
VALUE = file
all:
	@printf '%s' '$(VALUE)' > result
`,
			env: []string{"MAKEFLAGS=VALUE=flags"}, result: "flags",
		},
		{
			name: "ignore-special-target",
			makefile: `.POSIX:
.IGNORE: all
all:
	@false
	@printf ok > result
`,
			result: "ok",
		},
		{
			name:     "recipe-continuation",
			makefile: ".POSIX:\nall:\n\t@printf '%s' 'one\\\n\ttwo' > result\n",
			result:   "one\\\ntwo",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ours, theirs := t.TempDir(), t.TempDir()
			for _, dir := range []string{ours, theirs} {
				write(t, filepath.Join(dir, "Makefile"), tc.makefile)
				for name, value := range tc.files {
					write(t, filepath.Join(dir, name), value)
				}
			}
			env := append([]string{"PATH=/bin:/usr/bin"}, tc.env...)
			_, ourErr, ourCode := runMake(t, ours, env, tc.args...)
			var gnuOut, gnuErr bytes.Buffer
			command := exec.Command(gnu, tc.args...)
			command.Dir, command.Env, command.Stdout, command.Stderr = theirs, env, &gnuOut, &gnuErr
			gnuCode := 0
			if err := command.Run(); err != nil {
				if exit, ok := err.(*exec.ExitError); ok {
					gnuCode = exit.ExitCode()
				} else {
					t.Fatal(err)
				}
			}
			if ourCode != gnuCode {
				t.Fatalf("exit ours=%d (%s), GNU=%d (%s)", ourCode, ourErr, gnuCode, gnuErr.String())
			}
			ourResult, ourReadErr := os.ReadFile(filepath.Join(ours, "result"))
			gnuResult, gnuReadErr := os.ReadFile(filepath.Join(theirs, "result"))
			if ourReadErr != nil || gnuReadErr != nil {
				t.Fatalf("read ours=%v GNU=%v", ourReadErr, gnuReadErr)
			}
			if string(ourResult) != string(gnuResult) || string(ourResult) != tc.result {
				t.Fatalf("result ours=%q GNU=%q want=%q", ourResult, gnuResult, tc.result)
			}
		})
	}
}

func findGNUmake43(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"gmake", "make"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		out, err := exec.Command(path, "--version").Output()
		if err == nil && (strings.Contains(string(out), "GNU Make 4.3") || os.Getenv("POSIX_MAKE_ALLOW_OTHER_GNU") == "1" && strings.Contains(string(out), "GNU Make")) {
			return path
		}
	}
	t.Skip("GNU Make 4.3 differential oracle is not installed")
	return ""
}
