//go:build live

package portal

// Run with: go test -tags live -run TestDreamApplyLive -v ./internal/portal/
// Opens the public start page of DreamApply portals and lists their controls.
// Read-only: it types nothing and presses nothing.

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// DreamApply instances we apply to. One flow should serve all of them.
var dreamApplyPortals = map[string]string{
	"unipd":       "https://apply.unipd.it/",
	"unipd-login": "https://apply.unipd.it/auth/dreamid",
}

func TestDreamApplyLive(t *testing.T) {
	path, err := exec.LookPath("google-chrome")
	if err != nil {
		t.Skip("Chrome not installed")
	}
	for id, url := range dreamApplyPortals {
		t.Run(id, func(t *testing.T) {
			r := &Runner{ChromePath: path, Headless: true}
			browser, closeBrowser, err := r.startBrowser(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer closeBrowser()
			ctx, cancel := context.WithTimeout(browser, 60*time.Second)
			defer cancel()
			if err := chromedp.Run(ctx, chromedp.Navigate(url)); err != nil {
				t.Fatalf("open %s: %v", url, err)
			}
			waitSettled(ctx)
			snap, err := snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s | %s | heading %q", snap.URL, snap.Title, snap.Heading)
			for _, f := range snap.Fields {
				t.Logf("  %-8s %-10s %-45s %q", f.Tag, f.Type, f.Selector, f.Label)
			}
			var links [][2]string
			_ = chromedp.Run(ctx, chromedp.Evaluate(`[...document.querySelectorAll('a[href]')]
				.filter(a => /regist|sign|account|apply|login|log in/i.test(a.innerText + ' ' + a.getAttribute('href')))
				.slice(0, 40).map(a => [a.innerText.replace(/\s+/g,' ').trim().slice(0,60), a.href])`, &links))
			for _, l := range links {
				t.Logf("  link %-60q %s", l[0], l[1])
			}
			if len(snap.Fields) == 0 {
				t.Error("no controls found: the page may have changed or failed to load")
			}
			if id == "unipd-login" {
				// The sign-in step the DreamApply flow relies on.
				if !strings.HasPrefix(snap.URL, dreamIDLogin) {
					t.Errorf("sign-in moved: now at %s", snap.URL)
				}
				var email, cont bool
				for _, f := range snap.Fields {
					email = email || f.Selector == dreamIDEmail
					cont = cont || (f.Tag == "button" && strings.EqualFold(f.Label, "Continue"))
				}
				if !email || !cont {
					t.Errorf("DreamID sign-in changed: email field %v, Continue button %v", email, cont)
				}
			}
		})
	}
}
