//go:build desktop && !nogui

// This file is only compiled with -tags desktop, on a machine that has the
// native webview headers (GTK3 + WebKitGTK 4.1 on Linux, WebView2 on Windows,
// WKWebView on macOS). The plain build never sees it, which is why the module
// can carry the Wails dependency without breaking `go build ./...` on a box
// with no GUI toolchain.
package desktop

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/llwsb001/shellkit/internal/core"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Run opens the window on the core's own HTTP surface: the shell adds no
// rendering code, it just points a native webview at the gateway that serves
// the same embedded page every other shell uses.
func Run(ctx context.Context, srv *core.Server) error {
	go func() { _ = srv.ListenAndServe(ctx) }()
	if err := waitPort(srv.Addr(), 5*time.Second); err != nil {
		return err
	}

	app := application.New(application.Options{
		Name:        "shellkit",
		Description: "notes, in a window",
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "shellkit",
		Width:     820,
		Height:    680,
		MinWidth:  480,
		MinHeight: 420,
		URL:       pageURL(srv.Addr()),
	})
	return app.Run()
}

func pageURL(addr string) string {
	h := addr
	if strings.HasPrefix(h, ":") {
		h = "127.0.0.1" + h
	}
	if i := strings.Index(h, "0.0.0.0"); i >= 0 {
		h = "127.0.0.1" + h[i+len("0.0.0.0"):]
	}
	return "http://" + h + "/"
}

func waitPort(addr string, d time.Duration) error {
	h := pageURL(addr)
	h = strings.TrimSuffix(strings.TrimPrefix(h, "http://"), "/")
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", h, 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("gateway did not come up on %s", addr)
}
