package ask

import (
	"fmt"
	"strings"
)

// renderFrame builds the trustworthy chrome shown above every prompt.
//
// The threat this answers cannot be fully solved, and it is worth being precise
// about what is and is not claimed. A compromised or prompt-injected agent can
// always ASK for the wrong thing — it could run `bashy ask --prompt "Enter your
// bank password"`, or skip bashy entirely and print a convincing fake prompt of
// its own. Nothing here prevents that.
//
// What the frame DOES guarantee is that whenever the real mechanism is used, the
// human has uncorrupted information about three things the requester cannot forge:
// who is asking, from where, and WHERE THE ANSWER WILL GO.
//
// The sink line is the one that earns its place. Every field above it is context;
// the destination is the thing that distinguishes "this program wants a token so
// it can log in" from "this program wants to print your token back into a
// transcript". An attack that harvests a credential has to declare its harvesting
// in a line the human reads before typing — which turns an invisible attack into a
// visible one. That is the same reason sudo announces the command it is about to
// run rather than just asking for a password.
//
// Every value here is observed by bashy about itself (pid, cwd, argv, detected
// harness); none of it comes from a flag. The only caller-supplied text is the
// prompt, which arrives sanitized and sits INSIDE the frame under an explicit
// untrusted-content banner — never above it, never as part of the chrome.
func renderFrame(r Request) string {
	var b strings.Builder

	line := func(label, value string) {
		if value == "" {
			return
		}
		fmt.Fprintf(&b, "│ %-14s %s\n", label, value)
	}

	b.WriteString("┌ bashy ask — a program is requesting a value from YOU\n")
	line("requested by", requesterLabel(r.Requester))
	line("principal", r.Requester.Principal)
	line("working dir", r.Requester.Cwd)
	line("command line", strings.Join(r.Requester.Argv, " "))
	if r.Name != "" {
		line("label", r.Name)
	}
	// Upper case, and phrased as a destination rather than a setting, because this
	// is the line a hurried human must not skim past.
	line("THE VALUE GOES", r.Sink.Describe())

	b.WriteString("├ message from the requesting program — UNTRUSTED, may be attacker-controlled:\n")
	prompt := r.Prompt
	if prompt == "" {
		prompt = "(no message supplied)"
	}
	fmt.Fprintf(&b, "│   %s\n", prompt)
	b.WriteString("└")
	return b.String()
}

// requesterLabel names the asking process as helpfully as the environment allows.
func requesterLabel(rq Requester) string {
	who := rq.Tool
	if who == "" {
		who = "an unidentified program"
	}
	return fmt.Sprintf("%s (pid %d, parent pid %d)", who, rq.PID, rq.PPID)
}

// promptLine is the short label on the input line itself, after the frame.
func promptLine(r Request) string {
	what := r.Name
	if what == "" {
		what = "Value"
	}
	if r.Secret {
		return fmt.Sprintf(" %s (input hidden): ", what)
	}
	return fmt.Sprintf(" %s: ", what)
}
