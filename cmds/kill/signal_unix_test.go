//go:build unix

package killcmd

import (
	"strconv"
	"strings"
	"testing"
)

func TestKillTranslatesConfiguredUrgSignal(t *testing.T) {
	urg := signalNumber(signalByName("URG"))
	if urg < 0 {
		t.Fatal("URG is not present in the native signal table")
	}
	for _, spelling := range []string{"URG", "urg", "urG", "SIGURG"} {
		code, out, errOut := invoke("-l", spelling)
		if code != 0 || strings.TrimSpace(out) != strconv.Itoa(urg) || errOut != "" {
			t.Fatalf("kill -l %s: code=%d out=%q err=%q", spelling, code, out, errOut)
		}
	}
	code, out, errOut := invoke("-l", strconv.Itoa(128+urg))
	if code != 0 || out != "URG\n" || errOut != "" {
		t.Fatalf("kill -l URG exit status: code=%d out=%q err=%q", code, out, errOut)
	}
}
