//go:build unix

package schedule

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConcurrentStartServiceHelper(t *testing.T) {
	if os.Getenv("BASHY_SERVICE_START_HELPER") != "1" {
		return
	}
	script := os.Getenv("BASHY_SERVICE_DAEMON_SCRIPT")
	serviceCommand = func(string, ...string) *exec.Cmd { return exec.Command(script) }
	st, err := StartService(time.Minute)
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
	script := filepath.Join(dir, "daemon.sh")
	scriptBody := "#!/bin/sh\nprintf '%s\\n' \"$$\" >> \"$BASHY_SERVICE_START_LOG\"\ntrap 'exit 0' TERM INT\nwhile :; do sleep 1; done\n"
	if err := os.WriteFile(script, []byte(scriptBody), 0o755); err != nil {
		t.Fatal(err)
	}
	// Exercise stale cleanup rather than beginning from an absent pidfile.
	if err := os.WriteFile(servicePidPath(), []byte("1073741824"), 0o644); err != nil {
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
				"BASHY_SERVICE_DAEMON_SCRIPT="+script,
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

	deadline := time.Now().Add(2 * time.Second)
	for {
		b, err := os.ReadFile(logPath)
		if err == nil && strings.TrimSpace(string(b)) != "" {
			lines := strings.Fields(string(b))
			if len(lines) != 1 || lines[0] != results[0] {
				t.Fatalf("daemon starts=%q, results=%q", b, results)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("daemon did not record startup: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	st := ServiceStatusOf()
	if !st.Running || strconv.Itoa(st.PID) != results[0] {
		t.Fatalf("managed status=%+v, results=%q", st, results)
	}
}
