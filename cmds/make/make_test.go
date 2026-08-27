package makecmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func runMake(t *testing.T, dir string, env []string, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: env, FS: tool.NewLocalFS(), Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &err}}
	code := runCapture(rc, args, &out, &err)
	return out.String(), err.String(), code
}

func runMakeInput(t *testing.T, dir string, env []string, input string, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: env, FS: tool.NewLocalFS(), Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &err}}
	code := run(rc, args)
	return out.String(), err.String(), code
}
func runCapture(rc *tool.RunContext, args []string, out, err *bytes.Buffer) int { return run(rc, args) }
func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBuildsDependenciesAndAutomaticMacros(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "Makefile"), `MSG = hello
all: result

result: input
	@printf '%s:%s:%s' '$(MSG)' '$<' '$@' > $@
`)
	write(t, filepath.Join(d, "input"), "source")
	_, errOut, code := runMake(t, d, nil)
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	b, err := os.ReadFile(filepath.Join(d, "result"))
	if err != nil || string(b) != "hello:input:result" {
		t.Fatalf("result=%q err=%v", b, err)
	}
}

func TestTimestampQuestionAndDryRun(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "Makefile"), "out: in\n\tprintf rebuilt > out\n")
	write(t, filepath.Join(d, "in"), "x")
	write(t, filepath.Join(d, "out"), "old")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(d, "out"), old, old); err != nil {
		t.Fatal(err)
	}
	_, _, code := runMake(t, d, nil, "-q", "out")
	if code != 1 {
		t.Fatalf("-q code=%d, want 1", code)
	}
	out, errOut, code := runMake(t, d, nil, "-n", "out")
	if code != 0 || errOut != "" || !strings.Contains(out, "printf rebuilt") {
		t.Fatalf("-n=(%q,%q,%d)", out, errOut, code)
	}
	b, _ := os.ReadFile(filepath.Join(d, "out"))
	if string(b) != "old" {
		t.Fatalf("dry run changed target: %q", b)
	}
	_, _, code = runMake(t, d, nil, "out")
	if code != 0 {
		t.Fatalf("build code=%d", code)
	}
	b, _ = os.ReadFile(filepath.Join(d, "out"))
	if string(b) != "rebuilt" {
		t.Fatalf("target=%q", b)
	}
	_, _, code = runMake(t, d, nil, "-q", "out")
	if code != 0 {
		t.Fatalf("up-to-date -q code=%d", code)
	}
}

func TestCommandLineMacroWinsAndEnvironmentE(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "Makefile"), "X = file\nout:\n\t@printf '%s' '$(X)' > out\n")
	_, errOut, code := runMake(t, d, []string{"X=environment"}, "X=command", "out")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	b, _ := os.ReadFile(filepath.Join(d, "out"))
	if string(b) != "command" {
		t.Fatalf("command macro=%q", b)
	}
	os.Remove(filepath.Join(d, "out"))
	_, _, code = runMake(t, d, []string{"X=environment"}, "-e", "out")
	if code != 0 {
		t.Fatal(code)
	}
	b, _ = os.ReadFile(filepath.Join(d, "out"))
	if string(b) != "environment" {
		t.Fatalf("environment macro=%q", b)
	}
}

func TestSuffixInference(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "Makefile"), `.c.o:
	@printf 'compiled %s' '$<' > $@
all: hello.o
`)
	write(t, filepath.Join(d, "hello.c"), "int main(void){}")
	_, errOut, code := runMake(t, d, nil, "all")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	b, _ := os.ReadFile(filepath.Join(d, "hello.o"))
	if string(b) != "compiled hello.c" {
		t.Fatalf("object=%q", b)
	}
}

func TestAllRequiredOptionsParseAndOrderedKeepStop(t *testing.T) {
	o, rest, err := parseArgs([]string{"-einpqrst", "-kS", "-fone", "-f", "two", "X=1", "all"}, options{})
	if err != nil || len(rest) != 2 {
		t.Fatalf("parse=(%+v,%q,%v)", o, rest, err)
	}
	if !o.envOverride || !o.ignore || o.keep || !o.dry || !o.print || !o.question || !o.noBuiltins || !o.silent || !o.touch {
		t.Fatalf("options=%+v", o)
	}
	if got := strings.Join(o.files, ","); got != "one,two" {
		t.Fatalf("files=%q", got)
	}
}

