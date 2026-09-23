package tool

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/interp/ownedexec"
)

func TestMain(m *testing.M) {
	if err := ownedexec.Adopt(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(126)
	}
	os.Exit(m.Run())
}

func TestOwnedExecLargeArgumentsAndEnvironment(t *testing.T) {
	if os.Getenv("COREUTILS_OWNED_EXEC_HELPER") == "1" {
		if len(os.Args) != 4 || os.Args[0] != "owned-argv0" || len(os.Args[3]) != 200000 || len(os.Getenv("COREUTILS_OWNED_EXEC_BIG")) != 200000 || os.Getenv(ownedexec.Marker) != "" {
			t.Fatalf("restored invocation: argv0=%q argc=%d last=%d env=%d marker=%q", os.Args[0], len(os.Args), len(os.Args[len(os.Args)-1]), len(os.Getenv("COREUTILS_OWNED_EXEC_BIG")), os.Getenv(ownedexec.Marker))
		}
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := exec.Command(executable, "-test.run=^TestOwnedExecLargeArgumentsAndEnvironment$", "--", strings.Repeat("é", 100000))
	c.Args[0] = "owned-argv0"
	c.Env = append(os.Environ(), "COREUTILS_OWNED_EXEC_HELPER=1", "COREUTILS_OWNED_EXEC_BIG="+strings.Repeat("x", 200000))
	var output bytes.Buffer
	c.Stdout, c.Stderr = &output, &output
	if err := StartOwnedCommand(c); err != nil {
		t.Fatal(err)
	}
	if c.Args[0] != "owned-argv0" || len(c.Args) != 4 || len(c.Env) == 0 || len(c.ExtraFiles) != 0 {
		t.Fatalf("launch mutated caller command: argv=%d argv0=%q env=%d extra=%d", len(c.Args), c.Args[0], len(c.Env), len(c.ExtraFiles))
	}
	if err := c.Wait(); err != nil {
		t.Fatalf("child: %v: %s", err, output.String())
	}
}
