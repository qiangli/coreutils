//go:build unix

package schedule

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/lockfile"
)

func appendServiceLog(text string) {
	if path := os.Getenv("BASHY_SERVICE_START_LOG"); path != "" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fmt.Fprintln(f, text)
			_ = f.Close()
		}
	}
}

func TestManagedDaemonProcessHelper(t *testing.T) {
	if os.Getenv("BASHY_MANAGED_DAEMON_HELPER") != "1" {
		return
	}
	lease, identity, err := claimDaemonLease()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if current, readErr := readIdentity(servicePidPath()); readErr == nil && current == identity {
			_ = os.Remove(servicePidPath())
		}
		_ = lease.Release()
		_ = os.Remove(leasePath(identity))
		_ = os.Remove(serviceStopPath(identity))
	}()
	appendServiceLog("start " + strconv.Itoa(identity.PID))
	for !daemonStopRequested(identity) {
		time.Sleep(5 * time.Millisecond)
	}
	appendServiceLog("stop-start " + strconv.Itoa(identity.PID))
	if os.Getenv("BASHY_SERVICE_IGNORE_STOP") == "1" {
		select {}
	}
	if delay, err := time.ParseDuration(os.Getenv("BASHY_SERVICE_TERM_DELAY")); err == nil {
		time.Sleep(delay)
	}
	appendServiceLog("stop-end " + strconv.Itoa(identity.PID))
}

func managedDaemonCommand(string, ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestManagedDaemonProcessHelper$")
	cmd.Env = append(os.Environ(), "BASHY_MANAGED_DAEMON_HELPER=1")
	return cmd
}

func TestConcurrentStartServiceHelper(t *testing.T) {
	if os.Getenv("BASHY_SERVICE_START_HELPER") != "1" {
		return
	}
	st, err := startService(time.Minute, managedDaemonCommand)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("BASHY_SERVICE_START_RESULT"), []byte(strconv.Itoa(st.PID)), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestStartServiceIsAtomicAcrossProcessesAndReplacesStalePID(t *testing.T) {
	state := withState(t)
	dir := filepath.Dir(state)
	logPath := filepath.Join(dir, "daemon-starts")
	// A legacy integer pidfile is stale metadata, even when it happens to name
	// this live test process.
	if err := os.WriteFile(servicePidPath(), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}

	const starters = 12
	results := make([]string, starters)
	errs := make([]error, starters)
	var wg sync.WaitGroup
	for i := 0; i < starters; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			result := filepath.Join(dir, fmt.Sprintf("result-%d", i))
			cmd := exec.Command(os.Args[0], "-test.run=^TestConcurrentStartServiceHelper$")
			cmd.Env = append(os.Environ(),
				"BASHY_SERVICE_START_HELPER=1",
				"BASHY_SERVICE_START_LOG="+logPath,
				"BASHY_SERVICE_START_RESULT="+result,
				"BASHY_SCHEDULE_STATE="+state,
			)
			if output, err := cmd.CombinedOutput(); err != nil {
				errs[i] = fmt.Errorf("helper: %w: %s", err, output)
				return
			}
			b, err := os.ReadFile(result)
			if err != nil {
				errs[i] = err
				return
			}
			results[i] = strings.TrimSpace(string(b))
		}(i)
	}
	wg.Wait()
	defer StopService()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i < len(results); i++ {
		if results[i] == "" || results[i] != results[0] {
			t.Fatalf("concurrent starts returned PIDs %q", results)
		}
	}
	deadline := time.Now().Add(time.Second)
	var b []byte
	var err error
	for time.Now().Before(deadline) {
		b, err = os.ReadFile(logPath)
		if err == nil && len(strings.Split(strings.TrimSpace(string(b)), "\n")) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err != nil || len(strings.Split(strings.TrimSpace(string(b)), "\n")) != 1 {
		t.Fatalf("daemon did not publish one ready event: log=%q err=%v", b, err)
	}
	st := ServiceStatusOf()
	if !st.Running || strconv.Itoa(st.PID) != results[0] {
		t.Fatalf("managed status=%+v, results=%q", st, results)
	}
}

func TestLiveUnrelatedPIDIsNeverTrustedOrSignaled(t *testing.T) {
	withState(t)
	unrelated := exec.Command("/bin/sh", "-c", "trap 'exit 77' TERM; while :; do sleep 1; done")
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unrelated.Process.Kill(); _, _ = unrelated.Process.Wait() }()
	stale := daemonIdentity{PID: unrelated.Process.Pid, Lease: "schedule-daemon-lease-stale.lock"}
	if err := writeIdentity(servicePidPath(), stale); err != nil {
		t.Fatal(err)
	}
	if st := ServiceStatusOf(); st.Running {
		t.Fatalf("unlocked stale identity reported running: %+v", st)
	}
	if st := StopService(); st.Running {
		t.Fatalf("stale identity survived stop: %+v", st)
	}
	if err := unrelated.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("unrelated live PID was signaled: %v", err)
	}
}

func TestDaemonElectionLeaseRejectsSecondClaim(t *testing.T) {
	withState(t)
	first, identity, err := claimDaemonLease()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = first.Release()
		_ = os.Remove(servicePidPath())
		_ = os.Remove(leasePath(identity))
	}()
	if _, _, err := claimDaemonLease(); err == nil {
		t.Fatal("second daemon acquired the lifetime election lease")
	}
	if st := ServiceStatusOf(); !st.Running || st.PID != os.Getpid() {
		t.Fatalf("lease-backed status=%+v", st)
	}
}

