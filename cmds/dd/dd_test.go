package ddcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, dir string, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestDdCopiesFileWithStatusNone(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "", "if=in", "of=out", "bs=2", "count=2", "status=none")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("dd: code=%d out=%q err=%q", code, out, errb)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "abcd" {
		t.Fatalf("out=%q want abcd", got)
	}
}

func TestDdStdinStdout(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), "hello", "bs=2", "count=2", "status=none")
	if code != 0 || errb != "" {
		t.Fatalf("dd stdin: code=%d err=%q", code, errb)
	}
	if out != "hell" {
		t.Fatalf("stdout=%q want hell", out)
	}
}

func TestDdSkipSeekNotrunc(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "out"), []byte("abcdefghij"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "if=in", "of=out", "bs=2", "skip=2", "seek=1", "count=2", "conv=notrunc", "status=none")
	if code != 0 {
		t.Fatalf("dd skip seek: code=%d err=%q", code, errb)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ab4567ghij" {
		t.Fatalf("out=%q want ab4567ghij", got)
	}
}

func TestDdErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "", "conv=lcase,ucase")
	if code != 2 || !strings.Contains(errb, "mutually exclusive") {
		t.Fatalf("case conversions: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "", "bad")
	if code != 2 || !strings.Contains(errb, "unrecognized operand") {
		t.Fatalf("bad operand: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "", "iflag=nonblock")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Fatalf("iflag unsupported: code=%d err=%q", code, errb)
	}
}

func TestParseBytesSyntax(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    int64
		wantErr error
	}{
		{name: "plain decimal", in: "128", want: 128},
		{name: "lowercase k", in: "1k", want: 1024},
		{name: "multiplication", in: "4x128", want: 512},
		{name: "mixed factors", in: "2x1k", want: 2048},
		{name: "empty", in: "", wantErr: strconv.ErrSyntax},
		{name: "leading x", in: "x128", wantErr: strconv.ErrSyntax},
		{name: "trailing x", in: "128x", wantErr: strconv.ErrSyntax},
		{name: "negative", in: "-1", wantErr: strconv.ErrSyntax},
		{name: "bad suffix", in: "1q", wantErr: strconv.ErrSyntax},
		{name: "overflow", in: "9223372036854775807x2", wantErr: strconv.ErrRange},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseBytes(tc.in)
			if tc.wantErr != nil {
				if err != tc.wantErr {
					t.Fatalf("parseBytes(%q) err=%v want %v", tc.in, err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("parseBytes(%q) = (%d, %v), want (%d, nil)", tc.in, got, err, tc.want)
			}
		})
	}
}

func TestDdOperandSizeSyntax(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "TP30 plain decimal bs", args: []string{"bs=128", "count=0", "status=none"}},
		{name: "TP30 plain decimal ibs", args: []string{"ibs=128", "count=0", "status=none"}},
		{name: "TP30 plain decimal obs", args: []string{"obs=128", "count=0", "status=none"}},
		{name: "TP30 plain decimal cbs", args: []string{"cbs=128", "conv=block", "count=0", "status=none"}},
		{name: "TP31 lowercase k bs", args: []string{"bs=1k", "count=0", "status=none"}},
		{name: "TP31 lowercase k ibs", args: []string{"ibs=1k", "count=0", "status=none"}},
		{name: "TP31 lowercase k obs", args: []string{"obs=1k", "count=0", "status=none"}},
		{name: "TP31 lowercase k cbs", args: []string{"cbs=1k", "conv=block", "count=0", "status=none"}},
		{name: "TP33 multiplicative bs", args: []string{"bs=4x128", "count=0", "status=none"}},
		{name: "TP33 multiplicative ibs", args: []string{"ibs=4x128", "count=0", "status=none"}},
		{name: "TP33 multiplicative obs", args: []string{"obs=4x128", "count=0", "status=none"}},
		{name: "TP33 multiplicative cbs", args: []string{"cbs=4x128", "conv=block", "count=0", "status=none"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, code := runTool(t, t.TempDir(), "", tc.args...)
			if code != 0 || out != "" || errb != "" {
				t.Fatalf("args=%v code=%d out=%q err=%q", tc.args, code, out, errb)
			}
		})
	}
}

func TestDdSeekOnRedirectedStandardOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "redirected-output")
	out, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: dir,
		Stdio: tool.Stdio{
			In:  strings.NewReader("0123456789"),
			Out: out,
			Err: &errb,
		},
	}
	code := cmd.Run(rc, []string{"ibs=10", "obs=10", "count=1", "seek=1", "status=none"})
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if code != 0 || errb.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := append(make([]byte, 10), []byte("0123456789")...)
	if !bytes.Equal(got, want) {
		t.Fatalf("redirected output=%v, want one skipped output block followed by input", got)
	}
}

