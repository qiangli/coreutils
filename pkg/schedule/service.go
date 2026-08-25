package schedule

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/lockfile"
	"github.com/spf13/cobra"
)

// The pid publication is metadata only. A daemon is authoritative exclusively
// while it holds the unique kernel-backed lease named by that publication.
type ServiceStatus struct {
	Running bool   `json:"running"`
	PID     int    `json:"pid,omitempty"`
	PidFile string `json:"pid_file"`
}

type daemonIdentity struct {
	PID   int    `json:"pid"`
	Lease string `json:"lease"`
}

func servicePidPath() string { return filepath.Join(filepath.Dir(statePath()), "schedule-daemon.pid") }
func serviceStartLockPath() string {
	return filepath.Join(filepath.Dir(statePath()), "schedule-daemon-start.lock")
}

func serviceElectionLockPath() string {
	return filepath.Join(filepath.Dir(statePath()), "schedule-daemon-lease.lock")
}

func serviceStopPath(identity daemonIdentity) string {
	return filepath.Join(filepath.Dir(statePath()), strings.TrimSuffix(identity.Lease, ".lock")+".stop")
}

func writeIdentity(path string, identity daemonIdentity) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(identity)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".schedule-daemon-pid-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func readIdentity(path string) (daemonIdentity, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return daemonIdentity{}, err
	}
	var identity daemonIdentity
	if err := json.Unmarshal(b, &identity); err != nil {
		return daemonIdentity{}, err
	}
	if identity.PID <= 0 || !validLeaseName(identity.Lease) {
		return daemonIdentity{}, errors.New("invalid schedule daemon identity")
	}
	return identity, nil
}

func validLeaseName(name string) bool {
	return strings.HasPrefix(name, "schedule-daemon-lease-") &&
		strings.HasSuffix(name, ".lock") && filepath.Base(name) == name
}

func leasePath(identity daemonIdentity) string {
	return filepath.Join(filepath.Dir(statePath()), identity.Lease)
}

// identityIsLive proves identity using the kernel lease. PID liveness alone is
// deliberately irrelevant: a stale pidfile may name a reused, unrelated PID.
func identityIsLive(identity daemonIdentity) bool {
	if identity.PID <= 0 || !validLeaseName(identity.Lease) {
		return false
	}
	electionOwner, elected := lockfile.Owner(serviceElectionLockPath())
	identityOwner, held := lockfile.Owner(leasePath(identity))
	if !elected || !held {
		return false
	}
	// Both independently authoritative kernel locks must carry the exact same
	// unguessable generation and process identity as the atomic publication.
	// A held global election lock plus an unrelated stale per-generation lock
	// is not proof that the published PID is the daemon.
	return electionOwner.Name == "bashy-schedule-daemon" &&
		electionOwner.PID == identity.PID && electionOwner.Intent == identity.Lease &&
		identityOwner.Name == electionOwner.Name &&
		identityOwner.PID == electionOwner.PID && identityOwner.Intent == electionOwner.Intent
}

func serviceStatusLocked() (ServiceStatus, daemonIdentity) {
	p := servicePidPath()
	st := ServiceStatus{PidFile: p}
	identity, err := readIdentity(p)
	if err == nil && identityIsLive(identity) {
		st.Running, st.PID = true, identity.PID
		return st, identity
	}
	return st, identity
}

func ServiceStatusOf() ServiceStatus {
	st, _ := serviceStatusLocked()
	return st
}

func newLeaseName() (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return "schedule-daemon-lease-" + hex.EncodeToString(token[:]) + ".lock", nil
}

// claimDaemonLease acquires the authority held for the daemon's full lifetime,
// then atomically publishes the unique lease name and PID.
type daemonLease struct {
	election *lockfile.Lock
	identity *lockfile.Lock
}

func (l *daemonLease) Release() error {
	if l == nil {
		return nil
	}
	return errors.Join(l.identity.Release(), l.election.Release())
}

func claimDaemonLease() (*daemonLease, daemonIdentity, error) {
	name, err := newLeaseName()
	if err != nil {
		return nil, daemonIdentity{}, err
	}
	identity := daemonIdentity{PID: os.Getpid(), Lease: name}
	election, err := lockfile.TryAcquire(serviceElectionLockPath(), lockfile.Holder{
		Name: "bashy-schedule-daemon", PID: identity.PID, Intent: identity.Lease,
	})
	if err != nil {
		return nil, daemonIdentity{}, fmt.Errorf("schedule daemon already running: %w", err)
	}
	identityLease, err := lockfile.TryAcquire(leasePath(identity), lockfile.Holder{
		Name: "bashy-schedule-daemon", PID: identity.PID, Intent: name,
	})
	if err != nil {
		_ = election.Release()
		return nil, daemonIdentity{}, err
	}
	lease := &daemonLease{election: election, identity: identityLease}
	if err := writeIdentity(servicePidPath(), identity); err != nil {
		_ = lease.Release()
		_ = os.Remove(leasePath(identity))
		return nil, daemonIdentity{}, err
	}
	_ = os.Remove(serviceStopPath(identity))
	return lease, identity, nil
}

func requestDaemonStop(identity daemonIdentity) error {
	if !validLeaseName(identity.Lease) {
		return errors.New("invalid schedule daemon identity")
	}
	return os.WriteFile(serviceStopPath(identity), []byte(identity.Lease+"\n"), 0o600)
}

