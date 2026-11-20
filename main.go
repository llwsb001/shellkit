// shellkit — notes in one file, reachable from a terminal, a browser, or a
// window. One core, several shells, one binary.
package main

import (
	"fmt"
	"os"

	"github.com/llwsb001/shellkit/internal/cli"
)

// version is stamped at build time: -ldflags "-X main.version=…".
var version = "dev"

func main() {
	if err := cli.Main(version, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "shellkit:", err)
		os.Exit(1)
	}
}
