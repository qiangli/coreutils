//go:build windows

package loggercmd

import (
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// Windows has no system log, so dialSystemLog's own refusal must name that,
// independent of anything run() does with it.
func TestDialSystemLogRefusesOnWindows(t *testing.T) {
	_, err := dialSystemLog(&tool.RunContext{}, defaultPriority, "tag")
	if err == nil {
		t.Fatal("dialSystemLog must fail on Windows: there is no system log to connect to")
	}
	if !strings.Contains(err.Error(), "Windows") {
		t.Errorf("error = %q, want it to name Windows", err.Error())
	}
}

// End to end, through the real (unfaked) openSink: a message operand on
// Windows must fail loudly rather than silently report success for a log
// that never received anything.
func TestLoggerCommandRefusesToClaimDeliveryOnWindows(t *testing.T) {
	_, errOut, code := exec(t, testEnv, "", "hello")
	if code == 0 {
		t.Fatal("logging on Windows must not exit 0: no system log exists to have received the message")
	}
	if !strings.Contains(errOut, "Windows") {
		t.Errorf("stderr = %q, want it to name Windows", errOut)
	}
}

// --help/--version must still work on Windows without needing the transport
// (which never opens there at all) — mirrors
// TestHelpAndVersionSucceedWithoutOpeningTheTransport but through the real
// seam instead of an installed fake, so it also proves dialSystemLog is
// never even called on this path.
func TestLoggerHelpAndVersionSucceedOnWindowsWithoutTheSystemLog(t *testing.T) {
	for _, arg := range []string{"--help", "--version"} {
		t.Run(arg, func(t *testing.T) {
			out, errOut, code := exec(t, testEnv, "", arg)
			if code != 0 {
				t.Fatalf("%s exit %d, stderr %q", arg, code, errOut)
			}
			if !strings.Contains(out, "logger") {
				t.Errorf("%s output = %q, want it to name the command", arg, out)
			}
		})
	}
}
