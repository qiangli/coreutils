//go:build linux

package renicecmd

import (
	"testing"
	"testing/fstest"
)

func TestLinuxProcParsers(t *testing.T) {
	if got, err := linuxStatPGroup([]byte("42 (name with ) chars) S 1 700 700 0")); err != nil || got != 700 {
		t.Errorf("stat pgrp=%d err=%v, want 700", got, err)
	}
	status := []byte("Name:\tfixture\nUid:\t101\t102\t4294967294\t104\n")
	if got, err := linuxStatusSavedUID(status); err != nil || got != 4294967294 {
		t.Errorf("saved uid=%d err=%v, want 4294967294", got, err)
	}
	for _, bad := range [][]byte{[]byte("no delimiter"), []byte("1 (x) S 2")} {
		if _, err := linuxStatPGroup(bad); err == nil {
			t.Errorf("malformed stat %q accepted", bad)
		}
	}
	if _, err := linuxStatusSavedUID([]byte("Name:\tx\n")); err == nil {
		t.Error("status without Uid accepted")
	}
}

func TestLinuxMembersUsesPGroupAndSavedUID(t *testing.T) {
	proc := fstest.MapFS{
		"10/stat":   {Data: []byte("10 (one) S 1 7 7 0")},
		"10/status": {Data: []byte("Uid:\t1\t2\t3\t4\n")},
		"11/stat":   {Data: []byte("11 (two ) name) S 1 8 8 0")},
		"11/status": {Data: []byte("Uid:\t1\t2\t4\t4\n")},
		"12/stat":   {Data: []byte("12 (three) S 1 7 7 0")},
		"12/status": {Data: []byte("Uid:\t1\t2\t3\t4\n")},
	}
	got, err := linuxMembers(proc, whichPGroup, 7)
	if err != nil || len(got) != 2 || got[0] != 10 || got[1] != 12 {
		t.Fatalf("pgrp members=%v err=%v, want [10 12]", got, err)
	}
	got, err = linuxMembers(proc, whichUser, 3)
	if err != nil || len(got) != 2 || got[0] != 10 || got[1] != 12 {
		t.Fatalf("saved-uid members=%v err=%v, want [10 12]", got, err)
	}
	if _, err := linuxMembers(proc, whichUser, 99); err == nil {
		t.Error("empty selector must report ESRCH")
	}
}

func TestLinuxMembersFailsClosedOnUnreadableOrMalformedRecord(t *testing.T) {
	proc := fstest.MapFS{
		"10/stat": {Data: []byte("malformed")},
	}
	if _, err := linuxMembers(proc, whichPGroup, 7); err == nil {
		t.Error("malformed process metadata must fail the whole snapshot")
	}
}
