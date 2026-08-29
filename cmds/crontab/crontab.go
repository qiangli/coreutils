// Package crontabcmd implements the POSIX crontab interface over pkg/schedule.
package crontabcmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/schedule"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{Name: "crontab", Synopsis: "Manage per-user cron tables (via bashy schedule store).", Usage: "crontab -l\n   or: crontab -e\n   or: crontab -r\n   or: crontab [-u USER] [FILE]"}

type cronIdentity struct{ name, home string }
type runConfig struct {
	accessDirs  []string
	currentUser func() (cronIdentity, error)
	euid        func() int
	runEditor   func(*tool.RunContext, string, string) error
}

func defaultRunConfig() runConfig {
	return runConfig{
		accessDirs: defaultCronAccessDirs(),
		currentUser: func() (cronIdentity, error) {
			u, err := user.Current()
			if err != nil {
				return cronIdentity{}, err
			}
			return cronIdentity{name: u.Username, home: u.HomeDir}, nil
		},
		euid: effectiveUID,
		runEditor: func(rc *tool.RunContext, editor, path string) error {
			editorPath := rc.ResolveCommand(editor)
			if editorPath == "" {
				return fmt.Errorf("editor %q was not found in invocation PATH", editor)
			}
			ctx := rc.Ctx
			if ctx == nil {
				ctx = context.Background()
			}
			ec := exec.CommandContext(ctx, editorPath, path)
			ec.Dir, ec.Env = rc.Dir, append([]string(nil), rc.Env...)
			ec.Stdin, ec.Stdout, ec.Stderr = rc.In, rc.Out, rc.Err
			return ec.Run()
		},
	}
}

func init() {
	cmd.Run = func(rc *tool.RunContext, args []string) int { return runWithConfig(rc, args, defaultRunConfig()) }
	tool.Register(cmd)
}
func runWithConfig(rc *tool.RunContext, args []string, cfg runConfig) int {
	fs := tool.NewFlags(cmd.Name)
	listFlag := fs.BoolP("list", "l", false, "list the current crontab")
	editFlag := fs.BoolP("edit", "e", false, "edit the current crontab")
	removeFlag := fs.BoolP("remove", "r", false, "remove the current crontab")
	fs.StringP("user", "u", "", "user whose crontab to operate on")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if fs.Changed("user") {
		return tool.NotSupported(rc, cmd, "operating on another user's crontab with -u")
	}
	modes := 0
	for _, set := range []bool{*listFlag, *editFlag, *removeFlag} {
		if set {
			modes++
		}
	}
	if modes > 1 {
		return tool.UsageError(rc, cmd, "options -e, -l, and -r are mutually exclusive")
	}
	if modes == 1 && len(operands) != 0 {
		return tool.UsageError(rc, cmd, "options -e, -l, and -r do not accept an operand")
	}
	if modes == 0 && len(operands) > 1 {
		return tool.UsageError(rc, cmd, "extra operand %q", operands[1])
	}
	identity, err := cfg.currentUser()
	if err != nil || identity.name == "" || identity.home == "" {
		fmt.Fprintf(rc.Err, "%s: cannot determine current user and home directory\n", cmd.Name)
		return 1
	}
	if code := checkCronAccess(rc, cfg, identity.name); code != 0 {
		return code
	}
	store := schedule.StoreFor(rc.Dir, rc.Env)
	switch {
	case *listFlag:
		return listCron(rc, store)
	case *editFlag:
		return editCron(rc, store, identity, cfg.runEditor)
	case *removeFlag:
		return removeCron(rc, store)
	default:
		return replaceCron(rc, store, operands, identity)
	}
}

func listCron(rc *tool.RunContext, store *schedule.Store) int {
	source, _, err := store.CronTable()
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot load schedule: %v\n", cmd.Name, err)
		return 1
	}
	if len(source) > 0 {
		if _, err := rc.Out.Write(source); err != nil {
			fmt.Fprintf(rc.Err, "%s: cannot write table: %v\n", cmd.Name, err)
			return 1
		}
	}
	return 0
}

