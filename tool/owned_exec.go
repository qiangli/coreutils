package tool

import (
	"crypto/sha256"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/interp/ownedexec"
)

// StartOwnedCommand starts a child using Bashy's bounded frame only when its
// executable is verified as a Bashy image. Ordinary programs keep native exec.
func StartOwnedCommand(c *exec.Cmd) error {
	env := c.Env
	if env == nil {
		env = os.Environ()
	}
	if err := CheckExecBudget(c.Args, env); err != nil {
		return err
	}
	if !needsOwnedFrame(c.Args, env) || !verifiedOwnedImage(c.Path) {
		return c.Start()
	}
	f, err := os.CreateTemp("", ".bashy-owned-exec-*")
	if err != nil {
		return err
	}
	path := f.Name()
	defer func() { _ = f.Close(); _ = os.Remove(path) }()
	nativeEnv := append([]string(nil), env...)
	frame := ownedexec.Frame{Args: append([]string(nil), c.Args...)}
	if runtime.GOOS != "windows" {
		frame.Env = env
		nativeEnv = earlyOwnedEnv(env)
	} else {
		for _, entry := range env {
			if strings.HasPrefix(entry, ownedexec.Marker+"=") {
				frame.Env = append(frame.Env, entry)
			}
		}
	}
	if err := ownedexec.Write(f, frame); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	handle, cleanup, err := installOwnedFrame(c, f)
	if err != nil {
		return err
	}
	defer cleanup()
	nativeEnv = append(nativeEnv, ownedexec.Marker+"="+strconv.FormatUint(handle, 10))
	c.Env = nativeEnv
	c.Args = []string{c.Path, ownedexec.Sentinel}
	return c.Start()
}

func needsOwnedFrame(args, env []string) bool {
	n := 0
	for _, arg := range args {
		n += len(arg) + 1
		if len(arg) > 64<<10 {
			return true
		}
	}
	if runtime.GOOS == "windows" && n > 8<<10 {
		return true
	}
	for _, entry := range env {
		n += len(entry) + 1
		if runtime.GOOS != "windows" && len(entry) > 64<<10 {
			return true
		}
	}
	return runtime.GOOS != "windows" && n > 96<<10
}

func verifiedOwnedImage(path string) bool {
	candidate, err := os.Stat(path)
	if err != nil || !candidate.Mode().IsRegular() {
		return false
	}
	self, err := os.Executable()
	if err != nil {
		return false
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	known := []string{self}
	dirs := []string{filepath.Dir(self)}
	if root := os.Getenv("BASHY_ROOT"); root != "" {
		dirs = append(dirs, filepath.Join(root, "usr", "bin"), filepath.Join(root, "bin"))
	}
	for _, dir := range dirs {
		for _, name := range []string{"bashy", "bash", "yoke", "coreutils"} {
			known = append(known, filepath.Join(dir, name+suffix))
		}
	}
	var candidateHash [32]byte
	hashed := false
	for _, name := range known {
		info, err := os.Stat(name)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if os.SameFile(candidate, info) {
			return true
		}
		if info.Size() != candidate.Size() {
			continue
		}
		if !hashed {
			candidateHash, err = fileHash(path)
			if err != nil {
				return false
			}
			hashed = true
		}
		hash, err := fileHash(name)
		if err == nil && hash == candidateHash {
			return true
		}
	}
	return false
}

func fileHash(path string) ([32]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return [32]byte{}, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [32]byte{}, err
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

func earlyOwnedEnv(env []string) []string {
	out := make([]string, 0, len(env))
	n := 0
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || len(entry) > 64<<10 || n+len(entry)+1 > 64<<10 {
			continue
		}
		if strings.HasPrefix(key, "GO") || strings.HasPrefix(key, "LD_") || strings.HasPrefix(key, "DYLD_") || strings.HasPrefix(key, "LC_") || strings.HasPrefix(key, "XDG_") || key == "PATH" || key == "HOME" || key == "TMPDIR" || key == "TZ" || key == "LANG" || key == "BASHY_ROOT" {
			out = append(out, entry)
			n += len(entry) + 1
		}
	}
	return out
}

// RunOwnedCommand starts and waits for a child through the same verified
// handoff boundary as StartOwnedCommand.
func RunOwnedCommand(c *exec.Cmd) error {
	if err := StartOwnedCommand(c); err != nil {
		return err
	}
	return c.Wait()
}
