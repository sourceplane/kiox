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
		Deprecated:         "use workspaces and 'kiox -- <command>' instead",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("'kiox run' has been removed; add the provider to a workspace and run it with 'kiox -- <command>' instead")
			}
			command := strings.Join(args, " ")
			if strings.TrimSpace(command) == "" {
				command = "<command>"
			}
			return fmt.Errorf("'kiox run' has been removed; add the provider to a workspace and run it with 'kiox -- %s' instead", command)
		},
	}
	return cmd
}