func TestDdSeekRejectsNonSeekableStandardOutput(t *testing.T) {
	_, errb, code := runTool(t, t.TempDir(), "input", "obs=2", "seek=1", "status=none")
	if code != 2 || !strings.Contains(errb, "non-seekable standard output") {
		t.Fatalf("code=%d stderr=%q", code, errb)
	}
}

func TestDdCaseConversionsAreSingleByteAndReblockBs(t *testing.T) {
	for _, tc := range []struct {
		name, conv, input, want string
	}{
		{"lcase", "lcase", "AbC!\xc4", "abc!\xc4"},
		{"ucase", "ucase", "aBc!\xe4", "ABC!\xe4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, code := runTool(t, t.TempDir(), tc.input, "bs=2", "conv="+tc.conv, "status=none")
			if code != 0 || errb != "" || out != tc.want {
				t.Fatalf("code=%d out=%q err=%q; want %q", code, out, errb, tc.want)
			}
		})
	}
}

func TestDdSwabPreservesOddByteInEachInputRecord(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), "abcde", "ibs=3", "obs=4", "conv=swab", "status=none")
	if code != 0 || errb != "" || out != "baced" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errb)
	}

	// sync precedes swab, so the padding byte participates in the final pair.
	out, errb, code = runTool(t, t.TempDir(), "abc", "ibs=4", "conv=sync,swab", "status=none")
	if code != 0 || errb != "" || !bytes.Equal([]byte(out), []byte{'b', 'a', 0, 'c'}) {
		t.Fatalf("sync swab: code=%d out=%v err=%q", code, []byte(out), errb)
	}
}

func TestDdNoerrorSyncNotruncKeepsExistingTail(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "out"), []byte("WXYZtail"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "a", "of=out", "ibs=4", "obs=1", "conv=noerror,sync,notrunc", "status=none")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{'a', 0, 0, 0, 't', 'a', 'i', 'l'}; !bytes.Equal(got, want) {
		t.Fatalf("out=%v want=%v", got, want)
	}
}

type dataAndErrorReader struct{ read bool }

func (r *dataAndErrorReader) Read(p []byte) (int, error) {
	if r.read {
		return 0, io.EOF
	}
	r.read = true
	copy(p, "ab")
	return 2, errors.New("injected read failure")
}

func TestDdNoerrorProcessesDataReturnedWithReadError(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Stdio: tool.Stdio{In: &dataAndErrorReader{}, Out: &out, Err: &errb},
	}
	cfg := config{ibs: 4, obs: 4, count: -1, noerror: true, sync: true, reblock: true, status: "none"}
	if code := copyDD(rc, cfg); code != 1 {
		t.Fatalf("code=%d want 1; err=%q", code, errb.String())
	}
	if want := []byte{'a', 'b', 0, 0}; !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("output=%v want=%v", out.Bytes(), want)
	}
	if !strings.Contains(errb.String(), "injected read failure") {
		t.Fatalf("missing read diagnostic: %q", errb.String())
	}
}

type errorOnlyReader struct {
	err error
}

func (r *errorOnlyReader) Read([]byte) (int, error) {
	if r.err == nil {
		return 0, io.EOF
	}
	err := r.err
	r.err = nil
	return 0, err
}

func TestDdNoerrorSyncPadsErrorOnlyInputBlock(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Stdio: tool.Stdio{In: &errorOnlyReader{err: errors.New("bad block")}, Out: &out, Err: &errb},
	}
	cfg := config{ibs: 4, obs: 2, count: -1, noerror: true, sync: true, reblock: true, status: "none"}
	if code := copyDD(rc, cfg); code != 1 {
		t.Fatalf("code=%d want 1; err=%q", code, errb.String())
	}
	if want := []byte{0, 0, 0, 0}; !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("output=%v want=%v", out.Bytes(), want)
	}
	if !strings.Contains(errb.String(), "bad block") {
		t.Fatalf("missing read diagnostic: %q", errb.String())
	}
}

func TestDdNoerrorWithoutSyncOmitsErrorOnlyInputBlock(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Stdio: tool.Stdio{In: &errorOnlyReader{err: errors.New("bad block")}, Out: &out, Err: &errb},
	}
	cfg := config{ibs: 4, obs: 2, count: -1, noerror: true, reblock: true, status: "none"}
	if code := copyDD(rc, cfg); code != 1 {
		t.Fatalf("code=%d want 1; err=%q", code, errb.String())
	}
	if out.Len() != 0 {
		t.Fatalf("output=%v want empty", out.Bytes())
	}
	if !strings.Contains(errb.String(), "bad block") {
		t.Fatalf("missing read diagnostic: %q", errb.String())
	}
}

