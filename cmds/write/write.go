// Package writecmd implements write(1) per POSIX.1-2017: read lines from
// standard input and write them to the terminal of a logged-in user.
//
//	write user_name [terminal]
//
// Three things about this utility are easy to get wrong, and all three are
// load-bearing.
//
// FIRST, the recipient is found by reading the login-accounting database
// (/var/run/utmp, /var/run/utmpx), which is a raw array of C structs in host
// byte order with no header and no version marker. The layouts live in
// utmp.go as DATA so the decoder is shared and both layouts stay testable on
// every build; platform_*.go selects the one that matches the running system.
//
// SECOND, POSIX leaves the choice UNSPECIFIED when the recipient is logged in
// more than once and no terminal operand is given. Unspecified is not licence
// to be arbitrary: an unpredictable choice means a message silently lands on
// a terminal nobody is watching. The rule implemented here is documented on
// selectTerminal and is total — same database in, same terminal out.
//
// THIRD, the permission gate is the recipient's terminal group-write bit, the
// same bit mesg(1) toggles (see cmds/mesg). Messaging is a property of the
// DEVICE, not a stored preference, so the check is a stat and nothing else.
// Denial is a diagnosed failure with a non-zero exit, never a silent drop:
// a message the sender believes was delivered is worse than an error.
package writecmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "write",
	Synopsis: "Write a message to another logged-in user's terminal.",
	Usage:    "write user_name [terminal]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

// Seams. Every one of these is an operating-system fact the tests cannot
// supply for real: there is no logged-in user, no /var/run/utmp and no
// terminal in a hermetic test run. Overriding them is how the behaviour under
// test stays the same code path that runs in production.
var (
	dbPath                 = defaultUtmpPath  // login-accounting database
	dbLayout               = activeUtmpLayout // struct layout of that database
	devDir                 = "/dev"           // where a ut_line name resolves
	supported              = platformSupported
	lookupUser             = func(name string) error { _, err := user.Lookup(name); return err }
	senderInfo             = defaultSenderInfo
	senderTTY              = defaultSenderTTY
	openSenderControlTTYFn = defaultOpenSenderControlTTY
	getVEOL                = defaultGetVEOL
	unblockIn              = defaultUnblockIn
	hostnameFn             = os.Hostname
	nowFn                  = time.Now
	statFn                 = os.Stat
	openTTYFn              = defaultOpenTTY
)

// defaultOpenTTY opens the recipient's terminal for writing. O_WRONLY only:
// write never reads the device, and asking for read access on a terminal
// owned by someone else would fail where a write-only open succeeds.
func defaultOpenTTY(path string) (io.WriteCloser, error) {
	return os.OpenFile(path, os.O_WRONLY, 0)
}

// defaultSenderInfo reports the sending account and its numeric uid. The uid
// is needed only to recognise the superuser, who is exempt from the recipient's
// message-permission bit exactly as the kernel exempts it from the file mode.
func defaultSenderInfo() (name string, uid int, err error) {
	u, err := user.Current()
	if err != nil {
		return "", -1, err
	}
	name = u.Username
	// Windows reports DOMAIN\name; the bare account name is what a banner
	// should carry. Harmless elsewhere - a Unix account name has no backslash.
	if i := strings.LastIndexByte(name, '\\'); i >= 0 {
		name = name[i+1:]
	}
	uid, err = strconv.Atoi(u.Uid)
	if err != nil {
		// A non-numeric uid (Windows SID) is simply "not root".
		uid = -1
	}
	return name, uid, nil
}

func run(rc *tool.RunContext, args []string) int {
	args = tool.AliasHelpVersion(args)
	fs := tool.NewFlags(cmd.Name)
	// POSIX write takes no options. Anything option-shaped therefore falls
	// through to tool.Parse, which rejects it with the contract diagnostic and
	// exit 2 rather than silently treating it as a user name.
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	switch {
	case len(operands) == 0:
		return tool.UsageError(rc, cmd, "missing user_name operand")
	case len(operands) > 2:
		return tool.UsageError(rc, cmd, "extra operand %q", operands[2])
	}
	target := operands[0]
	wantTTY := ""
	if len(operands) == 2 {
		wantTTY = normalizeTTY(operands[1])
		if wantTTY == "" {
			return tool.UsageError(rc, cmd, "invalid terminal operand %q", operands[1])
		}
	}

	controlW := openSenderControlTTYFn(rc)

	if !supported {
		fmt.Fprintf(controlW, "write: %v\n", errPlatform)
		return 1
	}

	if err := lookupUser(target); err != nil {
		fmt.Fprintf(controlW, "write: %s: no such user\n", target)
		return 1
	}

	sender, uid, err := senderInfo()
	if err != nil {
		fmt.Fprintf(controlW, "write: cannot determine the sending user: %v\n", err)
		return 1
	}

	records, err := readUtmpFile(dbPath, dbLayout)
	if err != nil {
		if errors.Is(err, errNoLayout) {
			fmt.Fprintf(controlW, "write: %v\n", err)
		} else {
			fmt.Fprintf(controlW, "write: %s: %v\n", dbPath, err)
		}
		return 1
	}

	myTTY := senderTTY(rc)
	line, failure, isMulti := selectTerminal(records, target, wantTTY, myTTY, uid == 0)
	if failure != "" {
		fmt.Fprintf(controlW, "write: %s\n", failure)
		return 1
	}
	if isMulti {
		fmt.Fprintf(controlW, "write: %s is logged in on more than one line; using %s\n", target, line)
	}

	path := ttyDevice(line)
	term, err := openTTYFn(path)
	if err != nil {
		fmt.Fprintf(controlW, "write: %s: %v\n", path, err)
		return 1
	}
	defer term.Close()

	if err := deliver(term, rc.In, sender, target, myTTY); err != nil {
		fmt.Fprintf(controlW, "write: %s: %v\n", path, err)
		return 1
	}
	return 0
}

