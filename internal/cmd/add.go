package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/workspace"
)

func newAddCommand(root *rootOptions) *cobra.Command {
	var plainHTTP bool

	cmd := &cobra.Command{
		Use:   "add <provider> [as <alias>]",
		Short: "Add a provider to the current or selected workspace",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias, source, err := parseWorkspaceProvider(args)
			if err != nil {
				return err
			}
			globalHome, err := ensureGlobalHome(root.Home)
			if err != nil {
				return err
			}
			target, err := resolveSelectedWorkspaceTarget(root, globalHome)
			if err != nil {
				return err
			}
			if target == nil {
				return fmt.Errorf("no workspace selected; run tinx workspace activate <workspace>, execute inside a workspace, or pass --workspace")
			}

			providerSource := normalizeInitSource(source)
			providerAlias := strings.TrimSpace(alias)
			if providerAlias == "" {
				providerAlias = defaultAliasForSource(providerSource)
			}
			if providerAlias == "" {
				return fmt.Errorf("could not derive alias for provider %q", source)
			}

			providers := cloneWorkspaceProviders(target.Config)
			providerSpec := workspace.Provider{Source: providerSource, PlainHTTP: plainHTTP}
			if existing, ok := providers[providerAlias]; ok {
				if existing.Source == providerSpec.Source && existing.PlainHTTP == providerSpec.PlainHTTP {
					writeLine(cmd.OutOrStdout(), "provider %s is already present in workspace %s", providerAlias, target.Config.Name())
					return nil
				}
				return fmt.Errorf("workspace provider alias %q already exists", providerAlias)
			}
			providers[providerAlias] = providerSpec
			target.Config.Providers = providers
			target.Config.Spec.Providers = nil

			manifestPath := workspace.ManifestPath(target.Root)
			if err := workspace.Save(manifestPath, target.Config); err != nil {
				return err
			}
			result, err := workspace.Sync(cmd.Context(), target.Root, target.Config, workspace.SyncOptions{
				Out: cmd.ErrOrStderr(),
			})
			if err != nil {
				return err
			}
			if err := rememberWorkspaceTarget(globalHome, target); err != nil {
				return err
			}
			writeLine(cmd.OutOrStdout(), "added provider %s -> %s", providerAlias, providerSource)
			writeLine(cmd.OutOrStdout(), "manifest: %s", manifestPath)
			writeLine(cmd.OutOrStdout(), "home: %s", result.Home)
			return nil
		},
	}
	cmd.Flags().BoolVar(&plainHTTP, "plain-http", false, "use plain HTTP for registry pulls in this workspace")
	return cmd
}

func parseWorkspaceProvider(args []string) (string, string, error) {
	if len(args) >= 3 && args[1] == "as" {
		return args[2], args[0], nil
	}
	if len(args) != 1 {
		return "", "", fmt.Errorf("add expects <provider> or <provider> as <alias>")
	}
	return "", args[0], nil
}

func cloneWorkspaceProviders(config workspace.Config) map[string]workspace.Provider {
	providers := config.ProviderMap()
	cloned := make(map[string]workspace.Provider, len(providers))
	for alias, provider := range providers {
		cloned[alias] = provider
	}
	return cloned
}
