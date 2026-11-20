// Package cli is the default shell: the command tree. It is the only shell
// every build has, and the one the container runs.
package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/llwsb001/shellkit/internal/configs"
	"github.com/llwsb001/shellkit/internal/core"
	"github.com/llwsb001/shellkit/internal/desktop"
	"github.com/llwsb001/shellkit/internal/tui"
	"github.com/llwsb001/shellkit/internal/ui"
	"github.com/llwsb001/shellkit/internal/update"
)

// Main runs the command the user typed.
func Main(version string, args []string) error {
	cmd := "help"
	if len(args) > 0 {
		cmd = args[0]
	}
	// Only the subcommand position counts: `add -h` may be a note about a flag.
	if cmd == "-h" || cmd == "--help" {
		usage(version)
		return nil
	}
	rest := args[1:]

	switch cmd {
	case "help", "-h", "--help":
		usage(version)
		return nil
	case "version":
		fmt.Println("shellkit", version)
		return nil
	case "hello":
		fmt.Printf("hello from shellkit %s\n  notes    %s\n  gateway  %s (web shell only)\n", version, core.File(), addr())
		return nil
	case "add", "a":
		text, tags, err := splitAdd(rest)
		if err != nil {
			return err
		}
		return add(text, tags)
	case "list", "ls":
		return list(rest)
	case "rm":
		if len(rest) != 1 {
			return fmt.Errorf("usage: shellkit rm <id>")
		}
		if err := core.Remove(rest[0]); err != nil {
			return err
		}
		fmt.Println("removed", rest[0])
		return nil
	case "configs":
		return configsCmd(rest)
	case "update":
		return update.Run(rest, version)
	case "serve", "web", "tui", "desktop":
		return withServer(cmd)
	default:
		return fmt.Errorf("unknown command %q — try: shellkit help", cmd)
	}
}

func addr() string {
	if a := os.Getenv(core.EnvAddr); a != "" {
		return a
	}
	return core.DefaultAddr
}

func showURL(a string) string {
	h := a
	if strings.HasPrefix(h, ":") {
		h = "127.0.0.1" + h
	}
	h = strings.Replace(h, "0.0.0.0", "127.0.0.1", 1)
	return "http://" + h
}

// splitAdd pulls -t/--tag out of the words and leaves the text alone, so
// shellkit add -t shopping buy oat milk keeps "buy oat milk" as the note.
func splitAdd(args []string) (string, []string, error) {
	var words, tags []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-t" || arg == "--tag":
			if i+1 == len(args) {
				return "", nil, fmt.Errorf("usage: shellkit add [-t tag]... <text>")
			}
			i++
			if strings.HasPrefix(args[i], "-") {
				return "", nil, fmt.Errorf("tag %q looks like a flag; write --tag=%s", args[i], args[i])
			}
			tags = append(tags, args[i])
		case strings.HasPrefix(arg, "--tag="):
			tags = append(tags, strings.TrimPrefix(arg, "--tag="))
		case arg == "--":
			words = append(words, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(arg, "-") && arg != "-":
			return "", nil, fmt.Errorf("unknown flag %q (usage: shellkit add [-t tag]... <text>)", arg)
		default:
			words = append(words, arg)
		}
	}
	text := strings.Join(words, " ")
	if strings.TrimSpace(text) == "" {
		return "", nil, fmt.Errorf("usage: shellkit add [-t tag]... <text>")
	}
	return text, tags, nil
}

func add(text string, tags []string) error {
	note, err := core.Add(text, tags)
	if err != nil {
		return err
	}
	shown := note.Text
	if len(note.Tags) > 0 {
		shown += "  [" + strings.Join(note.Tags, ",") + "]"
	}
	fmt.Printf("%s  %s\n", note.ID, shown)
	return nil
}

func list(rest []string) error {
	q, limit, err := parseList(rest)
	if err != nil {
		return err
	}
	notes, err := core.List(q, limit)
	if err != nil {
		return err
	}
	if len(notes) == 0 {
		if q == "" {
			fmt.Println("no notes yet — shellkit add \"something worth keeping\"")
		} else {
			fmt.Printf("nothing matches %q\n", q)
		}
		return nil
	}
	for _, n := range notes {
		tags := ""
		if len(n.Tags) > 0 {
			tags = " [" + strings.Join(n.Tags, ",") + "]"
		}
		fmt.Printf("%s  %s  %s%s\n", n.At.Local().Format("2006-01-02 15:04"), n.ID, oneLine(n.Text), tags)
	}
	return nil
}

