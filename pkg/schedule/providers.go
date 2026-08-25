package schedule

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
