package mailxcmd

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mailxpkg "github.com/qiangli/coreutils/pkg/mailx"
	"github.com/qiangli/coreutils/tool"
)

func runMailxEnv(t *testing.T, dir, input string, env []string, args ...string) (string, string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Dir: dir, Env: env, FS: tool.NewLocalFS(), Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &errOut}}
	code := run(rc, args)
	return out.String(), errOut.String(), code
}

func TestPOSIXStartupAliasesAndNoSystemRC(t *testing.T) {
	d := t.TempDir()
	system := filepath.Join(d, "system.mailrc")
	user := filepath.Join(d, "user.mailrc")
	if err := os.WriteFile(system, []byte("alias staff bob\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(user, []byte("alias friends carol\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := []string{"LOGNAME=alice", "HOME=" + d, "MAILX_SPOOL=" + filepath.Join(d, "spool"), "MAILX_SYSTEM_RC=" + system, "MAILRC=" + user}
	_, stderr, code := runMailxEnv(t, d, "system\n", env, "staff")
	if code != 0 {
		t.Fatalf("system alias: code=%d stderr=%q", code, stderr)
	}
	if es, e := mailxpkg.ReadMbox(filepath.Join(d, "spool", "bob")); e != nil || len(es) != 1 {
		t.Fatalf("bob: %d %v", len(es), e)
	}
	_, stderr, code = runMailxEnv(t, d, "user\n", env, "friends")
	if code != 0 {
		t.Fatalf("user alias: code=%d stderr=%q", code, stderr)
	}
	if es, e := mailxpkg.ReadMbox(filepath.Join(d, "spool", "carol")); e != nil || len(es) != 1 {
		t.Fatalf("carol: %d %v", len(es), e)
	}
	_, stderr, code = runMailxEnv(t, d, "literal\n", env, "-n", "staff")
	if code != 0 {
		t.Fatalf("-n: code=%d stderr=%q", code, stderr)
	}
	if es, e := mailxpkg.ReadMbox(filepath.Join(d, "spool", "staff")); e != nil || len(es) != 1 {
		t.Fatalf("literal staff: %d %v", len(es), e)
	}
}

func TestPOSIXReadDispositionMovesToMBOXAndAgesNewMail(t *testing.T) {
	d := t.TempDir()
	mailbox := seedMailbox(t, d, "read-me", "leave-me")
	mbox := filepath.Join(d, "mbox")
	env := []string{"LOGNAME=alice", "HOME=" + d, "MAIL=" + mailbox, "MBOX=" + mbox}
	_, stderr, code := runMailxEnv(t, d, "p 1\nq\n", env, "-N", "-n")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	system, e := mailxpkg.ReadMbox(mailbox)
	if e != nil || len(system) != 1 {
		t.Fatalf("system=%d err=%v", len(system), e)
	}
	if got := system[0].Message.HeaderValues("Status"); len(got) != 1 || got[0] != "O" {
		t.Fatalf("retained Status=%#v", got)
	}
	saved, e := mailxpkg.ReadMbox(mbox)
	if e != nil || len(saved) != 1 {
		t.Fatalf("mbox=%d err=%v", len(saved), e)
	}
	if got := firstHeader(saved[0].Message, "Subject", ""); got != "read-me" {
		t.Fatalf("saved subject=%q", got)
	}
}

func TestPOSIXMessageSelectorsAndStateCommands(t *testing.T) {
	d := t.TempDir()
	mailbox := seedMailbox(t, d, "alpha", "beta", "gamma")
	save := filepath.Join(d, "selected")
	env := []string{"LOGNAME=alice", "HOME=" + d, "MAIL=" + mailbox}
	input := "s /beta " + save + "\nd 1-2\nu :d\nh ^\nq\n"
	_, stderr, code := runMailxEnv(t, d, input, env, "-N", "-n")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	es, e := mailxpkg.ReadMbox(save)
	if e != nil || len(es) != 1 {
		t.Fatalf("saved=%d err=%v stderr=%q", len(es), e, stderr)
	}
	if got := firstHeader(es[0].Message, "Subject", ""); got != "beta" {
		t.Fatalf("subject=%q", got)
	}
}

func TestPOSIXComposeEscapesUpdateEnvelopeAndBody(t *testing.T) {
	d := t.TempDir()
	s := &mailSession{rc: &tool.RunContext{Dir: d, Env: []string{"LOGNAME=alice", "HOME=" + d, "MAILX_SPOOL=" + filepath.Join(d, "spool")}, FS: tool.NewLocalFS(), Stdio: tool.Stdio{In: strings.NewReader("line\n~s changed\n~c carol\n~t bob\n~.\n"), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}}, invoked: cmd, user: "alice", vars: defaultVariables(&tool.RunContext{Env: []string{"HOME=" + d}}), aliases: map[string][]string{}, ignore: map[string]bool{}, retain: map[string]bool{}, alts: map[string]bool{}, reader: nil}
	s.reader = bufio.NewReader(s.rc.In)
	body, to, cc, _, subject, send, err := s.readComposition(nil, nil, nil, "old")
	if err != nil || !send || string(body) != "line\n" || subject != "changed" || strings.Join(to, ",") != "bob" || strings.Join(cc, ",") != "carol" {
		t.Fatalf("body=%q to=%v cc=%v subject=%q send=%v err=%v", body, to, cc, subject, send, err)
	}
}

func TestCommitMboxChangesPreservesConcurrentAppend(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "box")
	for _, sub := range []string{"one", "two"} {
		m := &mailxpkg.Message{Headers: []mailxpkg.Header{{Name: "From", Value: "bob"}, {Name: "Subject", Value: sub}}, Body: []byte("body\n")}
		if e := mailxpkg.AppendMbox(path, "bob", time.Unix(0, 0), m); e != nil {
			t.Fatal(e)
		}
	}
	snap, e := mailxpkg.ReadMbox(path)
	if e != nil {
		t.Fatal(e)
	}
	updated := cloneMboxEntries(snap)
	setMessageStatus(updated[0].Message, "O")
	late := &mailxpkg.Message{Headers: []mailxpkg.Header{{Name: "From", Value: "carol"}, {Name: "Subject", Value: "late"}}, Body: []byte("body\n")}
	if e = mailxpkg.AppendMbox(path, "carol", time.Unix(1, 0), late); e != nil {
		t.Fatal(e)
	}
	if e = mailxpkg.CommitMboxChanges(path, snap, updated, []bool{true, false}); e != nil {
		t.Fatal(e)
	}
	got, e := mailxpkg.ReadMbox(path)
	if e != nil || len(got) != 2 {
		t.Fatalf("got=%d err=%v", len(got), e)
	}
	if firstHeader(got[1].Message, "Subject", "") != "late" {
		t.Fatalf("late append lost")
	}
}

func TestPOSIXMailboxSpecialNamesAndMBOXPrepend(t *testing.T) {
	d := t.TempDir()
	system := seedMailbox(t, d, "fresh")
	mbox := filepath.Join(d, "mbox")
	old := &mailxpkg.Message{Headers: []mailxpkg.Header{{Name: "From", Value: "old"}, {Name: "Subject", Value: "old"}}, Body: []byte("old\n")}
	if err := mailxpkg.AppendMbox(mbox, "old", time.Unix(0, 0), old); err != nil {
		t.Fatal(err)
	}
	env := []string{"LOGNAME=alice", "HOME=" + d, "MAIL=" + system, "MBOX=" + mbox}
	_, stderr, code := runMailxEnv(t, d, "p\nq\n", env, "-N", "-n")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	es, err := mailxpkg.ReadMbox(mbox)
	if err != nil || len(es) != 2 {
		t.Fatalf("entries=%d err=%v", len(es), err)
	}
	if firstHeader(es[0].Message, "Subject", "") != "fresh" {
		t.Fatalf("noappend did not prepend")
	}
	s := &mailSession{rc: &tool.RunContext{Dir: d, Env: env}, vars: defaultVariables(&tool.RunContext{Env: env}), user: "alice", previous: filepath.Join(d, "previous")}
	if got, _ := s.mailboxName("%"); got != system {
		t.Fatalf("%%=%q want %q", got, system)
	}
	if got, _ := s.mailboxName("&"); got != mbox {
		t.Fatalf("&=%q want %q", got, mbox)
	}
	if got, _ := s.mailboxName("#"); got != s.previous {
		t.Fatalf("#=%q", got)
	}
}

func TestPOSIXCompositionInterruptAndIgnore(t *testing.T) {
	d := t.TempDir()
	out := &bytes.Buffer{}
	rc := &tool.RunContext{Dir: d, Env: []string{"HOME=" + d}, Stdio: tool.Stdio{Out: out, Err: &bytes.Buffer{}}}
	input := make(chan composeRead)
	signals := make(chan os.Signal)
	s := &mailSession{rc: rc, invoked: cmd, user: "alice", vars: defaultVariables(rc), input: input, interrupt: signals, stopInterrupt: func() {}, receive: true}
	type result struct {
		send bool
		err  error
	}
	done := make(chan result, 1)
	go func() { _, _, _, _, _, send, err := s.readComposition(nil, nil, nil, ""); done <- result{send, err} }()
	input <- composeRead{line: "partial\n"}
	signals <- os.Interrupt
	signals <- os.Interrupt
	r := <-done
	if r.send || r.err != nil {
		t.Fatalf("send=%v err=%v", r.send, r.err)
	}
	dead, err := os.ReadFile(filepath.Join(d, "dead.letter"))
	if err != nil || string(dead) != "partial\n" {
		t.Fatalf("dead=%q err=%v", dead, err)
	}

	out.Reset()
	input = make(chan composeRead)
	signals = make(chan os.Signal)
	s = &mailSession{rc: rc, invoked: cmd, user: "alice", vars: defaultVariables(rc), input: input, interrupt: signals, stopInterrupt: func() {}, receive: true}
	s.vars["ignore"] = ""
	done = make(chan result, 1)
	go func() {
		body, _, _, _, _, send, err := s.readComposition(nil, nil, nil, "")
		if string(body) != "" {
			err = fmt.Errorf("body=%q", body)
		}
		done <- result{send, err}
	}()
	signals <- os.Interrupt
	input <- composeRead{line: "discarded\n"}
	input <- composeRead{line: "~.\n"}
	r = <-done
	if !r.send || r.err != nil {
		t.Fatalf("ignore send=%v err=%v", r.send, r.err)
	}
	if !strings.Contains(out.String(), "@\n") {
		t.Fatalf("ignore marker=%q", out.String())
	}

	out.Reset()
	input = make(chan composeRead)
	signals = make(chan os.Signal)
	s = &mailSession{rc: rc, invoked: cmd, user: "alice", vars: defaultVariables(rc), input: input, interrupt: signals, stopInterrupt: func() {}}
	type promptResult struct {
		abort bool
		err   error
	}
	promptDone := make(chan promptResult, 1)
	go func() {
		_, abort, err := s.promptField("Subject", "", nil)
		promptDone <- promptResult{abort, err}
	}()
	signals <- os.Interrupt
	signals <- os.Interrupt
	pr := <-promptDone
	if !pr.abort || pr.err != nil {
		t.Fatalf("prompt abort=%v err=%v", pr.abort, pr.err)
	}
}

func TestPOSIXCommandMinimumAbbreviations(t *testing.T) {
	want := map[string]string{"re": "reply", "res": "reply", "ret": "retain", "fol": "followup", "fold": "file", "sh": "shell", "si": "size", "so": "source"}
	for input, expected := range want {
		got, err := mailCommand(input)
		if err != nil || got != expected {
			t.Errorf("mailCommand(%q)=%q,%v want %q", input, got, err, expected)
		}
	}
}

func TestPOSIXClusteredFileOptions(t *testing.T) {
	d := t.TempDir()
	mailbox := seedMailbox(t, d, "clustered")
	env := []string{"LOGNAME=alice", "HOME=" + d, "MAILRC=/dev/null"}
	_, stderr, code := runMailxEnv(t, d, "x\n", env, "-fin", mailbox)
	if code != 0 {
		t.Fatalf("-fin code=%d stderr=%q", code, stderr)
	}
}

func TestPOSIXKeepAndNoKeepEmptyMailboxDisposition(t *testing.T) {
	for _, keep := range []bool{false, true} {
		d := t.TempDir()
		mailbox := seedMailbox(t, d, "only")
		env := []string{"LOGNAME=alice", "HOME=" + d, "MAIL=" + mailbox}
		input := "d\nq\n"
		if keep {
			input = "set keep\nd\nq\n"
		}
		_, stderr, code := runMailxEnv(t, d, input, env, "-N", "-n")
		if code != 0 {
			t.Fatalf("keep=%v code=%d stderr=%q", keep, code, stderr)
		}
		info, err := os.Stat(mailbox)
		if keep {
			if err != nil || info.Size() != 0 {
				t.Fatalf("keep mailbox: info=%v err=%v", info, err)
			}
		} else if !os.IsNotExist(err) {
			t.Fatalf("nokeep mailbox still exists: %v", err)
		}
	}
}

func TestPOSIXPreserveSurvivesReadAndAgesStatus(t *testing.T) {
	d := t.TempDir()
	mailbox := seedMailbox(t, d, "keep-me")
	env := []string{"LOGNAME=alice", "HOME=" + d, "MAIL=" + mailbox}
	_, stderr, code := runMailxEnv(t, d, "hold\np\nq\n", env, "-N", "-n")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	entries, err := mailxpkg.ReadMbox(mailbox)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries=%d err=%v", len(entries), err)
	}
	if status := entries[0].Message.HeaderValues("Status"); len(status) != 1 || status[0] != "RO" {
		t.Fatalf("Status=%v", status)
	}
}

