// Package writecmd implements write(1) per POSIX.1-2008 (Issue 7, 2016
// Edition): read lines from standard input and write them to the terminal of
// a logged-in user.
//
//	write user_name [terminal]
//
// Four things about this utility are easy to get wrong, and all four are
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
//
// FOURTH, there are THREE distinct sinks and they must not be confused:
//
//   - The RECIPIENT'S TERMINAL carries the banner, the message body and the
//     closing "EOT\n". POSIX OUTPUT FILES.
//   - The SENDER'S CONTROLLING TERMINAL carries only the two alerts POSIX
//     requires once the recipient banner has been written successfully.
//   - STANDARD OUTPUT carries the informational message naming the selected
//     terminal when the recipient is logged in more than once.
//   - STANDARD ERROR carries diagnostics and nothing else, per POSIX STDERR.
//
// Standard output is never used for anything else, in any code path.
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
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/qiangli/coreutils/pkg/ctype"
	"github.com/qiangli/coreutils/pkg/locale"
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
	nowFn                  = time.Now
	statFn                 = os.Stat
	openTTYFn              = defaultOpenTTY
	sessionActiveFn        = defaultSessionActive
	sessionOwnsTerminalFn  = defaultSessionOwnsTerminal
	terminalDeviceFn       = defaultTerminalDevice
)

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

type ctypeProvider interface {
	IsPrint(byte) (bool, error)
	IsSpace(byte) (bool, error)
	Close() error
}

var openCTypeFn = func(name string) (ctypeProvider, error) { return ctype.Open(name) }

// charClasses answers "may this character reach the recipient unchanged?" for
// the sender's LC_CTYPE. POSIX: characters from the print or space
// classifications are sent through; every other non-printable character is
// rendered as an implementation-defined printable sequence (here: caret and
// meta notation, the historical shape).
//
// Two representations, because a locale is one of two shapes:
//
//   - pass[] is a BYTE table, used for the C/POSIX locale and for single-byte
//     locales resolved through the real glibc provider (pkg/ctype).
//   - multibyte selects UTF-8 mode, where classification runs on the decoded
//     RUNE and a printable rune's bytes are copied through untouched. Byte
//     classification would shred every non-ASCII character into M- notation,
//     which is exactly the mangling this mode exists to prevent.
type charClasses struct {
	pass      [256]bool
	multibyte bool
}

// cPass fills the byte table with the C/POSIX locale's print+space classes.
func (c *charClasses) cPass() {
	for b := byte(0x20); b <= 0x7e; b++ {
		c.pass[b] = true
	}
	for _, b := range []byte{'\t', '\n', '\v', '\f', '\r'} {
		c.pass[b] = true
	}
}

// loadCharClasses resolves LC_CTYPE and builds the classifier for it.
//
// It cannot fail. An unusable LC_CTYPE — a name pkg/ctype does not accept, a
// host with no glibc, a platform with no provider at all — falls back to the
// C locale's classes, which is what a C program gets when setlocale() fails.
// Refusing to deliver the message because the locale is exotic would trade a
// cosmetic rendering question for total loss of the utility's function, and
// POSIX reserves a non-zero exit for "not logged on or permission denied".
// UTF-8 classification uses Go's Unicode tables because pkg/ctype's provider
// is byte-oriented; unsupported non-UTF-8 locales retain the documented C
// fallback. Diagnostics are not yet localized through LC_MESSAGES/NLSPATH.
func loadCharClasses(env []string) *charClasses {
	name := locale.Resolve(env, locale.CType)
	c := new(charClasses)
	switch {
	case name == "C" || name == "POSIX":
		c.cPass()
		return c
	case isUTF8Locale(name):
		// The codeset is UTF-8: classify decoded runes, not bytes.
		c.cPass()
		c.multibyte = true
		return c
	}
	p, err := openCTypeFn(name)
	if err != nil {
		c.cPass()
		return c
	}
	defer func() { _ = p.Close() }()
	for i := 0; i < 256; i++ {
		printable, perr := p.IsPrint(byte(i))
		if perr != nil {
			var fallback charClasses
			fallback.cPass()
			return &fallback
		}
		space, serr := p.IsSpace(byte(i))
		if serr != nil {
			var fallback charClasses
			fallback.cPass()
			return &fallback
		}
		c.pass[i] = printable || space
	}
	return c
}

