package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Wayshard/wayshard/internal/app"
	"github.com/Wayshard/wayshard/internal/paths"
	"github.com/Wayshard/wayshard/internal/sandbox"
	"github.com/Wayshard/wayshard/internal/version"
)

func main() {
	// The same binary doubles as the sandbox helper used to confine harness
	// and tool processes.
	if sandbox.MaybeRunHelper(os.Args) {
		return
	}
	listen := flag.String("listen", "127.0.0.1:7420", "HTTP listen address (loopback by default)")
	advertise := flag.String("advertise", "", "externally reachable URL to embed in pairing invitations")
	data := flag.String("data", "", "data directory (default: platform user data dir)")
	flag.Parse()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	log.Info("wayshard server", "version", version.Version, "commit", version.Commit)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	dataDir := *data
	if dataDir == "" {
		dataDir = paths.DataDir()
	}
	a, err := app.Open(ctx, app.Config{DataDir: dataDir, Listen: *listen, Advertise: *advertise, Log: log})
	if err != nil {
		log.Error("open", "err", err)
		os.Exit(1)
	}
	defer a.Close()
	if err := a.Run(ctx); err != nil {
		log.Error("run", "err", err)
		os.Exit(1)
	}
}
