package schedule

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/lockfile"
	"github.com/spf13/cobra"
)

// The scheduler as a supervised background service, so the outpost bashy-service
// supervisor can keep `schedule daemon` alive across reboots (the same shape loom
// and `sdlc service` use). The supervisor drives `bashy schedule {start,status,
// stop}`; the foreground loop those launch is `schedule daemon`. status prints
// "running" / "stopped" — the exact token outpost's bashyServiceRunning greps for.

// ServiceStatus is the daemon's lifecycle state.
type ServiceStatus struct {
	Running bool   `json:"running"`
	PID     int    `json:"pid,omitempty"`
	PidFile string `json:"pid_file"`
}

// servicePidPath is the daemon pidfile, kept next to the schedule JSON store so
// it follows the same $BASHY_SCHEDULE_STATE / UserConfigDir resolution.
func servicePidPath() string {
	return filepath.Join(filepath.Dir(statePath()), "schedule-daemon.pid")
}

func writePid(path string, pid int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".schedule-daemon-pid-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(strconv.Itoa(pid)); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func readPid(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(b)))
}

// ServiceStatusOf reports whether the daemon is running (pidfile + live process).
func ServiceStatusOf() ServiceStatus {
	p := servicePidPath()
	st := ServiceStatus{PidFile: p}
	pid, err := readPid(p)
	if err != nil || pid <= 0 {
		return st
	}
	if processAlive(pid) {
		st.Running, st.PID = true, pid
	}
	return st
}

var (
	serviceExecutable = os.Executable
	serviceCommand    = exec.Command
)

func serviceStartLockPath() string {
	return filepath.Join(filepath.Dir(statePath()), "schedule-daemon-start.lock")
}

// claimDaemonPID publishes the foreground daemon under the same lifecycle
// lock used by StartService. A child launched by StartService sees its own PID
// already published; an independently launched second daemon is refused.
func claimDaemonPID() (pidPath string, retErr error) {
	startLock, err := lockfile.Acquire(serviceStartLockPath(), lockfile.Holder{
		Name: "bashy-schedule", Intent: "publish schedule daemon pid",
	})
	if err != nil {
		return "", err
	}
	defer func() { retErr = errors.Join(retErr, startLock.Release()) }()

	p := servicePidPath()
	if pid, err := readPid(p); err == nil && pid > 0 && pid != os.Getpid() && processAlive(pid) {
		return "", fmt.Errorf("schedule daemon already running as pid %d", pid)
	}
	if err := writePid(p, os.Getpid()); err != nil {
		return "", err
	}
	return p, nil
}

// StartService launches `schedule daemon` detached in the background. The
// start lock covers status inspection, stale-pid cleanup, spawn, and atomic
// pidfile publication, so concurrent processes cannot both become the managed
// daemon.
func StartService(interval time.Duration) (status ServiceStatus, retErr error) {
	startLock, err := lockfile.Acquire(serviceStartLockPath(), lockfile.Holder{
		Name: "bashy-schedule", Intent: "start schedule daemon",
	})
	if err != nil {
		return ServiceStatus{}, err
	}
	defer func() { retErr = errors.Join(retErr, startLock.Release()) }()

	if st := ServiceStatusOf(); st.Running {
		return st, nil
	}
	// A dead, malformed, or partially published pidfile carries no authority.
	// Remove it while holding the start lock before publishing a replacement.
	p := servicePidPath()
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return ServiceStatus{}, err
	}

	exe, err := serviceExecutable()
	if err != nil {
		return ServiceStatus{}, err
	}
	args := []string{"schedule", "daemon"}
	if interval > 0 {
		args = append(args, "--interval", interval.String())
	}
	cmd := serviceCommand(exe, args...)
	applyBackgroundProcAttrs(cmd)
	if err := cmd.Start(); err != nil {
		return ServiceStatus{}, err
	}
	pid := cmd.Process.Pid
	if err := writePid(p, pid); err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return ServiceStatus{}, err
	}
	_ = cmd.Process.Release()
	return ServiceStatus{Running: true, PID: pid, PidFile: p}, nil
}

// StopService signals the daemon's process group to stop and clears the pidfile.
func StopService() ServiceStatus {
	p := servicePidPath()
	startLock, err := lockfile.Acquire(serviceStartLockPath(), lockfile.Holder{
		Name: "bashy-schedule", Intent: "stop schedule daemon",
	})
	if err != nil {
		return ServiceStatus{PidFile: p}
	}
	defer startLock.Release()
	if pid, err := readPid(p); err == nil && pid > 0 && processAlive(pid) {
		_ = signalStop(pid)
	}
	_ = os.Remove(p)
	return ServiceStatus{PidFile: p}
}

// printServiceStatus prints a line containing "running" or "stopped" — the token
// outpost's bashyServiceRunning parses.
func printServiceStatus(w io.Writer, st ServiceStatus, asJSON bool, action string) {
	if asJSON {
		b, _ := json.Marshal(st)
		fmt.Fprintln(w, string(b))
		return
	}
	state := "stopped"
	if st.Running {
		state = "running"
	}
	if action != "" {
		fmt.Fprintf(w, "schedule daemon %s: %s (pid=%d)\n", action, state, st.PID)
	} else {
		fmt.Fprintf(w, "%s (pid=%d)\n", state, st.PID)
	}
}

func startCmd() *cobra.Command {
	var interval time.Duration
	var asJSON bool
	c := &cobra.Command{
		Use:   "start",
		Short: "Start the scheduler daemon in the background (supervised service)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := StartService(interval)
			if err != nil {
				return err
			}
			printServiceStatus(cmd.OutOrStdout(), st, asJSON, "started")
			return nil
		},
	}
	c.Flags().DurationVar(&interval, "interval", time.Minute, "daemon tick interval")
	c.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return c
}

func statusCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "status",
		Short: "Report scheduler daemon status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			printServiceStatus(cmd.OutOrStdout(), ServiceStatusOf(), asJSON, "")
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return c
}

func stopServiceCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "stop",
		Short: "Stop the scheduler daemon",
		RunE: func(cmd *cobra.Command, _ []string) error {
			printServiceStatus(cmd.OutOrStdout(), StopService(), asJSON, "stopped")
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return c
}
