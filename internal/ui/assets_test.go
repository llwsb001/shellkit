//go:build !nogui

package ui

// Byte budgets for the hand-written page. Every number here is a ceiling, not
// a target: the check fails when a file grows past it, so adding a framework
// or a 300 KB chart library cannot happen quietly. When a size increase is
// deliberate, raise the number in the same commit and say why.

import (
	"io/fs"
	"strconv"
	"testing"
)

var budgets = map[string]int{
	"app.js":     3400,
	"app.css":    3000,
	"index.html": 1700,
}

func TestAssetBudget(t *testing.T) {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for name, max := range budgets {
		b, err := fs.ReadFile(sub, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		total += len(b)
		if len(b) > max {
			t.Errorf("%s is %s bytes, budget %s — raise it deliberately or shrink the file",
				name, comma(len(b)), comma(max))
		}
	}
	// Below the sum of the per-file ceilings, so the total is a real limit
	// and not a check that can never fire.
	if total > 7800 {
		t.Errorf("page total %s bytes, budget 7800", comma(total))
	}
	t.Logf("page is %s bytes across %d files", comma(total), len(budgets))
}

func comma(n int) string { return strconv.Itoa(n) }
