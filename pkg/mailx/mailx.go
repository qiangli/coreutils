package mailx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrBusy           = errors.New("mailx: mailbox lock is busy")
	ErrMalformed      = errors.New("mailx: malformed message")
	ErrMissingMailbox = errors.New("mailx: mailbox path is required")
)

type Header struct {
	Name  string
	Value string
	Raw   []byte
}

type Message struct {
	Headers    []Header
	Body       []byte
	RawHeaders []byte
	Separator  []byte
}

// Validate checks that the message structure and headers conform to RFC rules
// and contain no header injection or malformed data.
func (m *Message) Validate() error {
	if m == nil {
		return fmt.Errorf("%w: nil message", ErrMalformed)
	}
	if len(m.RawHeaders) > 0 {
		if _, _, err := parseHeaders(m.RawHeaders); err != nil {
			return fmt.Errorf("%w: invalid RawHeaders: %v", ErrMalformed, err)
		}
	}
	if len(m.Headers) == 0 && len(m.RawHeaders) == 0 {
		return fmt.Errorf("%w: message has no headers", ErrMalformed)
	}
	for _, h := range m.Headers {
		if !isValidHeaderName(h.Name) {
			return fmt.Errorf("%w: invalid header name %q", ErrMalformed, h.Name)
		}
		if !isValidHeaderValue(h.Value) {
			return fmt.Errorf("%w: invalid header value for %q", ErrMalformed, h.Name)
		}
		if len(h.Raw) > 0 {
			if !isValidRawHeaderLine(h.Raw) {
				return fmt.Errorf("%w: invalid raw header for %q", ErrMalformed, h.Name)
			}
		}
	}
	return nil
}

func isValidHeaderName(name string) bool {
	if len(name) == 0 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c < 0x21 || c > 0x7e || c == ':' {
			return false
		}
	}
	return true
}

func isValidHeaderValue(val string) bool {
	for i := 0; i < len(val); i++ {
		c := val[i]
		if c == '\r' {
			return false
		}
		if (c < 0x20 && c != '\n' && c != '\t') || c == 0x7f {
			return false
		}
	}
	if strings.HasPrefix(val, "\n") || strings.HasSuffix(val, "\n") || strings.Contains(val, "\n\n") {
		return false
	}
	return true
}

func isValidHeaderValueText(val string) bool {
	for i := 0; i < len(val); i++ {
		c := val[i]
		if (c < 0x20 && c != '\t') || c == 0x7f {
			return false
		}
	}
	return true
}

func isValidRawHeaderLine(raw []byte) bool {
	lines := splitLines(raw)
	if len(lines) == 0 {
		return false
	}
	for i, line := range lines {
		for _, c := range line {
			if (c < 0x20 && c != '\r' && c != '\n' && c != '\t') || c == 0x7f {
				return false
			}
		}
		if i == 0 {
			colon := bytes.IndexByte(line, ':')
			if colon <= 0 || !isValidHeaderName(string(line[:colon])) {
				return false
			}
		} else {
			if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
				return false
			}
		}
	}
	return true
}

func validateSender(sender string) error {
	for i := 0; i < len(sender); i++ {
		c := sender[i]
		if c < 0x20 || c == 0x7f {
			return fmt.Errorf("%w: invalid envelope sender: control character %U", ErrMalformed, rune(c))
		}
	}
	return nil
}

func ParseMessage(data []byte) (*Message, error) {
	split, delimLen, err := splitMessage(data)
	if err != nil {
		return nil, err
	}
	headers, raw, err := parseHeaders(data[:split])
	if err != nil {
		return nil, err
	}
	body := append([]byte(nil), data[split+delimLen:]...)
	msg := &Message{
		Headers:    headers,
		Body:       body,
		RawHeaders: raw,
		Separator:  append([]byte(nil), data[split:split+delimLen]...),
	}
	if err := msg.Validate(); err != nil {
		return nil, err
	}
	return msg, nil
}

