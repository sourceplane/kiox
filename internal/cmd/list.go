package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/state"
	"github.com/sourceplane/tinx/internal/workspace"
)

const defaultInventoryScopeName = "default"

type inventoryScope struct {
	Name      string
	Type      string
	Root      string
	Home      string
	Active    bool
	Status    string
	Detail    string
	Providers []providerInventory
}

type providerInventory struct {
	Aliases    []string
	Entrypoint string
	Ref        string
	Version    string
	Status     string
	Runtime    string
	Invoke     string
}

func newListCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List workspaces and installed providers",
	}
	cmd.AddCommand(newListWorkspacesCommand(root))
	cmd.AddCommand(newListProvidersCommand(root))
	return cmd
}

func newListWorkspacesCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspaces",
		Short: "List workspace scopes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			globalHome, err := ensureGlobalHome(root.Home)
			if err != nil {
				return err
			}
			scopes, err := listWorkspaceScopes(globalHome)
			if err != nil {
				return err
			}
			renderWorkspaceScopes(cmd.OutOrStdout(), scopes)
			return nil
		},
	}
	return cmd
}

func newListProvidersCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "providers [workspace|default]",
		Short: "List installed providers for the current, named, or default scope",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reference := ""
			if len(args) == 1 {
				reference = args[0]
			}
			scope, err := resolveProviderScope(root, reference)
			if err != nil {
				return err
			}
			renderProviderScope(cmd.OutOrStdout(), scope)
			return nil
		},
	}
	return cmd
}

func listWorkspaceScopes(globalHome string) ([]inventoryScope, error) {
	known, err := state.LoadWorkspaces(globalHome)
	if err != nil {
		return nil, err
	}
	activeRoot, err := state.LoadActiveWorkspace(globalHome)
	if err != nil {
		return nil, err
	}
	activeRoot = normalizeInventoryPath(activeRoot)

	namesByRoot := make(map[string][]string)
	for name, root := range known {
		trimmedName := strings.TrimSpace(name)
		trimmedRoot := normalizeInventoryPath(root)
		if trimmedName == "" || trimmedRoot == "" {
			continue
		}
		namesByRoot[trimmedRoot] = append(namesByRoot[trimmedRoot], trimmedName)
	}

	scopes := make([]inventoryScope, 0, len(namesByRoot)+1)
	for root, names := range namesByRoot {
		scope, err := inspectWorkspaceScope(root, names, activeRoot, false)
		if err != nil {
			return nil, err
		}
		scopes = append(scopes, scope)
	}
	defaultScope, err := inspectDefaultScope(globalHome, false)
	if err != nil {
		return nil, err
	}
	scopes = append(scopes, defaultScope)

	sort.Slice(scopes, func(i, j int) bool {
		if scopes[i].Type == defaultInventoryScopeName {
			return false
		}
		if scopes[j].Type == defaultInventoryScopeName {
			return true
		}
		if scopes[i].Active != scopes[j].Active {
			return scopes[i].Active
		}
		return scopes[i].Name < scopes[j].Name
	})
	return scopes, nil
}

func resolveProviderScope(root *rootOptions, reference string) (inventoryScope, error) {
	globalHome, err := ensureGlobalHome(root.Home)
	if err != nil {
		return inventoryScope{}, err
	}

	trimmed := strings.TrimSpace(reference)
	switch strings.ToLower(trimmed) {
	case "", "current":
		target, err := resolveCurrentWorkspaceTarget(globalHome)
		if err != nil {
			return inventoryScope{}, err
		}
		if target == nil {
			return inspectDefaultScope(globalHome, true)
		}
		return inspectWorkspaceScope(target.Root, []string{target.Config.Name()}, normalizeInventoryPath(target.Root), true)
	case defaultInventoryScopeName, "global":
		return inspectDefaultScope(globalHome, true)
	default:
		target, err := resolveWorkspaceTarget(trimmed, globalHome)
		if err != nil {
			return inventoryScope{}, err
		}
		activeRoot, err := state.LoadActiveWorkspace(globalHome)
		if err != nil {
			return inventoryScope{}, err
		}
		return inspectWorkspaceScope(target.Root, []string{target.Config.Name()}, normalizeInventoryPath(activeRoot), true)
	}
}

