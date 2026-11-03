//go:build unix

package tool_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	_ "github.com/qiangli/coreutils/cmds/env"
	_ "github.com/qiangli/coreutils/cmds/find"
	_ "github.com/qiangli/coreutils/cmds/timeout"
	"github.com/qiangli/coreutils/tool"
)

func descriptorContext(t *testing.T) (*tool.RunContext, *bytes.Buffer, *int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	path := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(path, []byte("inherited\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out := new(bytes.Buffer)
	cleaned := new(int)
	rc := &tool.RunContext{Ctx: ctx, Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"}, Stdio: tool.Stdio{Out: out, Err: out}}
	rc.PrepareChildDescriptors = func(c *exec.Cmd) (func(), error) {
		args := append([]string(nil), c.Args...)
		env := append([]string(nil), c.Env...)
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		for len(c.ExtraFiles) < 6 {
			c.ExtraFiles = append(c.ExtraFiles, nil)
		}
		if c.ExtraFiles[5] != nil {
			f.Close()
			return nil, errors.New("occupied descriptor")
		}
		c.ExtraFiles[5] = f
		if !reflect.DeepEqual(args, c.Args) || !reflect.DeepEqual(env, c.Env) {
			t.Fatal("descriptor handoff changed args/env")
		}
		return func() {
			*cleaned++
			if err := f.Close(); err != nil {
				t.Error(err)
			}
		}, nil
	}
	return rc, out, cleaned
}

func TestInvocationChildDescriptorWrappers(t *testing.T) {
	for _, name := range []string{"env", "timeout", "find"} {
		t.Run(name, func(t *testing.T) {
			rc, out, cleaned := descriptorContext(t)
			args := []string{"/bin/cat", "/dev/fd/8"}
			if name == "timeout" {
				args = append([]string{"5"}, args...)
			}
			if name == "find" {
				args = append([]string{rc.Dir, "-exec"}, append(args, ";")...)
			}
			if code := tool.Lookup(name).Run(rc, args); code != 0 {
				t.Fatalf("status%d: %s", code, out.String())
			}
			if out.String() != "inherited\n" || *cleaned != 1 {
				t.Fatalf("output=%q cleanup=%d", out.String(), *cleaned)
			}
		})
	}
}

func TestInvocationChildDescriptorStartAndFallback(t *testing.T) {
	rc, out, cleaned := descriptorContext(t)
	c, err := rc.StartCommand("/bin/cat", []string{"/dev/fd/8"}, nil, out, out)
	if err != nil {
		t.Fatal(err)
	}
	if *cleaned != 1 {
		t.Fatal("parent duplicate survived Start")
	}
	if err := c.Wait(); err != nil {
		t.Fatal(err)
	}
	if out.String() != "inherited\n" {
		t.Fatal(out.String())
	}
	// An executable text file exercises a separate prepared ENOEXEC retry.
	script := filepath.Join(t.TempDir(), "plain-script")
	if err := os.WriteFile(script, []byte("cat /dev/fd/8\n"), 0700); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	c, err = rc.StartCommand(script, nil, nil, out, out)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Wait(); err != nil {
		t.Fatal(err)
	}
	if *cleaned != 3 || out.String() != "inherited\n" {
		t.Fatalf("cleanup=%d output=%q", *cleaned, out.String())
	}
}

func TestInvocationChildDescriptorFailureCleanup(t *testing.T) {
	rc, _, cleaned := descriptorContext(t)
	c := exec.Command("/no-such-s557-child")
	if err := rc.StartOwnedCommand(c); err == nil {
		t.Fatal("missing child succeeded")
	}
	if *cleaned != 1 || c.Process != nil {
		t.Fatalf("cleanup=%d process=%v", *cleaned, c.Process)
	}
	sentinel := errors.New("refused descriptor handoff")
	rc.PrepareChildDescriptors = func(*exec.Cmd) (func(), error) { return func() { *cleaned++ }, sentinel }
	if err := rc.StartOwnedCommand(exec.Command("/usr/bin/true")); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if *cleaned != 2 {
		t.Fatalf("preparation failure cleanup=%d", *cleaned)
	}
}
