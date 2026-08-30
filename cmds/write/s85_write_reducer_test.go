package writecmd

// Sprint 85 write-kill-nohup disposition reducer — identity write:22.
//
// This file is the hermetic, suite-free reducer for the s85-write-kill-nohup
// probe contract (see docs/posix-interface-audits/
// sprint-85-write-kill-nohup-disposition.md). The identity sits on the
// terminal/login-session/mesg authority boundary: whether a sender may reach
// a recipient is decided by the login-accounting record, the liveness of the
// session it names, the existence of the device, and the device's mesg(1)
// group-write bit — narrowed by an explicit terminal operand, with the
// superuser exempt.
//
// The reducer replays that ladder end to end through the public run() entry
// with BYTE-EXACT transcripts on every sink: recipient terminal, sender's
// controlling terminal, stdout, stderr, and exit status. Byte-exactness is
// the point — a transcript, not a substring, is the artifact two arms of a
// matched control run would have to produce identically for attribution.
//
// Nothing here touches the host: no /var/run database, no os/user, no real
// terminal, no signal to a real process. Every OS fact arrives through the
// package seams with synthetic recipients and session records, and the first
// test asserts that contract instead of merely promising it. The licensed
// suite was not consulted; the expectations are derived from the public
// POSIX.1 Issue 7 write(1) page and the documented selection rule.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// bannerAt is the exact banner write emits under the fixture's frozen clock:
// nowFn is epoch+90m = 2026-08-22 10:30:00 UTC, rendered in time.ANSIC.
const bannerAt = "Message from %s (%s) [Sat Aug 22 10:30:00 2026]...\n"

// TestS85ReducerNeverTouchesTheHost encodes the probe contract's safety rule
// as an executable assertion: after the fixture is installed, the login
// database is not the host's, the device directory is not /dev, and every
// "terminal" the reducer can write to is a plain regular file. If this test
// fails, the reducer has become invasive and every other test in the file is
// untrustworthy.
func TestS85ReducerNeverTouchesTheHost(t *testing.T) {
	w := install(t, fixture{
		uid: 1000, myTTY: "pts/1",
		logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}},
	})
	if dbPath == defaultUtmpPath {
		t.Fatalf("reducer must not read the host login database %q", dbPath)
	}
	if !strings.Contains(filepath.ToSlash(dbPath), "/utmp") {
		t.Fatalf("dbPath %q is not the fixture's synthetic database", dbPath)
	}
	if devDir == "/dev" || devDir == "" {
		t.Fatalf("reducer must not resolve devices under the host /dev, got %q", devDir)
	}
	for line, path := range w.devs {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat fake terminal %s: %v", line, err)
		}
		if !fi.Mode().IsRegular() {
			t.Fatalf("fake terminal %s must be a regular file, got mode %v", line, fi.Mode())
		}
	}
}

