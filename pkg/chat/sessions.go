package chat

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// The live-instance registry moved to pkg/room — it is the host room's membership,
// shared by every launch path (chat/weave/foreman/meet), not a chat-private board.
// What stays here is only how a chat session names itself.

var idSanitize = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// sessionID is a short, human-typable handle: the label the caller would say (nick
// or tool) plus the launcher pid, unique per host. Readable enough to
// `chat steer claude-12345 "..."` without copy-pasting a hash.
func sessionID(l Launch) string {
	label := strings.TrimSpace(l.Nick)
	if label == "" || label == l.Binding() {
		label = l.ToolName
	}
	label = strings.Trim(idSanitize.ReplaceAllString(label, "-"), "-")
	if label == "" {
		label = "agent"
	}
	return fmt.Sprintf("%s-%d", label, os.Getpid())
}