func editCron(rc *tool.RunContext, store *schedule.Store, identity cronIdentity, runEditor func(*tool.RunContext, string, string) error) int {
	content, exists, err := store.CronTable()
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot load schedule: %v\n", cmd.Name, err)
		return 1
	}
	if !exists {
		content = []byte("# Edit this file to introduce cron jobs.\n#\n# Each line: MIN HOUR DOM MON DOW  command\n")
	}
	if rc.Dir == "" {
		fmt.Fprintf(rc.Err, "%s: invocation working directory is required for editing\n", cmd.Name)
		return 1
	}
	tmp, err := os.CreateTemp(rc.Dir, ".crontab-*.txt")
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot create temp file: %v\n", cmd.Name, err)
		return 1
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		fmt.Fprintf(rc.Err, "%s: cannot write temp file: %v\n", cmd.Name, err)
		return 1
	}
	if err := tmp.Close(); err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot close temp file: %v\n", cmd.Name, err)
		return 1
	}
	editor := rc.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	if err := runEditor(rc, editor, tmpPath); err != nil {
		fmt.Fprintf(rc.Err, "%s: editor returned error: %v\n", cmd.Name, err)
		return 1
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot read edited file: %v\n", cmd.Name, err)
		return 1
	}
	return installCronTable(rc, store, data, identity)
}

func removeCron(rc *tool.RunContext, store *schedule.Store) int {
	if err := store.RemoveCron(); err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot save schedule: %v\n", cmd.Name, err)
		return 1
	}
	return 0
}

func replaceCron(rc *tool.RunContext, store *schedule.Store, operands []string, identity cronIdentity) int {
	var data []byte
	var err error
	if len(operands) == 0 {
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, rc.In); err != nil {
			return tool.UsageError(rc, cmd, "cannot read stdin: %v", err)
		}
		data = buf.Bytes()
	} else {
		if rc.Dir == "" && !filepath.IsAbs(operands[0]) {
			return tool.UsageError(rc, cmd, "invocation working directory is required to resolve %q", operands[0])
		}
		data, err = os.ReadFile(rc.Path(operands[0]))
		if err != nil {
			return tool.UsageError(rc, cmd, "cannot read %q: %v", operands[0], err)
		}
	}
	return installCronTable(rc, store, data, identity)
}

func installCronTable(rc *tool.RunContext, store *schedule.Store, source []byte, identity cronIdentity) int {
	defaultShell, err := cronShellPath()
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", cmd.Name, err)
		return 1
	}
	baseEnv := []string{"HOME=" + identity.home, "LOGNAME=" + identity.name, "PATH=" + cronDefaultPATH(), "SHELL=" + defaultShell}
	newJobs, errs := parseCronTab(source, baseEnv, identity)
	if len(errs) > 0 {
		for _, parseErr := range errs {
			fmt.Fprintf(rc.Err, "%s: %v\n", cmd.Name, parseErr)
		}
		return 1
	}
	now := time.Now()
	for _, job := range newJobs {
		job.CreatedAt = now
		next, nextErr := schedule.ComputeNext(job, now)
		if nextErr != nil {
			fmt.Fprintf(rc.Err, "%s: cannot compute next run for %q: %v\n", cmd.Name, job.Spec, nextErr)
			return 1
		}
		job.NextRun = next
	}
	if err := store.ReplaceCron(source, newJobs); err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot save schedule: %v\n", cmd.Name, err)
		return 1
	}
	return 0
}

func parseCronTab(source []byte, baseEnv []string, identity cronIdentity) ([]*schedule.Job, []error) {
	var jobs []*schedule.Job
	var errs []error
	env := append([]string(nil), baseEnv...)
	lines := bytes.Split(source, []byte{'\n'})
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	for index, raw := range lines {
		lineNo := index + 1
		line := strings.TrimSuffix(string(raw), "\r")
		left := strings.TrimLeft(line, " \t")
		if left == "" || strings.HasPrefix(left, "#") {
			continue
		}
		if name, value, ok := splitEnvironmentAssignment(left); ok {
			if name == "SHELL" {
				if err := validateCronShell(value); err != nil {
					errs = append(errs, fmt.Errorf("line %d: invalid SHELL: %v", lineNo, err))
					continue
				}
			}
			env = setEnvironment(env, name, value)
			continue
		}
		spec, rawCommand, ok := splitCronLine(left)
		if !ok {
			errs = append(errs, fmt.Errorf("line %d: not enough fields (need 5 cron fields + command)", lineNo))
			continue
		}
		command, stdin := splitCronCommand(rawCommand)
		jobs = append(jobs, &schedule.Job{
			ID: strconv.FormatInt(time.Now().UnixNano()+int64(lineNo), 36), Kind: "cron", Spec: spec,
			Command: []string{environmentValue(env, "SHELL"), "-c", command}, Dir: identity.home,
			Stdin: stdin, StdinSet: true, Env: append([]string(nil), env...), EnvSet: true,
			Umask: 0o022, UmaskSet: true, POSIXCron: true, MailOutput: true, MailTo: identity.name, Enabled: true,
		})
	}
	return jobs, errs
}

