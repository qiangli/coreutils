//go:build unix

package mesgcmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/cmds/internal/session"
	whodb "github.com/qiangli/coreutils/pkg/who"
	"github.com/qiangli/coreutils/tool"
	"golang.org/x/term"
)

// defaultTTYName resolves the first terminal attached to standard input,
// standard output, or standard error, in that order, as POSIX mesg requires.
func defaultTTYName(rc *tool.RunContext) (string, error) {
	// RunContext streams are the command's standard streams. Reading the
	// process globals here would inspect the embedding shell instead, and a
	// character-device check alone would misclassify /dev/null as a terminal.
	for _, stream := range []any{rc.In, rc.Out, rc.Err} {
		f, ok := stream.(*os.File)
		if !ok || !term.IsTerminal(int(f.Fd())) {
			continue
		}
		if name, err := ttyPath(f); err == nil && name != "" {
			return name, nil
		}
	}
	if name, ok := agentTTYName(rc.Env); ok {
		return name, nil
	}
	return "", errors.New("not a terminal")
}

func agentTTYName(env []string) (string, bool) {
	if filepath.Clean(session.DefaultFileForEnv(env)) != filepath.Clean(whodb.FileForEnv(env)) {
		return "", false
	}
	id := ""
	for _, key := range []string{"BASHY_AGENT_ID", "WEAVE_AGENT", "BASHY_AGENT"} {
		if v, ok := envLookup(env, key); ok && strings.TrimSpace(v) != "" {
			id = strings.TrimSpace(v)
			break
		}
	}
	if id == "" {
		return "", false
	}
	records, err := session.ReadEnv(whodb.FileForEnv(env), env)
	if err != nil {
		return "", false
	}
	for _, r := range records {
		if session.IsUser(r) && r.User == id && r.TTY != "" {
			return filepath.Join(whodb.PTYDirForEnv(env), filepath.FromSlash(r.TTY)), true
		}
	}
	return "", false
}

func envLookup(env []string, key string) (string, bool) {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return env[i][len(prefix):], true
		}
	}
	return "", false
}
