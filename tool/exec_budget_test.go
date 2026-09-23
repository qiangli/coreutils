package tool

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"

	"mvdan.cc/sh/v3/execbudget"
)

func TestStartCommandEnforcesAdvertisedExecBudget(t *testing.T) {
	// The nonexistent path proves the budget check runs before host launch.
	rc := &RunContext{Env: []string{"BIG=" + strings.Repeat("x", execbudget.BashyArgMax)}}
	_, err := rc.StartCommand("definitely-not-a-command", nil, nil, nil, nil)
	if !errors.Is(err, syscall.E2BIG) {
		t.Fatalf("StartCommand error = %v, want E2BIG", err)
	}
}

func TestCheckExecBudgetCountsInheritedEnvironment(t *testing.T) {
	const name = "BASHY_TEST_INHERITED_EXEC_BUDGET"
	old, hadOld := os.LookupEnv(name)
	t.Cleanup(func() {
		if hadOld {
			_ = os.Setenv(name, old)
		} else {
			_ = os.Unsetenv(name)
		}
	})
	if err := os.Setenv(name, strings.Repeat("x", execbudget.BashyArgMax)); err != nil {
		t.Skipf("host cannot set oversized process environment: %v", err)
	}
	if err := CheckExecBudget([]string{"command"}, nil); !errors.Is(err, syscall.E2BIG) {
		t.Fatalf("inherited environment: got %v, want E2BIG", err)
	}
	if err := CheckExecBudget([]string{"command"}, []string{}); err != nil {
		t.Fatalf("explicit empty environment: got %v, want no limit error", err)
	}
}