func splitEnvironmentAssignment(line string) (name, value string, ok bool) {
	eq := strings.IndexByte(line, '=')
	if eq <= 0 {
		return "", "", false
	}
	name = line[:eq]
	if !isEnvironmentName(name) {
		return "", "", false
	}
	return name, line[eq+1:], true
}
func isEnvironmentName(name string) bool {
	for i, r := range name {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return name != ""
}
func setEnvironment(env []string, name, value string) []string {
	prefix := name + "="
	out := env[:0]
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			out = append(out, entry)
		}
	}
	return append(out, prefix+value)
}
func environmentValue(env []string, name string) string {
	prefix := name + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return env[i][len(prefix):]
		}
	}
	return ""
}

func splitCronCommand(raw string) (command, stdin string) {
	var translated strings.Builder
	translated.Grow(len(raw))
	for i := 0; i < len(raw); i++ {
		// A backslash escapes only a following percent-sign. Before any other
		// byte it is command data the shell must still receive, so it passes
		// through unchanged (a stripped backslash would rewrite constructs
		// such as printf 'a\tb').
		if raw[i] == '\\' && i+1 < len(raw) && raw[i+1] == '%' {
			i++
			translated.WriteByte('%')
			continue
		}
		if raw[i] == '%' {
			translated.WriteByte('\n')
			continue
		}
		translated.WriteByte(raw[i])
	}
	value := translated.String()
	if index := strings.IndexByte(value, '\n'); index >= 0 {
		// The command is parsed from a crontab text line with its record
		// terminator removed.  Once an unescaped percent introduces standard
		// input, that terminator belongs to the input data too: cron supplies
		// a complete final line, not an unterminated byte sequence.
		return value[:index], value[index+1:] + "\n"
	}
	return value, ""
}

func checkCronAccess(rc *tool.RunContext, cfg runConfig, username string) int {
	for _, dir := range cfg.accessDirs {
		allow, deny := filepath.Join(dir, "cron.allow"), filepath.Join(dir, "cron.deny")
		if _, err := os.Stat(allow); err == nil {
			permitted, policyErr := accessFileContains(allow, username)
			if policyErr != nil || !permitted {
				fmt.Fprintf(rc.Err, "%s: user %s is not authorized\n", cmd.Name, username)
				return 1
			}
			return 0
		} else if !os.IsNotExist(err) {
			fmt.Fprintf(rc.Err, "%s: user %s is not authorized\n", cmd.Name, username)
			return 1
		}
		if _, err := os.Stat(deny); err == nil {
			denied, policyErr := accessFileContains(deny, username)
			if policyErr != nil || denied {
				fmt.Fprintf(rc.Err, "%s: user %s is not authorized\n", cmd.Name, username)
				return 1
			}
			return 0
		} else if !os.IsNotExist(err) {
			fmt.Fprintf(rc.Err, "%s: user %s is not authorized\n", cmd.Name, username)
			return 1
		}
	}
	if cfg.euid() == 0 {
		return 0
	}
	fmt.Fprintf(rc.Err, "%s: user %s is not authorized\n", cmd.Name, username)
	return 1
}

func accessFileContains(path, username string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	found := false
	lines := bytes.Split(data, []byte{'\n'})
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	for index, raw := range lines {
		line := string(raw)
		if line == "" || strings.ContainsAny(line, " \t\r\v\f") {
			return false, fmt.Errorf("malformed policy line %d", index+1)
		}
		if line == username {
			found = true
		}
	}
	return found, nil
}

func splitCronLine(line string) (spec, command string, ok bool) {
	var fields [5]string
	rest := line
	for i := range fields {
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" {
			return "", "", false
		}
		index := strings.IndexAny(rest, " \t")
		if index < 0 {
			return "", "", false
		}
		fields[i], rest = rest[:index], rest[index:]
	}
	rest = strings.TrimLeft(rest, " \t")
	if rest == "" {
		return "", "", false
	}
	return strings.Join(fields[:], " "), rest, true
}