func splitMessage(data []byte) (int, int, error) {
	if i := bytes.Index(data, []byte("\r\n\r\n")); i >= 0 {
		return i, 4, nil
	}
	if i := bytes.Index(data, []byte("\n\n")); i >= 0 {
		return i, 2, nil
	}
	return 0, 0, fmt.Errorf("%w: missing header/body separator", ErrMalformed)
}

func parseHeaders(data []byte) ([]Header, []byte, error) {
	lines := splitLines(data)
	if len(lines) == 0 {
		return nil, nil, fmt.Errorf("%w: empty header block", ErrMalformed)
	}
	headers := make([]Header, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if len(headers) == 0 {
				return nil, nil, fmt.Errorf("%w: continuation without header", ErrMalformed)
			}
			prev := &headers[len(headers)-1]
			prev.Raw = append(prev.Raw, line...)
			trimmed := strings.TrimRight(string(bytes.TrimLeft(line, " \t")), "\r\n")
			if !isValidHeaderValueText(trimmed) {
				return nil, nil, fmt.Errorf("%w: invalid header continuation value", ErrMalformed)
			}
			prev.Value += "\n" + trimmed
			continue
		}
		colon := bytes.IndexByte(line, ':')
		if colon <= 0 {
			return nil, nil, fmt.Errorf("%w: invalid header line %q", ErrMalformed, strings.TrimRight(string(line), "\r\n"))
		}
		name := string(line[:colon])
		if !isValidHeaderName(name) {
			return nil, nil, fmt.Errorf("%w: invalid header name %q", ErrMalformed, name)
		}
		val := strings.TrimRight(strings.TrimLeft(string(line[colon+1:]), " \t"), "\r\n")
		if !isValidHeaderValueText(val) {
			return nil, nil, fmt.Errorf("%w: invalid header value", ErrMalformed)
		}
		headers = append(headers, Header{
			Name:  name,
			Value: val,
			Raw:   append([]byte(nil), line...),
		})
	}
	return headers, append([]byte(nil), data...), nil
}

func splitLines(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	var out [][]byte
	start := 0
	for start < len(data) {
		next := bytes.IndexByte(data[start:], '\n')
		if next < 0 {
			out = append(out, append([]byte(nil), data[start:]...))
			break
		}
		end := start + next + 1
		out = append(out, append([]byte(nil), data[start:end]...))
		start = end
	}
	return out
}

// Bytes serializes the message back to mbox wire format. When RawHeaders is
// set (the normal ParseMessage result), it is emitted verbatim and Headers
// edits are ignored — callers that mutate Headers after parsing must clear
// RawHeaders (and any per-header Raw) to have their edits reflected.
func (m *Message) Bytes() []byte {
	var b bytes.Buffer
	if len(m.RawHeaders) != 0 {
		b.Write(m.RawHeaders)
	} else {
		for _, h := range m.Headers {
			if len(h.Raw) != 0 {
				b.Write(h.Raw)
				continue
			}
			lines := strings.Split(h.Value, "\n")
			fmt.Fprintf(&b, "%s: %s\n", h.Name, lines[0])
			for _, cont := range lines[1:] {
				fmt.Fprintf(&b, "\t%s\n", cont)
			}
		}
	}
	if len(m.Separator) != 0 {
		b.Write(m.Separator)
	} else {
		b.WriteString("\n")
	}
	b.Write(m.Body)
	return b.Bytes()
}

func (m *Message) HeaderValues(name string) []string {
	var out []string
	for _, h := range m.Headers {
		if strings.EqualFold(h.Name, name) {
			out = append(out, h.Value)
		}
	}
	return out
}

type Transport interface {
	Deliver(context.Context, *Message, []string) error
}

type LocalMboxTransport struct {
	MailboxPath string
	Sender      string
	Now         func() time.Time
}

