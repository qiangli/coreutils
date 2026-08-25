// Package batchcmd implements batch(1): schedule a command to run when system
// load permits.
package batchcmd

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"time"

	posixlocale "github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/pkg/schedule"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "batch",
	Synopsis: "Schedule a command to run as soon as possible.",
	Usage:    "batch",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) != 0 {
		return tool.UsageError(rc, cmd, "batch does not accept operands")
	}
	if code := checkBatchAccess(rc); code != 0 {
		return code
	}
	identity, err := schedule.AuthenticatedIdentity()
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", cmd.Name, err)
		return 1
	}

	cwd := rc.Dir
	if cwd == "" {
		return tool.UsageError(rc, cmd, "invocation working directory is required")
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rc.In); err != nil {
		return tool.UsageError(rc, cmd, "cannot read stdin: %v", err)
	}
	cmdText := buf.String()

	loc, err := batchLocation(rc.Getenv("TZ"))
	if err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
	}
	now := time.Now().In(loc)
	when := now
	id := strconv.FormatInt(now.UnixNano(), 36)
	shell := rc.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
	}
	j := &schedule.Job{
		ID:             id,
		Kind:           "at",
		Spec:           when.Format(time.RFC3339),
		Command:        []string{shell, "-c", cmdText},
		Dir:            cwd,
		Queue:          "b",
		Env:            append([]string(nil), rc.Env...),
		EnvSet:         true,
		Enabled:        true,
		MailOutput:     true,
		MailCompletion: true,
		MailTo:         identity.Name,
		OwnerUID:       identity.UID,
		OwnerName:      identity.Name,
		BatchLoad:      true,
		CreatedAt:      now,
		NextRun:        when,
	}
	if rc.UmaskSet {
		j.Umask, j.UmaskSet = uint32(rc.Umask.Perm()), true
	} else if mask, ok := processUmask(); ok {
		j.Umask, j.UmaskSet = mask, true
	}

	formatted, err := formatJobTime(rc, when)
	if err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
	}
	if err := schedule.StoreFor(rc.Dir, rc.Env).SubmitJobWithConfirmation(j, func() error {
		_, err := fmt.Fprintf(rc.Err, "job %s at %s\n", id, formatted)
		return err
	}); err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot submit job: %v\n", cmd.Name, err)
		return 1
	}
	return 0
}

func formatJobTime(rc *tool.RunContext, t time.Time) (string, error) {
	loc, err := batchLocation(rc.Getenv("TZ"))
	if err != nil {
		return "", err
	}
	formatter, err := posixlocale.ResolveTime(rc.Env)
	if err != nil {
		return "", err
	}
	return formatter.FormatAtJobTime(t.In(loc)), nil
}
