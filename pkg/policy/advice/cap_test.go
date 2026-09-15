// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package advice

import (
	"context"
	"reflect"
	"testing"

	"github.com/qiangli/coreutils/pkg/atlas"
)

func mustCap(t *testing.T, spec string) Cap {
	t.Helper()
	c, err := ParseCap(spec)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestParseCap(t *testing.T) {
	c := mustCap(t, " net , read ,net")
	if got := c.String(); got != "net,read" {
		t.Fatalf("cap = %q, want canonical sorted dedup", got)
	}
	for _, bad := range []string{"", " , ", "read,exec", "pure", "cred", "READ"} {
		if _, err := ParseCap(bad); err == nil {
			t.Fatalf("ParseCap(%q) accepted", bad)
		}
	}
}

// The cap vocabulary is exactly the dhnt-6 lattice: everything
// atlas.ProjectEffects can emit parses as a cap atom.
func TestCapVocabularyCoversProjection(t *testing.T) {
	projected := atlas.ProjectEffects(atlas.Effects())
	if len(projected) == 0 {
		t.Fatal("projection emitted nothing")
	}
	for _, e := range projected {
		if !capVocabulary[e] {
			t.Fatalf("projected atom %q is not a cap atom", e)
		}
	}
	// "time" is in the lattice but nothing projects to it; it still parses.
	mustCap(t, "time")
}

func TestExceeded(t *testing.T) {
	c := mustCap(t, "read,net")
	cases := []struct {
		name    string
		effects []string
		want    []string
	}{
		{"within cap", []string{atlas.EffRead, atlas.EffNet}, nil},
		{"remote folds into net", []string{atlas.EffRemote, atlas.EffRead}, nil},
		{"write and destroy exceed, sorted", []string{atlas.EffDestroy, atlas.EffWrite}, []string{"destroy", "write"}},
		{"mixed reports only the excess", []string{atlas.EffRead, atlas.EffSpend}, []string{"spend"}},
		// Atoms the projection drops are outside what a cap constrains.
		{"unprojected atoms pass", []string{atlas.EffPure, atlas.EffExec, atlas.EffCred, atlas.EffPriv, atlas.EffPersist}, nil},
		{"no effects", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.Exceeded(tc.effects); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Exceeded(%v) = %v, want %v", tc.effects, got, tc.want)
			}
		})
	}
}

// A nested guard intersects with the outer cap and can never widen it.
func TestNestedCapsNeverWiden(t *testing.T) {
	ctx := WithCap(context.Background(), mustCap(t, "read"))
	outer, ok := CapFrom(ctx)
	if !ok || outer.String() != "read" {
		t.Fatalf("outer cap = %v, %t", outer, ok)
	}
	// Inner guard asks for MORE than the outer allows: effective stays "read".
	inner := WithCap(ctx, mustCap(t, "read,net,write"))
	eff, ok := CapFrom(inner)
	if !ok || eff.String() != "read" {
		t.Fatalf("widening attempt: effective = %q, want read", eff)
	}
	if got := eff.Exceeded([]string{atlas.EffNet}); !reflect.DeepEqual(got, []string{"net"}) {
		t.Fatalf("net allowed through a widened nested cap: %v", got)
	}
	// Inner guard narrows: effective is the intersection.
	narrowed := WithCap(ctx, mustCap(t, "net"))
	eff, _ = CapFrom(narrowed)
	if eff.String() != "" {
		t.Fatalf("disjoint intersection = %q, want empty", eff)
	}
	if got := eff.Exceeded([]string{atlas.EffRead}); !reflect.DeepEqual(got, []string{"read"}) {
		t.Fatalf("empty cap allowed read: %v", got)
	}
	// The outer context is untouched — a child's cap never leaks upward.
	outer, _ = CapFrom(ctx)
	if outer.String() != "read" {
		t.Fatalf("outer cap mutated to %q", outer)
	}
	// No cap on a fresh context.
	if _, ok := CapFrom(context.Background()); ok {
		t.Fatal("fresh context reported a cap")
	}
}
