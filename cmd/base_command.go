package cmd

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

type BaseCommand struct {
	cobra.Command
	versionInfo *VersionInfo
}

func (command *BaseCommand) setupLogger(verbose bool) {

	var level slog.Level

	var logger *slog.Logger

	if verbose {

		level = slog.LevelDebug
	} else {

		level = slog.LevelInfo
	}

	logger = slog.New(
		slog.NewTextHandler(
			os.Stdout,
			&slog.HandlerOptions{
				AddSource: verbose,
				Level:     level,
			}),
	)

	slog.SetDefault(logger)

	slog.Info("logger init ok",
		slog.Bool("verbose", verbose),
		slog.String("version", command.versionInfo.Version),
	)
}
