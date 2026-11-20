//go:build desktop && nogui

package desktop

import (
	"context"
	"fmt"

	"github.com/llwsb001/shellkit/internal/core"
)

// A window with no page to show is not a shape worth building: refuse it
// instead of opening a 501.
func Run(context.Context, *core.Server) error {
	return fmt.Errorf("desktop needs the page: build with -tags desktop, not -tags \"desktop nogui\"")
}
