// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package dag

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMeshCommandArgs(t *testing.T) {
	if n, a := meshCommandArgs("ssh", "dragon"); n != "ssh" || !reflect.DeepEqual(a, []string{"dragon", "bash", "-s"}) {
		t.Fatalf("simple = %q %v", n, a)
	}
	if n, a := meshCommandArgs("ssh -p 2222 -i key", "big"); n != "ssh" || !reflect.DeepEqual(a, []string{"-p", "2222", "-i", "key", "big", "bash", "-s"}) {
		t.Fatalf("flags = %q %v", n, a)
	}
	if n, _ := meshCommandArgs("", "h"); n != "ssh" {
		t.Fatalf("empty default = %q", n)
	}
}

func TestMeshExecutorHostlessRunsLocal(t *testing.T) {
	// A target with no Host must run locally even under the mesh executor.
	dir := t.TempDir()
	out := new(bytes.Buffer)
	res := meshExecutor{Remote: "ssh"}.Execute(context.Background(),
		&Task{Name: "x", Body: "echo local-ran\n"},
		TaskIO{Dir: dir, Env: os.Environ(), Stdout: out, Stderr: new(bytes.Buffer)})
	if res.Status != StatusDone {
		t.Fatalf("status = %s (%v)", res.Status, res.Err)
	}
	if out.String() != "local-ran\n" {
		t.Errorf("hostless body did not run locally; out=%q", out.String())
	}
}

func TestMeshExecutorDispatchesToRemote(t *testing.T) {
	// Fake "remote" transport that runs locally: it drops the host arg and execs
	// the rest (`bash -s`), so the body — fed on stdin — runs here. This exercises
	// the full dispatch path (argv build + stdin body + capture) hermetically.
	dir := t.TempDir()
	fake := filepath.Join(dir, "fakeremote")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nshift\nexec \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := new(bytes.Buffer)
	res := meshExecutor{Remote: fake}.Execute(context.Background(),
		&Task{Name: "remote", Host: "bigbox", Body: "echo ran-via-mesh on $(echo here)\n"},
		TaskIO{Dir: dir, Env: os.Environ(), Stdout: out, Stderr: new(bytes.Buffer)})
	if res.Status != StatusDone {
		t.Fatalf("status = %s (%v)", res.Status, res.Err)
	}
	if res.Host != "bigbox" {
		t.Errorf("host not recorded: %q", res.Host)
	}
	if !bytes.Contains(out.Bytes(), []byte("ran-via-mesh")) {
		t.Errorf("mesh body did not run; out=%q", out.String())
	}
}
