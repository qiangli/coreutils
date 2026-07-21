// Package resourcescmd implements `resources`: report fleet utilization across
// providers (Anthropic, OpenAI, Google, Zhipu, Moonshot, DeepSeek) and bands (L1-L4).
package resourcescmd

import (
	"encoding/json"
	"fmt"

	"github.com/qiangli/coreutils/pkg/resources"
	"github.com/qiangli/coreutils/pkg/weavecli"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "resources",
	Synopsis: "Report fleet utilization across providers, bands, and meters.",
	Usage:    "resources fleet [--json]",
}

func init() {
	cmd.Run = run
	tool.Register(cmd)
}

func run(rc *tool.RunContext, args []string) int {
	subargs := args
	if len(args) > 0 && args[0] == "fleet" {
		subargs = args[1:]
	} else if len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintln(rc.Out, "Usage: resources fleet [--json]")
		return 0
	}

	fs := tool.NewFlags(cmd.Name)
	asJSON := fs.Bool("json", weavecli.IsAgent(), "emit a bashy-resources-v1 envelope")
	operands, code := tool.Parse(rc, cmd, fs, subargs)
	if code >= 0 {
		return code
	}
	_ = operands

	fr, err := resources.CollectFleetResources(rc.Ctx)
	if err != nil {
		fmt.Fprintf(rc.Err, "resources fleet: %v\n", err)
		return 1
	}

	if *asJSON {
		b, err := json.MarshalIndent(fr, "", "  ")
		if err != nil {
			fmt.Fprintf(rc.Err, "resources fleet: %v\n", err)
			return 1
		}
		fmt.Fprintln(rc.Out, string(b))
		return 0
	}

	fmt.Fprint(rc.Out, resources.FormatTable(fr))
	return 0
}
