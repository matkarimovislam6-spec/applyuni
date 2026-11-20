//go:build live

package portal

// Run with: go test -tags live -run TestPolimiLive ./internal/portal/
// Opens the real Polimi registration page and checks our selectors still
// exist. Read-only: it types nothing and presses nothing but "Sign up".

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestPolimiLive(t *testing.T) {
	path, err := exec.LookPath("google-chrome")
	if err != nil {
		t.Skip("Chrome not installed")
	}
	alloc, cancel := chromedp.NewExecAllocator(context.Background(), append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(path))...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(alloc)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var text string
	err = chromedp.Run(ctx,
		chromedp.Navigate(polimiRegisterURL),
		chromedp.Click(`a[href*="evn_REGISTRATI"]`, chromedp.ByQuery),
		chromedp.WaitVisible("#input_cognome", chromedp.ByQuery),
		chromedp.Text("body", &text, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("open registration: %v", err)
	}
	if !strings.Contains(strings.ToLower(text), "registration step 1/4") {
		t.Errorf("not on step 1/4")
	}
	for _, sel := range PolimiSelectors() {
		var n int
		if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelectorAll(`+"`"+sel+"`"+`).length`, &n)); err != nil || n == 0 {
			t.Errorf("selector %s missing on the live page", sel)
		}
	}
	var uz string
	chromedp.Run(ctx, chromedp.Evaluate(`[...document.querySelectorAll('#input_cittadin option')].find(o => o.value === '`+polimiUzbekistan+`')?.textContent.trim()`, &uz))
	if uz != "UZBEK" {
		t.Errorf("citizenship %s is %q, want UZBEK", polimiUzbekistan, uz)
	}
}