type chunkReader struct {
	data   []byte
	chunks []int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := len(p)
	if len(r.chunks) > 0 {
		n = min(n, r.chunks[0])
		r.chunks = r.chunks[1:]
	}
	n = min(n, len(r.data))
	copy(p, r.data[:n])
	r.data = r.data[n:]
	return n, nil
}

func TestReadInputBlockFullblockAccumulatesShortReads(t *testing.T) {
	r := &chunkReader{data: []byte("abcdefghij"), chunks: []int{1, 2, 1, 4, 2}}
	buf := make([]byte, 4)
	n, err := readInputBlock(r, buf, true)
	if n != 4 || err != nil || string(buf) != "abcd" {
		t.Fatalf("first block: n=%d err=%v data=%q", n, err, buf)
	}
	n, err = readInputBlock(r, buf, true)
	if n != 4 || err != nil || string(buf) != "efgh" {
		t.Fatalf("second block: n=%d err=%v data=%q", n, err, buf)
	}
	n, err = readInputBlock(r, buf, true)
	if n != 2 || err != io.EOF || string(buf[:n]) != "ij" {
		t.Fatalf("tail: n=%d err=%v data=%q", n, err, buf[:n])
	}
}

func TestDdFullblockPreservesSkipAndCount(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Stdio: tool.Stdio{
			In:  &chunkReader{data: []byte("abcdefghijkl"), chunks: []int{1, 2, 1, 3, 1, 2, 2}},
			Out: &out,
			Err: &errb,
		},
	}
	cfg := config{ibs: 4, obs: 2, skip: 1, count: 1, fullblock: true, reblock: true, status: "none"}
	if code := copyDD(rc, cfg); code != 0 {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
	if got := out.String(); got != "efgh" {
		t.Fatalf("output=%q want efgh", got)
	}
}

func TestDdSyncPadsEachShortInputRecordBeforeReblocking(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Stdio: tool.Stdio{
			In:  &chunkReader{data: []byte("abcde"), chunks: []int{2, 3}},
			Out: &out,
			Err: &errb,
		},
	}
	cfg := config{ibs: 4, obs: 3, count: -1, sync: true, reblock: true, status: "noxfer"}
	if code := copyDD(rc, cfg); code != 0 {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
	if got, want := out.Bytes(), []byte{'a', 'b', 0, 0, 'c', 'd', 'e', 0}; !bytes.Equal(got, want) {
		t.Fatalf("output=%v want=%v", got, want)
	}
	if want := "0+2 records in\n2+1 records out\n"; errb.String() != want {
		t.Fatalf("status=%q want=%q", errb.String(), want)
	}
}

func TestDdSyncPreservesFullblockAndCount(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Stdio: tool.Stdio{
			In:  &chunkReader{data: []byte("abcdef"), chunks: []int{1, 1, 2, 1, 1}},
			Out: &out,
			Err: &errb,
		},
	}
	cfg := config{
		ibs: 4, obs: 2, count: 2, fullblock: true, sync: true,
		reblock: true, status: "noxfer",
	}
	if code := copyDD(rc, cfg); code != 0 {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
	if got, want := out.Bytes(), []byte{'a', 'b', 'c', 'd', 'e', 'f', 0, 0}; !bytes.Equal(got, want) {
		t.Fatalf("output=%v want=%v", got, want)
	}
	if want := "1+1 records in\n4+0 records out\n"; errb.String() != want {
		t.Fatalf("status=%q want=%q", errb.String(), want)
	}
}

func TestDdSyncWithBsCountsPaddedOutputRecord(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), "abc", "bs=4", "conv=sync")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if got, want := []byte(out), []byte{'a', 'b', 'c', 0}; !bytes.Equal(got, want) {
		t.Fatalf("output=%v want=%v", got, want)
	}
	if want := "0+1 records in\n1+0 records out\n4 bytes copied\n"; errb != want {
		t.Fatalf("status=%q want=%q", errb, want)
	}
}

func TestDdBlockSyncUsesSpacePadding(t *testing.T) {
	out, errb, code := runTool(
		t, t.TempDir(), "012\nabcde\n",
		"ibs=5", "cbs=5", "conv=block,sync", "status=noxfer",
	)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "012  abcde" {
		t.Fatalf("output=%q want %q", out, "012  abcde")
	}
	if want := "2+0 records in\n0+1 records out\n"; errb != want {
		t.Fatalf("status=%q want=%q", errb, want)
	}

	out, errb, code = runTool(
		t, t.TempDir(), "012\nabcdefg\n",
		"ibs=5", "cbs=5", "conv=block,sync", "status=noxfer",
	)
	if code != 0 {
		t.Fatalf("partial code=%d err=%q", code, errb)
	}
	if out != "012  abcde     " {
		t.Fatalf("partial output=%q want %q", out, "012  abcde     ")
	}
	if want := "2+1 records in\n0+1 records out\n1 truncated record\n"; errb != want {
		t.Fatalf("partial status=%q want=%q", errb, want)
	}
}