func TestGroupedFileOption(t *testing.T) {
	o, _, err := parseArgs([]string{"-enfBuildfile"}, options{})
	if err != nil || !o.envOverride || !o.dry || len(o.files) != 1 || o.files[0] != "Buildfile" {
		t.Fatalf("options=%+v err=%v", o, err)
	}
}

func TestStdinAndMultipleMakefiles(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "extra.mk"), "X = extra\n")
	stdin := "X = stdin\nall:\n\t@printf '%s' '$(X)' > result\n"
	_, stderr, code := runMakeInput(t, d, nil, stdin, "-f", "extra.mk", "-f", "-")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	b, _ := os.ReadFile(filepath.Join(d, "result"))
	if string(b) != "stdin" {
		t.Fatalf("result=%q", b)
	}
}

func TestMakeflagsPrecedenceMacroAndRecipeEnvironment(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "Makefile"), "X = file\nall:\n\t@printf '%s|%s|%s' '$(X)' \"$$X\" \"$$MAKEFLAGS\" > result\n")
	_, stderr, code := runMake(t, d, []string{"PATH=/bin:/usr/bin", "MAKEFLAGS=k X=flags", "X=environment", "SHELL=/bin/false"}, "-S", "X=command")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	b, _ := os.ReadFile(filepath.Join(d, "result"))
	got := string(b)
	if !strings.HasPrefix(got, "command|command|") || !strings.Contains(got, "X=command") {
		t.Fatalf("result=%q", got)
	}
}

func TestMakeflagsBareLettersAndEnvironmentOverride(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "Makefile"), "X = file\nall:\n\t@printf '%s' '$(X)' > result\n")
	_, stderr, code := runMake(t, d, []string{"PATH=/bin:/usr/bin", "MAKEFLAGS=e", "X=environment"})
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	b, _ := os.ReadFile(filepath.Join(d, "result"))
	if string(b) != "environment" {
		t.Fatalf("result=%q", b)
	}
}

func TestMakeflagsQuotedMacroRoundTrip(t *testing.T) {
	encoded := canonicalMakeflags(options{dry: true}, [][2]string{{"MESSAGE", "hello world"}})
	args, err := splitMakeflags(encoded)
	if err != nil {
		t.Fatal(err)
	}
	_, operands, err := parseArgs(args, options{})
	if err != nil || len(operands) != 1 || operands[0] != "MESSAGE=hello world" {
		t.Fatalf("encoded=%q args=%q operands=%q err=%v", encoded, args, operands, err)
	}
}

func TestInlineIncludeLazyMacroAndSuffixSubstitution(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "vars.mk"), "SRC = a.c b.c\n")
	write(t, filepath.Join(d, "Makefile"), `.POSIX:
include vars.mk
NAME = before
COPY = $(NAME)
NAME = after
OBJS = $(SRC:.c=.o)
all: ; @printf '%s|%s' '$(COPY)' '$(OBJS)' > result
`)
	_, stderr, code := runMake(t, d, []string{"PATH=/bin:/usr/bin"})
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	b, _ := os.ReadFile(filepath.Join(d, "result"))
	if string(b) != "after|a.o b.o" {
		t.Fatalf("result=%q", b)
	}
}

func TestIncludeDepthSixteen(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "Makefile"), "include inc0.mk\n")
	for i := 0; i < 16; i++ {
		body := "all: ; @printf ok > result\n"
		if i < 15 {
			body = fmt.Sprintf("include inc%d.mk\n", i+1)
		}
		write(t, filepath.Join(d, fmt.Sprintf("inc%d.mk", i)), body)
	}
	_, stderr, code := runMake(t, d, []string{"PATH=/bin:/usr/bin"})
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	b, _ := os.ReadFile(filepath.Join(d, "result"))
	if string(b) != "ok" {
		t.Fatalf("result=%q", b)
	}
}

func TestSuffixSubstitutionPreservesSeparators(t *testing.T) {
	if got := suffixSubstitute("one.c\t two.c  three.h", ".c", ".o"); got != "one.o\t two.o  three.h" {
		t.Fatalf("got=%q", got)
	}
}

func TestPrerequisitesAccumulateAndLastRecipeWins(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "Makefile"), `all: one
all: two
	@printf '%s' '$?' > result
one two:
	@printf x > $@
`)
	_, stderr, code := runMake(t, d, []string{"PATH=/bin:/usr/bin"})
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	b, _ := os.ReadFile(filepath.Join(d, "result"))
	if string(b) != "one two" {
		t.Fatalf("result=%q", b)
	}
}

