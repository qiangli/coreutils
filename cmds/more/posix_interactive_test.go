package morecmd

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func memoryPager(text string, screenful int) (*pager, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
	o := options{screenful: screenful, width: 80, fromLine: 1}
	d := newDocument("fixture", strings.NewReader(text), nil, 80, o)
	d.all()
	return &pager{rc: rc, out: bufio.NewWriter(&out), o: o, files: []string{"fixture"}, doc: d, marks: make(map[byte]displayPosition), half: max(1, screenful/2)}, &out, &errb
}

func TestPOSIXCommandGrammarParsesCountsArgumentsAndControls(t *testing.T) {
	tests := []struct {
		input string
		want  moreCommand
	}{
		{"12f", moreCommand{count: 12, counted: true, key: 'f'}},
		{"/!needle\n", moreCommand{key: '/', arg: "!needle"}},
		{"?back\r", moreCommand{key: '?', arg: "back"}},
		{"ma", moreCommand{key: 'm', sub: 'a'}},
		{"'a", moreCommand{key: '\'', sub: 'a'}},
		{":e file name\n", moreCommand{key: ':', sub: 'e', arg: " file name"}},
		{":t symbol\n", moreCommand{key: ':', sub: 't', arg: " symbol"}},
		{"ZZ", moreCommand{key: 'Z', sub: 'Z'}},
		{string([]byte{0x04}), moreCommand{key: 0x04}},
	}
	for _, tt := range tests {
		in := &stringInput{Reader: strings.NewReader(tt.input)}
		got, err := readCommand(in)
		if err != nil || got != tt.want {
			t.Errorf("readCommand(%q) = %#v, %v; want %#v", tt.input, got, err, tt.want)
		}
	}
}

func TestPOSIXMovementMarksAndPositionCommands(t *testing.T) {
	p, _, errb := memoryPager("1\n2\n3\n4\n5\n6\n", 3)
	p.next = 3
	for _, step := range []struct {
		cmd  moreCommand
		want int
	}{
		{moreCommand{key: ' '}, 3},
		{moreCommand{key: 'b'}, 0},
		{moreCommand{key: 'j', count: 2, counted: true}, 2},
		{moreCommand{key: 'k'}, 1},
		{moreCommand{key: 'd'}, 2},
		{moreCommand{key: 'u'}, 1},
		{moreCommand{key: 'G'}, 3},
		{moreCommand{key: 'g', count: 2, counted: true}, 1},
	} {
		if !p.execute(step.cmd, false) || p.top != step.want {
			t.Fatalf("command %#v top=%d want=%d stderr=%q", step.cmd, p.top, step.want, errb.String())
		}
		p.next = min(len(p.doc.rows), p.top+3)
	}
	if !p.execute(moreCommand{key: 'm', sub: 'a'}, false) {
		t.Fatal("mark ended pager")
	}
	if got := p.marks['a']; got.row != 3 || got.offset != 2 {
		t.Fatalf("mark current position=%+v, want third display row/file row 3", got)
	}
	p.top = 4
	p.execute(moreCommand{key: '\'', sub: 'a'}, false)
	if p.top != 1 {
		t.Fatalf("return to mark top=%d, want 1", p.top)
	}
}

func TestPOSIXCountedForwardAndBackwardUseLines(t *testing.T) {
	p, _, errb := memoryPager("1\n2\n3\n4\n5\n6\n7\n8\n9\n", 3)
	p.next = 3
	if !p.execute(moreCommand{key: 'f', count: 2, counted: true}, false) || p.top != 2 {
		t.Fatalf("2f top=%d stderr=%q", p.top, errb.String())
	}
	if !p.execute(moreCommand{key: 'b', count: 1, counted: true}, false) || p.top != 1 {
		t.Fatalf("1b top=%d stderr=%q", p.top, errb.String())
	}
}

func TestPOSIXPreviousPositionTracksAnyLargeMovement(t *testing.T) {
	p, _, _ := memoryPager("1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n", 3)
	p.top, p.next = 1, 4
	p.execute(moreCommand{key: 'f', count: 4, counted: true}, false)
	if p.previous != (displayPosition{row: 3, offset: 2}) {
		t.Fatalf("previous=%+v", p.previous)
	}
	p.execute(moreCommand{key: '\'', sub: '\''}, false)
	if p.top != 1 {
		t.Fatalf("'' restored top=%d, want 1", p.top)
	}
}

