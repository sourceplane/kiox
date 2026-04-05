package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newUseCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:        "use <workspace> [-- command...]",
		Short:      "Deprecated: activate a workspace",
		Hidden:     true,
		Deprecated: "use 'tinx workspace activate <workspace>' instead",
		Args:       cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			beforeDash, afterDash := splitArgsAtDash(cmd, args)
			if len(beforeDash) == 0 {
				return fmt.Errorf("workspace is required")
			}
			return activateWorkspace(cmd, root, beforeDash[0], afterDash)
		},
	}
	return cmd
}