func TestSingleSuffixInferenceAndSuffixAppendClear(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "Makefile"), `.SUFFIXES: .x
.x:
	@printf '%s|%s|%s' '$<' '$*' '$@' > $@
all: generated
`)
	write(t, filepath.Join(d, "generated.x"), "source")
	_, stderr, code := runMake(t, d, []string{"PATH=/bin:/usr/bin"})
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	b, _ := os.ReadFile(filepath.Join(d, "generated"))
	if string(b) != "generated.x|generated|generated" {
		t.Fatalf("generated=%q", b)
	}

	m := newMakefile(false)
	if err := m.parse(&tool.RunContext{Dir: d, FS: tool.NewLocalFS()}, strings.NewReader(".SUFFIXES: .x\n.SUFFIXES:\n.SUFFIXES: .z\n"), "test", map[string]bool{}, 0); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(m.suffixes, " "); got != ".z" {
		t.Fatalf("suffixes=%q", got)
	}
}

func TestAutomaticDirectoryFilenameVariants(t *testing.T) {
	d := t.TempDir()
	if err := os.Mkdir(filepath.Join(d, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(d, "sub", "in"), "x")
	write(t, filepath.Join(d, "Makefile"), "sub/out: sub/in\n\t@printf '%s|%s|%s|%s' '$(@D)' '$(@F)' '$(<D)' '$(<F)' > $@\n")
	_, stderr, code := runMake(t, d, []string{"PATH=/bin:/usr/bin"}, "sub/out")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	b, _ := os.ReadFile(filepath.Join(d, "sub", "out"))
	if string(b) != "sub|out|sub|in" {
		t.Fatalf("result=%q", b)
	}
}

func TestQuestionDryAndTouchExecutePlusRecipe(t *testing.T) {
	for _, mode := range []string{"-q", "-n", "-t"} {
		t.Run(mode, func(t *testing.T) {
			d := t.TempDir()
			write(t, filepath.Join(d, "Makefile"), "out:\n\t+@printf x >> plus\n\t@printf y > out\n")
			_, _, code := runMake(t, d, []string{"PATH=/bin:/usr/bin"}, mode, "out")
			if mode == "-q" && code != 1 {
				t.Fatalf("code=%d", code)
			}
			if mode != "-q" && code != 0 {
				t.Fatalf("code=%d", code)
			}
			b, err := os.ReadFile(filepath.Join(d, "plus"))
			if err != nil || string(b) != "x" {
				t.Fatalf("plus=%q err=%v", b, err)
			}
		})
	}
}

func TestTouchCreatesAndReportsButSkipsRecipeLessTarget(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "Makefile"), "all: dep\n\ndep:\n\tprintf made > dep\n")
	out, stderr, code := runMake(t, d, []string{"PATH=/bin:/usr/bin"}, "-t", "dep")
	if code != 0 || stderr != "" || !strings.Contains(out, "touch dep") {
		t.Fatalf("(%q,%q,%d)", out, stderr, code)
	}
	if _, err := os.Stat(filepath.Join(d, "dep")); err != nil {
		t.Fatal(err)
	}
}

func TestNoWorkMessageAndEqualTimestamp(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "Makefile"), "out: in\n\tprintf bad > out\n")
	write(t, filepath.Join(d, "in"), "x")
	write(t, filepath.Join(d, "out"), "ok")
	now := time.Now().Add(-time.Minute).Truncate(time.Second)
	if err := os.Chtimes(filepath.Join(d, "in"), now, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(d, "out"), now, now); err != nil {
		t.Fatal(err)
	}
	out, stderr, code := runMake(t, d, []string{"PATH=/bin:/usr/bin"}, "out")
	if code != 0 || stderr != "" || !strings.Contains(out, "nothing to be done") {
		t.Fatalf("(%q,%q,%d)", out, stderr, code)
	}
	b, _ := os.ReadFile(filepath.Join(d, "out"))
	if string(b) != "ok" {
		t.Fatalf("out=%q", b)
	}
}

func TestRecipeLessLogicalUpdatePropagatesToDependent(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "Makefile"), "middle: source\nfinal: middle\n\t@printf rebuilt > final\n")
	for name, value := range map[string]string{"source": "s", "middle": "m", "final": "f"} {
		write(t, filepath.Join(d, name), value)
	}
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	for name, offset := range map[string]time.Duration{"middle": 0, "final": time.Minute, "source": 2 * time.Minute} {
		when := base.Add(offset)
		if err := os.Chtimes(filepath.Join(d, name), when, when); err != nil {
			t.Fatal(err)
		}
	}
	_, stderr, code := runMake(t, d, []string{"PATH=/bin:/usr/bin"}, "final")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	b, _ := os.ReadFile(filepath.Join(d, "final"))
	if string(b) != "rebuilt" {
		t.Fatalf("final=%q", b)
	}
}