func TestDdBlockAndUnblockConversions(t *testing.T) {
	out, errb, code := runTool(
		t, t.TempDir(), "a\nbb\n", "cbs=3", "conv=block", "status=none",
	)
	if code != 0 || errb != "" || out != "a  bb " {
		t.Fatalf("block: code=%d out=%q err=%q", code, out, errb)
	}
	out, errb, code = runTool(
		t, t.TempDir(), "a  bb ", "cbs=3", "conv=unblock", "status=none",
	)
	if code != 0 || errb != "" || out != "a\nbb\n" {
		t.Fatalf("unblock: code=%d out=%q err=%q", code, out, errb)
	}
	out, errb, code = runTool(
		t, t.TempDir(), "a",
		"ibs=5", "cbs=5", "conv=unblock,sync", "status=none",
	)
	if code != 0 || errb != "" || out != "a\n" {
		t.Fatalf("unblock sync: code=%d out=%q err=%q", code, out, errb)
	}
}

// POSIX: seek= preserves the skipped-over output blocks; without
// conv=notrunc the file is truncated at the seek offset, not to zero.
func TestDdSeekPreservesExistingPrefix(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("BB"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "out"), []byte("AAAAAAAA"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "if=in", "of=out", "bs=4", "seek=1", "status=none")
	if code != 0 {
		t.Fatalf("dd seek: code=%d err=%q", code, errb)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "AAAABB" {
		t.Fatalf("out=%q want AAAABB (prefix preserved, truncated at seek)", got)
	}
}

// Without seek=, the default truncation still empties an existing file.
func TestDdDefaultTruncatesExistingOutput(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("BB"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "out"), []byte("AAAAAAAA"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "if=in", "of=out", "status=none")
	if code != 0 {
		t.Fatalf("dd: code=%d err=%q", code, errb)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "BB" {
		t.Fatalf("out=%q want BB", got)
	}
}

// With ibs=/obs=, output is re-blocked into obs-sized records and
// "records out" counts those, not the input records.
func TestDdReblocksOutputRecords(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("abcdefgh"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "if=in", "of=out", "ibs=1", "obs=4")
	if code != 0 {
		t.Fatalf("dd reblock: code=%d err=%q", code, errb)
	}
	want := "8+0 records in\n2+0 records out\n8 bytes copied\n"
	if errb != want {
		t.Fatalf("status=%q want %q", errb, want)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "abcdefgh" {
		t.Fatalf("out=%q want abcdefgh", got)
	}
}

// bs= disables re-blocking: each input block is written as read, so
// records out mirrors records in.
func TestDdBsWritesRecordsAsRead(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in"), []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "", "if=in", "of=out", "bs=4")
	if code != 0 {
		t.Fatalf("dd bs: code=%d err=%q", code, errb)
	}
	want := "1+1 records in\n1+1 records out\n6 bytes copied\n"
	if errb != want {
		t.Fatalf("status=%q want %q", errb, want)
	}
}

func TestDdBsTakesPrecedenceRegardlessOfOperandOrder(t *testing.T) {
	out, errb, code := runTool(
		t, t.TempDir(), "abcd", "bs=3", "ibs=1", "obs=1", "status=noxfer",
	)
	if code != 0 {
		t.Fatalf("dd bs precedence: code=%d err=%q", code, errb)
	}
	if out != "abcd" {
		t.Fatalf("output=%q want abcd", out)
	}
	if want := "1+1 records in\n1+1 records out\n"; errb != want {
		t.Fatalf("status=%q want=%q", errb, want)
	}
}

func TestDdRejectsNegativeCounts(t *testing.T) {
	for _, operand := range []string{"count=-1", "skip=-1", "seek=-1"} {
		_, errb, code := runTool(t, t.TempDir(), "data", operand, "status=none")
		if code != 2 || !strings.Contains(errb, "invalid number") {
			t.Errorf("operand %s: code=%d err=%q", operand, code, errb)
		}
	}
}

func TestDdRejectsByteSizeOverflow(t *testing.T) {
	_, errb, code := runTool(t, t.TempDir(), "", "bs=9223372036854775807K", "status=none")
	if code != 2 || !strings.Contains(errb, "invalid number") {
		t.Fatalf("overflow: code=%d err=%q", code, errb)
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestDdDetectsShortWrites(t *testing.T) {
	if err := writeAll(shortWriter{}, []byte("abc")); err != io.ErrShortWrite {
		t.Fatalf("writeAll error=%v want %v", err, io.ErrShortWrite)
	}
}
