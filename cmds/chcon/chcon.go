// Package chconcmd implements chcon(1): change the SELinux security
// context of each FILE.
package chconcmd

import (
	"fmt"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "chcon",
	Synopsis: "Change the SELinux security context of each FILE.",
	Usage: `chcon [OPTION]... CONTEXT FILE...
  or:  chcon [OPTION]... [-u USER] [-r ROLE] [-l RANGE] [-t TYPE] FILE...
  or:  chcon [OPTION]... --reference=RFILE FILE...`,
}

func init() { cmd.Run = run; tool.Register(cmd) }

type derefMode int

const (
	derefNever derefMode = iota
	derefCmdLine
	derefAlways
)

type chconMode int

const (
	modeContext chconMode = iota
	modeComponents
	modeReference
)

type contextParts struct {
	user *string
	role *string
	typ  *string
	rang *string
}

type chconOp struct {
	mode          chconMode
	context       string
	reference     string
	files         []string
	parts         contextParts
	recursive     bool
	verbose       bool
	preserveRoot  bool
	noDereference bool
	deref         derefMode
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	recursive := fs.BoolP("recursive", "R", false, "operate on files and directories recursively")
	verbose := fs.BoolP("verbose", "v", false, "output a diagnostic for every file processed")
	preserveRoot := fs.Bool("preserve-root", false, "fail to operate recursively on '/'")
	noPreserveRoot := fs.Bool("no-preserve-root", false, "do not treat '/' specially (the default)")
	reference := fs.String("reference", "", "use RFILE's security context rather than specifying a CONTEXT value")
	user := fs.StringP("user", "u", "", "set user USER in the target security context")
	role := fs.StringP("role", "r", "", "set role ROLE in the target security context")
	typ := fs.StringP("type", "t", "", "set type TYPE in the target security context")
	rang := fs.StringP("range", "l", "", "set range RANGE in the target security context")
	fs.Bool("dereference", false, "affect the referent of each symbolic link (the default)")
	noDereference := fs.BoolP("no-dereference", "h", false, "affect symbolic links instead of their referents")
	noTraverse := fs.BoolP("P", "P", false, "never follow symbolic links (with -R)")
	cmdLineH := fs.BoolP("H", "H", false, "follow command-line symbolic links (with -R)")
	followL := fs.BoolP("L", "L", false, "follow every symbolic link encountered (with -R)")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	components := contextParts{}
	componentMode := false
	if fs.Changed("user") {
		components.user = user
		componentMode = true
	}
	if fs.Changed("role") {
		components.role = role
		componentMode = true
	}
	if fs.Changed("type") {
		components.typ = typ
		componentMode = true
	}
	if fs.Changed("range") {
		components.rang = rang
		componentMode = true
	}
	referenceMode := *reference != ""
	switch {
	case referenceMode && componentMode:
		return tool.UsageError(rc, cmd, "cannot specify both --reference and context component options")
	case referenceMode:
		if looksLikeContextOperand(operands) {
			return tool.UsageError(rc, cmd, "cannot specify a CONTEXT value with --reference")
		}
		if len(operands) == 0 {
			return tool.UsageError(rc, cmd, "missing operand")
		}
	case componentMode:
		if looksLikeContextOperand(operands) {
			return tool.UsageError(rc, cmd, "cannot specify a CONTEXT value with context component options")
		}
		if len(operands) == 0 {
			return tool.UsageError(rc, cmd, "missing operand")
		}
	default:
		if len(operands) == 0 {
			return tool.UsageError(rc, cmd, "missing operand")
		}
		if len(operands) == 1 {
			return tool.UsageError(rc, cmd, "missing operand after '%s'", operands[0])
		}
	}

	op := chconOp{
		parts:         components,
		recursive:     *recursive,
		verbose:       *verbose,
		preserveRoot:  *preserveRoot && !*noPreserveRoot,
		noDereference: *noDereference,
		deref:         computeDeref(*followL, *cmdLineH),
	}
	if *noTraverse {
		op.deref = derefNever
	}
	if isBool(fs, "dereference") {
		op.noDereference = false
	}
	switch {
	case referenceMode:
		op.mode = modeReference
		op.reference = *reference
		op.files = operands
	case componentMode:
		op.mode = modeComponents
		op.files = operands
	default:
		op.mode = modeContext
		op.context = operands[0]
		op.files = operands[1:]
	}
	return applyContext(rc, op)
}

func computeDeref(followLOrDeref, cmdLineH bool) derefMode {
	if followLOrDeref {
		return derefAlways
	}
	if cmdLineH {
		return derefCmdLine
	}
	return derefNever
}

func isBool(fs interface{ GetBool(string) (bool, error) }, name string) bool {
	v, err := fs.GetBool(name)
	return err == nil && v
}

func mergeContext(current string, parts contextParts) (string, error) {
	fields := strings.SplitN(current, ":", 4)
	if len(fields) < 3 {
		return "", fmt.Errorf("invalid security context '%s'", current)
	}
	if parts.user != nil {
		fields[0] = *parts.user
	}
	if parts.role != nil {
		fields[1] = *parts.role
	}
	if parts.typ != nil {
		fields[2] = *parts.typ
	}
	if parts.rang != nil {
		if len(fields) == 3 {
			fields = append(fields, *parts.rang)
		} else {
			fields[3] = *parts.rang
		}
	}
	if len(fields) == 4 && fields[3] == "" {
		fields = fields[:3]
	}
	return strings.Join(fields, ":"), nil
}

func looksLikeContextOperand(operands []string) bool {
	if len(operands) < 2 {
		return false
	}
	return len(strings.SplitN(operands[0], ":", 4)) >= 3
}

func reportChconErr(rc *tool.RunContext, display, context string, err error) {
	fmt.Fprintf(rc.Err, "chcon: failed to change context of '%s' to '%s': %v\n", display, context, tool.SysErr(err))
}
