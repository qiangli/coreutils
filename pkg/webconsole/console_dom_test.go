// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build verifydom

// End-to-end browser checks for the console's UI.
//
// WHY THESE EXIST. Every other test in this package reads the SERVED BYTES:
// it can assert that a string is present, and nothing more. It cannot see the
// cascade, it cannot see the DOM, and it cannot see a script that throws. Four
// shipped defects in one sitting all lived in exactly that blind spot:
//
//   - both password-eye icons rendered, because the UA's [hidden] rule does not
//     apply to an inline <svg> — a cascade fact;
//   - the eye never swapped, because `hidden` is an HTMLElement property that
//     SVGElement does not have — a DOM fact;
//   - the Files Apps control vanished, because an <svg> gets no box from
//     `.action i` — a layout fact;
//   - Settings stopped opening, because buildPairing threw NotFoundError before
//     the dialog was shown — a runtime fact, and the one that proves the point:
//     a fix in the pairing section broke a control three sections away, and
//     every byte-level test still passed.
//
// So the rule these encode: a page that throws is a broken page, whatever its
// bytes say. assertNoJSErrors is the check that would have caught all four.
//
// Run:  go test ./pkg/webconsole -tags verifydom
// They need a Chrome on the host, which is why they are behind a tag.

package webconsole

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/qiangli/coreutils/pkg/websession"
)

// domEnv boots a console, a browser, and an exception recorder. The recorder is
// the point: chromedp does not fail a run because the page threw, so an
// uncaught error is invisible unless something is listening for it.
func domEnv(t *testing.T, opts Options) (string, context.Context, func() []string) {
	t.Helper()
	h := newTestHandler(t, opts)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	ctx, cancel := chromedp.NewContext(context.Background())
	t.Cleanup(cancel)
	ctx, tcancel := context.WithTimeout(ctx, 90*time.Second)
	t.Cleanup(tcancel)

	var errs []string
	chromedp.ListenTarget(ctx, func(ev any) {
		switch e := ev.(type) {
		case *runtime.EventExceptionThrown:
			errs = append(errs, "uncaught exception: "+e.ExceptionDetails.Error())
		case *runtime.EventConsoleAPICalled:
			if e.Type == "error" {
				for _, a := range e.Args {
					errs = append(errs, "console.error: "+string(a.Value))
				}
			}
		}
	})
	return srv.URL, ctx, func() []string { return errs }
}

func assertNoJSErrors(t *testing.T, where string, errs []string) {
	t.Helper()
	for _, e := range errs {
		t.Errorf("%s: %s", where, e)
	}
}

// The launcher must render its tiles and throw nothing.
func TestDOMLauncherRendersCleanly(t *testing.T) {
	base, ctx, errs := domEnv(t, Options{})

	var body, tiles string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/"),
		chromedp.Sleep(1500*time.Millisecond),
		chromedp.Evaluate(`document.body.innerText.slice(0,200)`, &body),
		chromedp.Evaluate(`String(document.querySelectorAll("a,button").length)`, &tiles),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	assertNoJSErrors(t, "launcher", errs())
	if !strings.Contains(body, "APPS") {
		t.Errorf("launcher did not render its app list: %q", body)
	}
	if tiles == "0" {
		t.Error("launcher rendered no interactive elements")
	}
}

// Settings must actually OPEN. It regressed to a dialog that stayed shut
// because a section built before it threw, and the byte-level tests could not
// tell: every string they look for was still in the page.
func TestDOMSettingsDialogOpens(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts Options
	}{
		{"pairing unarmed", Options{}},
		{"pairing armed", Options{
			Pairing:       true,
			PairStorePath: t.TempDir() + "/pair.json",
			Sessions:      websession.NewStore(time.Hour, []byte("test-key-test-key-test-key-32byt")),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, ctx, errs := domEnv(t, tc.opts)

			var open, text string
			if err := chromedp.Run(ctx,
				chromedp.Navigate(base+"/"),
				chromedp.Sleep(1500*time.Millisecond),
				chromedp.Click(`#settings-btn`, chromedp.ByQuery),
				chromedp.Sleep(1200*time.Millisecond),
				chromedp.Evaluate(`String(document.getElementById("settings")?.open)`, &open),
				chromedp.Evaluate(`document.getElementById("settings")?.innerText||""`, &text),
			); err != nil {
				t.Fatalf("chromedp: %v", err)
			}
			assertNoJSErrors(t, "settings", errs())
			if open != "true" {
				t.Fatalf("Settings did not open (dialog.open=%s)", open)
			}
			// Every section the dialog promises must be built, not just the
			// ones before whichever one threw.
			// Compare case-insensitively: the sheet uppercases headings in CSS
			// and innerText returns RENDERED text, so "Appearance" arrives as
			// "APPEARANCE".
			lower := strings.ToLower(text)
			for _, want := range []string{"appearance", "open apps", "background", "sections", "phone pairing", "favorites"} {
				if !strings.Contains(lower, want) {
					t.Errorf("Settings is missing the %q section", want)
				}
			}
		})
	}
}