func daemonStopRequested(identity daemonIdentity) bool {
	b, err := os.ReadFile(serviceStopPath(identity))
	return err == nil && strings.TrimSpace(string(b)) == identity.Lease
}

type serviceCommandFactory func(executable string, args ...string) *exec.Cmd

const (
	serviceReadyTimeout = 5 * time.Second
	serviceStopTimeout  = 2 * time.Second
)

func StartService(interval time.Duration) (ServiceStatus, error) {
	return startService(interval, exec.Command)
}

// startService does not report readiness until the child owns and publishes
// its kernel lease. The injected command factory is immutable and test-local.
func startService(interval time.Duration, command serviceCommandFactory) (status ServiceStatus, retErr error) {
	startLock, err := lockfile.Acquire(serviceStartLockPath(), lockfile.Holder{
		Name: "bashy-schedule", Intent: "start schedule daemon",
	})
	if err != nil {
		return ServiceStatus{}, err
	}
	defer func() { retErr = errors.Join(retErr, startLock.Release()) }()

	if st, stale := serviceStatusLocked(); st.Running {
		return st, nil
	} else if validLeaseName(stale.Lease) {
		_ = os.Remove(leasePath(stale))
	}
	p := servicePidPath()
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return ServiceStatus{}, err
	}

	exe, err := os.Executable()
	if err != nil {
		return ServiceStatus{}, err
	}
	args := []string{"schedule", "daemon"}
	if interval > 0 {
		args = append(args, "--interval", interval.String())
	}
	cmd := command(exe, args...)
	applyBackgroundProcAttrs(cmd)
	if err := cmd.Start(); err != nil {
		return ServiceStatus{}, err
	}
	pid := cmd.Process.Pid
	deadline := time.Now().Add(serviceReadyTimeout)
	for time.Now().Before(deadline) {
		identity, readErr := readIdentity(p)
		if readErr == nil && identity.PID == pid && identityIsLive(identity) {
			_ = cmd.Process.Release()
			return ServiceStatus{Running: true, PID: pid, PidFile: p}, nil
		}
		if readErr == nil && identity.PID != pid && identityIsLive(identity) {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
			return ServiceStatus{Running: true, PID: identity.PID, PidFile: p}, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
	if identity, err := readIdentity(p); err == nil && identity.PID == pid && !identityIsLive(identity) {
		_ = os.Remove(p)
		_ = os.Remove(leasePath(identity))
	}
	return ServiceStatus{}, fmt.Errorf("schedule daemon pid %d did not publish its lifetime lease", pid)
}

func waitLeaseRelease(identity daemonIdentity, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for identityIsLive(identity) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	return !identityIsLive(identity)
}

// StopService never signals a numeric PID. PID validation followed by kill(2)
// has an unavoidable exit/reuse race, so stop uses the daemon's unguessable
// lifetime generation as a cooperative request and waits for that exact pair
// of kernel leases to be released. An unresponsive daemon remains reported as
// running; an unrelated process can never be killed as an escalation fallback.
func StopService() ServiceStatus {
	return stopService(serviceStopTimeout)
}

func stopService(timeout time.Duration) ServiceStatus {
	p := servicePidPath()
	startLock, err := lockfile.Acquire(serviceStartLockPath(), lockfile.Holder{
		Name: "bashy-schedule", Intent: "stop schedule daemon",
	})
	if err != nil {
		return ServiceStatus{PidFile: p}
	}
	defer startLock.Release()

	st, identity := serviceStatusLocked()
	if !st.Running {
		_ = os.Remove(p)
		if validLeaseName(identity.Lease) {
			_ = os.Remove(leasePath(identity))
		}
		return ServiceStatus{PidFile: p}
	}
	if err := requestDaemonStop(identity); err != nil {
		return ServiceStatus{Running: true, PID: identity.PID, PidFile: p}
	}
	if !waitLeaseRelease(identity, timeout) {
		return ServiceStatus{Running: true, PID: identity.PID, PidFile: p}
	}
	if current, err := readIdentity(p); err == nil && current == identity {
		_ = os.Remove(p)
	}
	_ = os.Remove(leasePath(identity))
	_ = os.Remove(serviceStopPath(identity))
	return ServiceStatus{PidFile: p}
}

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
	c := &cobra.Command{Use: "start", Short: "Start the scheduler daemon in the background (supervised service)", RunE: func(cmd *cobra.Command, _ []string) error {
		st, err := StartService(interval)
		if err != nil {
			return err
		}
		printServiceStatus(cmd.OutOrStdout(), st, asJSON, "started")
		return nil
	}}
	c.Flags().DurationVar(&interval, "interval", time.Minute, "daemon tick interval")
	c.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return c
}

func statusCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{Use: "status", Short: "Report scheduler daemon status", RunE: func(cmd *cobra.Command, _ []string) error {
		printServiceStatus(cmd.OutOrStdout(), ServiceStatusOf(), asJSON, "")
		return nil
	}}
	c.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return c
}

func stopServiceCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{Use: "stop", Short: "Stop the scheduler daemon", RunE: func(cmd *cobra.Command, _ []string) error {
		printServiceStatus(cmd.OutOrStdout(), StopService(), asJSON, "stopped")
		return nil
	}}
	c.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return c
}