func (t LocalMboxTransport) Deliver(_ context.Context, msg *Message, _ []string) error {
	if t.MailboxPath == "" {
		return ErrMissingMailbox
	}
	when := time.Now().UTC()
	if t.Now != nil {
		when = t.Now().UTC()
	}
	return AppendMbox(t.MailboxPath, t.Sender, when, msg)
}

// AppendMbox appends a message to the specified mailbox file.
// All pre-I/O validations (envelope sender, message structure, header names/values)
// are performed strictly before creating any directory, lock file, or mailbox file,
// ensuring nil or invalid messages leave no filesystem artifacts.
//
// Single-write assembly and truncate on write/close failure provide best-effort
// in-process rollback, not crash-atomic transaction guarantees (e.g. system crashes
// or power loss mid-write cannot be rolled back by process-level truncate).
func AppendMbox(path, sender string, when time.Time, msg *Message) error {
	if path == "" {
		return ErrMissingMailbox
	}
	if msg == nil {
		return fmt.Errorf("%w: nil message", ErrMalformed)
	}
	if err := validateSender(sender); err != nil {
		return err
	}
	if err := msg.Validate(); err != nil {
		return err
	}

	// Directory creation happens after validation, so invalid arguments leave no directory artifact.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mailx: create mailbox directory: %w", err)
	}

	// acquireLock never retries or breaks a stale lock: a lock file left
	// behind by a crashed writer blocks all future delivery until an
	// operator removes it by hand. That recovery path is explicitly not
	// supported here.
	unlock, err := acquireLock(path + ".lock")
	if err != nil {
		return err
	}
	defer unlock()

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("mailx: open mailbox: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
	}()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("mailx: stat mailbox: %w", err)
	}
	origSize := info.Size()

	// The envelope, escaped message, and trailing separator are assembled
	// into one buffer and issued as a single Write so a mid-message failure
	// can never leave a half-written entry split across separate syscalls.
	// Note: single-write assembly and truncate rollback are best-effort
	// in-process rollback mechanisms, not crash-atomic system guarantees.
	var buf bytes.Buffer
	buf.WriteString(fromLine(sender, when))
	escaped := escapeFromLines(msg.Bytes())
	buf.Write(escaped)
	if len(escaped) == 0 || escaped[len(escaped)-1] != '\n' {
		buf.WriteByte('\n')
	}
	buf.WriteByte('\n')

	if _, err := f.Write(buf.Bytes()); err != nil {
		_ = f.Truncate(origSize)
		if origSize == 0 {
			_ = os.Remove(path)
		}
		return fmt.Errorf("mailx: write mailbox message: %w", err)
	}

	closed = true
	if err := f.Close(); err != nil {
		_ = os.Truncate(path, origSize)
		if origSize == 0 {
			_ = os.Remove(path)
		}
		return fmt.Errorf("mailx: close mailbox: %w", err)
	}
	return nil
}

func fromLine(sender string, when time.Time) string {
	if sender == "" {
		sender = "unknown"
	}
	return fmt.Sprintf("From %s %s\n", sender, when.Format("Mon Jan _2 15:04:05 2006"))
}

func escapeFromLines(data []byte) []byte {
	lines := splitLines(data)
	if len(lines) == 0 {
		return nil
	}
	var out bytes.Buffer
	for _, line := range lines {
		if bytes.HasPrefix(line, []byte("From ")) {
			out.WriteByte('>')
		}
		out.Write(line)
	}
	return out.Bytes()
}

// acquireLock is a one-shot, non-blocking exclusive lock: it never retries
// and never inspects the age of an existing lock file. A lock left behind by
// a process that died mid-delivery is indistinguishable from one held by a
// live writer and will return ErrBusy forever until removed by hand — stale
// lock recovery is not supported.
func acquireLock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, ErrBusy
		}
		return nil, fmt.Errorf("mailx: create mailbox lock: %w", err)
	}
	closed := false
	return func() {
		if !closed {
			closed = true
			_ = f.Close()
			_ = os.Remove(path)
		}
	}, nil
}