// The login form's show/hide control must show exactly ONE icon and swap it.
func TestDOMPasswordToggleSwaps(t *testing.T) {
	base, ctx, errs := domEnv(t, Options{})

	var eye0, off0, type0, eye1, off1, type1 string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/login"),
		chromedp.WaitVisible(`#password-toggle`, chromedp.ByQuery),
		chromedp.Evaluate(`getComputedStyle(document.querySelector(".eye")).display`, &eye0),
		chromedp.Evaluate(`getComputedStyle(document.querySelector(".eye-off")).display`, &off0),
		chromedp.Evaluate(`document.getElementById("login-password").type`, &type0),
		chromedp.Click(`#password-toggle`, chromedp.ByQuery),
		chromedp.Evaluate(`getComputedStyle(document.querySelector(".eye")).display`, &eye1),
		chromedp.Evaluate(`getComputedStyle(document.querySelector(".eye-off")).display`, &off1),
		chromedp.Evaluate(`document.getElementById("login-password").type`, &type1),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	assertNoJSErrors(t, "login", errs())
	if eye0 == "none" || off0 != "none" {
		t.Errorf("initial state must show ONLY the eye: eye=%s eye-off=%s", eye0, off0)
	}
	if eye1 != "none" || off1 == "none" {
		t.Errorf("after click must show ONLY the crossed-out eye: eye=%s eye-off=%s", eye1, off1)
	}
	if type0 != "password" || type1 != "text" {
		t.Errorf("input type must go password -> text, got %s -> %s", type0, type1)
	}
}

// Pairing: no on/off switch, a QR when asked, and Refresh only once there is a
// code to replace.
func TestDOMPairingSection(t *testing.T) {
	base, ctx, errs := domEnv(t, Options{
		Pairing:       true,
		PairStorePath: t.TempDir() + "/pair.json",
		Sessions:      websession.NewStore(time.Hour, []byte("test-key-test-key-test-key-32byt")),
	})

	var toggles, refreshBefore, panel, refreshAfter string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/"),
		chromedp.Sleep(1500*time.Millisecond),
		chromedp.Click(`#settings-btn`, chromedp.ByQuery),
		chromedp.Sleep(1200*time.Millisecond),
		chromedp.Evaluate(`String(document.querySelectorAll("#pair-toggle").length)`, &toggles),
		chromedp.Evaluate(`String(document.getElementById("pair-refresh")?.hidden)`, &refreshBefore),
		chromedp.Click(`#pair-btn`, chromedp.ByQuery),
		chromedp.Sleep(1800*time.Millisecond),
		chromedp.Evaluate(`(document.getElementById("pair-panel")?.innerHTML||"").slice(0,300)`, &panel),
		chromedp.Evaluate(`String(document.getElementById("pair-refresh")?.hidden)`, &refreshAfter),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	assertNoJSErrors(t, "pairing", errs())
	if toggles != "0" {
		t.Errorf("phone pairing still has an on/off switch (%s found)", toggles)
	}
	if !strings.Contains(panel, "data:image/png;base64,") {
		t.Errorf("no QR rendered after asking for a code: %q", panel)
	}
	if refreshBefore != "true" || refreshAfter != "false" {
		t.Errorf("refresh visibility wrong: before=%s after=%s (want true -> false)",
			refreshBefore, refreshAfter)
	}
}

