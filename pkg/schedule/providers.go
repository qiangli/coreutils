package schedule

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	mailxpkg "github.com/qiangli/coreutils/pkg/mailx"
	gopsload "github.com/shirou/gopsutil/v4/load"
)

// HostLoadAverage reads the host's one-minute load average without consulting
// command output or mutable scheduler state.
func HostLoadAverage() (float64, error) {
	avg, err := gopsload.Avg()
	if err != nil {
		return 0, err
	}
	return avg.Load1, nil
}

// DiscoverMailDelivery locates a sendmail-compatible local MTA. Discovery is
// bounded to PATH and the two traditional absolute installations.
func DiscoverMailDelivery() (MailDelivery, error) {
	var candidates []string
	if path, err := exec.LookPath("sendmail"); err == nil {
		candidates = append(candidates, path)
	}
	candidates = append(candidates, "/usr/sbin/sendmail", "/usr/lib/sendmail")
	seen := make(map[string]bool)
	for _, path := range candidates {
		path = filepath.Clean(path)
		if seen[path] {
			continue
		}
		seen[path] = true
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return SendmailDelivery(path), nil
		}
	}
	return nil, ErrMailDeliveryUnsupported
}

// DiscoverLocalMailDelivery builds the file-backed delivery provider used by
// the pure-Go mail/mailx applets.  at and batch jobs carry their submission
// environment, so the scheduler can deliver completion mail to the same local
// spool even when the daemon itself has no sendmail installation.
//
// A local spool must be explicit.  Falling back to the daemon's HOME would
// silently deliver a submitted user's mail to the service account instead.
func DiscoverLocalMailDelivery(env []string) (MailDelivery, error) {
	values := make(map[string]string, len(env))
	for _, entry := range env {
		if key, value, ok := strings.Cut(entry, "="); ok {
			values[key] = value
		}
	}
	spool := values["MAILX_SPOOL"]
	mailbox := values["MAIL"]
	owner := values["LOGNAME"]
	if owner == "" {
		owner = values["USER"]
	}
	if spool == "" && mailbox == "" {
		return nil, ErrMailDeliveryUnsupported
	}
	return func(recipient string, content []byte) error {
		if !localMailAddress(recipient) {
			return fmt.Errorf("invalid local mail recipient %q", recipient)
		}
		path := ""
		if recipient == owner {
			path = mailbox
		}
		if path == "" && spool != "" {
			path = filepath.Join(spool, recipient)
		}
		if path == "" {
			return ErrMailDeliveryUnsupported
		}
		now := time.Now().UTC()
		msg := &mailxpkg.Message{
			Headers: []mailxpkg.Header{
				{Name: "Date", Value: now.Format("Mon, 02 Jan 2006 15:04:05 -0700")},
				{Name: "From", Value: "scheduler"},
				{Name: "To", Value: recipient},
				{Name: "Subject", Value: "scheduled job output"},
			},
			Body: append([]byte(nil), content...),
		}
		return mailxpkg.AppendMbox(path, "scheduler", now, msg)
	}, nil
}

func localMailAddress(value string) bool {
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, "@!%/:\\\r\n") {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') &&
			r != '.' && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

// SendmailDelivery builds a delivery provider around an already trusted MTA
// executable path. Recipient is an argv element, never shell text.
func SendmailDelivery(path string) MailDelivery {
	return func(recipient string, content []byte) error {
		if recipient == "" || strings.ContainsAny(recipient, "\r\n") {
			return errors.New("invalid empty or multiline mail recipient")
		}
		var message bytes.Buffer
		fmt.Fprintf(&message, "To: %s\nSubject: scheduled job output\n\n", recipient)
		message.Write(content)
		cmd := exec.Command(path, "-i", "--", recipient)
		cmd.Stdin = &message
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("sendmail: %w: %s", err, strings.TrimSpace(string(output)))
		}
		return nil
	}
}
