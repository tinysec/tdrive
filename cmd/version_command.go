package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

type VersionCommand struct {
	BaseCommand
}

func NewVersionCommand(versionInfo *VersionInfo) *cobra.Command {

	var command VersionCommand

	command.versionInfo = versionInfo

	command.Use = "version"

	command.Short = "show version info"

	command.Long = "show version info"

	command.Args = cobra.NoArgs

	command.RunE = func(_ *cobra.Command, args []string) error {

		return command.onExecute()
	}

	return &command.Command
}

func (command *VersionCommand) onExecute() error {

	fmt.Printf("version: %s\n", command.versionInfo.Version)

	return nil
}