func TestPOSIXPromptFolderAndDisabledEscape(t *testing.T) {
	d := t.TempDir()
	mailbox := seedMailbox(t, d, "one")
	env := []string{"LOGNAME=alice", "HOME=" + d, "MAIL=" + mailbox}
	out, stderr, code := runMailxEnv(t, d, "set prompt=X>\nx\n", env, "-N", "-n")
	if code != 0 || !strings.Contains(out, "X>") {
		t.Fatalf("prompt: code=%d out=%q stderr=%q", code, out, stderr)
	}
	s := &mailSession{rc: &tool.RunContext{Dir: d, Env: env}, vars: defaultVariables(&tool.RunContext{Env: env}), cwd: d}
	if got := s.resolve("+literal"); got != filepath.Join(d, "+literal") {
		t.Fatalf("unset folder expansion=%q", got)
	}
	s.vars["folder"] = "saved"
	if got := s.resolve("+box"); got != filepath.Join(d, "saved", "box") {
		t.Fatalf("folder expansion=%q", got)
	}
	words, splitErr := splitMailWords(`file "+literal"`)
	if splitErr != nil || len(words) != 2 || !words[1].protectedLeading {
		t.Fatalf("quoted leading plus metadata=%v err=%v", words, splitErr)
	}
	s.vars["escape"] = ""
	s.reader = bufio.NewReader(strings.NewReader("~.\n"))
	s.rc.Stdio.In = s.reader
	body, _, _, _, _, send, err := s.readComposition(nil, nil, nil, "")
	if err != nil || !send || string(body) != "~.\n" {
		t.Fatalf("disabled escape body=%q send=%v err=%v", body, send, err)
	}
}
