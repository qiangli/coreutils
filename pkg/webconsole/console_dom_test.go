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
	"encoding/json"
	"net/http/httptest"
	goruntime "runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
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

	alloc, acancel := chromedp.NewExecAllocator(context.Background(), browserOpts()...)
	t.Cleanup(acancel)
	ctx, cancel := chromedp.NewContext(alloc)
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

// browserOpts builds the browser launch flags.
//
// Chrome's own sandbox is left ON everywhere it works. On Linux it does not:
// Ubuntu 23.10+ (which is what the CI runner is) disables unprivileged user
// namespaces through AppArmor, so the zygote finds no usable sandbox and
// Chrome aborts before the first navigation — every test in this file failed
// that way on the Linux leg while passing on macOS and Windows.
//
// --no-sandbox is the documented workaround and is safe HERE for a reason that
// does not generalise: this browser is disposable, it is launched by the test,
// and the only thing it ever loads is the loopback console under test. The
// alternative is not a safer Linux run, it is no Linux run at all.
func browserOpts() []chromedp.ExecAllocatorOption {
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	if goruntime.GOOS == "linux" {
		opts = append(opts, chromedp.NoSandbox)
	}
	return opts
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

// Long press on a tile must NOT open the browser's context menu.
//
// Favouriting on a phone already works: the long press puts the tile in its
// hover state, the star appears, and it can be tapped — verified on a real
// Samsung, which also corrected an earlier assumption here that a favorite
// could never be added on touch at all. What ruins the gesture is that the same
// press opens the browser's own long list of menu items over the star. So the
// only thing to fix, and the only thing pinned here, is that menu.
//
// It emulates a real touch device (mobile metrics + touch events) rather than
// setEmulatedMedia, which does not implement the `hover` feature — measured:
// matchMedia("(hover: none)") still reported false under it.
func TestDOMLongPressMenuSuppressedOnTiles(t *testing.T) {
	base, ctx, errs := domEnv(t, Options{})

	var hoverNone, onTileTouch, onTileMouse, offTile string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/"),
		emulation.SetDeviceMetricsOverride(390, 844, 3, true),
		emulation.SetTouchEmulationEnabled(true).WithMaxTouchPoints(5),
		chromedp.Navigate(base+"/"),
		chromedp.Sleep(1800*time.Millisecond),
		chromedp.Evaluate(`String(matchMedia("(hover: none)").matches)`, &hoverNone),

		// A touch long-press on a tile: the menu must be suppressed.
		chromedp.Evaluate(`(()=>{const w=document.querySelector(".tile-wrap");
			if(!w) return "NO TILE";
			const ev=new PointerEvent("contextmenu",{bubbles:true,cancelable:true,pointerType:"touch"});
			w.dispatchEvent(ev); return String(ev.defaultPrevented);})()`, &onTileTouch),

		// A MOUSE right-click keeps its menu: a desktop user has no long press
		// and must not lose copy-link or open-in-new-tab.
		chromedp.Evaluate(`(()=>{const w=document.querySelector(".tile-wrap");
			const ev=new PointerEvent("contextmenu",{bubbles:true,cancelable:true,pointerType:"mouse"});
			w.dispatchEvent(ev); return String(ev.defaultPrevented);})()`, &onTileMouse),

		// And suppression is SCOPED to tiles: the rest of the page keeps its
		// menu, so copy/share are not taken away console-wide.
		chromedp.Evaluate(`(()=>{const ev=new PointerEvent("contextmenu",{bubbles:true,cancelable:true,pointerType:"touch"});
			document.body.dispatchEvent(ev); return String(ev.defaultPrevented);})()`, &offTile),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	assertNoJSErrors(t, "long-press", errs())

	if hoverNone != "true" {
		t.Fatalf("device emulation did not produce a no-hover device (matchMedia=%s)", hoverNone)
	}
	if onTileTouch != "true" {
		t.Errorf("the browser long-press menu still opens on a tile (defaultPrevented=%s); "+
			"it covers the star the press just revealed", onTileTouch)
	}
	if onTileMouse != "false" {
		t.Errorf("a MOUSE right-click on a tile lost its menu (defaultPrevented=%s); "+
			"suppression must apply to the touch gesture only", onTileMouse)
	}
	if offTile != "false" {
		t.Errorf("context menu suppressed off-tile too (defaultPrevented=%s); "+
			"that would cost copy-link and share across the whole console", offTile)
	}
}

// The operator chooses what a paired phone may reach, in Settings.
//
// Before this, a pass was always board/mb/relay and `bashy apps pair --allow`
// was the only way to widen it — so a phone that hit "terminal is not in the
// scope" got a refusal with no way to act on it. Two properties matter and both
// are pinned: the shell and the filesystem are OFF by default (a grant must be
// a decision, not an unread default), and ticking one is actually honoured in
// the minted ticket.
func TestDOMPairingScopeIsChosenByTheOperator(t *testing.T) {
	base, ctx, errs := domEnv(t, Options{
		Pairing:       true,
		PairStorePath: t.TempDir() + "/pair.json",
		Sessions:      websession.NewStore(time.Hour, []byte("test-key-test-key-test-key-32byt")),
	})

	var boxes, checkedByDefault, scopeAfter string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/"),
		chromedp.Sleep(1500*time.Millisecond),
		chromedp.Click(`#settings-btn`, chromedp.ByQuery),
		chromedp.Sleep(1200*time.Millisecond),
		chromedp.Evaluate(`String(document.querySelectorAll(".pair-scope-box").length)`, &boxes),
		chromedp.Evaluate(`JSON.stringify([...document.querySelectorAll(".pair-scope-box")]
			.filter(b=>b.checked).map(b=>b.value).sort())`, &checkedByDefault),

		// Tick the shell, ask for a code, and read back what the server minted.
		chromedp.Evaluate(`(()=>{const t=[...document.querySelectorAll(".pair-scope-box")]
			.find(b=>b.value==="terminal");
			if(!t) return "NO TERMINAL BOX"; t.checked=true; return "ticked";})()`, new(string)),
		chromedp.Evaluate(`fetch("api/pair",{method:"POST",
			headers:{Accept:"application/json","Content-Type":"application/json"},
			body:JSON.stringify({scope:selectedPairScope()})})
			.then(r=>r.json()).then(d=>JSON.stringify((d.scope||[]).sort()))`, &scopeAfter,
			func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
				return p.WithAwaitPromise(true)
			}),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	assertNoJSErrors(t, "pair scope", errs())

	if boxes == "0" {
		t.Fatal("Settings offers no way to choose what a paired phone may reach")
	}
	// Default-deny: a shell or the filesystem must never be granted by a
	// default nobody read.
	if strings.Contains(checkedByDefault, "terminal") || strings.Contains(checkedByDefault, "files") {
		t.Errorf("terminal/files are pre-granted by default: %s", checkedByDefault)
	}
	if !strings.Contains(checkedByDefault, "board") {
		t.Errorf("the read-and-communicate default is missing: %s", checkedByDefault)
	}
	// And the choice must actually reach the ticket.
	if !strings.Contains(scopeAfter, "terminal") {
		t.Errorf("ticking terminal did not widen the minted pass: scope=%s", scopeAfter)
	}
}

