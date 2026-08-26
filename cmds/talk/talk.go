// Package talkcmd implements the POSIX talk(1) interface for two users logged
// in on the same host.
//
// Historical BSD talk used talkd and network sockets. POSIX deliberately does
// not require remote-host operation. This implementation was written from the
// public interface, not from BSD C source: it rejects host-qualified names,
// writes the invitation to a real logged-in terminal, and exchanges encrypted,
// authenticated datagrams through ephemeral AF_UNIX sockets. Only short-lived
// public-key endpoint metadata touches disk. No transcript is posted to the
// public message board and all session entries are removed on exit.
package talkcmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/term"

	"github.com/qiangli/coreutils/cmds/internal/session"
	corelocale "github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{Name: "talk", Synopsis: "Talk to another user on this host.", Usage: "talk address [terminal]"}

func init() { cmd.Run = run; tool.Register(cmd) }

type account struct{ Name, UID string }

type conversation interface {
	ID() string
	Join() (bool, error)
	Send(string) error
	Poll() ([]string, bool, error)
	Close() error
	Cleanup() error
}

type terminalInput interface {
	Poll(context.Context, time.Duration) (inputEvent, bool)
	Close() error
}

type inputEvent struct {
	data []byte
	err  error
}

var (
	isTerminalFn = func(stream any) bool {
		f, ok := stream.(interface{ Fd() uintptr })
		return ok && term.IsTerminal(int(f.Fd()))
	}
	readSessionsFn = func([]string) ([]session.Record, error) {
		posixEnv := []string{"POSIXLY_CORRECT=1"}
		return session.ReadEnv(session.DefaultFileForEnv(posixEnv), posixEnv)
	}
	recipientPermittedFn = defaultRecipientPermitted
	currentAccountFn     = currentOSAccount
	lookupAccountFn      = lookupOSAccount
	notifyTTYFn          = notifyTerminal
	openConversationFn   = func(root string, self, peer account) (conversation, error) {
		return openLocalConversation(root, self, peer)
	}
	newInputFn           = newTerminalInput
	fileOwnerUIDFn       = fileOwnerUID
	processAliveFn       = processAlive
	validateSharedRootFn = validateSharedRoot
	pollInterval         = 100 * time.Millisecond
	watchInterruptFn     = func() (<-chan os.Signal, func()) {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt)
		return ch, func() { signal.Stop(ch) }
	}
)

func run(rc *tool.RunContext, args []string) int {
	args = tool.AliasHelpVersion(args)
	fs := tool.NewFlags(cmd.Name)
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	switch {
	case len(operands) == 0:
		return tool.UsageError(rc, cmd, "missing address operand")
	case len(operands) > 2:
		return tool.UsageError(rc, cmd, "extra operand %q", operands[2])
	}
	address := strings.TrimSpace(operands[0])
	if err := validateLocalAddress(address); err != nil {
		return diagnostic(rc, "%v", err)
	}
	terminal := ""
	if len(operands) == 2 {
		terminal = strings.TrimSpace(operands[1])
		if err := validateTerminal(terminal); err != nil {
			return diagnostic(rc, "%v", err)
		}
	}
	if !isTerminalFn(rc.In) {
		return diagnostic(rc, "standard input is not a terminal")
	}
	if !isTerminalFn(rc.Out) {
		return diagnostic(rc, "standard output is not a terminal")
	}

	self, err := currentAccountFn()
	if err != nil {
		return diagnostic(rc, "determine local OS identity: %v", err)
	}
	peer, record, err := resolveRecipient(rc.Env, address, terminal)
	if err != nil {
		return diagnostic(rc, "%v", err)
	}
	if self.UID == peer.UID {
		return diagnostic(rc, "cannot talk to yourself (%s)", self.Name)
	}
	root := rc.Getenv("BASHY_TALK_DIR")
	if root == "" {
		root = defaultTalkRoot()
	} else {
		root = rc.Path(root)
	}
	conv, err := openConversationFn(root, self, peer)
	if err != nil {
		return diagnostic(rc, "create private local session: %v", err)
	}
	defer conv.Cleanup()
	if err := notifyTTYFn(record, peer, invitationText(self.Name, terminal)); err != nil {
		return diagnostic(rc, "notify %s on %s: %v", peer.Name, record.TTY, err)
	}
	if _, err := fmt.Fprintf(rc.Out, "[talk with %s; local private session pending]\n", peer.Name); err != nil {
		return diagnostic(rc, "write terminal: %v", err)
	}
	if _, err := fmt.Fprintf(rc.Out, "waiting for %s to respond with: talk %s\n", peer.Name, self.Name); err != nil {
		return diagnostic(rc, "write terminal: %v", err)
	}

	ctx := rc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	interrupt, stopInterrupt := watchInterruptFn()
	defer stopInterrupt()
	interval := effectivePollInterval()
	for {
		joined, joinErr := conv.Join()
		if joinErr != nil {
			return diagnostic(rc, "join private local session: %v", joinErr)
		}
		if joined {
			break
		}
		select {
		case <-ctx.Done():
			return 0
		case <-interrupt:
			return 0
		case <-time.After(interval):
		}
	}
	if _, err := fmt.Fprintf(rc.Out, "[private session %s connected]\n", conv.ID()); err != nil {
		return diagnostic(rc, "write terminal: %v", err)
	}
	return converse(rc, peer.Name, conv, interrupt)
}

