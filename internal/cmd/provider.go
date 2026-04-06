package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/state"
	"github.com/sourceplane/tinx/internal/workspace"
)

func newProviderCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "provider",
		Aliases: []string{"providers", "p"},
		Short:   "Manage workspace providers and provider inventory",
	}
	cmd.AddCommand(newProviderListCommand(root))
	cmd.AddCommand(newProviderAddCommand(root))
	cmd.AddCommand(newProviderRemoveCommand(root))
	cmd.AddCommand(newProviderUpdateCommand(root))
	return cmd
}

func newProviderListCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [workspace|default]",
		Short: "List providers for the current, named, or default scope",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProviderListCommand(cmd, root, args)
		},
	}
	return cmd
}

func newProviderAddCommand(root *rootOptions) *cobra.Command {
	var plainHTTP bool
	cmd := &cobra.Command{
		Use:   "add <provider> [as <alias>]",
		Short: "Add a provider to the current or selected workspace",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAddProviderCommand(cmd, root, args, plainHTTP)
		},
	}
	cmd.Flags().BoolVar(&plainHTTP, "plain-http", false, "use plain HTTP for registry pulls in this workspace")
	return cmd
}

func newProviderRemoveCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <provider-or-alias>",
		Short: "Remove a provider from the current or selected workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemoveProviderCommand(cmd, root, args[0])
		},
	}
	return cmd
}

func newProviderUpdateCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [provider-or-alias...]",
		Short: "Refresh provider metadata for the current or selected workspace",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdateProviderCommand(cmd, root, args)
		},
	}
	return cmd
}

func runRemoveProviderCommand(cmd *cobra.Command, root *rootOptions, selector string) error {
	globalHome, target, err := resolveRequiredWorkspaceTarget(cmd, root)
	if err != nil {
		return err
	}
	home := workspace.Home(target.Root)
	currentAliases, err := state.LoadAliases(home)
	if err != nil {
		return err
	}
	alias, ref, err := matchWorkspaceProviderSelection(target.Config, currentAliases, selector)
	if err != nil {
		return err
	}
	providers := cloneWorkspaceProviders(target.Config)
	delete(providers, alias)
	target.Config.Providers = providers
	target.Config.Spec.Providers = nil
	if ref != "" && !providerKeyStillReferenced(currentAliases, target.Config, alias, ref) {
		if err := removeProviderCache(home, ref); err != nil {
			return err
		}
	}
	manifestPath := workspace.ManifestPath(target.Root)
	if err := workspace.Save(manifestPath, target.Config); err != nil {
		return err
	}
	result, err := workspace.Sync(cmd.Context(), target.Root, target.Config, workspace.SyncOptions{Out: cmd.ErrOrStderr(), GlobalHome: globalHome})
	if err != nil {
		return err
	}
	if err := rememberWorkspaceTarget(globalHome, target); err != nil {
		return err
	}
	writeLine(cmd.OutOrStdout(), "removed provider %s", alias)
	writeLine(cmd.OutOrStdout(), "manifest: %s", manifestPath)
	writeLine(cmd.OutOrStdout(), "home: %s", result.Home)
	return nil
}

func runUpdateProviderCommand(cmd *cobra.Command, root *rootOptions, selectors []string) error {
	globalHome, target, err := resolveRequiredWorkspaceTarget(cmd, root)
	if err != nil {
		return err
	}
	home := workspace.Home(target.Root)
	currentAliases, err := state.LoadAliases(home)
	if err != nil {
		return err
	}
	aliasesToRefresh := target.Config.ProviderAliases()
	if len(selectors) > 0 {
		aliasesToRefresh = aliasesToRefresh[:0]
		seen := map[string]struct{}{}
		for _, selector := range selectors {
			alias, _, err := matchWorkspaceProviderSelection(target.Config, currentAliases, selector)
			if err != nil {
				return err
			}
			if _, ok := seen[alias]; ok {
				continue
			}
			seen[alias] = struct{}{}
			aliasesToRefresh = append(aliasesToRefresh, alias)
		}
	}
	if len(aliasesToRefresh) == 0 {
		writeLine(cmd.OutOrStdout(), "workspace %s has no providers to update", target.DisplayName())
		return nil
	}
	for _, alias := range aliasesToRefresh {
		if err := removeProviderCache(home, currentAliases[alias]); err != nil {
			return err
		}
	}
	result, err := workspace.Sync(cmd.Context(), target.Root, target.Config, workspace.SyncOptions{Out: cmd.ErrOrStderr(), GlobalHome: globalHome, RefreshAliases: aliasesToRefresh})
	if err != nil {
		return err
	}
	if err := rememberWorkspaceTarget(globalHome, target); err != nil {
		return err
	}
	sort.Strings(aliasesToRefresh)
	writeLine(cmd.OutOrStdout(), "updated providers: %s", strings.Join(aliasesToRefresh, ", "))
	writeLine(cmd.OutOrStdout(), "home: %s", result.Home)
	return nil
}

func resolveRequiredWorkspaceTarget(cmd *cobra.Command, root *rootOptions) (string, *workspaceTarget, error) {
	globalHome, err := ensureGlobalHome(root.Home)
	if err != nil {
		return "", nil, err
	}
	target, err := resolveSelectedWorkspaceTarget(root, globalHome)
	if err != nil {
		return "", nil, err
	}
	if target == nil {
		return "", nil, fmt.Errorf("no workspace selected; run tinx workspace use <workspace>, execute inside a workspace, or pass --workspace")
	}
	if err := requireReadyWorkspaceTarget(target); err != nil {
		return "", nil, err
	}
	return globalHome, target, nil
}

func matchWorkspaceProviderSelection(config workspace.Config, aliases map[string]string, selector string) (string, string, error) {
	trimmed := strings.TrimSpace(selector)
	if trimmed == "" {
		return "", "", fmt.Errorf("provider selection is required")
	}
	if _, ok := config.ProviderMap()[trimmed]; ok {
		return trimmed, aliases[trimmed], nil
	}
	matches := make([]string, 0, 1)
	for alias, provider := range config.ProviderMap() {
		if strings.TrimSpace(provider.Source) == trimmed || normalizeInitSource(provider.Source) == trimmed {
			matches = append(matches, alias)
			continue
		}
		if aliases[alias] == trimmed {
			matches = append(matches, alias)
			continue
		}
		if state.ProviderRefFromKey(aliases[alias]) == trimmed {
			matches = append(matches, alias)
		}
	}
	if len(matches) == 0 {
		return "", "", fmt.Errorf("workspace provider %q was not found", trimmed)
	}
	if len(matches) > 1 {
		sort.Strings(matches)
		return "", "", fmt.Errorf("workspace provider %q is ambiguous; use one of: %s", trimmed, strings.Join(matches, ", "))
	}
	return matches[0], aliases[matches[0]], nil
}

func providerKeyStillReferenced(aliases map[string]string, config workspace.Config, removedAlias, removedRef string) bool {
	for _, alias := range config.ProviderAliases() {
		if alias == removedAlias {
			continue
		}
		if aliases[alias] == removedRef {
			return true
		}
	}
	return false
}

func removeProviderCache(home, ref string) error {
	if strings.TrimSpace(ref) == "" {
		return nil
	}
	namespace, name, version, err := state.SplitProviderKey(ref)
	if err != nil {
		return nil
	}
	if err := os.RemoveAll(state.VersionRoot(home, namespace, name, version)); err != nil {
		return fmt.Errorf("remove provider cache for %s: %w", ref, err)
	}
	return nil
}