// isUTF8Locale reports whether a POSIX locale name names the UTF-8 codeset.
// The codeset is the part after '.', with any '@modifier' stripped; spelling
// varies ("en_US.UTF-8", "en_US.utf8", "C.UTF-8"), so the comparison folds
// case and ignores the hyphen.
func isUTF8Locale(name string) bool {
	dot := strings.IndexByte(name, '.')
	if dot < 0 {
		codeset := strings.ToLower(strings.ReplaceAll(name, "-", ""))
		return codeset == "utf8"
	}
	codeset := name[dot+1:]
	if at := strings.IndexByte(codeset, '@'); at >= 0 {
		codeset = codeset[:at]
	}
	codeset = strings.ToLower(strings.ReplaceAll(codeset, "-", ""))
	return codeset == "utf8"
}

// defaultOpenTTY opens the recipient's terminal for writing. O_WRONLY only:
// write never reads the device, and asking for read access on a terminal
// owned by someone else would fail where a write-only open succeeds.
func defaultOpenTTY(path string) (io.WriteCloser, error) {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}
	if !terminalFile(f) {
		_ = f.Close()
		return nil, fmt.Errorf("not a terminal")
	}
	return f, nil
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

	if !supported {
		fmt.Fprintf(rc.Err, "write: %v\n", errPlatform)
		return 1
	}
	classes := loadCharClasses(rc.Env)

	if err := lookupUser(target); err != nil {
		fmt.Fprintf(rc.Err, "write: %s: no such user\n", target)
		return 1
	}

	sender, uid, err := senderInfo()
	if err != nil {
		fmt.Fprintf(rc.Err, "write: cannot determine the sending user: %v\n", err)
		return 1
	}

	records, err := readUtmpFile(dbPath, dbLayout)
	if err != nil {
		if errors.Is(err, errNoLayout) {
			fmt.Fprintf(rc.Err, "write: %v\n", err)
		} else {
			fmt.Fprintf(rc.Err, "write: %s: %v\n", dbPath, err)
		}
		return 1
	}

	myTTY := senderTTY(rc)
	var senderControl io.WriteCloser
	if myTTY != "" {
		senderControl, err = openSenderControlTTYFn(rc, myTTY)
		if err != nil {
			fmt.Fprintf(rc.Err, "write: sender terminal %s: %v\n", myTTY, err)
			return 1
		}
	}
	if senderControl != nil {
		defer func() {
			if senderControl != nil {
				_ = senderControl.Close()
			}
		}()
	}
	// Prefer the login identity attached to the authenticated sending terminal.
	// user.Current remains the fallback for non-interactive callers and systems
	// whose accounting database has no sender record.
	for _, record := range records {
		if normalizeTTY(record.Line) == myTTY && sessionActiveFn(record.PID) &&
			sessionOwnsTerminalFn(record.PID, ttyDevice(record.Line)) {
			sender = record.User
			break
		}
	}
	line, failure, isMulti := selectTerminal(records, target, wantTTY, myTTY, uid == 0)
	if failure != "" {
		fmt.Fprintf(rc.Err, "write: %s\n", failure)
		return 1
	}
	path := ttyDevice(line)
	term, err := openTTYFn(path)
	if err != nil {
		fmt.Fprintf(rc.Err, "write: %s: %v\n", path, err)
		return 1
	}
	if err := writeBanner(term, sender, myTTY); err != nil {
		err = errors.Join(err, term.Close())
		fmt.Fprintf(rc.Err, "write: %s: %v\n", path, err)
		return 1
	}
	if isMulti {
		notice := fmt.Sprintf("write: %s is logged in on more than one line; using %s\n", target, line)
		if err := writeString(rc.Out, notice); err != nil {
			err = errors.Join(err, term.Close())
			fmt.Fprintf(rc.Err, "write: standard output: %v\n", err)
			return 1
		}
	}
	if senderControl != nil {
		if err := writeString(senderControl, "\a\a"); err != nil {
			err = errors.Join(err, term.Close())
			fmt.Fprintf(rc.Err, "write: sender terminal %s: %v\n", myTTY, err)
			return 1
		}
		control := senderControl
		senderControl = nil
		if err := control.Close(); err != nil {
			err = errors.Join(err, term.Close())
			fmt.Fprintf(rc.Err, "write: sender terminal %s: %v\n", myTTY, err)
			return 1
		}
	}

	deliveryErr := deliverBody(term, rc.In, classes)
	closeErr := term.Close()
	if err := errors.Join(deliveryErr, closeErr); err != nil {
		fmt.Fprintf(rc.Err, "write: %s: %v\n", path, err)
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
//  3. The sender's OWN terminal is dropped, BUT ONLY when no terminal operand
//     was given. Writing to it echoes the message back into the session that
//     is typing it, which is never the intent, and it is the one case a user
//     can hit by accident (`write $USER`). Naming it explicitly is not an
//     accident, so `write $USER $(tty)` is honoured.
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
//
// Deliberately NOT used: terminal idle time (atime of the device), the
// historical heuristic. It needs a stat field Go does not expose portably, and
// it makes the choice depend on filesystem mount options (noatime, relatime) —
// so the same fleet would select differently host to host.
//
// isMulti reports that the recipient had more than one candidate login and no
// terminal operand narrowed it: the condition POSIX attaches the informational
// message to. It is reported even when selection then fails, because the
// caller only emits the notice on the success path.
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

	isMulti = wantTTY == "" && len(candidates) > 1

	sort.SliceStable(candidates, func(i, j int) bool {
		if !candidates[i].Time.Equal(candidates[j].Time) {
			return candidates[i].Time.After(candidates[j].Time)
		}
		return candidates[i].Line < candidates[j].Line
	})

	var denied []string
	skippedSelf, missing := false, 0
	for _, r := range candidates {
		if wantTTY == "" && myTTY != "" && normalizeTTY(r.Line) == myTTY {
			skippedSelf = true
			continue
		}
		fi, err := statFn(ttyDevice(r.Line))
		if err != nil {
			missing++
			continue
		}
		if r.PID <= 0 || !sessionActiveFn(r.PID) || !terminalDeviceFn(ttyDevice(r.Line)) ||
			!sessionOwnsTerminalFn(r.PID, ttyDevice(r.Line)) {
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

// readLine reads bytes until NL, the terminal's canonical EOL character, or
// EOF, and normalises a completed input record to a single NL byte.
//
// POSIX: "Whenever a line of input as delimited by an NL, EOF, or EOL special
// character is accumulated while in canonical input mode, the accumulated data
// shall be written on the other user's terminal." EOF (VEOF, normally ^D) is
// not a byte the reader ever sees — in canonical mode the driver turns it into
// a zero-length read, which surfaces here as io.EOF — so only NL and EOL are
// byte comparisons. veol == 0 means the terminal has EOL disabled
// (_POSIX_VDISABLE), which is the default, and no second delimiter applies.
func readLine(br *bufio.Reader, veol byte) (string, error) {
	var buf []byte
	for {
		b, err := br.ReadByte()
		if err != nil {
			return string(buf), err
		}
		if b == '\n' || (veol != 0 && b == veol) {
			return string(append(buf, '\n')), nil
		}
		buf = append(buf, b)
	}
}

// writeString rejects a writer that reports success after consuming only a
// prefix. Silently accepting that contract violation can lose a BEL, banner,
// message byte, or EOT marker while reporting successful delivery.
func writeString(w io.Writer, s string) error {
	n, err := io.WriteString(w, s)
	if err != nil {
		return err
	}
	if n != len(s) {
		return io.ErrShortWrite
	}
	return nil
}

// writeBanner writes the exact POSIX Issue 7 prescribed initial message.
func writeBanner(w io.Writer, sender, senderTTY string) error {
	from := senderTTY
	if from == "" {
		// No controlling terminal (a script, a pipe, an agent harness). The
		// banner still has to say something, and "?" is the honest answer -
		// inventing a plausible tty name would misattribute the message.
		from = "?"
	}

	banner := fmt.Sprintf("Message from %s (%s) [%s]...\n",
		sender, from, nowFn().Format(time.ANSIC))
	return writeString(w, banner)
}

// deliverBody writes message records and the closing EOT marker. The caller
// writes and verifies the banner before alerting the sender.
func deliverBody(w io.Writer, in io.Reader, classes *charClasses) error {
	// An *os.File input can be waited on with poll(2). Other readers either
	// provide deadlines, identify themselves as finite in-memory input, or are
	// rejected explicitly; no path abandons a blocked reader goroutine.
	var owned *os.File
	if f, ok := in.(*os.File); ok {
		if dup, err := duplicateInputFile(f); err == nil {
			owned = dup
			defer owned.Close()
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	if owned != nil {
		return deliverPolled(w, owned, getVEOL(in), classes, sigCh)
	}
	return deliverStream(w, in, getVEOL(in), classes, sigCh)
}

// deliver is retained as a focused test seam. Production uses writeBanner and
// deliverBody separately so alerts can occur strictly between them.
func deliver(w io.Writer, in io.Reader, sender, senderTTY, _ string, classes *charClasses) error {
	if err := writeBanner(w, sender, senderTTY); err != nil {
		return err
	}
	return deliverBody(w, in, classes)
}

// finish closes the message. POSIX: the interrupt and end-of-file characters
// "cause write to write an appropriate message ("EOT\n" in the POSIX locale)
// to the recipient's terminal and exit".
//
// pending is the sender's accumulated but undelimited bytes. They are
// delivered — they were typed, and dropping them silently truncates the
// message — followed by the NL that frames them, so "EOT" always begins a
// line of its own on the recipient's terminal.
func finish(w io.Writer, pending []byte, classes *charClasses) error {
	if len(pending) > 0 {
		if err := writeString(w, sanitize(string(append(pending, '\n')), classes)); err != nil {
			return err
		}
	}
	return writeString(w, "EOT\n")
}

type readDeadliner interface {
	SetReadDeadline(time.Time) error
}

type finiteReader interface {
	Len() int
}

// deliverStream handles non-file readers without adopting or closing them.
// Deadline-capable streams are polled with read deadlines. In-memory readers
// advertise a finite remaining length and cannot remain blocked. An opaque
// reader with neither property cannot be made interruptible through io.Reader
// without abandoning a blocked goroutine or closing a caller-owned object, so
// it is rejected explicitly rather than hanging the embedding process.
func deliverStream(w io.Writer, in io.Reader, veol byte, classes *charClasses, sigCh <-chan os.Signal) error {
	if in == nil {
		return finish(w, nil, classes)
	}
	if deadline, ok := in.(readDeadliner); ok {
		return deliverDeadline(w, in, deadline, veol, classes, sigCh)
	}
	if _, ok := in.(finiteReader); !ok {
		return errors.New("input reader cannot be interrupted safely")
	}
	br := bufio.NewReader(in)
	for {
		select {
		case <-sigCh:
			return finish(w, nil, classes)
		default:
		}
		line, err := readLine(br, veol)
		if errors.Is(err, io.EOF) {
			return finish(w, []byte(line), classes)
		}
		if line != "" {
			if werr := writeString(w, sanitize(line, classes)); werr != nil {
				return werr
			}
		}
		if err != nil {
			return err
		}
	}
}

func deliverDeadline(w io.Writer, in io.Reader, deadline readDeadliner, veol byte, classes *charClasses, sigCh <-chan os.Signal) error {
	defer deadline.SetReadDeadline(time.Time{})
	var line []byte
	for {
		select {
		case <-sigCh:
			return finish(w, line, classes)
		default:
		}
		if err := deadline.SetReadDeadline(time.Now().Add(pollInterval)); err != nil {
			return err
		}
		var one [1]byte
		n, err := in.Read(one[:])
		if n == 1 {
			if one[0] == '\n' || (veol != 0 && one[0] == veol) {
				line = append(line, '\n')
				if werr := writeString(w, sanitize(string(line), classes)); werr != nil {
					return werr
				}
				line = line[:0]
			} else {
				line = append(line, one[0])
			}
		}
		if errors.Is(err, io.EOF) {
			return finish(w, line, classes)
		}
		if err != nil && !os.IsTimeout(err) {
			return err
		}
	}
}

// deliverPolled copies a file-backed input, waiting on poll(2) so an interrupt
// is honoured immediately — including while the sender is part-way through a
// line. The descriptor is a DUPLICATE (see deliver): returning early closes
// only this copy, never the caller's stdin, and leaves no reader behind.
func deliverPolled(w io.Writer, in *os.File, veol byte, classes *charClasses, sigCh <-chan os.Signal) error {
	var line []byte
	for {
		select {
		case <-sigCh:
			return finish(w, line, classes)
		default:
		}
		ready, err := waitInputReadable(in, pollInterval)
		if err != nil {
			return err
		}
		if !ready {
			continue
		}
		var one [1]byte
		n, rerr := in.Read(one[:])
		if n == 1 {
			if one[0] == '\n' || (veol != 0 && one[0] == veol) {
				line = append(line, '\n')
				if err := writeString(w, sanitize(string(line), classes)); err != nil {
					return err
				}
				line = line[:0]
			} else {
				line = append(line, one[0])
			}
		}
		if errors.Is(rerr, io.EOF) {
			return finish(w, line, classes)
		}
		if rerr != nil {
			return rerr
		}
	}
}

// pollInterval bounds how long an interrupt can wait behind a blocked read.
// It is a wakeup budget, not a busy loop: poll(2) sleeps for the whole
// interval when nothing arrives.
const pollInterval = 100 * time.Millisecond

// sanitize renders one line safely on someone else's terminal.
//
// This is not cosmetic. The recipient's terminal interprets what arrives, so
// forwarding raw control bytes hands the sender the ability to reprogram
// another user's session — clear the screen, redefine keys, or on some
// terminals inject a command line. POSIX leaves the rendering of non-printable
// characters implementation-defined, so caret notation (^[ for ESC) is both
// permitted and the historically expected shape.
//
// Two POSIX rules constrain what may NOT be rewritten:
//
//   - "Typing <alert> shall write the <alert> character to the recipient's
//     terminal." BEL is therefore passed through as the byte 0x07, never
//     rendered as ^G, even though it is a control character. It is the one
//     control character the standard names as deliverable, and it cannot
//     carry a control sequence on its own.
//   - "Typing characters from LC_CTYPE classifications print or space shall
//     cause those characters to be sent to the recipient's terminal." So the
//     pass set is the SENDER'S RESOLVED LC_CTYPE, not a hard-coded ASCII
//     table — see loadCharClasses.
//
// In a UTF-8 locale classification runs on the decoded rune and a printable
// rune's bytes are copied through byte-for-byte; only an invalid byte falls
// back to meta notation.
func sanitize(s string, classes *charClasses) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\a':
			// POSIX: the alert character reaches the recipient as itself.
			b.WriteByte(c)
			i++
		case classes.multibyte && c >= utf8.RuneSelf:
			r, size := utf8.DecodeRuneInString(s[i:])
			switch {
			case r == utf8.RuneError && size == 1:
				// Not valid UTF-8 in a UTF-8 locale: show the raw byte in
				// meta notation rather than passing an unknown 8-bit byte to
				// the terminal.
				writeMeta(&b, c)
			case unicode.IsPrint(r) || unicode.IsSpace(r):
				b.WriteString(s[i : i+size])
			default:
				for j := 0; j < size; j++ {
					writeMeta(&b, s[i+j])
				}
			}
			i += size
		case classes.pass[c]:
			// LC_CTYPE calls it print or space: POSIX sends it through.
			b.WriteByte(c)
			i++
		case c < 0x20 || c == 0x7f:
			writeCaret(&b, c)
			i++
		case c >= utf8.RuneSelf:
			writeMeta(&b, c)
			i++
		default:
			// A graphic ASCII byte the locale declined to classify. cat -v,
			// the notation this follows, renders 0x20-0x7e as itself, and it
			// cannot carry a control sequence on its own. Unreachable with any
			// real LC_CTYPE, where those bytes are always print.
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// writeCaret renders a C0 control character or DEL in caret notation.
func writeCaret(b *strings.Builder, c byte) {
	b.WriteByte('^')
	b.WriteByte(c ^ 0x40)
}

// writeMeta renders a byte with the high bit set in meta notation, with the
// remaining seven bits themselves rendered in caret notation when they name a
// control character.
func writeMeta(b *strings.Builder, c byte) {
	b.WriteString("M-")
	lo := c & 0x7f
	if lo < 0x20 || lo == 0x7f {
		writeCaret(b, lo)
		return
	}
	b.WriteByte(lo)
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
