package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"tdrive/internal/app"
	"tdrive/internal/config"
	"tdrive/internal/meta"
)

type VersionInfo struct {
	Version string `json:"version"`
}

// RootCommand is the program entry point. Running it directly starts the file
// disk server; version is the only subcommand.
type RootCommand struct {
	BaseCommand

	flags   serverFlags
	verbose bool
}

func NewRootCommand(versionInfo *VersionInfo) *RootCommand {

	var program RootCommand

	program.versionInfo = versionInfo

	program.Use = meta.Name + " [root]"

	program.Short = "Personal web file disk (HTTP + FTP + WebDAV)"

	program.Long = "A single-user web file disk that stores files directly on the filesystem and serves them over HTTP, FTP, and WebDAV. Run with no arguments to serve the current directory over HTTP (and WebDAV) on :3000 with no authentication; pass a directory to serve it instead."

	program.Args = cobra.MaximumNArgs(1)

	program.flags.register(&program.Command)
	program.Flags().BoolVar(&program.verbose, "verbose", false, "enable debug logging")

	program.RunE = func(cobraCommand *cobra.Command, args []string) error {

		return program.onExecute(cobraCommand, args)
	}

	program.AddCommand(NewVersionCommand(versionInfo))

	return &program
}

// onExecute resolves configuration and runs the server until termination. The
// run path adapts to its environment: under the Windows Service Control Manager
// it speaks the SCM protocol; otherwise it shuts down gracefully on SIGINT/SIGTERM
// (which is what systemd and Docker send).
func (command *RootCommand) onExecute(cobraCommand *cobra.Command, args []string) error {

	// 1. Initialize logging.
	command.setupLogger(command.verbose)

	// 2. Resolve configuration: defaults, then CLI flags on top.
	var cfg *config.Config = config.New()

	var err error = command.flags.apply(cobraCommand, args, cfg)
	if nil != err {
		return err
	}

	err = cfg.Validate()
	if nil != err {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// 3. Build the application.
	var application *app.App

	application, err = app.New(cfg)
	if nil != err {
		return fmt.Errorf("init application: %w", err)
	}

	// 4. Run until terminated (Windows SCM or POSIX signals).
	return app.Run(application)
}

func Execute(versionInfo *VersionInfo) error {

	return NewRootCommand(versionInfo).Execute()
}
