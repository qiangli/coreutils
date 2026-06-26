// Package shell adapts the coreutils tool registry to mvdan.cc/sh/v3 so
// any embedding shell exposes the whole pure-Go userland as in-process
// commands. The adapter is an interp.ExecHandler middleware: argv[0] that
// names a registered tool runs via tool.Run (no process spawned, no PATH
// lookup); anything else falls through to the next handler.
//
// Precedence is pure-Go first — uniformity across Linux/macOS/Windows is
// the product. A host that wants the opposite (defer to system binaries,
// e.g. a strict GNU-bash drop-in) simply does not wire this middleware.
//
// This package imports sh; sh never imports coreutils. The core `tool`
// package stays sh-free — only this adapter and its consumers pull sh.
package shell

import (
	"context"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/tool"
)

// Handler returns an interp.ExecHandler middleware that intercepts every
// registered tool. Wire it via interp.ExecHandlers(shell.Handler()) — or,
// for a persistent runner, append it to the middleware chain before the
// default exec handler.
func Handler() func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return HandlerFunc(nil)
}

// HandlerFunc is like Handler but takes an optional predicate deciding
// whether argv[0] should be served from the registry. A nil predicate
// intercepts every registered tool. Use a predicate to carve out names a
// host wants to keep routing to PATH (e.g. allow only the agentic `yc`
// verbs while leaving file/text tools to the system).
func HandlerFunc(intercept func(name string) bool) func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			if len(args) == 0 {
				return next(ctx, args)
			}
			name := args[0]
			if intercept != nil && !intercept(name) {
				return next(ctx, args)
			}
			t := tool.Lookup(name)
			if t == nil {
				return next(ctx, args)
			}
			hc := interp.HandlerCtx(ctx)
			rc := &tool.RunContext{
				Ctx: ctx,
				Dir: hc.Dir,
				Env: envSlice(hc.Env),
				FS:  tool.NewLocalFS(),
				Stdio: tool.Stdio{
					In:  hc.Stdin,
					Out: hc.Stdout,
					Err: hc.Stderr,
				},
			}
			code := t.Run(rc, args[1:])
			if code != 0 {
				return interp.ExitStatus(uint8(code))
			}
			return nil
		}
	}
}

// envSlice flattens the interpreter environment into the os.Environ()
// shape tool.RunContext expects ("KEY=VALUE"). Only set variables are
// included, matching how a spawned process would observe the environment.
func envSlice(env expand.Environ) []string {
	if env == nil {
		return nil
	}
	var out []string
	env.Each(func(name string, vr expand.Variable) bool {
		if vr.IsSet() {
			out = append(out, name+"="+vr.String())
		}
		return true
	})
	return out
}