func inspectWorkspaceScope(root string, registeredNames []string, activeRoot string, includeProviders bool) (inventoryScope, error) {
	root = normalizeInventoryPath(root)
	sort.Strings(registeredNames)
	scope := inventoryScope{
		Name:   firstNonEmpty(registeredNames...),
		Type:   "workspace",
		Root:   root,
		Home:   workspace.Home(root),
		Active: root != "" && root == activeRoot,
		Status: "ready",
	}
	if scope.Name == "" {
		scope.Name = filepath.Base(root)
	}

	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			scope.Status = "missing"
			scope.Detail = "workspace root does not exist"
			return scope, nil
		}
		return inventoryScope{}, fmt.Errorf("stat workspace root %s: %w", root, err)
	}
	if !info.IsDir() {
		scope.Status = "invalid"
		scope.Detail = "workspace root is not a directory"
		return scope, nil
	}

	config, _, err := loadWorkspaceConfigAtRoot(root)
	if err != nil {
		scope.Status = "invalid"
		scope.Detail = err.Error()
		return scope, nil
	}
	if name := strings.TrimSpace(config.Name()); name != "" {
		scope.Name = name
	}

	if includeProviders {
		providers, err := inspectProviderInventory(scope.Home)
		if err != nil {
			return inventoryScope{}, err
		}
		scope.Providers = providers
	}
	return scope, nil
}

func inspectDefaultScope(globalHome string, includeProviders bool) (inventoryScope, error) {
	scope := inventoryScope{
		Name:   defaultInventoryScopeName,
		Type:   defaultInventoryScopeName,
		Home:   normalizeInventoryPath(globalHome),
		Status: "ready",
	}
	if includeProviders {
		providers, err := inspectProviderInventory(globalHome)
		if err != nil {
			return inventoryScope{}, err
		}
		scope.Providers = providers
	}
	return scope, nil
}

