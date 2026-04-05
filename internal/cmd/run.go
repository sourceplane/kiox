package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newRunCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:                "run <provider-or-alias> [args...]",
		Short:              "Deprecated: execution must go through workspace shells",
		Hidden:             true,
		Deprecated:         "use workspaces and 'tinx -- <command>' instead",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("'tinx run' has been removed; add the provider to a workspace and run it with 'tinx -- <command>' instead")
			}
			command := strings.Join(args, " ")
			if strings.TrimSpace(command) == "" {
				command = "<command>"
			}
			return fmt.Errorf("'tinx run' has been removed; add the provider to a workspace and run it with 'tinx -- %s' instead", command)
		},
	}
	return cmd
}
