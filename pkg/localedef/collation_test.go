package localedef

import (
	"strings"
	"testing"
)

func TestCompileCollation(t *testing.T) {
	src, err := ParseSource(strings.NewReader("LC_COLLATE\ncollating-element <ch> from \"ch\"\ncollating-symbol <first>\norder_start forward;backward\n<first>\n<z> <first>;<z>\n<a> <first>;<a>\n<ch> <ch>;<ch>\norder_end\nEND LC_COLLATE\n"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := Compile("custom", src, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Has("LC_COLLATE") {
		t.Fatal("compiled locale drops LC_COLLATE")
	}
}