func TestPOSIXSkipStartsCountLinesAfterLastDisplayed(t *testing.T) {
	p, _, _ := memoryPager("1\n2\n3\n4\n5\n6\n7\n8\n", 3)
	p.next = 3
	p.execute(moreCommand{key: 's'}, false)
	if p.top != 3 {
		t.Fatalf("s top=%d, want first line after displayed line 3", p.top)
	}
	p.top, p.next = 0, 3
	p.execute(moreCommand{key: 's', count: 2, counted: true}, false)
	if p.top != 4 {
		t.Fatalf("2s top=%d, want second line after displayed line 3", p.top)
	}
}

func TestPOSIXLineScrollWritesEntireCountButScreenScrollDoesNot(t *testing.T) {
	text := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n"
	for _, tt := range []struct {
		name string
		cmd  moreCommand
		top  int
		want string
	}{
		{"space", moreCommand{key: ' ', count: 5, counted: true}, 0, "4\n5\n6\n7\n8\n"},
		{"k", moreCommand{key: 'k', count: 5, counted: true}, 7, "3\n4\n5\n6\n7\n"},
		{"u", moreCommand{key: 'u', count: 5, counted: true}, 7, "3\n4\n5\n6\n7\n"},
		{"f", moreCommand{key: 'f', count: 5, counted: true}, 0, "6\n7\n8\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, out, _ := memoryPager(text, 3)
			p.top, p.next = tt.top, tt.top+3
			if !p.execute(tt.cmd, false) || !p.render() {
				t.Fatal("command/render failed")
			}
			if err := p.out.Flush(); err != nil {
				t.Fatal(err)
			}
			if got := out.String(); got != tt.want {
				t.Fatalf("output=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestPOSIXInsufficientRegularFileMovementKeepsScreen(t *testing.T) {
	for _, cmd := range []moreCommand{
		{key: ' ', count: 99, counted: true},
		{key: 'b', count: 99, counted: true},
		{key: 'g', count: 99, counted: true},
		{key: 'G', count: 99, counted: true},
	} {
		p, _, errb := memoryPager("1\n2\n3\n4\n5\n", 3)
		p.top, p.next = 1, 4
		if !p.execute(cmd, false) || p.top != 1 || !strings.Contains(errb.String(), "\a") {
			t.Fatalf("command=%#v top=%d stderr=%q", cmd, p.top, errb.String())
		}
	}
}

func TestPOSIXFileNavigationPastBoundsRepromptsUnchanged(t *testing.T) {
	p, _, errb := memoryPager("one\ntwo\n", 2)
	p.files, p.fileIndex, p.top, p.next = []string{"fixture"}, 0, 0, 2
	for _, cmd := range []moreCommand{{key: ':', sub: 'n', count: 2, counted: true}, {key: ':', sub: 'p'}} {
		if !p.execute(cmd, false) || p.doc.name != "fixture" || p.top != 0 || p.next != 2 {
			t.Fatalf("command=%#v changed state name=%q top=%d next=%d", cmd, p.doc.name, p.top, p.next)
		}
	}
	if strings.Count(errb.String(), "no file in requested direction") != 2 {
		t.Fatalf("navigation diagnostics=%q", errb.String())
	}
}

func TestPOSIXSearchIgnoreCaseInvertRepeatAndReverse(t *testing.T) {
	p, _, errb := memoryPager("Alpha\nbeta\nGAMMA\nbeta two\n", 2)
	p.o.ignoreCase = true
	if !p.searchFor("beta", true, false, 1, 0) || p.top != 1 {
		t.Fatalf("forward search top=%d stderr=%q", p.top, errb.String())
	}
	if !p.execute(moreCommand{key: 'n'}, false) || p.top != 3 {
		t.Fatalf("repeat search top=%d stderr=%q", p.top, errb.String())
	}
	if !p.execute(moreCommand{key: 'N'}, false) || p.top != 1 {
		t.Fatalf("reverse repeat top=%d stderr=%q", p.top, errb.String())
	}
	if !p.searchFor("beta", true, true, 1, 0) || p.top != 2 {
		t.Fatalf("inverted search top=%d stderr=%q", p.top, errb.String())
	}
}

func TestPOSIXSearchHonorsCarriedCharacterLocales(t *testing.T) {
	cMode, err := moreCharacterMode("POSIX")
	if err != nil || cMode != charactersByte {
		t.Fatalf("POSIX mode=%v,%v", cMode, err)
	}
	cMatcher, err := compileMoreMatcher(options{charMode: cMode, ctypeName: "POSIX", collateName: "POSIX", ignoreCase: true}, "Ä")
	if err != nil {
		t.Fatal(err)
	}
	if matched, err := cMatcher("ä"); err != nil || matched {
		t.Fatalf("C/POSIX -i matched non-ASCII bytes: matched=%v err=%v", matched, err)
	}
	utfMode, err := moreCharacterMode("de_DE.UTF-8")
	if err != nil || utfMode != charactersUTF8 {
		t.Fatalf("UTF-8 mode=%v,%v", utfMode, err)
	}
	utfMatcher, err := compileMoreMatcher(options{charMode: utfMode, ctypeName: "de_DE.UTF-8", collateName: "de_DE.UTF-8", ignoreCase: true}, "Ä")
	if err != nil {
		t.Fatal(err)
	}
	if matched, err := utfMatcher("ä"); err != nil || !matched {
		t.Fatalf("UTF-8 -i match=%v err=%v", matched, err)
	}
	if _, err := compileMoreMatcher(options{charMode: utfMode, ctypeName: "de_DE.UTF-8", collateName: "de_DE.UTF-8"}, "[[:alpha:]]"); err == nil {
		t.Fatal("unsupported UTF-8 locale class did not fail closed")
	}
	if isoMode, err := moreCharacterMode("de_DE.ISO-8859-1"); err != nil || isoMode != charactersByte {
		t.Fatalf("ISO-8859-1 mode=%v,%v", isoMode, err)
	}
}

func TestPOSIXFoldingUsesByteOrUTF8CharacterSemantics(t *testing.T) {
	iso := newDocument("iso", bytes.NewReader([]byte{0xe4, 0xf6, '\n'}), nil, 1, options{charMode: charactersByte})
	iso.all()
	if len(iso.rows) != 2 || !bytes.Equal(iso.rows[0].data, []byte{0xe4}) || !bytes.Equal(iso.rows[1].data, []byte{0xf6, '\n'}) {
		t.Fatalf("single-byte locale rows=%+v", iso.rows)
	}
	utf := newDocument("utf", strings.NewReader("ää\n"), nil, 1, options{charMode: charactersUTF8})
	utf.all()
	if len(utf.rows) != 2 || string(utf.rows[0].data) != "ä" || string(utf.rows[1].data) != "ä\n" {
		t.Fatalf("UTF-8 locale rows=%+v", utf.rows)
	}
}

func TestPOSIXTagLineAndPatternStart(t *testing.T) {
	dir := t.TempDir()
	content := "one\nneedle extra\ntwo\nneedle\nfour\n"
	if err := os.WriteFile(filepath.Join(dir, "source"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tags"), []byte("line\tsource\t4\npat\tsource\t/^needle$/;\"\nbeyond\tsource\t99\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"line", "pat"} {
		out, errb, code := runMore(t, dir, "", "-t", tag)
		if code != 0 || errb != "" || out != "needle\nfour\n" {
			t.Errorf("more -t %s = (%q,%q,%d)", tag, out, errb, code)
		}
	}
	_, _, pat, err := resolveTag(&tool.RunContext{Dir: dir}, "pat")
	if err != nil || pat != "^needle$" {
		t.Fatalf("anchored tag pattern=%q,%v", pat, err)
	}
	out, errb, code := runMore(t, dir, "", "-t", "beyond", "-p", "q")
	if code != 0 || out != content || !strings.Contains(errb, "line 99 does not exist") {
		t.Fatalf("beyond -t with -p = (%q,%q,%d)", out, errb, code)
	}
	_, errb, code = runMore(t, dir, "", "-t", "missing")
	if code == 0 || !strings.Contains(errb, "not found") {
		t.Fatalf("missing tag code=%d stderr=%q", code, errb)
	}
}

func TestPOSIXColonTagBeyondEOFUsesDefaultScreen(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "source"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tags"), []byte("beyond\tsource\t99\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	p := &pager{rc: rc, out: bufio.NewWriter(&out), o: options{screenful: 2, width: 80}, files: []string{"source"}, suppressCommands: make(map[int]bool)}
	if !p.openFile(0) || !p.execute(moreCommand{key: ':', sub: 't', arg: " beyond"}, false) || p.top != 0 {
		t.Fatalf(":t beyond top=%d stderr=%q", p.top, errb.String())
	}
	if !strings.Contains(errb.String(), "line 99 does not exist") {
		t.Fatalf(":t beyond missing informational message: %q", errb.String())
	}
}

func TestPOSIXEditorSelectionAndResumePosition(t *testing.T) {
	p, _, errb := memoryPager("one\ntwo\nthree\n", 2)
	p.top = 1
	p.rc.Dir = t.TempDir()
	editor := filepath.Join(p.rc.Dir, "vi")
	if err := os.WriteFile(editor, []byte("editor fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	p.rc.Env = []string{"EDITOR=vi", "PATH=" + p.rc.Dir}
	p.tty = &ttyChannel{}
	orig := runEditor
	var gotEditor, gotName string
	var gotLine int
	var gotTTY *ttyChannel
	runEditor = func(_ context.Context, _ *tool.RunContext, tty *ttyChannel, editor string, line int, name string) error {
		gotTTY = tty
		gotEditor, gotLine, gotName = editor, line, name
		return nil
	}
	t.Cleanup(func() { runEditor = orig })
	if !p.execute(moreCommand{key: 'v'}, false) || gotTTY != p.tty || gotEditor != editor || gotLine != 2 || !strings.HasSuffix(gotName, "fixture") || p.top != 1 {
		t.Fatalf("editor=(%q,%d,%q) top=%d stderr=%q", gotEditor, gotLine, gotName, p.top, errb.String())
	}
}

func TestPOSIXFileNavigationReloadHelpAndPosition(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"one": "old\n", "two": "second\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	p := &pager{
		rc: rc, out: bufio.NewWriter(&out), o: options{screenful: 2, width: 80},
		files: []string{"one", "two"}, marks: make(map[byte]displayPosition), suppressCommands: make(map[int]bool),
	}
	if !p.openFile(0) || !p.execute(moreCommand{key: ':', sub: 'n'}, false) || p.doc.name != "two" {
		t.Fatalf(":n file=%q stderr=%q", p.doc.name, errb.String())
	}
	if !p.execute(moreCommand{key: ':', sub: 'p'}, false) || p.doc.name != "one" {
		t.Fatalf(":p file=%q stderr=%q", p.doc.name, errb.String())
	}
	oldCode := p.exitCode
	if !p.execute(moreCommand{key: ':', sub: 'e', arg: " missing"}, false) ||
		p.doc.name != "one" || p.exitCode != oldCode || len(p.files) != 2 {
		t.Fatalf("failed :e file=%q status=%d files=%v stderr=%q", p.doc.name, p.exitCode, p.files, errb.String())
	}
	if err := os.WriteFile(filepath.Join(dir, "one"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !p.execute(moreCommand{key: 'R'}, false) {
		t.Fatal("R ended pager")
	}
	p.doc.all()
	if got := p.doc.lines[0]; got != "new\n" {
		t.Fatalf("R content=%q", got)
	}
	p.top, p.next = 0, 1
	savedDoc := p.doc
	p.tty = commandTTY(strings.Repeat(" ", 8))
	if !p.execute(moreCommand{key: 'h'}, false) || !p.execute(moreCommand{key: '='}, false) {
		t.Fatal("help or position ended pager")
	}
	if p.doc != savedDoc || p.top != 0 || p.next != 1 {
		t.Fatalf("help did not restore file/screen: doc=%p/%p top=%d next=%d", p.doc, savedDoc, p.top, p.next)
	}
	if err := p.out.Flush(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "more commands") || !strings.Contains(errb.String(), "line 1") {
		t.Fatalf("help=%q position=%q", out.String(), errb.String())
	}
}

func TestPOSIXExamineHashTracksPreviouslyExaminedFile(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"one", "two", "three"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	p := &pager{
		rc: rc, out: bufio.NewWriter(&out), o: options{screenful: 2, width: 80},
		files: []string{"one", "two", "three"}, marks: make(map[byte]displayPosition), suppressCommands: make(map[int]bool),
	}
	if !p.openFile(0) || !p.openFile(2) || p.previousExamined != "one" {
		t.Fatalf("setup current=%q previous=%q", p.doc.name, p.previousExamined)
	}
	if !p.execute(moreCommand{key: ':', sub: 'e', arg: " #"}, false) || p.doc.name != "one" {
		t.Fatalf(":e # current=%q previous=%q stderr=%q", p.doc.name, p.previousExamined, errb.String())
	}
}

func TestPOSIXExamineRejectsNonSeekableWithoutChangingScreen(t *testing.T) {
	p, _, errb := memoryPager("current\n", 2)
	p.rc.Dir = t.TempDir()
	p.files = []string{"fixture"}
	p.top, p.next = 0, 1
	orig := openInput
	openInput = func(_ *tool.RunContext, _ string) (io.Reader, io.Closer, error) {
		return bytes.NewBufferString("pipe\n"), nil, nil
	}
	t.Cleanup(func() { openInput = orig })
	oldDoc := p.doc
	if !p.execute(moreCommand{key: ':', sub: 'e', arg: " pipe"}, false) ||
		p.doc != oldDoc || p.top != 0 || p.next != 1 || p.exitCode != 0 {
		t.Fatalf("failed nonseekable :e doc=%p/%p top=%d next=%d status=%d stderr=%q",
			p.doc, oldDoc, p.top, p.next, p.exitCode, errb.String())
	}
}

func TestPOSIXMoreLinesColumnsPrecedence(t *testing.T) {
	rc := &tool.RunContext{Env: []string{"LINES=9", "COLUMNS=31", "MORE=-n 5"}}
	ch := &ttyChannel{hasFd: true}
	orig := getTerminalSize
	getTerminalSize = func(int) (int, int, error) { return 80, 24, nil }
	t.Cleanup(func() { getTerminalSize = orig })
	rows, cols := terminalSize(rc, ch, 5)
	if rows != 5 || cols != 31 {
		t.Fatalf("terminalSize = %d,%d; want 5,31", rows, cols)
	}
	if got := parseMORE(rc.Env); strings.Join(got, "|") != "-n|5" {
		t.Fatalf("parseMORE=%q", got)
	}
}

func TestPOSIXInteractiveSearchUsesTerminalCommandInput(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{In: strings.NewReader("alpha\nBETA\ngamma\n"), Out: &out, Err: &errb}}
	withPagerTTY(t, rc.Out, commandTTY("/beta\nq"), 2, 80)
	if code := run(rc, []string{"-i"}); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}
	if got := out.String(); got != "alpha\nBETA\n" {
		t.Fatalf("content=%q", got)
	}
}

func TestDocumentStillPagesBeforePipeEOF(t *testing.T) {
	gate := make(chan struct{})
	d := newDocument("-", &gatedSource{first: []byte("one\n"), gate: gate}, nil, 80, options{})
	d.ensure(1)
	if len(d.rows) != 1 || string(d.rows[0].data) != "one\n" || d.eof {
		t.Fatalf("rows=%v eof=%v", d.rows, d.eof)
	}
	close(gate)
}

func TestPOSIXInitialCommandStopsAfterFailure(t *testing.T) {
	p, _, errb := memoryPager("one\ntwo\nthree\nfour\n", 2)
	p.initialCommands("QG")
	if p.top != 0 || !p.commandFailed || !strings.Contains(errb.String(), "unknown command") {
		t.Fatalf("top=%d commandFailed=%v stderr=%q", p.top, p.commandFailed, errb.String())
	}
}

func TestPOSIXInitialCommandSeesLogicalFirstScreen(t *testing.T) {
	p, _, _ := memoryPager("1\n2\n3\n4\n5\n6\n7\n", 3)
	p.o.command = "s"
	p.runFileCommands()
	if p.next != 3 || p.top != 3 {
		t.Fatalf("-p s next=%d top=%d, want logical first screen ending at 3 then top 3", p.next, p.top)
	}
}

func TestPOSIXEOFDispatch(t *testing.T) {
	p, _, _ := memoryPager("1\n2\n3\n4\n", 2)
	p.files = []string{"fixture", "later"}
	p.fileIndex, p.top, p.next = 0, 2, 4
	if !p.execute(moreCommand{key: 'b'}, true) || p.fileIndex != 0 || p.top != 0 {
		t.Fatalf("non-advancing EOF command file=%d top=%d", p.fileIndex, p.top)
	}
	p.fileIndex = 1
	if p.execute(moreCommand{key: 'b'}, true) || !p.quit {
		t.Fatalf("last-file EOF non-advance returned true or did not quit")
	}
}

func TestPOSIXEOFAdvanceOpensNextFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "next"), []byte("next\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, _, errb := memoryPager("current\n", 2)
	p.rc.Dir = dir
	p.files = []string{"fixture", "next"}
	p.fileIndex, p.next = 0, 1
	if !p.execute(moreCommand{key: 'j'}, true) || p.fileIndex != 1 || p.doc.name != "next" {
		t.Fatalf("EOF j file=%d name=%q stderr=%q", p.fileIndex, p.doc.name, errb.String())
	}
}

func TestPOSIXWordExpansionUsesWholeVariableNamesAndInProcessCommandSubstitution(t *testing.T) {
	dir := t.TempDir()
	processDir, err := os.Getwd()
	if err != nil || filepath.Clean(processDir) == filepath.Clean(dir) {
		t.Fatalf("test requires process cwd distinct from RunContext.Dir: cwd=%q dir=%q err=%v", processDir, dir, err)
	}
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nested", "only.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	rc := &tool.RunContext{Dir: dir, Env: []string{"HOME=" + dir, "FOO=value", "FOObar=whole"}}
	if got, err := expandFilename(rc, `"$FOObar"`); err != nil || got != "whole" {
		t.Fatalf("whole variable expansion=%q,%v", got, err)
	}
	if got, err := expandFilename(rc, `"${FOO} bar"`); err != nil || got != "value bar" {
		t.Fatalf("quoted expansion=%q,%v", got, err)
	}
	if got, err := expandFilename(rc, `nested/*.txt`); err != nil || got != "nested/only.txt" {
		t.Fatalf("pathname expansion=%q,%v", got, err)
	}
	if got, err := expandFilename(rc, `$(printf safe)`); err != nil || got != "safe" {
		t.Fatalf("in-process command substitution=%q,%v", got, err)
	}
	if _, err := expandFilename(rc, `$(external-program)`); err == nil {
		t.Fatal("external command unexpectedly escaped the in-process word-expansion provider")
	}
}

func TestPOSIXInitialCommandRunsForEveryNewFile(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"one", "two"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("1\n2\n3\n4\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
	withPagerTTY(t, rc.Out, commandTTY(" "), 3, 80)
	if code := run(rc, []string{"-e", "-p", "G", "one", "two"}); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}
	if got, want := out.String(), "3\n4\n3\n4\n"; got != want {
		t.Fatalf("output=%q, want %q", got, want)
	}
}

func TestPOSIXCleanPrintMayBeIgnoredForDumbTerminal(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Env: []string{"TERM=dumb"},
		Stdio: tool.Stdio{In: strings.NewReader("one\n"), Out: &out, Err: &errb},
	}
	withPagerTTY(t, rc.Out, commandTTY("q"), 2, 80)
	if code := run(rc, []string{"-c"}); code != 0 || strings.Contains(errb.String(), "\x1b[") {
		t.Fatalf("TERM=dumb -c code=%d stderr=%q", code, errb.String())
	}
}

var _ io.Reader = (*gatedSource)(nil)