// selectTerminal resolves the recipient's terminal, or returns the diagnostic
// explaining why there is none. line is a bare ut_line name.
//
// THE SELECTION RULE (POSIX leaves this unspecified; this implementation makes
// it total and documented):
//
//  1. Candidates are the USER_PROCESS records whose ut_user equals user_name,
//     narrowed to the terminal operand when one was given.
//  2. A candidate whose device does not exist is dropped — the database
//     routinely retains entries for sessions whose device is gone.
//  3. The sender's OWN terminal is dropped. Writing to it echoes the message
//     back into the session that is typing it, which is never the intent, and
//     it is the one case a user can hit by accident (`write $USER`).
//  4. A candidate whose device denies messages (group-write clear, the bit
//     mesg(1) owns) is dropped, unless the sender is the superuser. Skipping
//     rather than failing outright matters: a user with mesg n on one terminal
//     and mesg y on another is reachable, and refusing on the first one
//     examined would make reachability depend on database order.
//  5. Of what remains, the MOST RECENT LOGIN wins (ut_tv descending), with the
//     terminal name ascending as the tie-break. Most-recent-login is the best
//     "where is this person actually sitting" signal available from the
//     database alone; the name tie-break makes the result independent of the
//     order records happen to appear in.
func selectTerminal(records []utmpRecord, target, wantTTY, myTTY string, isRoot bool) (line, failure string, isMulti bool) {
	var candidates []utmpRecord
	for _, r := range records {
		if r.User != target {
			continue
		}
		if wantTTY != "" && normalizeTTY(r.Line) != wantTTY {
			continue
		}
		candidates = append(candidates, r)
	}
	if len(candidates) == 0 {
		if wantTTY != "" {
			return "", fmt.Sprintf("%s is not logged in on %s", target, wantTTY), false
		}
		return "", fmt.Sprintf("%s is not logged in", target), false
	}

	isMulti = (wantTTY == "" && len(candidates) > 1)

	sort.SliceStable(candidates, func(i, j int) bool {
		if !candidates[i].Time.Equal(candidates[j].Time) {
			return candidates[i].Time.After(candidates[j].Time)
		}
		return candidates[i].Line < candidates[j].Line
	})

	var denied []string
	skippedSelf, missing := false, 0
	for _, r := range candidates {
		if myTTY != "" && normalizeTTY(r.Line) == myTTY {
			skippedSelf = true
			continue
		}
		fi, err := statFn(ttyDevice(r.Line))
		if err != nil {
			missing++
			continue
		}
		// The same bit mesg(1) reads and writes. See cmds/mesg: the terminal's
		// group-write permission IS the message-permission state, so this is a
		// stat and a mask, not a lookup in some separate registry.
		if !isRoot && fi.Mode().Perm()&0o020 == 0 {
			denied = append(denied, r.Line)
			continue
		}
		return r.Line, "", isMulti
	}

	switch {
	case len(denied) > 0:
		return "", fmt.Sprintf("%s has messages disabled on %s", target, denied[0]), false
	case skippedSelf:
		return "", "you cannot write to your own terminal", false
	case missing > 0:
		return "", fmt.Sprintf("%s: no accessible terminal", target), false
	default:
		return "", fmt.Sprintf("%s is not logged in", target), false
	}
}

// readLine reads bytes until newline (\n), canonical EOL (veol if non-zero), or EOF.
func readLine(br *bufio.Reader, veol byte) (string, error) {
	var buf []byte
	for {
		b, err := br.ReadByte()
		if err != nil {
			return string(buf), err
		}
		if b == '\n' || (veol != 0 && b == veol) {
			return string(buf), nil
		}
		buf = append(buf, b)
	}
}

