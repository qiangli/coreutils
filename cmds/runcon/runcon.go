package runconcmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	selinux "github.com/opencontainers/selinux/go-selinux"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{Name: "runcon", Synopsis: "Run command with specified SELinux context.", Usage: "runcon CONTEXT COMMAND [ARG]...\n   or: runcon [OPTION]... [COMMAND [ARG]...]"}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	compute := fs.BoolP("compute", "c", false, "compute process transition context")
	user := fs.StringP("user", "u", "", "set user in SELinux context")
	role := fs.StringP("role", "r", "", "set role in SELinux context")
	typ := fs.StringP("type", "t", "", "set type in SELinux context")
	level := fs.StringP("range", "l", "", "set range in SELinux context")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	custom := *compute || *user != "" || *role != "" || *typ != "" || *level != ""
	if !custom && len(operands) == 0 {
		label, err := selinux.CurrentLabel()
		if err != nil {
			fmt.Fprintf(rc.Err, "runcon: %v\n", err)
			return 1
		}
		fmt.Fprintln(rc.Out, label)
		return 0
	}
	var label string
	var command []string
	if custom {
		base, err := selinux.CurrentLabel()
		if err != nil {
			fmt.Fprintf(rc.Err, "runcon: %v\n", err)
			return 1
		}
		ctx, err := selinux.NewContext(base)
		if err != nil {
			fmt.Fprintf(rc.Err, "runcon: %v\n", err)
			return 1
		}
		if *user != "" {
			ctx["user"] = *user
		}
		if *role != "" {
			ctx["role"] = *role
		}
		if *typ != "" {
			ctx["type"] = *typ
		}
		if *level != "" {
			ctx["level"] = *level
		}
		label = ctx.Get()
		command = operands
	} else {
		if len(operands) < 2 {
			return tool.UsageError(rc, cmd, "missing command")
		}
		label = operands[0]
		command = operands[1:]
	}
	if len(command) == 0 {
		fmt.Fprintln(rc.Out, label)
		return 0
	}
	if *compute {
		current, _ := selinux.CurrentLabel()
		if computed, err := selinux.ComputeCreateContext(current, label, "process"); err == nil && computed != "" {
			label = computed
		}
	}
	return runWithLabel(rc, label, command)
}

func runWithLabel(rc *tool.RunContext, label string, argv []string) int {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := selinux.SetExecLabel(label); err != nil {
		fmt.Fprintf(rc.Err, "runcon: failed to set exec context: %v\n", err)
		return 1
	}
	defer selinux.SetExecLabel("")
	c := exec.CommandContext(rc.Ctx, rc.ResolveExecutable(argv[0]), argv[1:]...)
	c.Dir = rc.Dir
	c.Env = rc.Env
	c.Stdin = rc.In
	c.Stdout = rc.Out
	c.Stderr = rc.Err
	err := c.Run()
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	if os.IsNotExist(err) {
		fmt.Fprintf(rc.Err, "runcon: failed to run command %q: %v\n", argv[0], err)
		return 127
	}
	fmt.Fprintf(rc.Err, "runcon: failed to run command %q: %v\n", argv[0], err)
	return 126
}
