// outage-radar runs one role chosen by the ROLE environment variable, or
// `outage-radar migrate` as a one-off command.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alisa/outage-radar/internal/app"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg := app.ConfigFromEnv()
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		return app.Migrate(ctx, cfg.DatabaseURL)
	}
	if len(os.Args) > 2 && os.Args[1] == "import-osm" { // import-osm <overpass.json> [ekatte]
		f, err := os.Open(os.Args[2])
		if err != nil {
			return err
		}
		defer f.Close()
		ekatte := "10135"
		if len(os.Args) > 3 {
			ekatte = os.Args[3]
		}
		return app.ImportOSM(ctx, cfg.DatabaseURL, f, ekatte)
	}
	if len(os.Args) > 3 && os.Args[1] == "provider-mode" { // provider-mode <provider> <shadow|live>
		return app.SetProviderMode(ctx, cfg.DatabaseURL, os.Args[2], os.Args[3], "cli")
	}
	if len(os.Args) > 1 && os.Args[1] == "shadow-alerts" { // shadow-alerts [provider]
		provider := ""
		if len(os.Args) > 2 {
			provider = os.Args[2]
		}
		return app.ShadowAlerts(ctx, cfg.DatabaseURL, provider, os.Stdout)
	}
	// Real job handlers are registered by later tickets.
	a, err := app.Start(ctx, cfg, nil, map[string]app.Handler{})
	if err != nil {
		return err
	}
	a.Wait()
	return nil
}
