//go:build verifydom

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

// The Phone pairing section must be present with pairing armed, and asking for
// a pass must render a QR rather than nothing.
func TestVerifyDOMPairingSection(t *testing.T) {
	h := newTestHandler(t, Options{
		Pairing:       true,
		PairStorePath: t.TempDir() + "/pair.json",
		Sessions:      websession.NewStore(time.Hour, []byte("test-key-test-key-test-key-32byt")),
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()
	ctx, tcancel := context.WithTimeout(ctx, 60*time.Second)
	defer tcancel()

	var errs []string
	chromedp.ListenTarget(ctx, func(ev any) {
		if e, ok := ev.(*runtime.EventExceptionThrown); ok {
			errs = append(errs, e.ExceptionDetails.Error())
		}
	})

	var sectionText, toggleCount, panelHTML string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(srv.URL+"/"),
		chromedp.Sleep(1200*time.Millisecond),
		chromedp.Click(`#settings-btn`, chromedp.ByQuery),
		chromedp.Sleep(1200*time.Millisecond),
		chromedp.Evaluate(`document.getElementById("pair-section")?.innerText.slice(0,240) || "NO SECTION"`, &sectionText),
		chromedp.Evaluate(`String(document.querySelectorAll("#pair-toggle").length)`, &toggleCount),
		chromedp.Click(`#pair-btn`, chromedp.ByQuery),
		chromedp.Sleep(1500*time.Millisecond),
		chromedp.Evaluate(`(document.getElementById("pair-panel")?.innerHTML || "").slice(0,200)`, &panelHTML),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	t.Logf("pair section text: %q", sectionText)
	t.Logf("#pair-toggle count: %s", toggleCount)
	t.Logf("pair panel html: %q", panelHTML)
	for _, e := range errs {
		t.Errorf("page JS error -> %s", e)
	}

	// The section reads like Appearance or Background: always present, no
	// on/off row. A switch implies a persistent setting, and a pairing pass is
	// a single-use, time-boxed credential — an action, not a state.
	if toggleCount != "0" {
		t.Errorf("#pair-toggle still exists (%s); phone pairing must not have an on/off switch", toggleCount)
	}
	if !strings.Contains(sectionText, "PHONE PAIRING") && !strings.Contains(sectionText, "Phone pairing") {
		t.Errorf("the Phone pairing section is not shown: %q", sectionText)
	}
	// Asking for a pass must actually produce the QR a phone can scan.
	if !strings.Contains(panelHTML, "data:image/png;base64,") {
		t.Errorf("no QR image rendered after asking for a code; panel = %q", panelHTML)
	}
}
