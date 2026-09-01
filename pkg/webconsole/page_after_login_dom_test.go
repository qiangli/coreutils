//go:build verifydom && e2e

package webconsole

import (
	"context"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/qiangli/coreutils/pkg/websession"
)

// Reproduces the reported flow: sign in on a login-gated console and see
// whether the page that follows actually renders.
func TestVerifyDOMPageAfterLogin(t *testing.T) {
	base := serve(t, Options{
		RequireLogin:  true,
		Pairing:       true,
		PairStorePath: t.TempDir() + "/pair.json",
		Auth:          stubAuth{password: "correct-horse"},
		Sessions:      websession.NewStore(time.Hour, []byte("test-key-test-key-test-key-32byt")),
	})

	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()
	ctx, tcancel := context.WithTimeout(ctx, 60*time.Second)
	defer tcancel()

	var errs []string
	chromedp.ListenTarget(ctx, func(ev any) {
		switch e := ev.(type) {
		case *runtime.EventExceptionThrown:
			errs = append(errs, "EXCEPTION: "+e.ExceptionDetails.Error())
		case *runtime.EventConsoleAPICalled:
			if e.Type == "error" {
				for _, a := range e.Args {
					errs = append(errs, "console.error: "+string(a.Value))
				}
			}
		}
	})

	var url, body, logoutHidden string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/login"),
		chromedp.WaitVisible(`#login-password`, chromedp.ByQuery),
		chromedp.SendKeys(`#login-password`, "correct-horse", chromedp.ByQuery),
		chromedp.Click(`button[type="submit"]`, chromedp.ByQuery),
		chromedp.Sleep(2500*time.Millisecond),
		chromedp.Location(&url),
		chromedp.Evaluate(`document.body.innerText.slice(0,300)`, &body),
		chromedp.Evaluate(`String(document.getElementById("logout")?.hidden)`, &logoutHidden),
	); err != nil {
		t.Fatalf("chromedp: %v", err)
	}
	t.Logf("after login, url = %s", url)
	t.Logf("after login, logout form hidden = %s", logoutHidden)
	t.Logf("after login, body text = %q", body)
	for _, e := range errs {
		t.Errorf("page JS error -> %s", e)
	}
	if len(body) == 0 {
		t.Error("the page after login is BLANK")
	}
}
