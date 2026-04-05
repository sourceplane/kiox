package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/resolver"
	"github.com/sourceplane/tinx/internal/state"
)

func newDispatchProviderCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "__dispatch-provider <provider-ref> [args...]",
		Hidden: true,
		FParseErrWhitelist: cobra.FParseErrWhitelist{
			UnknownFlags: true,
		},
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := ensureHome(root.Home)
			if err != nil {
				return err
			}
			providerRef := args[0]
			resolvedRef := providerRef
			if aliases, err := state.LoadAliases(home); err == nil {
				if aliased, ok := aliases[providerRef]; ok {
					resolvedRef = aliased
				}
			}
			resolvedRef = resolver.ResolveProviderSource(resolvedRef)
			providerMeta, err := resolveProviderForRun(home, resolvedRef, false, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			invocation := planProviderInvocation(providerMeta, args[1:])
			if invocation.showProviderHelp {
				writeProviderHelp(cmd.OutOrStdout(), providerRef, providerRef, providerMeta)
				return nil
			}
			if invocation.showCapabilityHelp {
				writeCapabilityHelp(cmd.OutOrStdout(), providerRef, providerRef, invocation.capability, providerMeta)
				return nil
			}
			if len(invocation.args) == 0 {
				return fmt.Errorf("provider %s requires a capability", providerRef)
			}
			return executeProviderCapability(cmd, home, providerRef, providerMeta, invocation.args)
		},
	}
	cmd.Flags().SetInterspersed(false)
	return cmd
}
