package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/Wayshard/wayshard/internal/app"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/paths"
	"github.com/Wayshard/wayshard/internal/provider"
	"github.com/Wayshard/wayshard/internal/sandbox"
	"github.com/Wayshard/wayshard/internal/version"
)

func main() {
	// The same binary doubles as the sandbox helper and as the in-namespace
	// provider shim.
	if sandbox.MaybeRunHelper(os.Args) {
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == provider.ShimArg {
		code := 2
		if len(os.Args) >= 3 {
			code = provider.ShimMain(os.Args[2])
		}
		os.Exit(code)
	}
	listen := flag.String("listen", "127.0.0.1:7420", "HTTP listen address (loopback by default)")
	advertise := flag.String("advertise", "", "externally reachable URL to embed in pairing invitations")
	data := flag.String("data", "", "data directory (default: platform user data dir)")
	allowProvider := flag.Bool("allow-provider-network", false, "permit provider-backed harness routes through the secure broker")
	providerDests := flag.String("provider-destination", "", "comma-separated authorized provider endpoints host:port")
	providerModel := flag.String("provider-model", "", "default model id for provider-backed harness routes")
	flag.Parse()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	log.Info("wayshard server", "version", version.Version, "commit", version.Commit)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	dataDir := *data
	if dataDir == "" {
		dataDir = paths.DataDir()
	}
	a, err := app.Open(ctx, app.Config{
		DataDir: dataDir, Listen: *listen, Advertise: *advertise, Log: log,
		AllowProviderNetwork: *allowProvider,
		ProviderDestinations: parseProviderDestinations(*providerDests),
		ProviderModel:        *providerModel,
	})
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

func parseProviderDestinations(s string) []domain.ProviderDestination {
	var out []domain.ProviderDestination
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		host, portStr, err := splitHostPortDefault(part)
		if err != nil {
			continue
		}
		port, err := strconv.Atoi(portStr)
		if err != nil || port <= 0 || port > 65535 {
			continue
		}
		out = append(out, domain.ProviderDestination{Host: host, Port: port})
	}
	return out
}

func splitHostPortDefault(s string) (string, string, error) {
	if i := strings.LastIndex(s, ":"); i >= 0 {
		return s[:i], s[i+1:], nil
	}
	return s, "443", nil
}
