package man1_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommunicationManualPagesAreComplete(t *testing.T) {
	for _, name := range []string{"mailx", "talk"} {
		data, err := os.ReadFile(filepath.Join(".", name+".1"))
		if err != nil {
			t.Fatal(err)
		}
		body := string(data)
		for _, section := range []string{".SH NAME", ".SH SYNOPSIS", ".SH DESCRIPTION", ".SH ENVIRONMENT", ".SH EXIT STATUS", ".SH STANDARDS"} {
			if !strings.Contains(body, section) {
				t.Errorf("%s is missing %s", name, section)
			}
		}
		if len(body) < 1800 {
			t.Errorf("%s is unexpectedly short (%d bytes)", name, len(body))
		}
	}
	alias, err := os.ReadFile(filepath.Join(".", "mail.1"))
	if err != nil {
		t.Fatal(err)
	}
	if string(alias) != ".so man1/mailx.1\n" {
		t.Fatal("mail.1 must resolve to the complete mailx manual")
	}
}
