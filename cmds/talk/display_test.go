package talkcmd

import (
	"bytes"
	"strings"
	"testing"
)

func testCaps(out *bytes.Buffer) terminalCaps {
	return terminalCaps{
		clear: "<clear>", cup: "<%p1%d,%p2%d>", el: "<el>", bel: "<bell>",
		rows: 10, cols: 40,
	}
}

func testDisplay(out *bytes.Buffer) *screenDisplay {
	return &screenDisplay{
		out: out, caps: testCaps(out), localLabel: "alice", peerLabel: "bob",
		controls: controlChars{erase: 0x7f, kill: 0x15, intr: 0x03, eof: 0x04},
		ctype:    "C.UTF-8",
	}
}

func TestTalkCharacterEventsKeepIndependentRegionsInSync(t *testing.T) {
	var senderOut, recipientOut bytes.Buffer
	sender := testDisplay(&senderOut)
	recipient := testDisplay(&recipientOut)
	wires, terminate, err := sender.Local([]byte("abc\x7fd\x15hello\n"), true)
	if err != nil || terminate {
		t.Fatalf("local: wires=%q terminate=%v err=%v", wires, terminate, err)
	}
	for _, wire := range wires {
		if err := recipient.Remote(wire); err != nil {
			t.Fatal(err)
		}
	}
	if got := string(sender.local.text); got != "hello\n" {
		t.Fatalf("sender region=%q", got)
	}
	if got := string(recipient.remote.text); got != "hello\n" {
		t.Fatalf("recipient region=%q", got)
	}
	if !strings.Contains(senderOut.String(), "talk: you (alice)") || !strings.Contains(senderOut.String(), "talk: bob") {
		t.Fatalf("screen did not render separate labelled regions: %q", senderOut.String())
	}
}

func TestTalkAlertRefreshAndConfiguredTerminationCharacters(t *testing.T) {
	var senderOut, recipientOut bytes.Buffer
	sender := testDisplay(&senderOut)
	recipient := testDisplay(&recipientOut)
	wires, terminate, err := sender.Local([]byte{'x', '\f', '\a'}, true)
	if err != nil || terminate {
		t.Fatalf("local: terminate=%v err=%v", terminate, err)
	}
	if len(wires) != 2 { // text and alert; control-L is local-only
		t.Fatalf("wire events=%q", wires)
	}
	for _, wire := range wires {
		if err := recipient.Remote(wire); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(recipientOut.String(), "<bell>") {
		t.Fatalf("alert did not reach recipient terminal: %q", recipientOut.String())
	}
	if _, terminate, err = sender.Local([]byte{sender.controls.intr}, true); err != nil || !terminate {
		t.Fatalf("interrupt: terminate=%v err=%v", terminate, err)
	}
	if _, terminate, err = sender.Local([]byte{sender.controls.eof}, true); err != nil || !terminate {
		t.Fatalf("eof: terminate=%v err=%v", terminate, err)
	}
	wires, terminate, err = sender.Local([]byte{'z', sender.controls.intr}, true)
	if err != nil || !terminate || len(wires) != 1 || !strings.Contains(wires[0], `"text":"z"`) {
		t.Fatalf("text-before-interrupt wires=%q terminate=%v err=%v", wires, terminate, err)
	}
}

func TestTalkPeerCloseAllowsOnlyLocalExit(t *testing.T) {
	var out bytes.Buffer
	d := testDisplay(&out)
	if err := d.PeerClosed("bob"); err != nil {
		t.Fatal(err)
	}
	wires, terminate, err := d.Local([]byte("must not be sent"), false)
	if err != nil || terminate || len(wires) != 0 || len(d.local.text) != 0 {
		t.Fatalf("post-close local=%q wires=%q terminate=%v err=%v", string(d.local.text), wires, terminate, err)
	}
	_, terminate, err = d.Local([]byte{d.controls.eof}, false)
	if err != nil || !terminate {
		t.Fatalf("post-close EOF terminate=%v err=%v", terminate, err)
	}
	if !strings.Contains(string(d.remote.text), "has terminated the session") {
		t.Fatalf("peer notification=%q", string(d.remote.text))
	}
}

func TestTalkUTF8SplitAcrossTerminalReads(t *testing.T) {
	var out bytes.Buffer
	d := testDisplay(&out)
	encoded := []byte("é")
	wires, terminate, err := d.Local(encoded[:1], true)
	if err != nil || terminate || len(wires) != 0 {
		t.Fatalf("first byte wires=%q terminate=%v err=%v", wires, terminate, err)
	}
	wires, terminate, err = d.Local(encoded[1:], true)
	if err != nil || terminate || len(wires) != 1 || string(d.local.text) != "é" {
		t.Fatalf("second byte region=%q wires=%q terminate=%v err=%v", string(d.local.text), wires, terminate, err)
	}
	wires, terminate, err = d.Local([]byte{0xc3, d.controls.eof}, true)
	if err != nil || !terminate || len(wires) != 1 || !strings.Contains(string(d.local.text), `\xC3`) {
		t.Fatalf("incomplete-before-EOF region=%q wires=%q terminate=%v err=%v", string(d.local.text), wires, terminate, err)
	}
}

func TestTalkScreenWrapUsesCharacterColumnWidth(t *testing.T) {
	lines := wrappedLines([]rune("世界"), 3)
	if len(lines) != 2 || lines[0] != "世" || lines[1] != "界" {
		t.Fatalf("wide-character wrapping=%q", lines)
	}
	if got := clipRunes("a世界", 3); got != "a世" {
		t.Fatalf("wide-character clipping=%q", got)
	}
}

func TestTalkRejectsMalformedOrLegacyPeerEvents(t *testing.T) {
	var out bytes.Buffer
	d := testDisplay(&out)
	for _, wire := range []string{"legacy text", wirePrefix + `{}`, wirePrefix + `{"kind":"erase","text":"x"}`, wirePrefix + `{"kind":"text","text":"\u001b[2J"}`} {
		if err := d.Remote(wire); err == nil {
			t.Errorf("accepted %q", wire)
		}
	}
}
