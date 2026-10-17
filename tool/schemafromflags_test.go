package tool

import (
	"reflect"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func TestSchemaFromFlags(t *testing.T) {
	fs := pflag.NewFlagSet("schema", pflag.ContinueOnError)
	fs.SortFlags = false
	fs.DurationP("timeout", "t", 2*time.Second, "timeout")
	fs.StringSliceP("tags", "s", []string{"red", "blue"}, "tags")
	fs.StringArray("paths", []string{"a", "b"}, "paths")
	fs.IntSlice("numbers", []int{1, 2}, "numbers")
	fs.BoolP("quiet", "q", false, "quiet")
	fs.Bool("enabled", true, "enabled")
	fs.StringP("output", "o", "out.txt", "output")
	fs.String("empty", "", "empty default")
	fs.IntP("count", "n", 3, "count")
	fs.StringSlice("empty-list", nil, "empty slice")
	fs.BoolP("hidden", "h", false, "hidden")
	fs.BoolP("old", "O", false, "deprecated")
	fs.BoolP("verbose", "v", false, "deprecated shorthand")
	for _, err := range []error{
		fs.MarkHidden("hidden"),
		fs.MarkDeprecated("old", "use quiet"),
		fs.MarkShorthandDeprecated("verbose", "use --verbose"),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	// Exercise Deprecated independently of MarkDeprecated also setting Hidden.
	fs.Lookup("old").Hidden = false
	if err := fs.Parse([]string{"-q", "--count=9", "--output=changed", "--tags=green", "operand"}); err != nil {
		t.Fatal(err)
	}
	want := ArgSchema{Flags: []ArgFlag{
		{Name: "count", Shorthand: "n", Type: "int", Default: "3"},
		{Name: "empty", Type: "string", Default: ""},
		{Name: "empty-list", Type: "stringSlice", Default: "[]"},
		{Name: "enabled", Type: "bool", Default: "true"},
		{Name: "numbers", Type: "intSlice", Default: "[1,2]"},
		{Name: "output", Shorthand: "o", Type: "string", Default: "out.txt"},
		{Name: "paths", Type: "stringArray", Default: "[a,b]"},
		{Name: "quiet", Shorthand: "q", Type: "bool", Default: "false"},
		{Name: "tags", Shorthand: "s", Type: "stringSlice", Default: "[red,blue]"},
		{Name: "timeout", Shorthand: "t", Type: "duration", Default: "2s"},
		{Name: "verbose", Type: "bool", Default: "false"},
	}}
	if got := SchemaFromFlags(fs); !reflect.DeepEqual(got, want) {
		t.Fatalf("SchemaFromFlags() = %#v, want %#v", got, want)
	}
	if fs.SortFlags || !fs.Parsed() || !reflect.DeepEqual(fs.Args(), []string{"operand"}) {
		t.Fatal("schema extraction changed flag set state")
	}
	for name, value := range map[string]string{"quiet": "true", "count": "9", "output": "changed", "tags": "[green]"} {
		f := fs.Lookup(name)
		if !f.Changed || f.Value.String() != value {
			t.Errorf("schema extraction changed %s: %#v", name, f)
		}
	}
}

func TestSchemaFromFlagsEmpty(t *testing.T) {
	for _, fs := range []*pflag.FlagSet{nil, pflag.NewFlagSet("empty", pflag.ContinueOnError)} {
		if got := SchemaFromFlags(fs); !reflect.DeepEqual(got, ArgSchema{}) {
			t.Errorf("SchemaFromFlags(%v) = %#v, want empty schema", fs, got)
		}
	}
}