func TestDaemonIdentityRequiresBothLeaseHoldersToMatchPublication(t *testing.T) {
	for _, tc := range []struct {
		name           string
		identityPID    int
		electionPID    int
		electionIntent string
		leasePID       int
		leaseIntent    string
	}{
		{name: "identity lease generation mismatch", identityPID: os.Getpid(), electionPID: os.Getpid(), leasePID: os.Getpid(), leaseIntent: "different-generation.lock"},
		{name: "identity lease PID mismatch", identityPID: os.Getpid(), electionPID: os.Getpid(), leasePID: os.Getpid() + 1},
		{name: "election generation mismatch", identityPID: os.Getpid(), electionPID: os.Getpid(), electionIntent: "different-generation.lock", leasePID: os.Getpid()},
		{name: "election PID mismatch", identityPID: os.Getpid(), electionPID: os.Getpid() + 1, leasePID: os.Getpid()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withState(t)
			name, err := newLeaseName()
			if err != nil {
				t.Fatal(err)
			}
			identity := daemonIdentity{PID: tc.identityPID, Lease: name}
			electionIntent := tc.electionIntent
			if electionIntent == "" {
				electionIntent = identity.Lease
			}
			election, err := lockfile.TryAcquire(serviceElectionLockPath(), lockfile.Holder{
				Name: "bashy-schedule-daemon", PID: tc.electionPID, Intent: electionIntent,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer election.Release()
			intent := tc.leaseIntent
			if intent == "" {
				intent = identity.Lease
			}
			wrong, err := lockfile.TryAcquire(leasePath(identity), lockfile.Holder{
				Name: "bashy-schedule-daemon", PID: tc.leasePID, Intent: intent,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer wrong.Release()
			if identityIsLive(identity) {
				t.Fatal("mismatched lifetime lock holders authenticated a daemon")
			}
		})
	}
}

func TestRealDaemonConsumesGenerationStopAndCleansIdentity(t *testing.T) {
	withState(t)
	daemon := NewScheduleCmd()
	daemon.SetArgs([]string{"daemon", "--interval", "1h"})
	daemon.SetOut(io.Discard)
	daemon.SetErr(io.Discard)
	done := make(chan error, 1)
	go func() { done <- daemon.Execute() }()

	var identity daemonIdentity
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		identity, _ = readIdentity(servicePidPath())
		if identityIsLive(identity) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !identityIsLive(identity) {
		t.Fatal("real daemon did not publish its lifetime identity")
	}
	if err := requestDaemonStop(identity); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("real daemon did not consume its generation stop request")
	}
	for _, path := range []string{servicePidPath(), leasePath(identity), serviceStopPath(identity)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("daemon artifact %s survived clean stop: %v", path, err)
		}
	}
}

func TestStopWaitsForLeaseReleaseBeforeConcurrentReplacement(t *testing.T) {
	state := withState(t)
	logPath := filepath.Join(filepath.Dir(state), "lifecycle")
	t.Setenv("BASHY_SERVICE_START_LOG", logPath)
	t.Setenv("BASHY_SERVICE_TERM_DELAY", "200ms")
	first, err := startService(time.Minute, managedDaemonCommand)
	if err != nil {
		t.Fatal(err)
	}
	stopDone := make(chan ServiceStatus, 1)
	go func() { stopDone <- StopService() }()
	time.Sleep(50 * time.Millisecond)
	second, err := startService(time.Minute, managedDaemonCommand)
	if err != nil {
		t.Fatal(err)
	}
	if stopped := <-stopDone; stopped.Running {
		t.Fatalf("stop returned before lease release: %+v", stopped)
	}
	defer StopService()
	if first.PID == second.PID {
		t.Fatalf("replacement reused old daemon pid %d", first.PID)
	}
	deadline := time.Now().Add(time.Second)
	want := []string{"start " + strconv.Itoa(first.PID), "stop-start " + strconv.Itoa(first.PID), "stop-end " + strconv.Itoa(first.PID), "start " + strconv.Itoa(second.PID)}
	var lines []string
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(logPath)
		if err == nil {
			lines = strings.Split(strings.TrimSpace(string(b)), "\n")
			if strings.Join(lines, "|") == strings.Join(want, "|") {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Fatalf("lifecycle ordering=%q, want %q", lines, want)
	}
	var stops sync.WaitGroup
	for range 8 {
		stops.Add(1)
		go func() { defer stops.Done(); _ = StopService() }()
	}
	stops.Wait()
	if st := ServiceStatusOf(); st.Running {
		t.Fatalf("concurrent stops left daemon running: %+v", st)
	}
}

func TestStopNeverEscalatesToUnsafePIDSignal(t *testing.T) {
	withState(t)
	t.Setenv("BASHY_SERVICE_IGNORE_STOP", "1")
	startedDaemon, err := startService(time.Minute, managedDaemonCommand)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if proc, findErr := os.FindProcess(startedDaemon.PID); findErr == nil {
			_ = proc.Kill()
		}
		deadline := time.Now().Add(time.Second)
		for ServiceStatusOf().Running && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
	}()
	started := time.Now()
	testTimeout := 100 * time.Millisecond
	if st := stopService(testTimeout); !st.Running || st.PID != startedDaemon.PID {
		t.Fatalf("unresponsive daemon was not retained as running: %+v", st)
	}
	if elapsed := time.Since(started); elapsed < testTimeout || elapsed > 2*testTimeout {
		t.Fatalf("bounded cooperative stop elapsed=%v", elapsed)
	}
	if st := ServiceStatusOf(); !st.Running || st.PID != startedDaemon.PID {
		t.Fatalf("stop falsely cleared live unresponsive daemon: %+v", st)
	}
}