// parseList reads [-n N] and treats everything else as one query. The query is
// joined, so a search with spaces does not need quotes.
func parseList(rest []string) (string, int, error) {
	var words []string
	limit := 0
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		switch {
		case arg == "-n" || arg == "--limit":
			if i+1 == len(rest) {
				return "", 0, fmt.Errorf("usage: shellkit list [query] [-n N]")
			}
			i++
			n, err := strconv.Atoi(rest[i])
			if err != nil || n < 0 {
				return "", 0, fmt.Errorf("limit must be a non-negative integer, got %q", rest[i])
			}
			limit = n
		case strings.HasPrefix(arg, "--limit="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--limit="))
			if err != nil || n < 0 {
				return "", 0, fmt.Errorf("limit must be a non-negative integer, got %q", arg)
			}
			limit = n
		default:
			words = append(words, arg)
		}
	}
	return strings.Join(words, " "), limit, nil
}

func oneLine(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ↵ ")
	if len([]rune(s)) > 100 {
		return string([]rune(s)[:100]) + "…"
	}
	return s
}

func withServer(shape string) error {
	// SIGTERM matters: docker stop sends it, and without it the container is
	// killed instead of shut down.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := core.New(addr())

	// serve is the headless shape on purpose; every shape that shows a page
	// mounts it explicitly.
	hasPage := shape == "web" || shape == "tui" || shape == "desktop"
	if hasPage {
		ui.Mount(srv)
	}

	switch shape {
	case "tui":
		return tui.Run(ctx, srv)
	case "desktop":
		return desktop.Run(ctx, srv)
	default:
		if hasPage {
			fmt.Printf("shellkit %s\n  gateway  %s\n  page     %s/\n  notes    %s\n",
				shape, addr(), showURL(addr()), core.File())
		} else {
			fmt.Printf("shellkit %s\n  gateway  %s (API only, no page)\n  notes    %s\n",
				shape, addr(), core.File())
		}
		return srv.ListenAndServe(ctx)
	}
}

func configsCmd(rest []string) error {
	if len(rest) == 0 || rest[0] == "list" {
		for _, t := range configs.Targets {
			v, err := t.Get()
			if err != nil {
				v = "error: " + err.Error()
			}
			if v == "" {
				v = "(no file)"
			}
			fmt.Printf("%-8s %-34s %s\n", t.Agent, t.File(), v)
		}
		return nil
	}
	switch rest[0] {
	case "get":
		if len(rest) != 2 {
			return fmt.Errorf("usage: shellkit configs get <agent>")
		}
		t, err := configs.Lookup(rest[1])
		if err != nil {
			return err
		}
		v, err := t.Get()
		if err != nil {
			return err
		}
		fmt.Println(v)
		return nil
	case "set":
		if len(rest) != 3 {
			return fmt.Errorf("usage: shellkit configs set <agent> <model>")
		}
		t, err := configs.Lookup(rest[1])
		if err != nil {
			return err
		}
		if err := t.Set(rest[2]); err != nil {
			return err
		}
		fmt.Printf("%s → %s  (%s)\n", t.Agent, rest[2], t.File())
		return nil
	}
	return fmt.Errorf("usage: shellkit configs [list | get <agent> | set <agent> <model>]")
}

func usage(version string) {
	fmt.Printf(`shellkit %s — a tiny capture tool with several shells

  shellkit add "text"            capture a line
  shellkit list [query] [-n N]   newest first, search text and tags
  shellkit rm <id>               delete one
  shellkit configs               show three real agent configs and their model
  shellkit configs set <agent> <model>   rewrite only that key
  shellkit serve                 API only, no page
  shellkit web                   same, plus the embedded page
  shellkit tui                   terminal list (needs -tags tui)
  shellkit desktop               native window (needs -tags desktop)
  shellkit update [--check]      self-update from GitHub releases
  shellkit version

notes file: %s
gateway:   %s
`, version, core.File(), addr())
}