func effectivePollInterval() time.Duration {
	if pollInterval <= 0 {
		return time.Millisecond
	}
	return pollInterval
}

func resolveRecipient(env []string, address, terminal string) (account, session.Record, error) {
	peer, err := lookupAccountFn(address)
	if err != nil {
		return account{}, session.Record{}, fmt.Errorf("%q is not a local OS account", address)
	}
	records, err := readSessionsFn(env)
	if err != nil {
		return account{}, session.Record{}, fmt.Errorf("read local login sessions: %w", err)
	}
	wantTTY := normalizeTerminal(terminal)
	matched, denied := false, false
	var firstPermissionErr error
	for _, record := range records {
		if !session.IsUser(record) || !strings.EqualFold(record.User, peer.Name) {
			continue
		}
		if wantTTY != "" && normalizeTerminal(record.TTY) != wantTTY {
			continue
		}
		matched = true
		permitted, permissionErr := recipientPermittedFn(record, peer)
		if permissionErr != nil {
			if wantTTY != "" {
				return account{}, session.Record{}, fmt.Errorf("check messaging permission on %s: %w", record.TTY, permissionErr)
			}
			if firstPermissionErr == nil {
				firstPermissionErr = fmt.Errorf("check messaging permission on %s: %w", record.TTY, permissionErr)
			}
			continue
		}
		if !permitted {
			denied = true
			continue
		}
		return peer, record, nil
	}
	if firstPermissionErr != nil {
		return account{}, session.Record{}, firstPermissionErr
	}
	if matched && denied {
		return account{}, session.Record{}, fmt.Errorf("%q is denying terminal messages with mesg", address)
	}
	if terminal != "" {
		return account{}, session.Record{}, fmt.Errorf("%q is not logged in on local terminal %q", address, terminal)
	}
	return account{}, session.Record{}, fmt.Errorf("%q is not a local logged-in user", address)
}

func defaultRecipientPermitted(record session.Record, peer account) (bool, error) {
	info, err := os.Stat(session.TTYPath(record.TTY))
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return false, fmt.Errorf("recipient device is not a terminal")
	}
	owner, err := fileOwnerUIDFn(session.TTYPath(record.TTY))
	if err != nil {
		return false, err
	}
	if owner != peer.UID {
		return false, fmt.Errorf("terminal is owned by uid %s, not %s", owner, peer.UID)
	}
	return info.Mode().Perm()&0o020 != 0, nil
}

func normalizeTerminal(name string) string {
	return strings.TrimPrefix(strings.TrimSpace(name), "/dev/")
}

// POSIX specifies no use for standard error by talk. Repository-wide syntax
// failures still use the flag framework's stderr/exit-2 contract.
func diagnostic(rc *tool.RunContext, format string, args ...any) int {
	_, _ = fmt.Fprintf(rc.Out, "talk: "+format+"\n", args...)
	return 1
}

func validateLocalAddress(address string) error {
	if address == "" {
		return fmt.Errorf("empty address")
	}
	if strings.ContainsAny(address, "@!:/\\\x00\r\n") {
		return fmt.Errorf("remote or host-qualified address %q is not supported; talk is localhost-only", address)
	}
	if address == "." || address == ".." {
		return fmt.Errorf("invalid local address %q", address)
	}
	return nil
}