func TestNoMakefileExistingTargetAndDryRunNoBogusNoWork(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "existing"), "x")
	out, stderr, code := runMake(t, d, nil, "existing")
	if code != 0 || stderr != "" || !strings.Contains(out, "nothing to be done") {
		t.Fatalf("(%q,%q,%d)", out, stderr, code)
	}
	write(t, filepath.Join(d, "Makefile"), "all:\n\t@printf x > all\n")
	out, stderr, code = runMake(t, d, []string{"PATH=/bin:/usr/bin"}, "-n")
	if code != 0 || stderr != "" || strings.Contains(out, "nothing to be done") || !strings.Contains(out, "printf x") {
		t.Fatalf("(%q,%q,%d)", out, stderr, code)
	}
}

func TestKeepGoingSkipsDependentAndBuildsIndependent(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "Makefile"), "bad:\n\tfalse\ndependent: bad\n\tprintf wrong > dependent\ngood:\n\tprintf good > good\n")
	_, stderr, code := runMake(t, d, []string{"PATH=/bin:/usr/bin"}, "-k", "dependent", "good")
	if code == 0 || !strings.Contains(stderr, "bad") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(d, "dependent")); !os.IsNotExist(err) {
		t.Fatalf("dependent err=%v", err)
	}
	b, err := os.ReadFile(filepath.Join(d, "good"))
	if err != nil || string(b) != "good" {
		t.Fatalf("good=%q err=%v", b, err)
	}
}

func TestDefaultRuleUsesTargetAsFirstMacro(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "Makefile"), ".DEFAULT:\n\t@printf '%s' '$<' > $@\nall: unknown\n")
	_, stderr, code := runMake(t, d, []string{"PATH=/bin:/usr/bin"})
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	b, _ := os.ReadFile(filepath.Join(d, "unknown"))
	if string(b) != "unknown" {
		t.Fatalf("unknown=%q", b)
	}
}

func TestBuiltinsAndNoBuiltins(t *testing.T) {
	m := newMakefile(false)
	for _, name := range []string{"MAKE", "AR", "CC", "CFLAGS", "FC", "FFLAGS", "SHELL"} {
		if _, ok := m.vars[name]; !ok {
			t.Errorf("missing %s", name)
		}
	}
	if len(m.rules[".c.o"]) == 0 || len(m.rules[".sh"]) == 0 {
		t.Fatal("missing built-in rules")
	}
	r := newMakefile(true)
	if len(r.suffixes) != 0 || len(r.rules) != 0 {
		t.Fatalf("-r builtins remain: suffixes=%q rules=%d", r.suffixes, len(r.rules))
	}
	if r.vars["SHELL"].value != "/bin/sh" {
		t.Fatalf("SHELL=%q", r.vars["SHELL"].value)
	}
}

func TestPrintDatabaseIncludesMacrosInferenceAndSpecialTargets(t *testing.T) {
	m := newMakefile(false)
	m.posixSeen, m.ignore["all"] = true, true
	m.rules["all"] = []*rule{{targets: []string{"all"}, deps: []string{"input"}, recipes: []string{"build"}}}
	var out bytes.Buffer
	m.printDB(&out)
	text := out.String()
	for _, required := range []string{"CC = c99", ".SUFFIXES:", ".POSIX:", ".IGNORE: all", ".c.o:", "all: input", "\tbuild"} {
		if !strings.Contains(text, required) {
			t.Errorf("database lacks %q\n%s", required, text)
		}
	}
}

func TestArchiveMemberTimestampAndAutomaticPercent(t *testing.T) {
	d := t.TempDir()
	archive := filepath.Join(d, "lib.a")
	writeArchive(t, archive, "member.o", time.Now().Add(-time.Hour), "old")
	write(t, filepath.Join(d, "source"), "new")
	write(t, filepath.Join(d, "Makefile"), "lib.a(member.o): source\n\t@printf '%s|%s' '$@' '$%' > result\n")
	_, stderr, code := runMake(t, d, []string{"PATH=/bin:/usr/bin"}, "lib.a(member.o)")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	b, _ := os.ReadFile(filepath.Join(d, "result"))
	if string(b) != "lib.a|member.o" {
		t.Fatalf("result=%q", b)
	}
}

