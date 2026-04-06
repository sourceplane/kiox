package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newExecCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:                "exec [--] <command> [args...]",
		Short:              "Run a command inside the workspace environment",
		DisableFlagParsing: true,
		Args:               cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			command := args
			if len(command) > 0 && command[0] == "--" {
				command = command[1:]
			}
			if len(command) == 0 {
				return fmt.Errorf("command is required")
			}
			if strings.TrimSpace(command[0]) == "" {
				return fmt.Errorf("command is required")
			}
			return runWorkspaceCommand(cmd, root, command)
		},
	}
	return cmd
}