// Command private-file-share sends files to a nearby phone, across this
// Wi-Fi, or anywhere on the internet — no account, nothing uploaded to a
// server this app runs itself. Bare invocation opens a local browser UI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/DavidMarsanic/private-file-share/internal/browser"
	"github.com/DavidMarsanic/private-file-share/internal/paths"
	"github.com/DavidMarsanic/private-file-share/internal/server"
)

const version = "0.1.0"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("private-file-share", flag.ContinueOnError)

	output := fs.String("output", "", "where received files are saved (default: your Downloads folder)")
	port := fs.Int("port", 0, "local UI server port (default: automatic)")
	showVersion := fs.Bool("version", false, "print the version and exit")
	fs.Usage = func() { printUsage(fs) }

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	if *showVersion {
		fmt.Println("private-file-share " + version)
		return 0
	}

	outputDir, err := paths.ResolveDownloadsDir(*output)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := server.New(ctx, outputDir)
	addr, err := srv.Start(*port)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	fmt.Fprintln(os.Stderr, "Private File Share running at", addr, "— press Ctrl+C to quit")

	// When a host process (securexe-launcher) is the one showing the UI —
	// in its own native window, so it can get a real Dock identity instead
	// of a spawned Chrome window — it sets this before starting us and
	// watches this same stderr line to discover the URL. Opening our own
	// Chrome window too would just leave a second, redundant one.
	if os.Getenv("SECUREXE_HOSTED") == "" {
		if err := browser.OpenAppWindow(addr + "/"); err != nil {
			fmt.Fprintln(os.Stderr, "couldn't open a window automatically:", err)
			fmt.Fprintln(os.Stderr, "open this URL manually:", addr+"/")
		}
	}

	<-ctx.Done()
	return 0
}

func printUsage(fs *flag.FlagSet) {
	fmt.Fprint(os.Stderr, `private-file-share — send files to a nearby phone, across this Wi-Fi,
or anywhere on the internet, without an account or a server that stores
your files.

Bare invocation opens a local browser UI.

Usage:
  private-file-share          open the browser UI

Flags:
`)
	fs.PrintDefaults()
}
