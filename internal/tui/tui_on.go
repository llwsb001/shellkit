//go:build tui

// Package tui is the terminal shell: no bubbletea, just ANSI and a reader on
// stdin, so a bare Linux box or Termux can run it with zero dependencies.
package tui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/llwsb001/shellkit/internal/core"
)

const (
	alt  = "\x1b[?1049h"
	off  = "\x1b[?1049l"
	home = "\x1b[H"
	clr  = "\x1b[2J"
	bold = "\x1b[1m"
	dim  = "\x1b[2m"
	rst  = "\x1b[0m"
)

// Run starts the gateway (so other shells can talk to it) and draws the note
// list until the user presses q.
func Run(ctx context.Context, srv *core.Server) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe(runCtx) }()
	select {
	case err := <-errc:
		return err // port taken, permission denied: say so instead of drawing
	case <-time.After(150 * time.Millisecond):
	case <-ctx.Done():
		return nil
	}

	fmt.Print(alt + clr)
	defer fmt.Print(off + rst)

	quit := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			if strings.TrimSpace(sc.Text()) == "q" {
				close(quit)
				return
			}
		}
	}()

	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		draw(srv)
		select {
		case <-quit:
			return nil
		case err := <-errc:
			return err
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

func draw(srv *core.Server) {
	fmt.Print(home + clr)
	st := srv.Snapshot()
	fmt.Printf("%sshellkit%s %s· %d notes · %s%s\n", bold, rst, dim, st.Count, st.File, rst)

	notes, err := core.List("", 40)
	if err != nil {
		fmt.Printf("  %sread error: %v%s\n", dim, err, rst)
	} else if len(notes) == 0 {
		fmt.Printf("\n  %snothing yet — shellkit add \"something worth keeping\"%s\n", dim, rst)
	} else {
		fmt.Println()
		for _, n := range notes {
			fmt.Printf("  %s  %s\n", dim+fmtTime(n), oneLine(n.Text)+rst)
		}
	}
	fmt.Printf("\n%s   q quit · refreshes once a second · %s%s\n", dim, srv.Addr(), rst)
}

func fmtTime(n core.Note) string { return n.At.Local().Format("01-02 15:04") }

func oneLine(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ↵ ")
	if len([]rune(s)) > 88 {
		s = string([]rune(s)[:88]) + "…"
	}
	return s
}
