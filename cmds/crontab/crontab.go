// Package crontabcmd implements crontab(1): manage per-user cron tables
// backed by the bashy schedule store. Each line is a standard 5-field
// cron expression followed by a command; the round-trip is idempotent
// (what -l prints can be reinstalled via crontab - or crontab FILE).
//
//	crontab -l          — list the current table
//	crontab -e          — edit the table in $EDITOR
//	crontab -r          — remove all cron entries
//	crontab [-u USER] FILE  — install a new table from FILE
//	crontab [-u USER] -     — install a new table from stdin
package crontabcmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/schedule"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "crontab",
	Synopsis: "Manage per-user cron tables (via bashy schedule store).",
	Usage: "crontab -l\n" +
		"   or: crontab -e\n" +
		"   or: crontab -r\n" +
		"   or: crontab [-u USER] FILE\n" +
		"   or: crontab [-u USER] -",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	listFlag := fs.BoolP("list", "l", false, "list the current crontab")
	editFlag := fs.BoolP("edit", "e", false, "edit the current crontab")
	removeFlag := fs.BoolP("remove", "r", false, "remove the current crontab")
	userFlag := fs.StringP("user", "u", "", "user whose crontab to operate on")

	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	_ = userFlag

	switch {
	case *listFlag:
		return listCron(rc)
	case *editFlag:
		return editCron(rc)
	case *removeFlag:
		return removeCron(rc)
	default:
		return replaceCron(rc, operands)
	}
}

func listCron(rc *tool.RunContext) int {
	lines, err := cronLines()
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", cmd.Name, err)
		return 1
	}
	for _, l := range lines {
		fmt.Fprintln(rc.Out, l)
	}
	return 0
}

func editCron(rc *tool.RunContext) int {
	lines, err := cronLines()
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", cmd.Name, err)
		return 1
	}

	content := strings.Join(lines, "\n")
	if content == "" {
		content = "# Edit this file to introduce cron jobs.\n#\n"
		content += "# Each line: MIN HOUR DOM MON DOW  command\n"
		content += "# Example:\n#   0 9 * * *  echo hello\n"
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	tmp, err := os.CreateTemp("", "crontab-*.txt")
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot create temp file: %v\n", cmd.Name, err)
		return 1
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		fmt.Fprintf(rc.Err, "%s: cannot write temp file: %v\n", cmd.Name, err)
		return 1
	}
	tmp.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi"
	}

	ec := exec.Command(editor, tmpPath)
	ec.Stdin = os.Stdin
	ec.Stdout = os.Stdout
	ec.Stderr = os.Stderr
	if err := ec.Run(); err != nil {
		fmt.Fprintf(rc.Err, "%s: editor returned error: %v\n", cmd.Name, err)
		return 1
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot read edited file: %v\n", cmd.Name, err)
		return 1
	}
	return installCronLines(rc, string(data))
}

func removeCron(rc *tool.RunContext) int {
	jobs, err := schedule.LoadJobs()
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot load schedule: %v\n", cmd.Name, err)
		return 1
	}
	kept := jobs[:0]
	for _, j := range jobs {
		if j.Kind == "cron" {
			continue
		}
		kept = append(kept, j)
	}
	if err := schedule.SaveJobs(kept); err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot save schedule: %v\n", cmd.Name, err)
		return 1
	}
	return 0
}

func replaceCron(rc *tool.RunContext, operands []string) int {
	var data []byte
	var err error

	switch {
	case len(operands) == 0:
		// Read from stdin.
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, rc.In); err != nil {
			return tool.UsageError(rc, cmd, "cannot read stdin: %v", err)
		}
		data = buf.Bytes()
	case operands[0] == "-":
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, rc.In); err != nil {
			return tool.UsageError(rc, cmd, "cannot read stdin: %v", err)
		}
		data = buf.Bytes()
	default:
		data, err = os.ReadFile(rc.Path(operands[0]))
		if err != nil {
			return tool.UsageError(rc, cmd, "cannot read %q: %v", operands[0], err)
		}
	}

	return installCronLines(rc, string(data))
}

func installCronLines(rc *tool.RunContext, content string) int {
	newJobs, errs := parseCronTab(content)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(rc.Err, "%s: %v\n", cmd.Name, e)
		}
	}

	jobs, err := schedule.LoadJobs()
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot load schedule: %v\n", cmd.Name, err)
		return 1
	}

	// Remove all existing cron jobs, keep everything else.
	kept := jobs[:0]
	for _, j := range jobs {
		if j.Kind == "cron" {
			continue
		}
		kept = append(kept, j)
	}

	now := time.Now()
	for _, j := range newJobs {
		j.CreatedAt = now
		next, nerr := schedule.ComputeNext(j, now)
		if nerr != nil {
			fmt.Fprintf(rc.Err, "%s: cannot compute next run for %q: %v\n", cmd.Name, j.Spec, nerr)
			continue
		}
		j.NextRun = next
		kept = append(kept, j)
	}

	if err := schedule.SaveJobs(kept); err != nil {
		fmt.Fprintf(rc.Err, "%s: cannot save schedule: %v\n", cmd.Name, err)
		return 1
	}

	if len(newJobs) > 0 {
		fmt.Fprintf(rc.Out, "installed %d cron job(s)\n", len(newJobs))
	}
	return 0
}

// cronLines returns the textual representation of all cron-kind jobs,
// one 5-field-cron line per job, suitable for display or editing.
func cronLines() ([]string, error) {
	jobs, err := schedule.LoadJobs()
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, j := range jobs {
		if j.Kind != "cron" {
			continue
		}
		lines = append(lines, j.Spec+" "+strings.Join(j.Command, " "))
	}
	return lines, nil
}

// parseCronTab parses crontab content into a slice of Job objects.
// Blank lines and lines starting with # are skipped. Each active line
// must have at least 6 fields: 5 cron fields + command.
func parseCronTab(content string) ([]*schedule.Job, []error) {
	var jobs []*schedule.Job
	var errs []error
	sc := bufio.NewScanner(strings.NewReader(content))
	lineNo := 0
	cwd, _ := os.Getwd()

	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			errs = append(errs, fmt.Errorf("line %d: not enough fields (need 5 cron fields + command)", lineNo))
			continue
		}
		spec := strings.Join(fields[:5], " ")
		cmdParts := fields[5:]
		id := strconv.FormatInt(time.Now().UnixNano()+int64(lineNo), 36)

		jobs = append(jobs, &schedule.Job{
			ID:      id,
			Kind:    "cron",
			Spec:    spec,
			Command: cmdParts,
			Dir:     cwd,
			Enabled: true,
		})
	}
	if err := sc.Err(); err != nil {
		errs = append(errs, err)
	}
	return jobs, errs
}
