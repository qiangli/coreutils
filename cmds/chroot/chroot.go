package chrootcmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{Name: "chroot", Synopsis: "Run command with a different root directory.", Usage: "chroot [OPTION] NEWROOT [COMMAND [ARG]...]"}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	groups := fs.String("groups", "", "supplementary groups as g1,g2,...")
	userspec := fs.String("userspec", "", "user and group as USER:GROUP")
	skipChdir := fs.Bool("skip-chdir", false, "do not change working directory to '/'")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		fmt.Fprintln(rc.Err, "chroot: missing operand")
		return 125
	}
	root := rc.Path(operands[0])
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		fmt.Fprintf(rc.Err, "chroot: cannot change root directory to %q: no such directory\n", operands[0])
		return 125
	}
	command := operands[1:]
	if len(command) == 0 {
		command = []string{"/bin/sh", "-i"}
	}
	cred, err := credential(*userspec, *groups)
	if err != nil {
		fmt.Fprintf(rc.Err, "chroot: %v\n", err)
		return 125
	}
	return runChroot(rc, root, command, *skipChdir, cred)
}

type credentialSpec struct {
	uid    uint32
	gid    uint32
	groups []uint32
}

func credential(spec, groupList string) (*credentialSpec, error) {
	if spec == "" && groupList == "" {
		return nil, nil
	}
	var uid, gid uint32
	var groups []uint32
	if spec != "" {
		u, g, _ := strings.Cut(spec, ":")
		if u != "" {
			id, err := lookupUID(u)
			if err != nil {
				return nil, err
			}
			uid = id
		}
		if g != "" {
			id, err := lookupGID(g)
			if err != nil {
				return nil, err
			}
			gid = id
		}
	}
	if groupList != "" {
		for _, g := range strings.Split(groupList, ",") {
			if strings.TrimSpace(g) == "" {
				continue
			}
			id, err := lookupGID(strings.TrimSpace(g))
			if err != nil {
				return nil, err
			}
			groups = append(groups, id)
		}
	}
	return &credentialSpec{uid: uid, gid: gid, groups: groups}, nil
}

func lookupUID(s string) (uint32, error) {
	if n, err := strconv.ParseUint(s, 10, 32); err == nil {
		return uint32(n), nil
	}
	u, err := user.Lookup(s)
	if err != nil {
		return 0, fmt.Errorf("invalid user %q", s)
	}
	n, err := strconv.ParseUint(u.Uid, 10, 32)
	return uint32(n), err
}

func lookupGID(s string) (uint32, error) {
	if n, err := strconv.ParseUint(s, 10, 32); err == nil {
		return uint32(n), nil
	}
	g, err := user.LookupGroup(s)
	if err != nil {
		return 0, fmt.Errorf("invalid group %q", s)
	}
	n, err := strconv.ParseUint(g.Gid, 10, 32)
	return uint32(n), err
}

func runChroot(rc *tool.RunContext, root string, argv []string, skipChdir bool, cred *credentialSpec) int {
	if !supportsChroot() {
		fmt.Fprintln(rc.Err, "chroot: operation is not supported on this platform")
		return 125
	}
	c := exec.CommandContext(rc.Ctx, argv[0], argv[1:]...)
	c.Env = rc.Env
	c.Stdin = rc.In
	c.Stdout = rc.Out
	c.Stderr = rc.Err
	if !skipChdir {
		c.Dir = "/"
	}
	setChroot(c, root, cred)
	err := c.Run()
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	if os.IsNotExist(err) {
		fmt.Fprintf(rc.Err, "chroot: failed to run command %q: %v\n", argv[0], err)
		return 127
	}
	fmt.Fprintf(rc.Err, "chroot: failed to run command %q: %v\n", argv[0], err)
	return 126
}
