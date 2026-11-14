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
	// Real job handlers are registered by later tickets.
	a, err := app.Start(ctx, cfg, nil, map[string]app.Handler{})
	if err != nil {
		return err
	}
	a.Wait()
	return nil
}
