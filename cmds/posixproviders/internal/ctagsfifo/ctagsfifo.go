// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Package ctagsfifo restores the POSIX output-file semantics that Universal
// Ctags lacks for FIFOs. Universal Ctags opens an existing output pathname for
// reading before writing it; with a FIFO and the usual waiting reader, both
// processes become readers and deadlock. The adapter keeps Universal Ctags as
// the tag generator, redirects only that output to a private regular file, and
// then becomes the FIFO's sole writer.
package ctagsfifo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// ExecFunc has the same contract as the external-provider executor.
type ExecFunc func(rc *tool.RunContext, name, path string, args []string) int

// fifoOpenTimeout bounds the FIFO rendezvous when the caller supplied no
// earlier context deadline. A FIFO with no reader must not wedge the multicall.
var fifoOpenTimeout = 2 * time.Second

// failureUnblockTimeout gives an already-starting consumer a small scheduling
// window to rendezvous and receive EOF after provider failure, without turning
// a provider error into another long wait when no consumer exists.
var failureUnblockTimeout = 100 * time.Millisecond

// Run invokes exec unchanged unless ctags' effective POSIX output pathname is
// an existing FIFO. It deliberately does not emulate ctags: the pinned provider
// remains responsible for parsing inputs and generating every output byte.
func Run(rc *tool.RunContext, name, path string, args []string, exec ExecFunc) int {
	plan := inspectArgs(args)
	if plan.passthrough || plan.stdout || plan.output == "" || plan.output == "-" {
		return exec(rc, name, path, args)
	}

	outputPath := plan.output
	if !filepath.IsAbs(outputPath) && rc.Dir != "" {
		outputPath = filepath.Join(rc.Dir, outputPath)
	}
	info, err := os.Stat(outputPath)
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		return exec(rc, name, path, args)
	}

	tmp, err := os.CreateTemp("", "bashy-ctags-")
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: create private output: %v\n", name, err)
		return 1
	}
	tmpPath := tmp.Name()
	tmpInfo, statErr := tmp.Stat()
	if statErr != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		fmt.Fprintf(rc.Err, "%s: create private output: %v\n", name, statErr)
		return 1
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		fmt.Fprintf(rc.Err, "%s: create private output: %v\n", name, err)
		return 1
	}
	defer os.Remove(tmpPath)

	rewritten := plan.rewrite(args, tmpPath)
	status := exec(rc, name, path, rewritten)
	if status != 0 {
		// Give an already-starting reader a short bounded window to rendezvous
		// and receive EOF. The provider's exact status (and ExitSignal) wins.
		bestEffortUnblock(outputPath, info)
		return status
	}

	ctx := rc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := copyToFIFO(ctx, tmpPath, tmpInfo, outputPath, info); err != nil {
		fmt.Fprintf(rc.Err, "%s: %s: %v\n", name, plan.output, err)
		return 1
	}
	return 0
}

type argPlan struct {
	output      string
	valueIndex  int
	prefixBytes int
	explicit    bool
	stdout      bool
	passthrough bool
}

// inspectArgs recognizes the POSIX ctags option grammar plus the readiness
// probe's exact --options=NONE guard. Other provider options pass through
// untouched: their argument arity belongs to the provider, and scanning an
// attached argument (for example Universal Ctags' -Iidentifier) as more option
// letters could falsely discover -f or -x. The last -f is effective; -x writes
// to stdout.
func inspectArgs(args []string) argPlan {
	p := argPlan{output: "tags", valueIndex: -1}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if len(a) < 2 || a[0] != '-' {
			break
		}
		if a[1] == '-' {
			// The public provider capability probe uses this exact pinned-provider
			// spelling to suppress host configuration. It has no output-path
			// effect, so parsing the following POSIX options remains unambiguous.
			if a == "--options=NONE" {
				continue
			}
			p.passthrough = true
			return p
		}
		body := a[1:]
		for j := 0; j < len(body); j++ {
			switch body[j] {
			case 'a':
			case 'x':
				p.stdout = true
			case 'f':
				p.explicit = true
				if j+1 < len(body) {
					p.output = body[j+1:]
					p.valueIndex = i
					p.prefixBytes = j + 2 // leading '-' plus option bytes through f
				} else if i+1 < len(args) {
					i++
					p.output = args[i]
					p.valueIndex = i
					p.prefixBytes = 0
				} else {
					// Preserve the provider's own missing-argument diagnostic.
					p.output = ""
					p.valueIndex = -1
					p.prefixBytes = 0
				}
				// f consumes the remainder of this option word as its argument.
				j = len(body)
			default:
				p.passthrough = true
				return p
			}
		}
	}
	return p
}

func (p argPlan) rewrite(args []string, output string) []string {
	result := append([]string(nil), args...)
	if p.passthrough {
		return result
	}
	if !p.explicit || p.valueIndex < 0 {
		return append([]string{"-f", output}, result...)
	}
	if p.prefixBytes == 0 {
		result[p.valueIndex] = output
	} else {
		result[p.valueIndex] = result[p.valueIndex][:p.prefixBytes] + output
	}
	return result
}

func copyToFIFO(ctx context.Context, source string, sourceOriginal os.FileInfo, target string, original os.FileInfo) error {
	in, err := openPrivateOutput(source, sourceOriginal)
	if err != nil {
		return err
	}
	openCtx, cancelOpen := context.WithTimeout(ctx, fifoOpenTimeout)
	out, err := openFIFO(openCtx, target, original, true)
	cancelOpen()
	if err != nil {
		_ = in.Close()
		return err
	}

	copyErr := copyPrivateOutput(ctx, out, in)
	closeOutErr := out.Close()
	closeInErr := in.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeOutErr != nil {
		return closeOutErr
	}
	return closeInErr
}

func bestEffortUnblock(target string, original os.FileInfo) {
	ctx, cancel := context.WithTimeout(context.Background(), failureUnblockTimeout)
	defer cancel()
	f, err := openFIFO(ctx, target, original, true)
	if err == nil {
		_ = f.Close()
	}
}
