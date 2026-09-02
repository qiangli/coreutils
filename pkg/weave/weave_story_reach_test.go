package weave

import (
	"strings"
	"testing"
	"time"
)

// A reminder is silent when there is nothing waiting. A surface that speaks up
// with no news is one an agent learns to ignore, which would cost exactly the
// message it exists to protect.
func TestUnreadReminderIsSilentWithNoMail(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if got := sprintUnreadReminder("nobody-home"); got != "" {
		t.Fatalf("reminder spoke with no unread mail: %q", got)
	}
}

// When it does speak it must carry the runnable command, not just a count.
// "You have mail" leaves the agent to work out how to read it; the point of a
// reminder at a busy moment is that acting on it costs nothing.
func TestUnreadReminderCarriesTheCommand(t *testing.T) {
	got := formatUnreadReminder(3, 12*time.Minute, "alice")
	for _, want := range []string{"3 unread", "12m", "bashy inbox --as alice"} {
		if !strings.Contains(got, want) {
			t.Errorf("reminder omits %q: %q", want, got)
		}
	}
}
