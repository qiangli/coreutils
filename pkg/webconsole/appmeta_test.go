// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package webconsole

import (
	"errors"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/atlas"
)

func TestParseAppSpec(t *testing.T) {
	for _, tc := range []struct {
		in      string
		bin     string
		port    int
		wantErr bool
	}{
		{in: "classgo", bin: "classgo"},
		{in: "classgo@8080", bin: "classgo", port: 8080},
		{in: "/opt/bin/classgo@8080", bin: "/opt/bin/classgo", port: 8080},
		{in: "/opt/b@d/classgo@8080", bin: "/opt/b@d/classgo", port: 8080},
		{in: "classgo@nope", wantErr: true},
		{in: "classgo@0", wantErr: true},
		{in: "classgo@99999", wantErr: true},
		{in: "", wantErr: true},
	} {
		bin, port, err := ParseAppSpec(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseAppSpec(%q): want error, got %q/%d", tc.in, bin, port)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseAppSpec(%q): %v", tc.in, err)
			continue
		}
		if bin != tc.bin || port != tc.port {
			t.Errorf("ParseAppSpec(%q) = %q/%d, want %q/%d", tc.in, bin, port, tc.bin, tc.port)
		}
	}
}

// A binary that does not speak the contract must be REFUSED, not half-parsed.
// `meta` is a common word: a stray subcommand answering something else, or a
// usage banner on stdout with exit 0, would otherwise produce a tile built from
// whatever happened to unmarshal.
func TestParseMetaDemandsPositiveIdentification(t *testing.T) {
	for _, in := range []string{
		`not json at all`,
		`{}`,
		`{"name":"x","port":80}`, // no schema_version
		`{"schema_version":"something-else","port":80}`, // wrong contract
		`usage: classgo meta [flags]`,                   // a banner, exit 0
	} {
		if _, err := ParseMeta([]byte(in)); !errors.Is(err, ErrNotAnApp) {
			t.Errorf("ParseMeta(%q) = %v, want ErrNotAnApp", in, err)
		}
	}
	m, err := ParseMeta([]byte(`{"schema_version":"` + MetaSchema + `","name":"ok","port":80}`))
	if err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}
	if m.Name != "ok" {
		t.Fatalf("Name = %q, want ok", m.Name)
	}
}

func TestNormalizeFallbackLadder(t *testing.T) {
	// The "use the bin and a generated icon" rung: nothing but a port.
	var m AppMeta
	m.Normalize("classgo", 8080)
	if m.Name != "classgo" || m.Label != "classgo" || m.Mount != "classgo" {
		t.Errorf("basename fallback: %+v", m)
	}
	if m.Icon != "" {
		t.Errorf("Icon = %q, want empty so the launcher generates one", m.Icon)
	}
	if m.Mode != atlas.WebProxy || m.Auth != AuthSystem || m.Port != 8080 {
		t.Errorf("defaults: %+v", m)
	}

	// An explicit --app <bin>@<port> beats the payload: the operator is looking
	// at this host, the binary is describing itself in the abstract.
	m2 := AppMeta{Port: 1234}
	m2.Normalize("classgo", 9999)
	if m2.Port != 9999 {
		t.Errorf("Port = %d, want the spec port 9999 to win", m2.Port)
	}
	m3 := AppMeta{Port: 1234}
	m3.Normalize("classgo", 0)
	if m3.Port != 1234 {
		t.Errorf("Port = %d, want the payload port when no spec port", m3.Port)
	}
}

func TestValidate(t *testing.T) {
	base := func() AppMeta {
		m := AppMeta{Port: 8080}
		m.Normalize("classgo", 0)
		return m
	}
	if err := base().Validate(map[string]bool{}); err != nil {
		t.Fatalf("baseline rejected: %v", err)
	}

	bad := func(name string, mutate func(*AppMeta), taken map[string]bool) {
		t.Helper()
		m := base()
		mutate(&m)
		if taken == nil {
			taken = map[string]bool{}
		}
		if err := m.Validate(taken); err == nil {
			t.Errorf("%s: want rejection, got nil", name)
		}
	}

	bad("console-reserved mount", func(m *AppMeta) { m.Mount = "meta" }, nil)
	// login/shell are unprotected TODAY only because no third party could
	// declare a mount at all; admitting one without this would let an app
	// shadow the console's own sign-in page.
	bad("login shadow", func(m *AppMeta) { m.Mount = "login" }, nil)
	bad("shell shadow", func(m *AppMeta) { m.Mount = "shell" }, nil)
	bad("atlas-reserved mount", func(m *AppMeta) { m.Mount = "api" }, nil)
	bad("slash in mount", func(m *AppMeta) { m.Mount = "a/b" }, nil)
	bad("duplicate mount", func(m *AppMeta) { m.Mount = "relay" }, map[string]bool{"relay": true})
	bad("non-proxy mode", func(m *AppMeta) { m.Mode = atlas.WebInProcess }, nil)
	bad("no port", func(m *AppMeta) { m.Port = 0 }, nil)
	bad("bad auth", func(m *AppMeta) { m.Auth = "root" }, nil)
	bad("relative login_path", func(m *AppMeta) { m.Auth = AuthCustom; m.LoginPath = "login" }, nil)

	// The icon is interpolated into an SVG d= attribute, so it must never be
	// able to close one.
	for _, icon := range []string{
		`M4 8" /><script>alert(1)</script>`,
		`"><img src=x onerror=y>`,
		`M4 8 <b>`,
	} {
		bad("icon injection "+icon, func(m *AppMeta) { m.Icon = icon }, nil)
	}
	for _, icon := range []string{"M4 8a2 2 0 0 1 2-2z", "🚀", "📊"} {
		m := base()
		m.Icon = icon
		if err := m.Validate(map[string]bool{}); err != nil {
			t.Errorf("icon %q rejected: %v", icon, err)
		}
	}
}

// The drift guard: every atlas web surface must render to a valid AppMeta and
// survive a round trip. This is what keeps `bashy <verb> meta`, `<bin> meta` and
// GET /meta from becoming three schemas.
func TestSurfaceRoundTripsThroughAppMeta(t *testing.T) {
	for name, w := range atlas.WebSurfaces() {
		m := FromSurface(name, w)
		if m.SchemaVersion != MetaSchema {
			t.Errorf("%s: schema %q", name, m.SchemaVersion)
		}
		if m.Label == "" || m.Mount == "" {
			t.Errorf("%s: empty label/mount: %+v", name, m)
		}
		var buf strings.Builder
		if err := WriteMeta(&buf, m); err != nil {
			t.Fatalf("%s: WriteMeta: %v", name, err)
		}
		back, err := ParseMeta([]byte(buf.String()))
		if err != nil {
			t.Fatalf("%s: round trip: %v", name, err)
		}
		if back.Label != m.Label || back.Mount != m.Mount || back.Icon != m.Icon || back.Tip != m.Tip {
			t.Errorf("%s: round trip changed the payload: %+v vs %+v", name, back, m)
		}
	}
}
