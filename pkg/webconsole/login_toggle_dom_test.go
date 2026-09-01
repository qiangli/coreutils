//go:build verifydom

package webconsole

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// A real-browser check that the password toggle shows exactly ONE icon and
// swaps it on click. Behind a build tag because it needs a Chrome on the host,
// which CI does not provide:
//
//	go test ./pkg/webconsole -tags verifydom -run TestVerifyDOMPasswordToggle
//
// It earns its keep: the source-level test next door cannot see the CASCADE,
// and the shipped bug lived exactly there. Removing .password-toggle
// svg[hidden] from login.go makes this report eye=block eye-off=block — both
// icons at once — while every other test in this package still passes.
func TestVerifyDOMPasswordToggle(t *testing.T) {
	h := newTestHandler(t, Options{})
	srv := httptest.NewServer(h)
	defer srv.Close()

	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()
	ctx, tcancel := context.WithTimeout(ctx, 60*time.Second)
	defer tcancel()

	var eye0, off0, eye1, off1, type0, type1 string
	err := chromedp.Run(ctx,
		chromedp.Navigate(srv.URL+"/login"),
		chromedp.WaitVisible(`#password-toggle`, chromedp.ByQuery),
		chromedp.Evaluate(`getComputedStyle(document.querySelector(".eye")).display`, &eye0),
		chromedp.Evaluate(`getComputedStyle(document.querySelector(".eye-off")).display`, &off0),
		chromedp.Evaluate(`document.getElementById("login-password").type`, &type0),
		chromedp.Click(`#password-toggle`, chromedp.ByQuery),
		chromedp.Evaluate(`getComputedStyle(document.querySelector(".eye")).display`, &eye1),
		chromedp.Evaluate(`getComputedStyle(document.querySelector(".eye-off")).display`, &off1),
		chromedp.Evaluate(`document.getElementById("login-password").type`, &type1),
	)
	if err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	t.Logf("initial : eye=%s eye-off=%s inputType=%s", eye0, off0, type0)
	t.Logf("clicked : eye=%s eye-off=%s inputType=%s", eye1, off1, type1)

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