func validateTerminal(name string) error {
	if name == "" {
		return fmt.Errorf("empty terminal operand")
	}
	if strings.HasPrefix(name, "/") || strings.ContainsAny(name, "@!:\\\x00\r\n") {
		return fmt.Errorf("invalid local terminal %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("invalid local terminal %q", name)
		}
	}
	return nil
}

func invitationText(from, terminal string) string {
	where := ""
	if terminal != "" {
		where = " on " + terminal
	}
	return fmt.Sprintf("\nMessage from %s\a\ntalk: connection requested by %s%s\ntalk: respond with: talk %s\n", from, from, where, from)
}

func converse(rc *tool.RunContext, peer string, conv conversation, interrupt <-chan os.Signal) int {
	ctx := rc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	in, err := newInputFn(rc.In)
	if err != nil {
		return diagnostic(rc, "prepare terminal input: %v", err)
	}
	defer func() { _ = conv.Close() }()
	defer func() { _ = in.Close() }()
	peerClosed := false
	for {
		messages, closedNow, pollErr := conv.Poll()
		if pollErr != nil {
			return diagnostic(rc, "read private local conversation: %v", pollErr)
		}
		for _, body := range messages {
			if _, err := fmt.Fprintf(rc.Out, "[%s] %s", peer, body); err != nil {
				return diagnostic(rc, "write terminal: %v", err)
			}
			if !strings.HasSuffix(body, "\n") {
				_, _ = io.WriteString(rc.Out, "\n")
			}
		}
		if closedNow && !peerClosed {
			peerClosed = true
			_, _ = fmt.Fprintf(rc.Out, "talk: %s has terminated the session; only local exit remains\n", peer)
		}
		select {
		case <-ctx.Done():
			return 0
		case <-interrupt:
			return 0
		default:
		}
		event, ready := in.Poll(ctx, effectivePollInterval())
		if !ready {
			continue
		}
		if event.err == io.EOF {
			return 0
		}
		if event.err != nil {
			return diagnostic(rc, "read terminal: %v", event.err)
		}
		if peerClosed {
			continue
		}
		body, refresh := printableInput(event.data, corelocale.Resolve(rc.Env, corelocale.CType))
		if refresh {
			if _, err := io.WriteString(rc.Out, "\x1b[2J\x1b[H"); err != nil {
				return diagnostic(rc, "refresh terminal: %v", err)
			}
		}
		if body != "" {
			if err := conv.Send(body); err != nil {
				return diagnostic(rc, "send message: %v", err)
			}
		}
	}
}

func printableInput(data []byte, ctypeName string) (string, bool) {
	var b strings.Builder
	refresh := false
	if !isUTF8Locale(ctypeName) {
		for _, c := range data {
			switch {
			case c == '\f':
				refresh = true
			case c == '\a' || (c >= 0x20 && c <= 0x7e) || strings.ContainsRune("\t\n\v\r", rune(c)):
				b.WriteByte(c)
			case c < 0x20:
				b.WriteByte('^')
				b.WriteByte(c + '@')
			case c == 0x7f:
				b.WriteString("^?")
			default:
				fmt.Fprintf(&b, "\\x%02X", c)
			}
		}
		return b.String(), refresh
	}
	for len(data) > 0 {
		r, n := utf8.DecodeRune(data)
		if r == utf8.RuneError && n == 1 {
			fmt.Fprintf(&b, "\\x%02X", data[0])
			data = data[1:]
			continue
		}
		data = data[n:]
		switch {
		case r == '\f':
			refresh = true
		case r == '\a' || unicode.IsPrint(r) || unicode.IsSpace(r):
			b.WriteRune(r)
		case r < 0x20:
			b.WriteByte('^')
			b.WriteByte(byte(r) + '@')
		case r == 0x7f:
			b.WriteString("^?")
		default:
			fmt.Fprintf(&b, "\\u{%X}", r)
		}
	}
	return b.String(), refresh
}

func isUTF8Locale(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if at := strings.IndexByte(name, '@'); at >= 0 {
		name = name[:at]
	}
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
		name = name[dot+1:]
	}
	name = strings.ReplaceAll(name, "-", "")
	return name == "utf8"
}
