package cmd

import (
	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/state"
)

func newRunCommand(root *rootOptions) *cobra.Command {
	var plainHTTP bool

	cmd := &cobra.Command{
		Use:   "run <provider-or-alias> <capability> [args...]",
		Short: "Execute a provider capability from an alias, installed provider, or OCI reference",
		FParseErrWhitelist: cobra.FParseErrWhitelist{
			UnknownFlags: true,
		},
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := ensureHome(root.Home)
			if err != nil {
				return err
			}
			input := args[0]
			aliasName := ""
			ref := input
			if aliases, err := state.LoadAliases(home); err == nil {
				if aliased, ok := aliases[input]; ok {
					aliasName = input
					ref = aliased
				}
			}
			providerMeta, err := resolveProviderForRun(home, ref, plainHTTP)
			if err != nil {
				return err
			}
			helpAlias := aliasName
			if helpAlias == "" {
				helpAlias = input
			}
			if len(args) == 1 || isHelpToken(args[1]) {
				writeProviderHelp(cmd.OutOrStdout(), helpAlias, ref, providerMeta)
				return nil
			}
			if len(args) >= 3 && isHelpToken(args[2]) {
				writeCapabilityHelp(cmd.OutOrStdout(), helpAlias, args[1], providerMeta)
				return nil
			}

			return executeProviderCapability(cmd, home, ref, providerMeta, args[1:])
		},
	}
	cmd.Flags().BoolVar(&plainHTTP, "plain-http", false, "use plain HTTP for registry pull/install")
	return cmd
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
