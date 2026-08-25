package paxcmd

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
)

func TestPAXOptionGrammarWhitespaceEscapedCommaAndTrailingComma(t *testing.T) {
	o, err := parsePAXOptions([]string{`  comment=hello\,world, delete=security.*, `}, paxWrite, "pax")
	if err != nil {
		t.Fatal(err)
	}
	if o.global["comment"] != "hello,world" || len(o.deletes) != 1 || o.deletes[0] != "security.*" {
		t.Fatalf("parsed=%+v", o)
	}
	for _, arg := range []string{"comment=x,,times", "=value", "bad key=value", "delete=["} {
		if _, err := parsePAXOptions([]string{arg}, paxWrite, "pax"); err == nil {
			t.Fatalf("parsePAXOptions(%q) succeeded", arg)
		}
	}
}

func TestPAXEscapedCommaCursorRetainsSuffixAndNextKeyword(t *testing.T) {
	o, err := parsePAXOptions([]string{`comment=a\,bc,uid=7`}, paxRead, "pax")
	if err != nil {
		t.Fatal(err)
	}
	if o.global["comment"] != "a,bc" || o.global["uid"] != "7" {
		t.Fatalf("parsed=%+v", o.global)
	}
}

func TestPAXOptionRepeatedPrecedenceDeleteAdditiveAndListConcatenation(t *testing.T) {
	o, err := parsePAXOptions([]string{
		"comment=first,delete=security.*", "comment=last,delete=SCHILY.*",
	}, paxRead, "pax")
	if err != nil {
		t.Fatal(err)
	}
	if o.global["comment"] != "last" || len(o.deletes) != 2 {
		t.Fatalf("parsed=%+v", o)
	}
	list, err := parsePAXOptions([]string{"listopt=%(path)s,", `listopt=\t%(size)u`}, paxList, "pax")
	if err != nil {
		t.Fatal(err)
	}
	if list.listFormat != `%(path)s,\t%(size)u` {
		t.Fatalf("list format=%q", list.listFormat)
	}
}

func TestPAXOptionModeAndFormatApplicabilityFailClosed(t *testing.T) {
	cases := []struct {
		args   []string
		mode   paxOptionMode
		format string
	}{
		{[]string{"times"}, paxRead, "pax"},
		{[]string{"linkdata"}, paxCopy, "pax"},
		{[]string{"listopt=x"}, paxWrite, "pax"},
		{[]string{"exthdr.name=x"}, paxRead, "pax"},
		{[]string{"invalid=write"}, paxWrite, "pax"},
		{[]string{"comment=x"}, paxWrite, "ustar"},
	}
	for _, tc := range cases {
		if _, err := parsePAXOptions(tc.args, tc.mode, tc.format); err == nil {
			t.Fatalf("parsePAXOptions(%q, %v, %q) succeeded", tc.args, tc.mode, tc.format)
		}
	}
}

func TestPAXWriteGlobalLocalTimesAndHeaderNames(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "file")
	if err := os.WriteFile(path, []byte("body"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantA := time.Unix(1000000000, 123000000)
	if err := os.Chtimes(path, wantA, time.Unix(1000000100, 456000000)); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := exec(t, d, "", "-w", "-x", "pax",
		"-o", "comment=global,globexthdr.name=global-%n",
		"-o", "comment:=local,times,exthdr.name=meta/%f", "file")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	tr := tar.NewReader(strings.NewReader(out))
	g, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	if g.Typeflag != tar.TypeXGlobalHeader || g.Name != "global-1" || g.PAXRecords["comment"] != "global" {
		t.Fatalf("global header=%+v", g)
	}
	h, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	if h.Name != "file" || h.PAXRecords["comment"] != "local" || h.PAXRecords["atime"] == "" {
		t.Fatalf("member=%+v records=%v", h, h.PAXRecords)
	}
	if got := firstRawHeaderNameByType(t, []byte(out), tar.TypeXHeader); got != "meta/file" {
		t.Fatalf("extended header name=%q", got)
	}
}

func TestPAXWriteDeletePatternsAreAdditive(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := exec(t, d, "", "-w", "-x", "pax",
		"-o", "comment=keep,delete=COREUTILS.*", "-o", "delete=SCHILY.*", "file")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	tr := tar.NewReader(strings.NewReader(out))
	g, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	if g.PAXRecords["comment"] != "keep" {
		t.Fatalf("global records=%v", g.PAXRecords)
	}
	h, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	for key := range h.PAXRecords {
		if strings.HasPrefix(key, "COREUTILS.") || strings.HasPrefix(key, "SCHILY.") {
			t.Fatalf("deleted record survived: %s=%q", key, h.PAXRecords[key])
		}
	}
}

func TestPAXLinkdataWritesDataForEveryHardLink(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "first"), []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(d, "first"), filepath.Join(d, "second")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	out, errOut, code := exec(t, d, "", "-w", "-x", "pax", "-o", "linkdata", "first", "second")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	members := rawRealMembers(t, []byte(out))
	if len(members) != 2 {
		t.Fatalf("members=%v", members)
	}
	for i, name := range []string{"first", "second"} {
		if members[i].name != name || string(members[i].data) != "payload" {
			t.Fatalf("member %d=%+v", i, members[i])
		}
	}
	if members[1].kind != tar.TypeLink || members[1].link != "first" {
		t.Fatalf("second link metadata=%+v", members[1])
	}
}

