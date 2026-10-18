package tool_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	_ "github.com/qiangli/coreutils/cmds/chmod"
	_ "github.com/qiangli/coreutils/cmds/chown"
	_ "github.com/qiangli/coreutils/cmds/cp"
	_ "github.com/qiangli/coreutils/cmds/install"
	_ "github.com/qiangli/coreutils/cmds/ln"
	_ "github.com/qiangli/coreutils/cmds/mkdir"
	_ "github.com/qiangli/coreutils/cmds/mv"
	_ "github.com/qiangli/coreutils/cmds/rm"
	_ "github.com/qiangli/coreutils/cmds/rmdir"
	_ "github.com/qiangli/coreutils/cmds/touch"
	_ "github.com/qiangli/coreutils/cmds/truncate"
	"github.com/qiangli/coreutils/tool"
)

type unreadableInput struct{}

func (unreadableInput) Read([]byte) (int, error) { panic("preview read stdin") }

func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		value := fmt.Sprintf("%v %d %v", fi.Mode(), fi.Size(), fi.ModTime())
		if fi.Mode().IsRegular() {
			b, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			value += string(b)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			b, e := os.Readlink(path)
			if e != nil {
				return e
			}
			value += b
		}
		result[path] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestEffectDryRunMutators(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"rm", []string{"-ri", "tree"}}, {"rm", []string{"-f", "file"}},
		{"mv", []string{"-bi", "file", "dest"}}, {"mv", []string{"tree", "newtree"}},
		{"cp", []string{"-bfi", "file", "dest"}}, {"cp", []string{"-a", "tree", "newtree"}},
		{"ln", []string{"-bfi", "file", "dest"}}, {"ln", []string{"-s", "missing", "newlink"}},
		{"chmod", []string{"-Rv", "000", "tree"}}, {"chown", []string{"-Rv", "0:0", "tree"}},
		{"touch", []string{"-t", "200001010000", "file", "newfile"}},
		{"mkdir", []string{"-pm", "700", "new/tree"}}, {"rmdir", []string{"-pv", "empty/child"}},
		{"truncate", []string{"-s", "2", "file", "newfile"}},
		{"install", []string{"-bD", "file", "new/tree/file"}}, {"install", []string{"-bC", "file", "dest"}},
		{"install", []string{"-dm", "700", "new/tree"}},
	}
	for _, tc := range cases {
		t.Run(tc.name+strings.Join(tc.args, "_"), func(t *testing.T) {
			dir := t.TempDir()
			for _, d := range []string{"tree/sub", "empty/child"} {
				if err := os.MkdirAll(filepath.Join(dir, d), 0755); err != nil {
					t.Fatal(err)
				}
			}
			for _, f := range []string{"file", "dest", "dest~", "tree/sub/file"} {
				if err := os.WriteFile(filepath.Join(dir, f), []byte(f+" contents"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			_ = os.Symlink("file", filepath.Join(dir, "tree", "link"))
			before := snapshot(t, dir)
			var out, errb bytes.Buffer
			rc := &tool.RunContext{Dir: dir, Stdio: tool.Stdio{In: unreadableInput{}, Out: &out, Err: &errb}}
			cmd := tool.Lookup(tc.name)
			code := cmd.Run(rc, append([]string{"--dry-run"}, tc.args...))
			if code != 0 {
				t.Fatalf("code=%d err=%s", code, &errb)
			}
			if tc.name != "chown" || len(rc.Effects) > 0 {
				if !strings.Contains(out.String(), "planned (local-preview, conditional):") {
					t.Fatalf("missing plan: %s", &out)
				}
			}
			if after := snapshot(t, dir); !reflect.DeepEqual(before, after) {
				t.Fatalf("dry-run mutated tree\nbefore=%v\nafter=%v", before, after)
			}
		})
	}
}

func TestEffectCertUnknownGolden(t *testing.T) {
	for _, name := range strings.Fields("rm mv cp ln chmod chown touch mkdir rmdir truncate install") {
		t.Run(name, func(t *testing.T) {
			for _, flag := range []string{"--dry-run", "--dry_run"} {
				var out, errb bytes.Buffer
				rc := &tool.RunContext{Env: []string{"VSC_PROFILE=cert"}, Stdio: tool.Stdio{In: unreadableInput{}, Out: &out, Err: &errb}}
				cmd := tool.Lookup(name)
				code := cmd.Run(rc, []string{flag})
				want := fmt.Sprintf("%s: unknown flag: %s\n%s: not every GNU flag is implemented in pure-Go coreutils — see '%s --help' for the supported subset\n", name, flag, name, name)
				if code != 2 || out.Len() != 0 || errb.String() != want {
					t.Fatalf("%s code=%d out=%q err=%q", flag, code, &out, &errb)
				}
			}
		})
	}
}

func TestEffectFalseAndAlias(t *testing.T) {
	for _, flag := range []string{"--dry_run", "--dry-run=false"} {
		t.Run(flag, func(t *testing.T) {
			dir := t.TempDir()
			var out, errb bytes.Buffer
			rc := &tool.RunContext{Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}
			cmd := tool.Lookup("mkdir")
			if code := cmd.Run(rc, []string{flag, "new"}); code != 0 {
				t.Fatal(code, &errb)
			}
			_, err := os.Stat(filepath.Join(dir, "new"))
			if flag == "--dry_run" && !os.IsNotExist(err) {
				t.Fatal("alias mutated")
			}
			if flag == "--dry-run=false" && err != nil {
				t.Fatal("false did not execute", err)
			}
		})
	}
}

// Golden hashes were captured from 8c2706f2 with VSC_PROFILE=cert.
func TestEffectCertHelpGolden(t *testing.T) {
	data, err := os.ReadFile("testdata/effect_cert_help_sha256.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden map[string]string
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	for name, want := range golden {
		t.Run(name, func(t *testing.T) {
			var out, errb bytes.Buffer
			rc := &tool.RunContext{Env: []string{"VSC_PROFILE=cert"}, Stdio: tool.Stdio{Out: &out, Err: &errb}}
			code := tool.Lookup(name).Run(rc, []string{"--help"})
			got := fmt.Sprintf("%x", sha256.Sum256(out.Bytes()))
			if code != 0 || errb.Len() != 0 || got != want {
				t.Fatalf("cert help differs from 8c2706f2: code=%d hash=%s want=%s err=%s", code, got, want, &errb)
			}
		})
	}
}
