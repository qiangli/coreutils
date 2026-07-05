package schedule

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o644)
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

// StartService launches `schedule daemon` detached in the background. Idempotent:
// a no-op when the daemon is already running (so the supervisor's 30s restart poll
// never spawns a duplicate).
func StartService(interval time.Duration) (ServiceStatus, error) {
	if st := ServiceStatusOf(); st.Running {
		return st, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return ServiceStatus{}, err
	}
	args := []string{"schedule", "daemon"}
	if interval > 0 {
		args = append(args, "--interval", interval.String())
	}
	cmd := exec.Command(exe, args...)
	applyBackgroundProcAttrs(cmd)
	if err := cmd.Start(); err != nil {
		return ServiceStatus{}, err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	p := servicePidPath()
	if err := writePid(p, pid); err != nil {
		return ServiceStatus{}, err
	}
	return ServiceStatus{Running: true, PID: pid, PidFile: p}, nil
}

// StopService signals the daemon's process group to stop and clears the pidfile.
func StopService() ServiceStatus {
	p := servicePidPath()
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
