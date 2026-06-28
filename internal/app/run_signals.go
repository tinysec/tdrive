package app

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// runWithSignals starts the application and blocks until SIGINT or SIGTERM
// triggers a graceful shutdown. This is the path used interactively on every
// platform and as the service path on non-Windows systems (systemd sends SIGTERM).
func runWithSignals(application *App) error {

	var ctx context.Context
	var stop context.CancelFunc

	ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error = application.Start()
	if nil != err {
		return err
	}

	<-ctx.Done()

	slog.Info("termination signal received, shutting down")

	return application.Shutdown()
}