// relay must wear the same header as the other apps.
//
// board, messages and terminal each state a brand block in markup — the
// panel's mark in a rounded tile, then bashy + the panel name — and receive the
// console's four-square all-apps control from injectChrome. relay is a MOUNTED
// SPA, so it is served outside that path and receives neither: it has to draw
// both itself, the same way the Files app does.
func TestDOMRelayHeaderMatchesTheOtherApps(t *testing.T) {
	base, ctx, errs := domEnv(t, Options{})

	var brand, wordmark, allAppsRects, allAppsHref string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/relay/"),
		chromedp.Sleep(3500*time.Millisecond),
		chromedp.Evaluate(`String(!!document.querySelector(`+"`"+`header a[title="bashy relay"]`+"`"+`))`, &brand),
		chromedp.Evaluate(`(document.querySelector(`+"`"+`header a[title="bashy relay"]`+"`"+`)?.innerText||"").trim()`, &wordmark),
		chromedp.Evaluate(`String(document.querySelectorAll(`+"`"+`header a[title="All apps"] svg rect`+"`"+`).length)`, &allAppsRects),
		chromedp.Evaluate(`document.querySelector(`+"`"+`header a[title="All apps"]`+"`"+`)?.getAttribute("href")||"NONE"`, &allAppsHref),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	assertNoJSErrors(t, "relay", errs())

	if brand != "true" {
		t.Error("relay has no brand block; board/messages/terminal all carry one")
	}
	// The wordmark is hidden below sm: the phone header also holds the rooms
	// button and the conversation title, which is what matters there. At this
	// viewport it must be present.
	if !strings.Contains(strings.ToLower(wordmark), "relay") {
		t.Errorf("relay's brand does not name the app: %q", wordmark)
	}
	if allAppsRects != "4" {
		t.Errorf("relay's all-apps control has %s rects, want the console's 4-square mark", allAppsRects)
	}
	if allAppsHref == "NONE" {
		t.Error("relay has no all-apps control")
	}
}