// TestS85Write22AuthorityBoundaryReducer walks the whole authority ladder.
// Each rung fixes the synthetic world, runs `write` once, and compares every
// observable byte. A rung that cannot be made green here without changing
// product behavior is the causal signature of a product red; as of this
// sprint every rung is green, which is recorded (with its limits) in the
// disposition doc.
func TestS85Write22AuthorityBoundaryReducer(t *testing.T) {
	// wantTTY describes one rung's expected observables. Zero-valued fields
	// mean "empty"; recipients/alerts list per-terminal transcript checks.
	type wantTTY struct {
		line       string
		transcript string // exact bytes that must have arrived, "" for none
	}
	cases := []struct {
		name    string
		fix     fixture
		args    []string
		deadSes bool // force sessionActiveFn to report the recorded PID dead
		stdout  string
		stderr  string
		exit    int
		ttys    []wantTTY
		alerts  string // exact bytes on the sender's controlling terminal
	}{
		{
			name: "account exists but has no session record",
			fix: fixture{uid: 1000, myTTY: "pts/1",
				logins: []login{{user: "carol", line: "pts/2", mode: writable, when: epoch}}},
			args:   []string{"bob"},
			stderr: "write: bob is not logged in\n",
			exit:   1,
			ttys:   []wantTTY{{"pts/2", ""}},
		},
		{
			name: "account does not exist",
			fix: fixture{uid: 1000, myTTY: "pts/1", unknown: true,
				logins: []login{{user: "bob", line: "pts/2", mode: writable, when: epoch}}},
			args:   []string{"nosuch"},
			stderr: "write: nosuch: no such user\n",
			exit:   1,
		},
		{
			name: "DEAD_PROCESS record is not a login",
			fix: fixture{uid: 1000, myTTY: "pts/1",
				logins: []login{{user: "carol", line: "pts/2", mode: writable, when: epoch}},
				dead:   []login{{user: "bob", line: "pts/3", when: epoch}}},
			args:   []string{"bob"},
			stderr: "write: bob is not logged in\n",
			exit:   1,
		},
		{
			name: "USER_PROCESS record whose session is gone",
			fix: fixture{uid: 1000, myTTY: "pts/1",
				logins: []login{{user: "bob", line: "pts/2", mode: writable, when: epoch}}},
			args:    []string{"bob"},
			deadSes: true,
			stderr:  "write: bob: no accessible terminal\n",
			exit:    1,
			ttys:    []wantTTY{{"pts/2", ""}},
		},
		{
			name: "mesg n on the only line denies the sender",
			fix: fixture{uid: 1000, myTTY: "pts/1",
				logins: []login{{user: "bob", line: "pts/9", mode: denied, when: epoch}}},
			args:   []string{"bob"},
			stderr: "write: bob has messages disabled on pts/9\n",
			exit:   1,
			ttys:   []wantTTY{{"pts/9", ""}},
			alerts: "",
		},
		{
			name: "terminal operand naming the denied line denies the sender",
			fix: fixture{uid: 1000, myTTY: "pts/1",
				logins: []login{
					{user: "bob", line: "pts/8", mode: writable, when: epoch},
					{user: "bob", line: "pts/9", mode: denied, when: epoch.Add(2 * time.Hour)},
				}},
			args:   []string{"bob", "pts/9"},
			stderr: "write: bob has messages disabled on pts/9\n",
			exit:   1,
			ttys:   []wantTTY{{"pts/8", ""}, {"pts/9", ""}},
		},
		{
			name: "mesg y single line delivers banner body EOT and alerts once",
			fix: fixture{uid: 1000, myTTY: "pts/1",
				logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}},
			args:   []string{"bob"},
			exit:   0,
			ttys:   []wantTTY{{"pts/9", "Message from alice (pts/1) [Sat Aug 22 10:30:00 2026]...\nhi\nEOT\n"}},
			alerts: "\a\a",
		},
		{
			name: "two live lines select the most recent and announce on stdout",
			fix: fixture{uid: 1000, myTTY: "pts/1",
				logins: []login{
					{user: "bob", line: "pts/2", mode: writable, when: epoch},
					{user: "bob", line: "pts/7", mode: writable, when: epoch.Add(2 * time.Hour)},
				}},
			args:   []string{"bob"},
			stdout: "write: bob is logged in on more than one line; using pts/7\n",
			exit:   0,
			ttys: []wantTTY{
				{"pts/7", "Message from alice (pts/1) [Sat Aug 22 10:30:00 2026]...\nhi\nEOT\n"},
				{"pts/2", ""},
			},
			alerts: "\a\a",
		},
		{
			name: "terminal operand naming the writable line suppresses the notice",
			fix: fixture{uid: 1000, myTTY: "pts/1",
				logins: []login{
					{user: "bob", line: "pts/2", mode: writable, when: epoch},
					{user: "bob", line: "pts/9", mode: denied, when: epoch.Add(2 * time.Hour)},
				}},
			args: []string{"bob", "pts/2"},
			exit: 0,
			ttys: []wantTTY{
				{"pts/2", "Message from alice (pts/1) [Sat Aug 22 10:30:00 2026]...\nhi\nEOT\n"},
				{"pts/9", ""},
			},
			alerts: "\a\a",
		},
		{
			name: "the superuser is exempt from the message bit",
			fix: fixture{sender: "root", uid: 0, myTTY: "pts/1",
				logins: []login{{user: "bob", line: "pts/9", mode: denied, when: epoch}}},
			args:   []string{"bob"},
			exit:   0,
			ttys:   []wantTTY{{"pts/9", "Message from root (pts/1) [Sat Aug 22 10:30:00 2026]...\nhi\nEOT\n"}},
			alerts: "\a\a",
		},
		{
			name: "no controlling terminal still delivers with the honest banner",
			fix: fixture{sender: "alice", uid: 1000,
				logins: []login{{user: "bob", line: "pts/9", mode: writable, when: epoch}}},
			args:   []string{"bob"},
			exit:   0,
			ttys:   []wantTTY{{"pts/9", "Message from alice (?) [Sat Aug 22 10:30:00 2026]...\nhi\nEOT\n"}},
			alerts: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var alerts bytes.Buffer
			tc.fix.controlW = &alerts
			w := install(t, tc.fix)
			if tc.deadSes {
				old := sessionActiveFn
				sessionActiveFn = func(int) bool { return false }
				t.Cleanup(func() { sessionActiveFn = old })
			}
			stdout, stderr, code := exec(t, "hi\n", tc.args...)
			if code != tc.exit {
				t.Errorf("exit = %d, want %d", code, tc.exit)
			}
			if stdout != tc.stdout {
				t.Errorf("stdout = %q, want %q", stdout, tc.stdout)
			}
			if stderr != tc.stderr {
				t.Errorf("stderr = %q, want %q", stderr, tc.stderr)
			}
			if got := alerts.String(); got != tc.alerts {
				t.Errorf("sender controlling terminal = %q, want %q", got, tc.alerts)
			}
			for _, want := range tc.ttys {
				if got := w.read(t, want.line); got != want.transcript {
					t.Errorf("terminal %s = %q, want %q", want.line, got, want.transcript)
				}
			}
		})
	}
}
