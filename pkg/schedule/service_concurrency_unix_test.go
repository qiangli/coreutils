//go:build unix

package schedule

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
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
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM)
	defer signal.Stop(sig)
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
	}()
	appendServiceLog("start " + strconv.Itoa(identity.PID))
	<-sig
	appendServiceLog("term-start " + strconv.Itoa(identity.PID))
	if os.Getenv("BASHY_SERVICE_IGNORE_TERM") == "1" {
		select {}
	}
	if delay, err := time.ParseDuration(os.Getenv("BASHY_SERVICE_TERM_DELAY")); err == nil {
		time.Sleep(delay)
	}
	appendServiceLog("term-end " + strconv.Itoa(identity.PID))
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
	b, err := os.ReadFile(logPath)
	if err != nil || len(strings.Split(strings.TrimSpace(string(b)), "\n")) != 1 {
		t.Fatalf("daemon starts=%q err=%v", b, err)
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
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	want := []string{"start " + strconv.Itoa(first.PID), "term-start " + strconv.Itoa(first.PID), "term-end " + strconv.Itoa(first.PID), "start " + strconv.Itoa(second.PID)}
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

func TestStopEscalatesAndWaitsForLeaseRelease(t *testing.T) {
	withState(t)
	t.Setenv("BASHY_SERVICE_IGNORE_TERM", "1")
	if _, err := startService(time.Minute, managedDaemonCommand); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if st := StopService(); st.Running {
		t.Fatalf("escalated stop left daemon running: %+v", st)
	}
	if elapsed := time.Since(started); elapsed < serviceStopTimeout || elapsed > 3*serviceStopTimeout {
		t.Fatalf("bounded escalation elapsed=%v", elapsed)
	}
	if st := ServiceStatusOf(); st.Running {
		t.Fatalf("daemon lease still held after escalation: %+v", st)
	}
}