// Every panel must load and throw nothing. This is the sweep that catches a fix
// in one app breaking another.
func TestDOMEveryPanelLoadsCleanly(t *testing.T) {
	for _, panel := range []string{"/", "/board/", "/mb/", "/files/", "/relay/", "/term/"} {
		t.Run(panel, func(t *testing.T) {
			base, ctx, errs := domEnv(t, Options{})
			var title string
			if err := chromedp.Run(ctx,
				chromedp.Navigate(base+panel),
				chromedp.Sleep(2500*time.Millisecond),
				chromedp.Evaluate(`document.title`, &title),
			); err != nil {
				t.Fatalf("chromedp %s: %v", panel, err)
			}
			assertNoJSErrors(t, panel, errs())
			if title == "" {
				t.Errorf("%s rendered no title", panel)
			}
		})
	}
}

// The Files app carries the console's own return control: four rounded squares,
// laid out with a real box, pointing at the LAUNCHER rather than back at
// itself. It is mounted rather than chrome-injected, so it never receives
// #all-apps-btn and has to draw the same mark itself.
func TestDOMFilesAppsControl(t *testing.T) {
	base, ctx, errs := domEnv(t, Options{})

	var present, rects, href, box string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/files/"),
		chromedp.Sleep(3500*time.Millisecond),
		chromedp.Evaluate(`String(!!document.querySelector(".apps-button"))`, &present),
		chromedp.Evaluate(`String(document.querySelectorAll(".apps-button svg rect").length)`, &rects),
		chromedp.Evaluate(`document.querySelector(".apps-button")?.getAttribute("href")||"NONE"`, &href),
		chromedp.Evaluate(`(()=>{const e=document.querySelector(".apps-button svg");
			if(!e) return "NO SVG";
			const r=e.getBoundingClientRect();
			return JSON.stringify({w:Math.round(r.width),h:Math.round(r.height)});})()`, &box),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	assertNoJSErrors(t, "files", errs())
	if present != "true" {
		t.Fatal("the Files header has no Apps control")
	}
	if rects != "4" {
		t.Errorf("Apps mark has %s rects, want the console's 4-square mark", rects)
	}
	if href != "/" {
		t.Errorf("Apps control points at %q, want %q (the launcher, not the files app)", href, "/")
	}
	// A control with no box is a control nobody can click — the exact way this
	// one disappeared when it stopped being an icon-font glyph.
	if strings.Contains(box, `"w":0`) || strings.Contains(box, `"h":0`) || box == "NO SVG" {
		t.Errorf("Apps mark has no layout box: %s", box)
	}
}

// Background is a setting that must actually TAKE. Choosing a swatch has to
// change what the launcher renders, not merely mark a button pressed — a
// pressed button with no visual change is the shape of a setting that silently
// does nothing.
func TestDOMBackgroundSwatchApplies(t *testing.T) {
	base, ctx, errs := domEnv(t, Options{})

	var swatches, before, pressed, after string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/"),
		chromedp.Sleep(1500*time.Millisecond),
		chromedp.Click(`#settings-btn`, chromedp.ByQuery),
		chromedp.Sleep(1200*time.Millisecond),
		chromedp.Evaluate(`String(document.querySelectorAll("#bg-swatches .swatch-btn").length)`, &swatches),
		chromedp.Evaluate(`document.querySelector("[data-bashy-bg]")?.getAttribute("data-bashy-bg")||"MISSING"`, &before),
		// The last swatch is a real background; the first is "None".
		chromedp.Evaluate(`(()=>{const b=[...document.querySelectorAll("#bg-swatches .swatch-btn")];
			if(!b.length) return "NONE"; b[b.length-1].click(); return "clicked";})()`, &pressed),
		chromedp.Sleep(1200*time.Millisecond),
		chromedp.Evaluate(`document.querySelector("[data-bashy-bg]")?.getAttribute("data-bashy-bg")||"MISSING"`, &after),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	assertNoJSErrors(t, "background", errs())
	if swatches == "0" {
		t.Fatal("the Background section rendered no swatches")
	}
	if pressed != "clicked" {
		t.Fatalf("could not click a background swatch: %s", pressed)
	}
	if before == "MISSING" || after == "MISSING" {
		t.Fatalf("no element carries data-bashy-bg (before=%q after=%q)", before, after)
	}
	if after == before {
		t.Errorf("choosing a background changed nothing (data-bashy-bg stayed %q)", before)
	}
}