// "Same header as the other apps" has to be MEASURED, or it drifts back into a
// lookalike. This reads the computed box of relay's header and brand and
// compares them with board's — the two are styled by separate stylesheets
// (board links app.css; relay is a mounted SPA that restates the rules), which
// is exactly the kind of duplication that silently diverges.
func TestDOMRelayHeaderMatchesBoardMetrics(t *testing.T) {
	base, ctx, errs := domEnv(t, Options{})

	probe := `(()=>{const h=document.querySelector("header");
		if(!h) return "NO HEADER";
		const cs=getComputedStyle(h);
		const logo=document.querySelector("header .logo, header .console-logo");
		const mark=document.querySelector("header #wordmark, header .console-wordmark");
		const m=mark?getComputedStyle(mark):null;
		return JSON.stringify({
			padding: cs.padding,
			borderBottomWidth: cs.borderBottomWidth,
			logo: logo?Math.round(logo.getBoundingClientRect().width):0,
			wordmarkSize: m?m.fontSize:"",
			wordmarkWeight: m?m.fontWeight:"",
		});})()`

	var boardBox, relayBox string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/board/"),
		chromedp.Sleep(1800*time.Millisecond),
		chromedp.Evaluate(probe, &boardBox),
		chromedp.Navigate(base+"/relay/"),
		chromedp.Sleep(3500*time.Millisecond),
		chromedp.Evaluate(probe, &relayBox),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	assertNoJSErrors(t, "relay metrics", errs())
	t.Logf("board header = %s", boardBox)
	t.Logf("relay header = %s", relayBox)

	if boardBox == "NO HEADER" || relayBox == "NO HEADER" {
		t.Fatalf("missing a header (board=%s relay=%s)", boardBox, relayBox)
	}
	if boardBox != relayBox {
		t.Errorf("relay's header does not match board's.\n board = %s\n relay = %s\n"+
			"They are styled by separate stylesheets — app.css and the meet SPA's "+
			"index.css — so a change to one must be made in both.", boardBox, relayBox)
	}
}

// relay is FIVE parts, in the console's shape: one bar across the top, then
// left / middle / right beneath it, then the footer.
//
// It used to be three columns with the app bar inside the MIDDLE one — the
// header began 280px in (measured in a live browser) and the app carried two
// brands, one in the bar and one in the sidebar. And being a mounted SPA it
// received neither the all-apps control nor the copyright footer that
// injectChrome gives every embedded app.
func TestDOMRelayIsFiveParts(t *testing.T) {
	base, ctx, errs := domEnv(t, Options{})

	var shape, brands, footer string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/relay/"),
		chromedp.Sleep(3500*time.Millisecond),
		chromedp.Evaluate(`(()=>{const h=document.querySelector("header");
			const f=document.querySelector(".console-foot,#app-foot");
			const b=document.body.getBoundingClientRect();
			const hr=h?h.getBoundingClientRect():null;
			return JSON.stringify({
				headerLeft: hr?Math.round(hr.left):-1,
				headerSpansPage: hr?Math.round(hr.width)===Math.round(b.width):false,
				hasFooter: !!f,
			});})()`, &shape),
		// One brand for the app, not one per panel.
		chromedp.Evaluate(`String(document.querySelectorAll(".console-brand").length)`, &brands),
		chromedp.Evaluate(`(document.querySelector(".console-foot")?.innerText||"").slice(0,60)`, &footer),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	assertNoJSErrors(t, "relay shape", errs())
	t.Logf("relay shape = %s", shape)

	if !strings.Contains(shape, `"headerLeft":0`) {
		t.Errorf("relay's app bar does not start at the page edge: %s — "+
			"it is inside a column instead of above all of them", shape)
	}
	if !strings.Contains(shape, `"headerSpansPage":true`) {
		t.Errorf("relay's app bar does not span the page: %s", shape)
	}
	if !strings.Contains(shape, `"hasFooter":true`) {
		t.Error("relay has no footer; injectChrome gives every embedded app one")
	}
	if brands != "1" {
		t.Errorf("relay shows %s brands; the app is named once, in the bar", brands)
	}
	if !strings.Contains(footer, "BASHY") {
		t.Errorf("relay's footer is not the console copyright line: %q", footer)
	}
}

