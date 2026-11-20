package core

// The one structural rule this project is built on: the core must not know
// that shells exist. If someone adds an import of internal/ui here, the whole
// "one binary, several shells" story stops being true, so the test fails at
// the door.

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var shellPkgs = []string{
	"github.com/llwsb001/shellkit/internal/cli",
	"github.com/llwsb001/shellkit/internal/desktop",
	"github.com/llwsb001/shellkit/internal/tui",
	"github.com/llwsb001/shellkit/internal/ui",
	"github.com/llwsb001/shellkit/internal/configs",
	"github.com/llwsb001/shellkit/internal/update",
}

func TestCoreDoesNotImportShells(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") {
			continue // tests included: a test-only leak is still a leak
		}
		f, err := parser.ParseFile(fset, filepath.Join(".", e.Name()), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			for _, shell := range shellPkgs {
				if p == shell || strings.HasPrefix(p, shell+"/") {
					t.Errorf("%s imports shell package %s", e.Name(), p)
				}
			}
			if strings.Contains(p, "wailsapp") {
				t.Errorf("%s imports a GUI toolkit (%s); the core stays headless", e.Name(), p)
			}
		}
	}
}