func TestTouchArchiveMemberUpdatesStoredTimestamp(t *testing.T) {
	d := t.TempDir()
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	writeArchive(t, filepath.Join(d, "lib.a"), "member.o", old, "old")
	write(t, filepath.Join(d, "source"), "new")
	write(t, filepath.Join(d, "Makefile"), "lib.a(member.o): source\n\t@false\n")
	_, stderr, code := runMake(t, d, []string{"PATH=/bin:/usr/bin"}, "-t", "lib.a(member.o)")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	info, err := archiveMemberInfo(tool.NewLocalFS(), filepath.Join(d, "lib.a"), "member.o")
	if err != nil || !info.ModTime().After(old) {
		t.Fatalf("mtime=%v err=%v", info.ModTime(), err)
	}
}

func TestArchiveMemberSuffixInference(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "member.c"), "source")
	write(t, filepath.Join(d, "Makefile"), `.SUFFIXES:
.SUFFIXES: .a .c
.c.a:
	@printf '%s|%s|%s|%s' '$<' '$*' '$@' '$%' > result
all: lib.a(member.o)
`)
	_, stderr, code := runMake(t, d, []string{"PATH=/bin:/usr/bin"})
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	b, _ := os.ReadFile(filepath.Join(d, "result"))
	if string(b) != "member.c|member|lib.a|member.o" {
		t.Fatalf("result=%q", b)
	}
}

func TestProjectdirSCCSLookupAndOverrideRecipe(t *testing.T) {
	d := t.TempDir()
	project := filepath.Join(d, "project")
	if err := os.MkdirAll(filepath.Join(project, "SCCS"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(project, "SCCS", "s.source"), "sccs")
	write(t, filepath.Join(d, "Makefile"), ".SCCS_GET: ; @printf retrieved > $@\nall: source\n")
	_, stderr, code := runMake(t, d, []string{"PATH=/bin:/usr/bin", "PROJECTDIR=" + project})
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	b, err := os.ReadFile(filepath.Join(d, "source"))
	if err != nil || string(b) != "retrieved" {
		t.Fatalf("source=%q err=%v", b, err)
	}
}

func TestCancellationRemovesCurrentTargetUnlessPrecious(t *testing.T) {
	for _, precious := range []bool{false, true} {
		t.Run(fmt.Sprintf("precious=%v", precious), func(t *testing.T) {
			d := t.TempDir()
			prefix := ""
			if precious {
				prefix = ".PRECIOUS: out\n"
			}
			write(t, filepath.Join(d, "Makefile"), prefix+"out:\n\tprintf partial > out; while :; do :; done\n")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var stdout, stderr bytes.Buffer
			rc := &tool.RunContext{Ctx: ctx, Dir: d, Env: []string{"PATH=/bin:/usr/bin"}, FS: tool.NewLocalFS(), Stdio: tool.Stdio{In: strings.NewReader(""), Out: &stdout, Err: &stderr}}
			done := make(chan int, 1)
			go func() { done <- run(rc, []string{"out"}) }()
			deadline := time.Now().Add(2 * time.Second)
			for {
				if _, err := os.Stat(filepath.Join(d, "out")); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("recipe did not create target")
				}
				time.Sleep(10 * time.Millisecond)
			}
			cancel()
			if code := <-done; code == 0 {
				t.Fatal("cancelled make succeeded")
			}
			_, err := os.Stat(filepath.Join(d, "out"))
			if precious && err != nil {
				t.Fatalf("precious target removed: %v", err)
			}
			if !precious && !os.IsNotExist(err) {
				t.Fatalf("ordinary target remains: %v stderr=%q", err, stderr.String())
			}
		})
	}
}

func writeArchive(t *testing.T, path, name string, mod time.Time, body string) {
	t.Helper()
	if len(name) > 15 {
		t.Fatal("test archive name too long")
	}
	header := []byte("!<arch>\n")
	entry := []byte(strings.Join([]string{
		fmt.Sprintf("%-16s", name+"/"), fmt.Sprintf("%-12d", mod.Unix()), fmt.Sprintf("%-6d", 0),
		fmt.Sprintf("%-6d", 0), fmt.Sprintf("%-8o", 0o644), fmt.Sprintf("%-10d", len(body)), "`\n",
	}, ""))
	header = append(header, entry...)
	header = append(header, body...)
	if len(body)%2 != 0 {
		header = append(header, '\n')
	}
	if err := os.WriteFile(path, header, 0o600); err != nil {
		t.Fatal(err)
	}
}
