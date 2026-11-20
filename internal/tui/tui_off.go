//go:build !tui

package tui

import (
	"context"
	"fmt"

	"github.com/llwsb001/shellkit/internal/core"
)

// Run is the stub a build without -tags tui gets. It refuses by name, so
// "one binary, several shells" never silently degrades into "missing command".
func Run(context.Context, *core.Server) error {
	return fmt.Errorf("this binary was built without -tags tui; rebuild: make tui")
}