func TestPAXReadAndListHeaderPrecedence(t *testing.T) {
	raw := makePAXPrecedenceArchive(t)
	d := t.TempDir()
	arc := filepath.Join(d, "archive.pax")
	if err := os.WriteFile(arc, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := exec(t, d, "", "-f", arc,
		"-o", "comment=command-global", "-o", "comment:=command-local",
		"-v", "-o", `listopt=%(comment)s:%(path)s:%(uid)u`)
	if code != 0 || errOut != "" || out != "command-local:archive-local:44\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	out, errOut, code = exec(t, d, "", "-f", arc,
		"-o", "comment=command-global", "-v", "-o", `listopt=%(comment)s:%(uid)u`)
	if code != 0 || errOut != "" || out != "archive-local:44\n" {
		t.Fatalf("local-over-global code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestPAXDeleteRestoresUnderlyingUSTARField(t *testing.T) {
	raw := makePAXPrecedenceArchive(t)
	raw = patchFirstRealHeaderName(t, raw, "base-name")
	d := t.TempDir()
	arc := filepath.Join(d, "archive.pax")
	if err := os.WriteFile(arc, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := exec(t, d, "", "-f", arc, "-o", "delete=path")
	if code != 0 || errOut != "" || out != "base-name\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestPAXEmptyPerFileOverrideIgnoresExtendedField(t *testing.T) {
	raw := patchFirstRealHeaderName(t, makePAXPrecedenceArchive(t), "base-name")
	d := t.TempDir()
	arc := filepath.Join(d, "archive.pax")
	if err := os.WriteFile(arc, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := exec(t, d, "", "-f", arc, "-o", "path:=")
	if code != 0 || errOut != "" || out != "base-name\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestPAXOptionsAcceptPhysicallyUSTARCompatiblePAXInput(t *testing.T) {
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	if err := tw.WriteHeader(&tar.Header{Name: "base", Typeflag: tar.TypeReg, Mode: 0o600, Format: tar.FormatUSTAR}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	d := t.TempDir()
	arc := filepath.Join(d, "minimal.pax")
	if err := os.WriteFile(arc, raw.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := exec(t, d, "", "-f", arc, "-o", "path:=renamed")
	if code != 0 || errOut != "" || out != "renamed\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestPAXArchiveGlobalPersistsAndEmptyRecordDeletes(t *testing.T) {
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	if err := tw.WriteHeader(&tar.Header{Name: "g1", Typeflag: tar.TypeXGlobalHeader, Format: tar.FormatPAX,
		PAXRecords: map[string]string{"comment": "global"}}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o600, Format: tar.FormatPAX}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.WriteHeader(&tar.Header{Name: "g2", Typeflag: tar.TypeXGlobalHeader, Format: tar.FormatPAX,
		PAXRecords: map[string]string{"comment": ""}}); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "three", Typeflag: tar.TypeReg, Mode: 0o600, Format: tar.FormatPAX}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	d := t.TempDir()
	arc := filepath.Join(d, "archive.pax")
	if err := os.WriteFile(arc, raw.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := exec(t, d, "", "-v", "-f", arc, "-o", `listopt=%(comment)s:%(path)s`)
	// The zero-length global removes comment, so asking for it on the third
	// member fails closed rather than leaking the prior global value.
	if code != 1 || out != "global:one\nglobal:two\n" || !strings.Contains(errOut, `unknown listopt keyword "comment"`) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	out, errOut, code = exec(t, d, "", "-v", "-f", arc, "-o", "comment=command", "-o", `listopt=%(comment)s:%(path)s`)
	if code != 0 || errOut != "" || out != "command:one\ncommand:two\ncommand:three\n" {
		t.Fatalf("command global precedence: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestPAXReadAndCopyPerFilePathOverride(t *testing.T) {
	raw := string(makePAXPrecedenceArchive(t))
	extract := t.TempDir()
	_, errOut, code := execPaxContext(t, extract, raw, 0o022, "-r", "-o", "path:=renamed")
	if code != 0 || errOut != "" || string(mustRead(t, filepath.Join(extract, "renamed"))) != "x" {
		t.Fatalf("read code=%d stderr=%q", code, errOut)
	}

	copyRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(copyRoot, "source"), []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(copyRoot, "dest"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errOut, code = exec(t, copyRoot, "", "-r", "-w", "-l", "-o", "path:=linked-name", "source", "dest")
	if code != 0 || errOut != "" {
		t.Fatalf("copy code=%d stderr=%q", code, errOut)
	}
	src, _ := os.Stat(filepath.Join(copyRoot, "source"))
	dst, err := os.Stat(filepath.Join(copyRoot, "dest", "linked-name"))
	if err != nil || !os.SameFile(src, dst) {
		t.Fatalf("direct link override err=%v same=%v", err, err == nil && os.SameFile(src, dst))
	}
}

func TestPAXLinkCopyMetadataOverrideFallsBackWithoutMutatingSource(t *testing.T) {
	d := t.TempDir()
	source := filepath.Join(d, "source")
	dest := filepath.Join(d, "dest")
	if err := os.WriteFile(source, []byte("body"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	original := time.Unix(1_000_000_000, 0)
	want := time.Unix(1_100_000_000, 0)
	if err := os.Chtimes(source, original, original); err != nil {
		t.Fatal(err)
	}
	_, errOut, code := exec(t, d, "", "-r", "-w", "-l", "-o", "mtime:="+strconv.FormatInt(want.Unix(), 10), "source", "dest")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	src, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	dst, err := os.Stat(filepath.Join(dest, "source"))
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(src, dst) {
		t.Fatal("metadata override must not mutate its source through a hard link")
	}
	if !src.ModTime().Equal(original) || !dst.ModTime().Equal(want) {
		t.Fatalf("source mtime=%v destination mtime=%v", src.ModTime(), dst.ModTime())
	}
}

func TestPAXInvalidActionsBypassAndUTF8(t *testing.T) {
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	if err := tw.WriteHeader(&tar.Header{Name: "valid", Typeflag: tar.TypeReg, Mode: 0o600, Size: 1,
		Format: tar.FormatPAX, PAXRecords: map[string]string{"comment": "forces-pax"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	rawBytes := raw.Bytes()
	rawBytes = patchFirstRealHeaderName(t, rawBytes, string([]byte{'b', 'a', 'd', 0xff}))
	d := t.TempDir()
	arc := filepath.Join(d, "archive.pax")
	if err := os.WriteFile(arc, rawBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := exec(t, d, "", "-f", arc)
	if code != 1 || out != string([]byte{'b', 'a', 'd', 0xff, '\n'}) || !strings.Contains(errOut, "value cannot be translated") {
		t.Fatalf("default code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	for _, action := range []string{"binary", "rename", "write"} {
		gotOut, gotErr, gotCode := exec(t, d, "", "-f", arc, "-o", "invalid="+action)
		if gotCode != code || gotOut != out || gotErr != errOut {
			t.Errorf("invalid=%s got (%d,%q,%q), bypass was (%d,%q,%q)", action, gotCode, gotOut, gotErr, code, out, errOut)
		}
	}
	out, errOut, code = exec(t, d, "", "-f", arc, "-o", "invalid=UTF-8")
	if code != 1 || !strings.Contains(errOut, "value cannot be translated") || out != string([]byte{'b', 'a', 'd', 0xff, '\n'}) {
		t.Fatalf("UTF-8 code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestPAXInvalidRenameOpensTTYOnlyForInvalidSelectedValues(t *testing.T) {
	originalOpen := openInteractiveTTY
	openCalls := 0
	openInteractiveTTY = func() (io.ReadWriteCloser, error) {
		openCalls++
		return nil, errors.New("no controlling tty")
	}
	t.Cleanup(func() { openInteractiveTTY = originalOpen })

	raw := makeAttributeArchive(t, archiveFixture{name: "valid", body: "x", mode: 0o600})
	validDest := t.TempDir()
	_, errOut, code := execPaxContext(t, validDest, raw, 0o022, "-r", "-o", "invalid=rename")
	if code != 0 || errOut != "" || openCalls != 0 || string(mustRead(t, filepath.Join(validDest, "valid"))) != "x" {
		t.Fatalf("valid read code=%d stderr=%q opens=%d", code, errOut, openCalls)
	}

	invalidDest := t.TempDir()
	longName := strings.Repeat("x", 256)
	_, errOut, code = execPaxContext(t, invalidDest, raw, 0o022, "-r", "-o", "invalid=rename", "-o", "path:="+longName)
	if code != 1 || openCalls != 1 || !strings.Contains(errOut, "no controlling tty") {
		t.Fatalf("invalid read code=%d stderr=%q opens=%d", code, errOut, openCalls)
	}
	entries, err := os.ReadDir(invalidDest)
	if err != nil || len(entries) != 0 {
		t.Fatalf("invalid read mutated destination: entries=%v err=%v", entries, err)
	}

	copyRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(copyRoot, "source"), []byte("copy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(copyRoot, "dest"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errOut, code = exec(t, copyRoot, "", "-r", "-w", "-l", "-o", "invalid=rename", "source", "dest")
	if code != 0 || errOut != "" || openCalls != 1 {
		t.Fatalf("valid copy code=%d stderr=%q opens=%d", code, errOut, openCalls)
	}
	src, _ := os.Stat(filepath.Join(copyRoot, "source"))
	dst, err := os.Stat(filepath.Join(copyRoot, "dest", "source"))
	if err != nil || !os.SameFile(src, dst) {
		t.Fatalf("valid -l copy did not retain hard link: err=%v", err)
	}

	invalidCopyDest := filepath.Join(copyRoot, "invalid-dest")
	if err := os.Mkdir(invalidCopyDest, 0o755); err != nil {
		t.Fatal(err)
	}
	_, errOut, code = exec(t, copyRoot, "", "-r", "-w", "-l", "-o", "invalid=rename", "-o", "path:="+longName, "source", "invalid-dest")
	if code != 1 || openCalls != 2 || !strings.Contains(errOut, "no controlling tty") {
		t.Fatalf("invalid copy code=%d stderr=%q opens=%d", code, errOut, openCalls)
	}
	entries, err = os.ReadDir(invalidCopyDest)
	if err != nil || len(entries) != 0 {
		t.Fatalf("invalid copy mutated destination: entries=%v err=%v", entries, err)
	}
}

func TestPAXInvalidBinaryCoversNamesOwnersAndExtendedStrings(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	local := "comment:=" + string([]byte{0xff}) + ",uname:=" + string([]byte{0xfe}) + ",gname:=" + string([]byte{0xfd})
	out, errOut, code := exec(t, d, "", "-w", "-x", "pax", "-o", "invalid=binary", "-o", local, "file")
	if code != 1 || !strings.Contains(errOut, "value cannot be translated") {
		t.Fatalf("local code=%d stderr=%q", code, errOut)
	}
	h, err := tar.NewReader(strings.NewReader(out)).Next()
	if err != nil || h.PAXRecords["hdrcharset"] != "BINARY" {
		t.Fatalf("local header=%+v records=%v err=%v", h, h.PAXRecords, err)
	}
	out, errOut, code = exec(t, d, "", "-w", "-x", "pax", "-o", "invalid=binary", "-o", "comment="+string([]byte{0xfc}), "file")
	if code != 1 || !strings.Contains(errOut, "value cannot be translated") {
		t.Fatalf("global code=%d stderr=%q", code, errOut)
	}
	g, err := tar.NewReader(strings.NewReader(out)).Next()
	if err != nil || g.Typeflag != tar.TypeXGlobalHeader || g.PAXRecords["hdrcharset"] != "BINARY" {
		t.Fatalf("global header=%+v records=%v err=%v", g, g.PAXRecords, err)
	}
	out, errOut, code = execPaxEnv(t, d, []string{"LC_CTYPE=C"}, "-w", "-x", "pax", "-o", "invalid=binary", "-o", "comment=ä", "file")
	if code != 1 || !strings.Contains(errOut, "value cannot be translated") {
		t.Fatalf("C locale code=%d stderr=%q", code, errOut)
	}
	g, err = tar.NewReader(strings.NewReader(out)).Next()
	if err != nil || g.PAXRecords["hdrcharset"] != "BINARY" {
		t.Fatalf("C locale header=%+v records=%v err=%v", g, g.PAXRecords, err)
	}
	out, errOut, code = execPaxEnv(t, d, []string{"LC_CTYPE=C.UTF-8"}, "-w", "-x", "pax", "-o", "invalid=binary", "-o", "comment=ä", "file")
	if code != 0 || errOut != "" {
		t.Fatalf("UTF-8 locale code=%d stderr=%q", code, errOut)
	}
	g, err = tar.NewReader(strings.NewReader(out)).Next()
	if err != nil || g.PAXRecords["hdrcharset"] != "" {
		t.Fatalf("UTF-8 locale header=%+v records=%v err=%v", g, g.PAXRecords, err)
	}
}

func TestPAXListFormatConversionsAndConcatenation(t *testing.T) {
	raw := makeAttributeArchive(t, archiveFixture{
		name: "file", body: "abc", mode: 0o751, uid: 12, gid: 34,
		mtime: time.Date(2020, 2, 3, 20, 5, 6, 0, time.UTC),
	})
	d := t.TempDir()
	arc := filepath.Join(d, "archive.pax")
	if err := os.WriteFile(arc, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := exec(t, d, "", "-f", arc,
		"-v", "-o", `listopt=%(path)s:%(uid)04u:%.4M:`, "-o", `listopt=%(mtime=%Y-%m-%d)T:%F`)
	if code != 0 || errOut != "" || out != "file:0012:-rwx:2020-02-03:file\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	h := &tar.Header{ModTime: time.Unix(0, 0)}
	timeFormat := mustPAXTimeFormatter(t, nil)
	got, err := formatPAXList(h, `%(mtime=%H%z)T`, time.FixedZone("minus-eight", -8*60*60), &timeFormat)
	if err != nil || got != "16-0800" {
		t.Fatalf("timezone list format=%q err=%v", got, err)
	}
}

func TestPAXListFormatKeywordDeviceConversionAndPublishedShape(t *testing.T) {
	h := &tar.Header{Name: "/usr/foo/bar", Mode: 0o660, Size: 1492, ModTime: time.Date(2003, 1, 12, 15, 53, 0, 0, time.UTC)}
	timeFormat := mustPAXTimeFormatter(t, nil)
	got, err := formatPAXList(h, `%M %(mtime=%b %e %H:%M %Y)T %(size)8.6D %(name)s`, time.UTC, &timeFormat)
	if err != nil || got != "-rw-rw---- Jan 12 15:53 2003   001492 /usr/foo/bar" {
		t.Fatalf("published shape=%q err=%v", got, err)
	}
	got, err = formatPAXList(h, `%.7(name)s`, time.UTC, nil)
	if err != nil || got != "/usr/fo" {
		t.Fatalf("post-precision keyword=%q err=%v", got, err)
	}
}

func mustPAXTimeFormatter(t *testing.T, env []string) locale.TimeFormatter {
	t.Helper()
	formatter, err := locale.ResolveTime(env)
	if err != nil {
		t.Fatal(err)
	}
	return formatter
}

func TestPAXInvalidTextUsesRunContextLocaleEncodability(t *testing.T) {
	for _, tc := range []struct {
		env     []string
		value   string
		invalid bool
	}{
		{[]string{"LC_CTYPE=C"}, "ä", true},
		{[]string{"LC_CTYPE=POSIX"}, "ASCII", false},
		{[]string{"LC_CTYPE=de_DE.ISO-8859-1"}, "ä", false},
		{[]string{"LC_CTYPE=de_DE.ISO-8859-1"}, "中文", true},
		{[]string{"LC_CTYPE=de_DE.ISO-8859-15"}, "€", false},
		{[]string{"LC_CTYPE=C.UTF-8"}, "中文", false},
		{[]string{"LC_CTYPE=unknown"}, "ä", true},
		{[]string{"LANG=C.UTF-8", "LC_CTYPE=C.UTF-8", "LC_ALL=C"}, "ä", true},
	} {
		rc := &tool.RunContext{Env: tc.env}
		if got := invalidPAXText(rc, tc.value); got != tc.invalid {
			t.Errorf("env=%v value=%q invalid=%v, want %v", tc.env, tc.value, got, tc.invalid)
		}
	}
}

func TestPAXCarriedCodesetByteTranscoding(t *testing.T) {
	latin1 := &tool.RunContext{Env: []string{"LC_CTYPE=de_DE.ISO-8859-1"}}
	if got, err := archiveTextToLocal(latin1, "ä", false); err != nil || got != "\xe4" {
		t.Fatalf("UTF-8 to Latin-1=%q err=%v", got, err)
	}
	if got, err := localTextToArchive(latin1, "\xe4"); err != nil || got != "ä" {
		t.Fatalf("Latin-1 to UTF-8=%q err=%v", got, err)
	}
	latin15 := &tool.RunContext{Env: []string{"LC_CTYPE=de_DE.ISO-8859-15"}}
	if got, err := archiveTextToLocal(latin15, "€", false); err != nil || got != "\xa4" {
		t.Fatalf("UTF-8 to Latin-15=%q err=%v", got, err)
	}
	if got, err := localTextToArchive(latin15, "\xa4"); err != nil || got != "€" {
		t.Fatalf("Latin-15 to UTF-8=%q err=%v", got, err)
	}
	ascii := &tool.RunContext{Env: []string{"LC_CTYPE=C"}}
	if _, err := archiveTextToLocal(ascii, "中文", false); err == nil {
		t.Fatal("unrepresentable C-locale value accepted")
	}
	if got, err := archiveTextToLocal(ascii, "中文", true); err != nil || got != "??" {
		t.Fatalf("lossy invalid=write=%q err=%v", got, err)
	}
}

func TestPAXMultipleInvalidFieldsChooseDeterministicFirstError(t *testing.T) {
	rc := &tool.RunContext{Env: []string{"LC_CTYPE=C.UTF-8"}}
	for i := 0; i < 200; i++ {
		_, _, err := localPAXValuesToArchive(rc, map[string]string{
			"vendor.zeta":  "\xff",
			"vendor.alpha": "\xfe",
		}, "bypass")
		if err == nil || err.Error() != "-o vendor.alpha value cannot be translated to UTF-8" {
			t.Fatalf("iteration %d local option error=%v", i, err)
		}

		h := &tar.Header{Uname: "\xff", Gname: "\xfe"}
		if err := translatePAXIdentityToArchive(rc, h, "bypass"); err == nil || err.Error() != "uname cannot be translated to UTF-8" {
			t.Fatalf("iteration %d identity error=%v", i, err)
		}

		err = applyPAXValues(&tar.Header{}, map[string]string{
			"uid":   "bad-uid",
			"gid":   "bad-gid",
			"atime": "bad-time",
		})
		if err == nil || err.Error() != `invalid atime extended-header value "bad-time"` {
			t.Fatalf("iteration %d typed value error=%v", i, err)
		}
	}

	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantDiagnostic := "pax: file: -o vendor.alpha value cannot be translated to UTF-8\n"
	for i := 0; i < 50; i++ {
		_, errOut, code := execPaxEnv(t, d, []string{"LC_CTYPE=C.UTF-8"},
			"-w", "-x", "pax", "-o", "vendor.zeta:=\xff,vendor.alpha:=\xfe", "file")
		if code != 1 || errOut != wantDiagnostic {
			t.Fatalf("iteration %d code=%d stderr=%q", i, code, errOut)
		}
	}
}

func TestPAXListAndReadTranslateUTF8ArchiveToLatin1Bytes(t *testing.T) {
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	h := &tar.Header{Name: "ä", Typeflag: tar.TypeReg, Mode: 0o600, Size: 1, Uname: "ä", Gname: "ä", Format: tar.FormatPAX,
		PAXRecords: map[string]string{"comment": "ä"}}
	if err := tw.WriteHeader(h); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	d := t.TempDir()
	arc := filepath.Join(d, "archive.pax")
	if err := os.WriteFile(arc, raw.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	env := []string{"LC_CTYPE=de_DE.ISO-8859-1", "LC_TIME=C"}
	out, errOut, code := execPaxEnv(t, d, env, "-v", "-f", arc, "-o", `listopt=%(path)s|%(uname)s|%(gname)s|%(comment)s`)
	if code != 0 || errOut != "" || out != "\xe4|\xe4|\xe4|\xe4\n" {
		t.Fatalf("list code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if runtime.GOOS == "darwin" {
		// Darwin rejects non-UTF-8 pathname bytes at the VFS boundary. The
		// byte-exact list assertion above remains portable; extraction runs on
		// the Linux certification target.
		return
	}
	extract := t.TempDir()
	_, errOut, code = execPaxEnv(t, extract, env, "-r", "-f", arc)
	if code != 0 || errOut != "" || string(mustRead(t, filepath.Join(extract, "\xe4"))) != "x" {
		t.Fatalf("read code=%d stderr=%q", code, errOut)
	}
}

func TestPAXWriteTranslatesLatin1OptionBytesToUTF8(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := execPaxEnv(t, d, []string{"LC_CTYPE=de_DE.ISO-8859-1"}, "-w", "-x", "pax", "-o", "comment:=\xe4,uname:=\xe4,gname:=\xe4", "file")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	h, err := tar.NewReader(strings.NewReader(out)).Next()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"comment", "uname", "gname"} {
		if h.PAXRecords[key] != "ä" {
			t.Errorf("%s=%q records=%v", key, h.PAXRecords[key], h.PAXRecords)
		}
	}
}

func TestPAXWriteTranslatesHeaderIdentityFieldsOnce(t *testing.T) {
	rc := &tool.RunContext{Env: []string{"LC_CTYPE=de_DE.ISO-8859-1"}}
	h := &tar.Header{Name: "already-UTF-8-ä", Linkname: "already-UTF-8-ä", Uname: "\xe4", Gname: "\xe4"}
	if err := translatePAXIdentityToArchive(rc, h, "binary"); err != nil {
		t.Fatal(err)
	}
	if h.Name != "already-UTF-8-ä" || h.Linkname != "already-UTF-8-ä" {
		t.Fatalf("path/link were double-transcoded: name=%q link=%q", h.Name, h.Linkname)
	}
	if h.Uname != "ä" || h.Gname != "ä" {
		t.Fatalf("identity fields uname=%q gname=%q", h.Uname, h.Gname)
	}
}

func TestPAXWriteTranslatesActualLatin1PathAndLinkBytes(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("raw non-UTF-8 filesystem names require the Linux certification target")
	}
	d := t.TempDir()
	rawName := "\xe4"
	if err := os.WriteFile(filepath.Join(d, rawName), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(rawName, filepath.Join(d, "link")); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := execPaxEnv(t, d, []string{"LC_CTYPE=de_DE.ISO-8859-1"}, "-w", "-x", "pax", rawName, "link")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	tr := tar.NewReader(strings.NewReader(out))
	first, err := tr.Next()
	if err != nil || first.Name != "ä" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	if _, err := io.Copy(io.Discard, tr); err != nil {
		t.Fatal(err)
	}
	second, err := tr.Next()
	if err != nil || second.Name != "link" || second.Linkname != "ä" {
		t.Fatalf("second=%+v err=%v", second, err)
	}
}

func TestPAXWriteInvalidBinaryPreservesActualNonUTF8PathLinkAndOption(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("raw non-UTF-8 filesystem names require the Linux certification target")
	}
	d := t.TempDir()
	rawName := "\xff"
	if err := os.WriteFile(filepath.Join(d, rawName), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(rawName, filepath.Join(d, "link")); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := execPaxEnv(t, d, []string{"LC_CTYPE=C.UTF-8"}, "-w", "-x", "pax", "-o", "invalid=binary", "-o", "comment:=\xfe", rawName, "link")
	if code != 1 || !strings.Contains(errOut, "value cannot be translated") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	tr := tar.NewReader(strings.NewReader(out))
	first, err := tr.Next()
	if err != nil || first.Name != rawName || first.PAXRecords["comment"] != "\xfe" || first.PAXRecords["hdrcharset"] != "BINARY" {
		t.Fatalf("first=%+v records=%v err=%v", first, first.PAXRecords, err)
	}
	if _, err := io.Copy(io.Discard, tr); err != nil {
		t.Fatal(err)
	}
	second, err := tr.Next()
	if err != nil || second.Linkname != rawName || second.PAXRecords["hdrcharset"] != "BINARY" {
		t.Fatalf("second=%+v records=%v err=%v", second, second.PAXRecords, err)
	}
}

func TestPAXReadInvalidActionTranslationMatrix(t *testing.T) {
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	if err := tw.WriteHeader(&tar.Header{Name: "中文", Typeflag: tar.TypeReg, Mode: 0o600, Size: 1, Format: tar.FormatPAX}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		action, extracted string
	}{
		{"bypass", ""},
		{"binary", "中文"},
		{"UTF-8", "中文"},
		{"write", "??"},
	} {
		t.Run(tc.action, func(t *testing.T) {
			d := t.TempDir()
			var out, errs bytes.Buffer
			rc := &tool.RunContext{Dir: d, Env: []string{"LC_CTYPE=de_DE.ISO-8859-1"}, Stdio: tool.Stdio{In: bytes.NewReader(raw.Bytes()), Out: &out, Err: &errs}}
			code := run(rc, []string{"-r", "-o", "invalid=" + tc.action})
			if code != 1 || !strings.Contains(errs.String(), "value cannot be translated") {
				t.Fatalf("code=%d stderr=%q", code, errs.String())
			}
			if tc.extracted == "" {
				if entries, err := os.ReadDir(d); err != nil || len(entries) != 0 {
					t.Fatalf("bypass entries=%v err=%v", entries, err)
				}
				return
			}
			if got := string(mustRead(t, filepath.Join(d, tc.extracted))); got != "x" {
				t.Fatalf("body=%q", got)
			}
		})
	}
}

func TestPAXListInvalidActionMatrixForUnrepresentableUTF8(t *testing.T) {
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	if err := tw.WriteHeader(&tar.Header{Name: "中文", Typeflag: tar.TypeReg, Mode: 0o600, Format: tar.FormatPAX}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	d := t.TempDir()
	arc := filepath.Join(d, "archive.pax")
	if err := os.WriteFile(arc, raw.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"binary", "bypass", "rename", "UTF-8", "write"} {
		out, errOut, code := execPaxEnv(t, d, []string{"LC_CTYPE=de_DE.ISO-8859-1"}, "-f", arc, "-o", "invalid="+action)
		if code != 1 || out != "中文\n" || !strings.Contains(errOut, "value cannot be translated") {
			t.Errorf("action=%s code=%d stdout=%q stderr=%q", action, code, out, errOut)
		}
	}
}

func TestPAXUnknownHeaderCharsetFailsClosed(t *testing.T) {
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	if err := tw.WriteHeader(&tar.Header{Name: "file", Typeflag: tar.TypeReg, Mode: 0o600, Format: tar.FormatPAX,
		PAXRecords: map[string]string{"hdrcharset": "KOI8-R"}}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	rc := &tool.RunContext{Env: []string{"LC_CTYPE=C.UTF-8"}, Stdio: tool.Stdio{In: bytes.NewReader(raw.Bytes()), Out: &out, Err: &errs}}
	if code := run(rc, nil); code != 1 || out.String() != "file\n" || !strings.Contains(errs.String(), "value cannot be translated") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errs.String())
	}
}

func TestPAXStandardUTF8HeaderCharsetIsTranslated(t *testing.T) {
	rc := &tool.RunContext{Env: []string{"LC_CTYPE=de_DE.ISO-8859-1"}}
	h := &tar.Header{Name: "ä", Uname: "ä", PAXRecords: map[string]string{"hdrcharset": "ISO-IR 10646 2000 UTF-8"}}
	invalid := translatePAXHeaderToLocal(rc, h, "bypass", true)
	if invalid.name || invalid.link || invalid.other || h.Name != "\xe4" || h.Uname != "\xe4" {
		t.Fatalf("header=%+v invalid=%+v", h, invalid)
	}
}

func TestPAXReadBypassesUntranslatableOwnerForBypassAndRename(t *testing.T) {
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	if err := tw.WriteHeader(&tar.Header{Name: "file", Uname: "中文", Typeflag: tar.TypeReg, Mode: 0o600, Size: 1, Format: tar.FormatPAX}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"bypass", "rename"} {
		t.Run(action, func(t *testing.T) {
			d := t.TempDir()
			var out, errs bytes.Buffer
			rc := &tool.RunContext{Dir: d, Env: []string{"LC_CTYPE=de_DE.ISO-8859-1"}, Stdio: tool.Stdio{In: bytes.NewReader(raw.Bytes()), Out: &out, Err: &errs}}
			if code := run(rc, []string{"-r", "-o", "invalid=" + action}); code != 1 || !strings.Contains(errs.String(), "value cannot be translated") {
				t.Fatalf("code=%d stderr=%q", code, errs.String())
			}
			if entries, err := os.ReadDir(d); err != nil || len(entries) != 0 {
				t.Fatalf("entries=%v err=%v", entries, err)
			}
		})
	}
}

func TestPAXCopyInvalidRenamePreflightsNameAndLinkFields(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "ä"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("中文", filepath.Join(d, "slink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(d, "dest"), 0o755); err != nil {
		t.Fatal(err)
	}
	tty := newFakeInteractiveTTY("renamed\ntarget\n")
	withInteractiveTTY(t, tty)
	_, errOut, code := execPaxEnv(t, d, []string{"LC_CTYPE=C"}, "-r", "-w", "-o", "invalid=rename", "ä", "slink", "dest")
	if code != 1 || !strings.Contains(errOut, "value cannot be translated") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	if got := string(mustRead(t, filepath.Join(d, "dest", "renamed"))); got != "x" {
		t.Fatalf("renamed body=%q", got)
	}
	target, err := os.Readlink(filepath.Join(d, "dest", "slink"))
	if err != nil || target != "target" {
		t.Fatalf("target=%q err=%v", target, err)
	}
	if got := tty.out.String(); got != "pax: rename ä? pax: rename 中文? " {
		t.Fatalf("prompt=%q", got)
	}
}

func TestPAXInvalidRenamePromptsForLinkFieldNotMemberName(t *testing.T) {
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	if err := tw.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "中文", Mode: 0o777, Format: tar.FormatPAX}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	tty := newFakeInteractiveTTY("replacement\n")
	withInteractiveTTY(t, tty)
	d := t.TempDir()
	// Run against the archive on stdin so no filesystem pathname encoding is involved.
	var out, errs bytes.Buffer
	rc := &tool.RunContext{Dir: d, Env: []string{"LC_CTYPE=de_DE.ISO-8859-1"}, Stdio: tool.Stdio{In: bytes.NewReader(raw.Bytes()), Out: &out, Err: &errs}}
	code := run(rc, []string{"-r", "-o", "invalid=rename"})
	if code != 1 || !strings.Contains(errs.String(), "value cannot be translated") {
		t.Fatalf("code=%d stderr=%q", code, errs.String())
	}
	target, err := os.Readlink(filepath.Join(d, "link"))
	if err != nil || target != "replacement" {
		t.Fatalf("link target=%q err=%v", target, err)
	}
	if got := tty.out.String(); got != "pax: rename 中文? " {
		t.Fatalf("prompt=%q", got)
	}
}

func TestPAXListFormatCPIOFieldNames(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "file"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	arc := filepath.Join(d, "archive.cpio")
	_, errOut, code := exec(t, d, "", "-w", "-x", "cpio", "-f", arc, "file")
	if code != 0 || errOut != "" {
		t.Fatalf("write code=%d stderr=%q", code, errOut)
	}
	out, errOut, code := exec(t, d, "", "-v", "-f", arc, "-o", `listopt=%(c_magic)s:%(c_name)s:%(c_namesize)u:%(c_filesize)u`)
	if code != 0 || errOut != "" || out != "070707:file:5:3\n" {
		t.Fatalf("list code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestPAXListFormatPreservesActualRawUSTARFields(t *testing.T) {
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	if err := tw.WriteHeader(&tar.Header{Name: "x", Typeflag: tar.TypeReg, Mode: 0o600, Format: tar.FormatUSTAR}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	raw := archive.Bytes()
	header := raw[:512]
	for i := range header[:100] {
		header[i] = 0
	}
	for i := range header[345:500] {
		header[345+i] = 0
	}
	copy(header[:100], "leaf")
	copy(header[345:500], "short-prefix")
	setRawTarChecksum(header)
	checksumText := strings.Trim(string(bytes.Trim(header[148:156], " \x00")), " ")
	checksum, err := strconv.ParseInt(checksumText, 8, 64)
	if err != nil {
		t.Fatal(err)
	}
	d := t.TempDir()
	arc := filepath.Join(d, "raw.ustar")
	if err := os.WriteFile(arc, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := exec(t, d, "", "-v", "-f", arc, "-o", `listopt=%(name)s|%(prefix)s|%(magic)s|%(version)s|%(chksum)u`)
	want := fmt.Sprintf("leaf|short-prefix|ustar|00|%d\n", checksum)
	if code != 0 || errOut != "" || out != want {
		t.Fatalf("code=%d stdout=%q stderr=%q want=%q", code, out, errOut, want)
	}
	_, errOut, code = exec(t, d, "", "-v", "-f", arc, "-o", `listopt=%(COREUTILS.internal.ustar.name)s`)
	if code != 1 || !strings.Contains(errOut, "unknown listopt keyword") {
		t.Fatalf("private transport exposed: code=%d stderr=%q", code, errOut)
	}
}

func TestPAXListFormatCPIOFiledataBinaryEmptyAndSymlink(t *testing.T) {
	d := t.TempDir()
	payload := []byte{'a', 0, 0xff, 'z'}
	if err := os.WriteFile(filepath.Join(d, "file"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "empty"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("file", filepath.Join(d, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	arc := filepath.Join(d, "archive.cpio")
	_, errOut, code := exec(t, d, "", "-w", "-x", "cpio", "-f", arc, "file", "empty", "link")
	if code != 0 || errOut != "" {
		t.Fatalf("write code=%d stderr=%q", code, errOut)
	}
	out, errOut, code := exec(t, d, "", "-v", "-f", arc, "-o", `listopt=%(c_filedata)s`)
	want := string(payload) + "\n\nfile\n"
	if code != 0 || errOut != "" || out != want {
		t.Fatalf("list code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestPAXListTimeLocaleNamesEncodingsAndPrecedence(t *testing.T) {
	raw := makeAttributeArchive(t, archiveFixture{
		name: "file", body: "x", mode: 0o600,
		mtime: time.Date(2024, time.March, 1, 2, 3, 4, 0, time.UTC),
	})
	d := t.TempDir()
	arc := filepath.Join(d, "archive.pax")
	if err := os.WriteFile(arc, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	format := `listopt=%(mtime=%a|%A|%b|%B|%r|%p|%x|%X)T`
	for _, tc := range []struct {
		name string
		env  []string
		want string
	}{
		{"German UTF-8", []string{"TZ=UTC", "LC_TIME=de_DE.UTF-8"}, "Fr|Freitag|Mär|März|||01.03.2024|02:03:04\n"},
		{"German Latin-1", []string{"TZ=UTC", "LC_TIME=de_DE.ISO-8859-1"}, "Fr|Freitag|M\xe4r|M\xe4rz|||01.03.2024|02:03:04\n"},
		{"LC_ALL precedence", []string{"TZ=UTC", "LC_TIME=de_DE.UTF-8", "LC_ALL=C"}, "Fri|Friday|Mar|March|02:03:04 AM|AM|03/01/24|02:03:04\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := execPaxEnv(t, d, tc.env, "-v", "-f", arc, "-o", format)
			if code != 0 || errOut != "" || out != tc.want {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
			}
		})
	}
	out, errOut, code := execPaxEnv(t, d, []string{"TZ=UTC", "LC_TIME=de_DE.UTF-8"}, "-v", "-f", arc, "-o", `listopt=%(mtime)T`)
	if code != 0 || errOut != "" || out != "Mär  1 02:03 2024\n" {
		t.Fatalf("default %%T code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	out, errOut, code = execPaxEnv(t, d, []string{"TZ=UTC", "LC_TIME=fr_FR.UTF-8"}, "-v", "-f", arc, "-o", `listopt=%(mtime)T`)
	if code != 1 || out != "" || !strings.Contains(errOut, "LC_TIME") {
		t.Fatalf("unsupported code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	out, errOut, code = execPaxEnv(t, d, []string{"TZ=UTC", "LC_TIME=de_DE.UTF-8"}, "-v", "-f", arc)
	if code != 0 || errOut != "" || !strings.Contains(out, "Mär  1 02:03") {
		t.Fatalf("verbose code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestPAXListTimeCompleteIssue7DateConversions(t *testing.T) {
	h := &tar.Header{ModTime: time.Date(2024, time.March, 1, 14, 5, 6, 0, time.UTC)}
	timeFormat := mustPAXTimeFormatter(t, []string{"LC_TIME=C"})
	format := `%(mtime=%c|%C|%D|%h|%I|%n|%p|%r|%t|%U|%V|%w|%W|%x|%X|%a|%A|%b|%B|%d|%e|%F|%g|%G|%H|%j|%m|%M|%R|%S|%T|%u|%y|%Y|%z|%Z|%%|%Ec|%Od)T`
	got, err := formatPAXList(h, format, time.UTC, &timeFormat)
	want := "Fri Mar  1 14:05:06 2024|20|03/01/24|Mar|02|\n|PM|02:05:06 PM|\t|08|09|5|09|03/01/24|14:05:06|Fri|Friday|Mar|March|01| 1|2024-03-01|24|2024|14|061|03|05|14:05|06|14:05:06|5|24|2024|+0000|UTC|%|Fri Mar  1 14:05:06 2024|01"
	if err != nil || got != want {
		t.Fatalf("all conversions=%q err=%v", got, err)
	}
}

func execPaxEnv(t *testing.T, dir string, env []string, args ...string) (string, string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Dir: dir, Env: env, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut}}
	code := run(rc, args)
	return out.String(), errOut.String(), code
}

func TestPAXMalformedListFormatFailsClosed(t *testing.T) {
	raw := makeAttributeArchive(t, archiveFixture{name: "file", body: "x", mode: 0o600})
	d := t.TempDir()
	arc := filepath.Join(d, "archive.pax")
	if err := os.WriteFile(arc, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	_, errOut, code := exec(t, d, "", "-v", "-f", arc, "-o", "listopt=%(unknown)s")
	if code != 1 || !strings.Contains(errOut, "unknown listopt keyword") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestPAXEmptyListFormatStillOverridesDefaultListing(t *testing.T) {
	raw := makeAttributeArchive(t, archiveFixture{name: "file", body: "x", mode: 0o600})
	d := t.TempDir()
	arc := filepath.Join(d, "archive.pax")
	if err := os.WriteFile(arc, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := exec(t, d, "", "-v", "-f", arc, "-o", "listopt=")
	if code != 0 || errOut != "" || out != "\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	out, errOut, code = exec(t, d, "", "-f", arc, "-o", "listopt=%(unknown)s")
	if code != 0 || errOut != "" || out != "file\n" {
		t.Fatalf("non-verbose code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func makePAXPrecedenceArchive(t *testing.T) []byte {
	t.Helper()
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	if err := tw.WriteHeader(&tar.Header{
		Name: "global", Typeflag: tar.TypeXGlobalHeader, Format: tar.FormatPAX,
		PAXRecords: map[string]string{"comment": "archive-global", "uid": "33"},
	}); err != nil {
		t.Fatal(err)
	}
	h := &tar.Header{
		Name: "archive-local", Typeflag: tar.TypeReg, Mode: 0o600, Size: 1, Uid: 44,
		Format: tar.FormatPAX, PAXRecords: map[string]string{
			"path": "archive-local", "comment": "archive-local", "uid": "44",
		},
	}
	if err := tw.WriteHeader(h); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return raw.Bytes()
}

func firstRawHeaderNameByType(t *testing.T, data []byte, kind byte) string {
	t.Helper()
	for off := 0; off+512 <= len(data); {
		h := data[off : off+512]
		if allZero(h) {
			break
		}
		size, err := rawTarSize(h)
		if err != nil {
			t.Fatal(err)
		}
		if h[156] == kind {
			return rawTarName(h)
		}
		off += 512 + int((size+511)&^511)
	}
	return ""
}

func patchFirstRealHeaderName(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	out := append([]byte(nil), data...)
	for off := 0; off+512 <= len(out); {
		h := out[off : off+512]
		if allZero(h) {
			break
		}
		size, err := rawTarSize(h)
		if err != nil {
			t.Fatal(err)
		}
		if h[156] != tar.TypeXGlobalHeader && h[156] != tar.TypeXHeader {
			if err := setRawTarName(h, name); err != nil {
				t.Fatal(err)
			}
			setRawTarChecksum(h)
			return out
		}
		off += 512 + int((size+511)&^511)
	}
	t.Fatal("no real header")
	return nil
}

type rawRealMember struct {
	name, link string
	kind       byte
	data       []byte
}

func rawRealMembers(t *testing.T, data []byte) []rawRealMember {
	t.Helper()
	var out []rawRealMember
	for off := 0; off+512 <= len(data); {
		h := data[off : off+512]
		if allZero(h) {
			break
		}
		size, err := rawTarSize(h)
		if err != nil {
			t.Fatal(err)
		}
		end := off + 512 + int((size+511)&^511)
		if end > len(data) {
			t.Fatal("truncated raw member")
		}
		if h[156] != tar.TypeXHeader && h[156] != tar.TypeXGlobalHeader {
			out = append(out, rawRealMember{
				name: rawTarName(h), link: string(bytes.TrimRight(h[157:257], "\x00")),
				kind: h[156], data: append([]byte(nil), data[off+512:off+512+int(size)]...),
			})
		}
		off = end
	}
	return out
}
