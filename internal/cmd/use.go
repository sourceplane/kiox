package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newUseCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use <workspace> [-- command...]",
		Short: "Select a workspace and optionally run a command inside its shell",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			beforeDash, afterDash := splitArgsAtDash(cmd, args)
			if len(beforeDash) == 0 {
				return fmt.Errorf("workspace is required")
			}
			return useWorkspace(cmd, root, beforeDash[0], afterDash)
		},
	}
	return cmd
}
