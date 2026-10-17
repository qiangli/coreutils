package tool

import (
	"sort"

	"github.com/spf13/pflag"
)

// SchemaFromFlags describes the public flags in fs without parsing or changing
// them. Hidden and deprecated flags are omitted, as are deprecated shorthands.
// Flags are ordered by name even when fs.SortFlags is false. A nil fs yields an
// empty schema; positional arguments, requiredness and enums are not inferred.
//
// Type and Default retain pflag's Value.Type() and DefValue verbatim, including
// duration and slice types and their textual defaults. ArgSchema's JSONSchema
// and Argv methods currently support only string, bool, int and float; other
// types remain descriptive metadata and those methods reject them.
//
// Certification note for common-options lint: coreutils has no shared runtime
// certification-profile switch in tool. The bashy_cert build tag selects cert
// implementations (for example pkg/ctype/ctype_glibc_cert.go). At runtime,
// applets such as cmds/nice test POSIXLY_CORRECT presence in RunContext.Env,
// including an empty value. The harness stages POSIXLY_CORRECT=1 and checks its
// propagation (docs/posix-owner-gate.md). This metadata API does not consult or
// change either signal; the vocabulary lint is report-only.
func SchemaFromFlags(fs *pflag.FlagSet) ArgSchema {
	var schema ArgSchema
	if fs == nil {
		return schema
	}
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Deprecated != "" {
			return
		}
		shorthand := f.Shorthand
		if f.ShorthandDeprecated != "" {
			shorthand = ""
		}
		schema.Flags = append(schema.Flags, ArgFlag{
			Name: f.Name, Shorthand: shorthand,
			Type: f.Value.Type(), Default: f.DefValue,
		})
	})
	sort.Slice(schema.Flags, func(i, j int) bool {
		return schema.Flags[i].Name < schema.Flags[j].Name
	})
	return schema
}
