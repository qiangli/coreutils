package m4cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/qiangli/coreutils/shell"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// runSystem executes syscmd text with the repository's pure-Go POSIX shell.
// Registered applets remain in-process through shell.Handler; other commands
// use the interpreter's cross-platform exec path. No host shell is selected.
func (p *processor) runSystem(src string) int {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangPOSIX)).Parse(strings.NewReader(src), "syscmd")
	if err != nil {
		fmt.Fprintf(p.rc.Err, "m4: syscmd: %v\n", err)
		return 2
	}
	opts := []interp.RunnerOption{
		interp.Env(expand.ListEnviron(p.rc.Env...)),
		interp.StdIO(strings.NewReader(""), p.rc.Out, p.rc.Err),
		interp.ExecHandlers(shell.Handler()),
	}
	if p.rc.Dir != "" {
		opts = append(opts, interp.Dir(p.rc.Dir))
	}
	runner, err := interp.New(opts...)
	if err != nil {
		fmt.Fprintf(p.rc.Err, "m4: syscmd: %v\n", err)
		return 127
	}
	ctx := p.rc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err = runner.Run(ctx, file); err == nil {
		return 0
	}
	var status interp.ExitStatus
	if errors.As(err, &status) {
		return int(status)
	}
	if ctx.Err() != nil {
		return 128
	}
	fmt.Fprintf(p.rc.Err, "m4: syscmd: %v\n", err)
	return 127
}