func inspectProviderInventory(home string) ([]providerInventory, error) {
	providers, err := state.ListInstalledProviders(home)
	if err != nil {
		return nil, err
	}
	aliases, err := state.LoadAliases(home)
	if err != nil {
		return nil, err
	}

	aliasesByRef := make(map[string][]string)
	for alias, ref := range aliases {
		trimmedAlias := strings.TrimSpace(alias)
		trimmedRef := strings.TrimSpace(ref)
		if trimmedAlias == "" || trimmedRef == "" {
			continue
		}
		aliasesByRef[trimmedRef] = append(aliasesByRef[trimmedRef], trimmedAlias)
	}

	items := make([]providerInventory, 0, len(providers)+len(aliasesByRef))
	for _, meta := range providers {
		ref := providerReference(meta)
		providerAliases := sortedStrings(aliasesByRef[ref])
		delete(aliasesByRef, ref)
		items = append(items, providerInventory{
			Aliases:    providerAliases,
			Entrypoint: displayEntrypoint(meta),
			Ref:        ref,
			Version:    fallbackDisplay(meta.Version),
			Status:     "ready",
			Runtime:    fallbackDisplay(strings.TrimSpace(meta.Runtime)),
			Invoke:     providerInvokeHint(meta, providerAliases),
		})
	}

	for ref, providerAliases := range aliasesByRef {
		providerAliases = sortedStrings(providerAliases)
		items = append(items, providerInventory{
			Aliases:    providerAliases,
			Entrypoint: "-",
			Ref:        ref,
			Version:    "-",
			Status:     "missing",
			Runtime:    "-",
			Invoke:     missingProviderInvokeHint(ref, providerAliases),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		leftName := primaryProviderDisplay(items[i])
		rightName := primaryProviderDisplay(items[j])
		if leftName != rightName {
			return leftName < rightName
		}
		return items[i].Ref < items[j].Ref
	})
	return items, nil
}

func renderWorkspaceScopes(w io.Writer, scopes []inventoryScope) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tTYPE\tSTATUS\tACTIVE\tROOT")
	for _, scope := range scopes {
		root := scope.Root
		if root == "" {
			root = "-"
		}
		active := "-"
		if scope.Active {
			active = "yes"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", scope.Name, scope.Type, scope.Status, active, root)
	}
	_ = tw.Flush()
}

func renderProviderScope(w io.Writer, scope inventoryScope) {
	writeLine(w, "Scope: %s", scope.Name)
	writeLine(w, "Type: %s", scope.Type)
	if scope.Root != "" {
		writeLine(w, "Root: %s", scope.Root)
	}
	writeLine(w, "Home: %s", scope.Home)
	writeLine(w, "Status: %s", scope.Status)
	if scope.Detail != "" {
		writeLine(w, "Detail: %s", scope.Detail)
	}
	if scope.Status != "ready" {
		return
	}
	writeLine(w, "")
	renderProviderTable(w, scope.Providers)
}

func renderProviderTable(w io.Writer, providers []providerInventory) {
	if len(providers) == 0 {
		writeLine(w, "  no providers installed")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ALIASES\tENTRYPOINT\tPROVIDER\tVERSION\tSTATUS\tRUNTIME\tINVOKE")
	for _, provider := range providers {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			displayAliases(provider.Aliases),
			fallbackDisplay(provider.Entrypoint),
			provider.Ref,
			fallbackDisplay(provider.Version),
			fallbackDisplay(provider.Status),
			fallbackDisplay(provider.Runtime),
			fallbackDisplay(provider.Invoke),
		)
	}
	_ = tw.Flush()
}

func loadWorkspaceConfigAtRoot(root string) (workspace.Config, string, error) {
	for _, name := range workspace.ManifestNames {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return workspace.Config{}, "", fmt.Errorf("stat workspace manifest: %w", err)
		}
		config, err := workspace.Load(path)
		if err != nil {
			return workspace.Config{}, "", err
		}
		return config, path, nil
	}
	return workspace.Config{}, "", fmt.Errorf("no workspace manifest found in %s", root)
}

func providerReference(meta state.ProviderMetadata) string {
	return strings.TrimSpace(meta.Namespace) + "/" + strings.TrimSpace(meta.Name)
}

func providerInvokeHint(meta state.ProviderMetadata, aliases []string) string {
	target := providerReference(meta)
	if len(aliases) > 0 {
		target = aliases[0]
	}
	prefix := "tinx "
	if len(aliases) == 0 {
		prefix = "tinx run "
	}
	return prefix + target + " <capability>"
}

func missingProviderInvokeHint(ref string, aliases []string) string {
	if len(aliases) > 0 {
		return "tinx " + aliases[0] + " ..."
	}
	return "tinx run " + ref + " ..."
}

func displayEntrypoint(meta state.ProviderMetadata) string {
	entrypoint := strings.TrimSpace(filepath.Base(meta.Entrypoint))
	if entrypoint == "" || entrypoint == "." || looksLikeManifestOrScript(entrypoint) {
		return "-"
	}
	return entrypoint
}

func displayAliases(aliases []string) string {
	if len(aliases) == 0 {
		return "-"
	}
	return strings.Join(aliases, ",")
}

func primaryProviderDisplay(provider providerInventory) string {
	if len(provider.Aliases) > 0 {
		return provider.Aliases[0]
	}
	if provider.Entrypoint != "" && provider.Entrypoint != "-" {
		return provider.Entrypoint
	}
	return provider.Ref
}

func normalizeInventoryPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	absPath, err := filepath.Abs(trimmed)
	if err != nil {
		return filepath.Clean(trimmed)
	}
	return filepath.Clean(absPath)
}

func sortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := append([]string(nil), values...)
	sort.Strings(cloned)
	return cloned
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func fallbackDisplay(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