// deliver writes the banner, the message body and the closing EOF/EOT marker.
func deliver(w io.Writer, in io.Reader, sender, target, senderTTY string) error {
	host, err := hostnameFn()
	if err != nil || host == "" {
		host = "localhost"
	}
	from := senderTTY
	if from == "" {
		// No controlling terminal (a script, a pipe, an agent harness). The
		// banner still has to say something, and "?" is the honest answer -
		// inventing a plausible tty name would misattribute the message.
		from = "?"
	}
	// The bell and the CR-LF pairs are the historical banner shape: the
	// recipient's terminal may be in raw mode, where a bare LF would leave the
	// cursor mid-line and shred the display of whatever they were running.
	banner := fmt.Sprintf("\a\r\nMessage from %s@%s on %s to %s at %s ...\r\n",
		sender, host, from, target, nowFn().Format("15:04"))
	if _, err := io.WriteString(w, banner); err != nil {
		return err
	}
	if in != nil {
		veol := getVEOL(in)
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGINT)
		defer signal.Stop(sigCh)

		type lineResult struct {
			line string
			err  error
		}
		lineChan := make(chan lineResult)
		stopReader := make(chan struct{})
		readerDone := make(chan struct{})

		go func() {
			defer close(readerDone)
			defer close(lineChan)
			br := bufio.NewReader(in)
			for {
				line, rerr := readLine(br, veol)
				if line != "" || rerr == nil {
					select {
					case lineChan <- lineResult{line: line, err: rerr}:
					case <-stopReader:
						return
					}
				}
				if rerr != nil {
					return
				}
			}
		}()

		var sigintReceived bool
		for {
			select {
			case <-sigCh:
				sigintReceived = true
				close(stopReader)
				unblockIn(in)
				<-readerDone
			case res, ok := <-lineChan:
				if !ok {
					break
				}
				if res.err == nil {
					if _, werr := io.WriteString(w, sanitize(res.line)+"\r\n"); werr != nil {
						close(stopReader)
						unblockIn(in)
						<-readerDone
						return werr
					}
				} else if errors.Is(res.err, io.EOF) {
					if res.line != "" {
						if _, werr := io.WriteString(w, sanitize(res.line)); werr != nil {
							close(stopReader)
							unblockIn(in)
							<-readerDone
							return werr
						}
					}
				} else {
					close(stopReader)
					unblockIn(in)
					<-readerDone
					return res.err
				}
				continue
			}
			break
		}

		if sigintReceived {
			_, err = io.WriteString(w, "EOT\r\n")
			return err
		}
	}
	_, err = io.WriteString(w, "EOF\r\n")
	return err
}

// sanitize renders one line safely on someone else's terminal.
//
// Printable multi-byte UTF-8 passes through unchanged: mangling it would be a
// gratuitous regression, and it cannot carry a control sequence.
func sanitize(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\a' || c == '\t' || c == '\r':
			b.WriteByte(c)
			i++
		case c == '\n':
			b.WriteString("\r\n")
			i++
		case c < 0x20 || c == 0x7f:
			b.WriteByte('^')
			b.WriteByte(c ^ 0x40)
			i++
		case c < 0x80:
			b.WriteByte(c)
			i++
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && size == 1 {
				// Not valid UTF-8: show the raw byte in meta notation rather
				// than passing an unknown 8-bit byte to the terminal.
				b.WriteString("M-")
				lo := c & 0x7f
				if lo < 0x20 || lo == 0x7f {
					b.WriteByte('^')
					lo ^= 0x40
				}
				b.WriteByte(lo)
				i++
				continue
			}
			if unicode.IsPrint(r) {
				b.WriteString(s[i : i+size])
			} else {
				if r <= 0xff {
					b.WriteString("M-")
					lo := byte(r) & 0x7f
					if lo < 0x20 || lo == 0x7f {
						b.WriteByte('^')
						lo ^= 0x40
					}
					b.WriteByte(lo)
				} else {
					fmt.Fprintf(&b, "<U+%04X>", r)
				}
			}
			i += size
		}
	}
	return b.String()
}

// normalizeTTY reduces a terminal operand or a ut_line value to the bare device
// name used for comparison, so `write bob pts/3` and `write bob /dev/pts/3`
// name the same terminal.
func normalizeTTY(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = filepath.Clean(s)
	if rel, err := filepath.Rel(devDir, s); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return s
}

// ttyDevice maps a bare ut_line name onto its device path.
func ttyDevice(line string) string {
	if filepath.IsAbs(line) {
		return line
	}
	return filepath.Join(devDir, line)
}
