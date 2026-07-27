package secrets

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRedactorRedactKnownValues(t *testing.T) {
	const fixture = "synthetic-unit-secret-0001"

	r := NewRedactor()
	if err := r.Register("UNIT_SECRET", fixture); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	got := string(r.Redact([]byte("before " + fixture + " after")))
	if got != "before [redacted:UNIT_SECRET] after" {
		t.Fatal("one-shot redaction produced unexpected output")
	}
	if strings.Contains(got, fixture) {
		t.Fatal("one-shot redaction leaked the registered value")
	}
}

func TestRedactorRefusesShortValues(t *testing.T) {
	r := NewRedactor()
	err := r.Register("TOO_SHORT", "tiny123")
	if !errors.Is(err, ErrSecretTooShort) {
		t.Fatalf("short value error = %v, want ErrSecretTooShort", err)
	}
	if err := r.Register("TOO_FEW_RUNES", "秘密秘密"); !errors.Is(err, ErrSecretTooShort) {
		t.Fatalf("short multibyte value error = %v, want ErrSecretTooShort", err)
	}
}

func TestRedactorPrefersKnownValueNameOverShape(t *testing.T) {
	const fixture = "sk-syntheticknowncredential000000"

	r := NewRedactor()
	if err := r.Register("KNOWN_KEY", fixture); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if err := r.SetShapeMode(ShapeMask); err != nil {
		t.Fatalf("SetShapeMode failed: %v", err)
	}

	got := string(r.Redact([]byte(fixture)))
	if got != "[redacted:KNOWN_KEY]" {
		t.Fatal("known value did not take precedence over its credential shape")
	}
}

func TestCredentialShapesDefaultToReportOnly(t *testing.T) {
	const shaped = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"

	r := NewRedactor()
	var findings []ShapeMatch
	r.SetShapeReporter(func(finding ShapeMatch) {
		findings = append(findings, finding)
	})

	got := string(r.Redact([]byte("token=" + shaped)))
	if got != "token="+shaped {
		t.Fatal("default shape handling changed output")
	}
	if len(findings) != 1 || findings[0].Kind != "github-token" {
		t.Fatal("default shape handling did not report the credential shape")
	}
}

func TestCredentialShapeMaskingIsExplicit(t *testing.T) {
	const shaped = "AKIA1234567890ABCDEF"

	r := NewRedactor()
	if err := r.SetShapeMode(ShapeMask); err != nil {
		t.Fatalf("SetShapeMode failed: %v", err)
	}
	got := string(r.Redact([]byte(shaped)))
	if got != "[redacted:aws-access-key]" {
		t.Fatal("explicit shape masking produced unexpected output")
	}
}

func TestDetectCredentialShapesDoesNotExposeText(t *testing.T) {
	headers := [][]byte{
		[]byte("-----BEGIN PRIVATE KEY-----"),
		[]byte("-----BEGIN TEST PRIVATE KEY-----"),
	}
	for _, input := range headers {
		findings := DetectCredentialShapes(input)
		if len(findings) != 1 {
			t.Fatalf("shape finding count = %d, want 1", len(findings))
		}
		if findings[0].Kind != "private-key" || findings[0].Offset != 0 {
			t.Fatal("private-key shape finding has unexpected metadata")
		}
	}
}

func TestRedactingWriterFlushesTailOnClose(t *testing.T) {
	const fixture = "synthetic-close-secret"

	r := NewRedactor()
	if err := r.Register("CLOSE_SECRET", fixture); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	var dst bytes.Buffer
	w := r.Writer(&dst)
	if _, err := w.Write([]byte("prefix " + fixture)); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if strings.Contains(dst.String(), fixture) {
		t.Fatal("Close leaked the registered tail")
	}
	if dst.String() != "prefix [redacted:CLOSE_SECRET]" {
		t.Fatal("Close produced unexpected output")
	}
}