// The three panels' own headers must be the same height, so one unbroken rule
// runs under the app bar instead of a ragged step between columns. They are
// three separate components and drifted to 68/56/68 once already.
func TestDOMRelayPanelHeadersLineUp(t *testing.T) {
	base, ctx, errs := domEnv(t, Options{})

	var heights string
	if err := chromedp.Run(ctx,
		// A desktop viewport, or there is nothing to line up: the sidebar is
		// hidden below lg and the details panel below xl, so a narrow window
		// measures them at height 0 and the check passes for the wrong reason.
		emulation.SetDeviceMetricsOverride(1600, 900, 1, false),
		chromedp.Navigate(base+"/relay/"),
		chromedp.Sleep(3500*time.Millisecond),
		chromedp.Evaluate(`(()=>{const sel="aside > div:first-child, main > header, [class*=border-l] > div:first-child";
			const hs=[...document.querySelectorAll(sel)].map(e=>Math.round(e.getBoundingClientRect().height));
			return JSON.stringify(hs);})()`, &heights),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	assertNoJSErrors(t, "relay panels", errs())
	t.Logf("panel header heights = %s", heights)

	var hs []int
	if err := json.Unmarshal([]byte(heights), &hs); err != nil || len(hs) < 3 {
		t.Fatalf("expected three panel headers, got %s", heights)
	}
	for _, h := range hs {
		if h == 0 {
			t.Fatalf("a panel header measured 0 — the panel is hidden, so this "+
				"would pass without comparing anything: %v", hs)
		}
	}
	for _, h := range hs[1:] {
		if h != hs[0] {
			t.Errorf("the panels' headers are different heights %v — the rule under the "+
				"app bar steps between columns", hs)
			break
		}
	}
}

// relay must follow the console's theme.
//
// The console stores the choice under its own localStorage key and puts
// data-theme on <html>; every other app keys off that attribute. relay keyed
// dark off a `.dark` class that nothing defined and nothing set, so it had no
// dark mode at all — measured: with the console dark and the system dark, relay
// still painted near-white (oklch(0.985 …)).
func TestDOMRelayFollowsTheConsoleTheme(t *testing.T) {
	base, ctx, errs := domEnv(t, Options{})

	read := `JSON.stringify({
		theme: document.documentElement.getAttribute("data-theme"),
		bg: getComputedStyle(document.body).backgroundColor,
	})`
	set := func(theme string) chromedp.Action {
		return chromedp.Evaluate(
			`localStorage.setItem("bashy.apps.config", JSON.stringify({theme:`+
				strconv.Quote(theme)+`}))`, nil)
	}

	var dark, light string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/relay/"),
		chromedp.Sleep(1500*time.Millisecond),
		set("dark"),
		chromedp.Navigate(base+"/relay/"),
		chromedp.Sleep(2500*time.Millisecond),
		chromedp.Evaluate(read, &dark),
		set("light"),
		chromedp.Navigate(base+"/relay/"),
		chromedp.Sleep(2500*time.Millisecond),
		chromedp.Evaluate(read, &light),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	assertNoJSErrors(t, "relay theme", errs())
	t.Logf("console dark  -> relay %s", dark)
	t.Logf("console light -> relay %s", light)

	if !strings.Contains(dark, `"theme":"dark"`) {
		t.Errorf("relay ignored the console's dark choice: %s", dark)
	}
	// The console's own dark background, so the two apps are the same colour
	// rather than two interpretations of "dark".
	if !strings.Contains(dark, "rgb(9, 9, 11)") {
		t.Errorf("relay's dark background is not the console's #09090b: %s", dark)
	}
	if !strings.Contains(light, `"theme":"light"`) {
		t.Errorf("relay ignored the console's light choice: %s", light)
	}
	if dark == light {
		t.Errorf("relay renders identically in both themes: %s", dark)
	}
}

// Meet and Chat are two views of the sidebar, so they are tabs — both visible,
// the selected one obvious, one tap to switch. They used to be a dropdown
// hanging off the app logo.
func TestDOMRelayMeetChatAreTabs(t *testing.T) {
	base, ctx, errs := domEnv(t, Options{})

	var tabs, labels, selected string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/relay/"),
		chromedp.Sleep(3000*time.Millisecond),
		chromedp.Evaluate(`String(document.querySelectorAll("[role=tab]").length)`, &tabs),
		chromedp.Evaluate(`JSON.stringify([...document.querySelectorAll("[role=tab]")].map(t=>t.innerText.trim()))`, &labels),
		chromedp.Evaluate(`String([...document.querySelectorAll("[role=tab]")].filter(t=>t.getAttribute("aria-selected")==="true").length)`, &selected),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	assertNoJSErrors(t, "relay tabs", errs())

	if tabs != "2" {
		t.Errorf("expected two tabs (Meet, Chat), found %s", tabs)
	}
	if !strings.Contains(labels, "Meet") || !strings.Contains(labels, "Chat") {
		t.Errorf("the tabs are not Meet and Chat: %s", labels)
	}
	// A tab strip whose selection is invisible is just two buttons.
	if selected != "1" {
		t.Errorf("%s tabs are marked selected, want exactly 1", selected)
	}
}

// No panel may ignore the theme. relay's sidebar was a dark navy in the LIGHT
// palette — left from this SPA's standalone design, where a permanently dark
// rail was the look. Beside a light console it read as a bug, because nothing
// else in the console has a panel that stays dark when the theme is light.
//
// The mark is checked against BOARD's rather than a literal: the console
// inverts --logo-* between themes, and relay must do whatever the others do.
func TestDOMRelayLightThemeHasNoDarkPanel(t *testing.T) {
	base, ctx, errs := domEnv(t, Options{})

	read := `(()=>{const a=document.querySelector("aside");
		const r=document.querySelector(".console-logo rect, #brand .logo rect");
		return JSON.stringify({
			body: getComputedStyle(document.body).backgroundColor,
			sidebar: a?getComputedStyle(a).backgroundColor:"",
			logo: r?getComputedStyle(r).fill:"",
		});})()`

	var relay, board string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/relay/"),
		chromedp.Sleep(1200*time.Millisecond),
		chromedp.Evaluate(`localStorage.setItem("bashy.apps.config",JSON.stringify({theme:"light"}))`, nil),
		chromedp.Navigate(base+"/relay/"),
		chromedp.Sleep(2500*time.Millisecond),
		chromedp.Evaluate(read, &relay),
		chromedp.Navigate(base+"/board/"),
		chromedp.Sleep(1800*time.Millisecond),
		chromedp.Evaluate(read, &board),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	assertNoJSErrors(t, "relay light", errs())
	t.Logf("light relay = %s", relay)
	t.Logf("light board = %s", board)

	// The console's light page colour, shared by both.
	if !strings.Contains(relay, "rgb(242, 242, 245)") {
		t.Errorf("relay's light page is not the console's #f2f2f5: %s", relay)
	}
	// A dark rail in a light theme is the defect this pins. White is the
	// console's card surface; anything near-black is the old navy back again.
	if !strings.Contains(relay, `"sidebar":"rgb(255, 255, 255)"`) {
		t.Errorf("relay's sidebar is not a light surface in the light theme: %s", relay)
	}
	// Whatever the console does with the mark, relay does too.
	var rj, bj map[string]string
	if json.Unmarshal([]byte(relay), &rj) == nil && json.Unmarshal([]byte(board), &bj) == nil {
		if rj["logo"] != bj["logo"] {
			t.Errorf("relay's mark is %s but board's is %s — the apps' marks must "+
				"track the same --logo-* tokens", rj["logo"], bj["logo"])
		}
	}
}

// ONE mark: same tile in every app and every theme.
//
// It used to invert — dark tile on light, light tile on dark — so the product's
// mark was two different objects depending on a setting, and the dark tile sat
// as a black block in a light header. The colour is computed rather than picked
// (the contrast table is in app.css beside the tokens): a single tile has to
// read against the light page, against the dark page, AND carry a glyph, and
// #2563eb has the best worst case at 3.85:1 with all three above the 3:1 WCAG
// non-text bar. The intuitive pale blue fails: #7dd3fc scores 1.49 against the
// light page and would all but vanish there.
func TestDOMOneMarkEverywhere(t *testing.T) {
	base, ctx, errs := domEnv(t, Options{})

	tile := `(()=>{const r=document.querySelector(".console-logo rect, #brand .logo rect");
		return r?getComputedStyle(r).fill:"NO MARK";})()`

	seen := map[string]string{}
	for _, theme := range []string{"light", "dark"} {
		for _, page := range []string{"/board/", "/mb/", "/term/", "/relay/"} {
			var got string
			if err := chromedp.Run(ctx,
				chromedp.Navigate(base+page),
				chromedp.Sleep(900*time.Millisecond),
				chromedp.Evaluate(`localStorage.setItem("bashy.apps.config",`+
					`JSON.stringify({theme:`+strconv.Quote(theme)+`}))`, nil),
				chromedp.Navigate(base+page),
				chromedp.Sleep(2200*time.Millisecond),
				chromedp.Evaluate(tile, &got),
			); err != nil {
				t.Fatalf("chromedp %s %s: %v", theme, page, err)
			}
			seen[theme+" "+page] = got
		}
	}
	assertNoJSErrors(t, "one mark", errs())

	var first, firstKey string
	for k, v := range seen {
		t.Logf("%-16s %s", k, v)
		if v == "NO MARK" {
			t.Errorf("%s has no mark", k)
			continue
		}
		if first == "" {
			first, firstKey = v, k
			continue
		}
		if v != first {
			t.Errorf("the mark differs: %s is %s but %s is %s — one mark, every app, "+
				"every theme", k, v, firstKey, first)
		}
	}
	// And it is the computed colour, not whatever happens to be consistent.
	if first != "rgb(37, 99, 235)" {
		t.Errorf("the mark is %s, want the computed #2563eb", first)
	}
}

// The Files panel must follow the console's theme too.
//
// It is a mounted third-party SPA (File Browser) with its OWN theme mechanism:
// a server-injected setting, else the media preference, written to
// documentElement.className. Nothing in that chain can see the console's
// choice, so Files sat dark while every other app was light — reported as
// "the files app always shows in dark mode".
//
// Same fix as relay: the served template resolves the console's stored choice
// before the bundle boots and writes it where this app already looks for it.
func TestDOMFilesFollowsTheConsoleTheme(t *testing.T) {
	base, ctx, errs := domEnv(t, Options{})

	read := `JSON.stringify({
		cls: document.documentElement.className,
		bg: getComputedStyle(document.body).backgroundColor,
	})`
	set := func(theme string) chromedp.Action {
		return chromedp.Evaluate(
			`localStorage.setItem("bashy.apps.config", JSON.stringify({theme:`+
				strconv.Quote(theme)+`}))`, nil)
	}

	var dark, light string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/files/"),
		chromedp.Sleep(1500*time.Millisecond),
		set("dark"),
		chromedp.Navigate(base+"/files/"),
		chromedp.Sleep(2500*time.Millisecond),
		chromedp.Evaluate(read, &dark),
		set("light"),
		chromedp.Navigate(base+"/files/"),
		chromedp.Sleep(2500*time.Millisecond),
		chromedp.Evaluate(read, &light),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	assertNoJSErrors(t, "files theme", errs())
	t.Logf("console dark  -> files %s", dark)
	t.Logf("console light -> files %s", light)

	if !strings.Contains(dark, `"cls":"dark"`) {
		t.Errorf("files ignored the console's dark choice: %s", dark)
	}
	if !strings.Contains(light, `"cls":"light"`) {
		t.Errorf("files ignored the console's light choice: %s", light)
	}
	if dark == light {
		t.Errorf("files renders identically in both themes: %s", dark)
	}
}
