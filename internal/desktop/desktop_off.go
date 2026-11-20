//go:build !desktop

package desktop

import (
	"context"
	"fmt"

	"github.com/llwsb001/shellkit/internal/core"
)

// Run is the stub a build without -tags desktop gets.
func Run(context.Context, *core.Server) error {
	return fmt.Errorf("this binary was built without -tags desktop; rebuild: make desktop")
}
