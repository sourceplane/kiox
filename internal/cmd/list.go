package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/state"
	"github.com/sourceplane/tinx/internal/workspace"
)

const defaultInventoryScopeName = "default"

type workspaceListOptions struct {
	Short      bool
	ActiveName string
}

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
		Use:     "list [workspace|default]",
		Aliases: []string{"ls"},
		Short:   "List providers or inspect workspace inventory",
		Args:    cobra.MaximumNArgs(1),
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
			renderWorkspaceScopes(cmd.OutOrStdout(), scopes, workspaceListOptions{ActiveName: activeWorkspaceScopeName(scopes)})
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
			return runProviderListCommand(cmd, root, args)
		},
	}
	return cmd
}

func runProviderListCommand(cmd *cobra.Command, root *rootOptions, args []string) error {
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
		return inspectWorkspaceScope(target.Root, []string{target.DisplayName()}, normalizeInventoryPath(target.Root), true)
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
		return inspectWorkspaceScope(target.Root, []string{target.DisplayName()}, normalizeInventoryPath(activeRoot), true)
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
		providers, err := inspectProviderInventory(scope.Home, true)
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
		providers, err := inspectProviderInventory(globalHome, false)
		if err != nil {
			return inventoryScope{}, err
		}
		scope.Providers = providers
	}
	return scope, nil
}

func inspectProviderInventory(home string, workspaceScope bool) ([]providerInventory, error) {
	providers, err := state.ListInstalledProviders(home)
	if err != nil {
		return nil, err
	}
	aliases, err := state.LoadAliases(home)
	if err != nil {
		return nil, err
	}

	aliasesByKey := make(map[string][]string)
	for alias, key := range aliases {
		trimmedAlias := strings.TrimSpace(alias)
		trimmedKey := strings.TrimSpace(key)
		if trimmedAlias == "" || trimmedKey == "" {
			continue
		}
		aliasesByKey[trimmedKey] = append(aliasesByKey[trimmedKey], trimmedAlias)
	}

	items := make([]providerInventory, 0, len(providers)+len(aliasesByKey))
	for _, meta := range providers {
		providerKey := state.MetadataKey(meta)
		ref := providerReference(meta)
		providerAliases := sortedStrings(aliasesByKey[providerKey])
		delete(aliasesByKey, providerKey)
		items = append(items, providerInventory{
			Aliases:    providerAliases,
			Entrypoint: displayEntrypoint(meta),
			Ref:        ref,
			Version:    fallbackDisplay(meta.Version),
			Status:     "ready",
			Runtime:    fallbackDisplay(strings.TrimSpace(meta.Runtime)),
			Invoke:     providerInvokeHint(meta, providerAliases, workspaceScope),
		})
	}

	for key, providerAliases := range aliasesByKey {
		providerAliases = sortedStrings(providerAliases)
		ref := state.ProviderRefFromKey(key)
		version := "-"
		if _, _, resolvedVersion, err := state.SplitProviderKey(key); err == nil {
			version = fallbackDisplay(resolvedVersion)
		}
		items = append(items, providerInventory{
			Aliases:    providerAliases,
			Entrypoint: "-",
			Ref:        ref,
			Version:    version,
			Status:     "missing",
			Runtime:    "-",
			Invoke:     missingProviderInvokeHint(ref, providerAliases, workspaceScope),
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

func renderWorkspaceScopes(w io.Writer, scopes []inventoryScope, opts workspaceListOptions) {
	if len(scopes) == 0 {
		if !opts.Short {
			writeLine(w, "no workspaces matched")
		}
		return
	}
	if opts.Short {
		for _, scope := range scopes {
			writeLine(w, "%s %s", activeMarker(scope.Active), scope.Name)
		}
		return
	}
	rows := make([][]string, 0, len(scopes))
	for _, scope := range scopes {
		rows = append(rows, []string{
			scope.Name,
			activeMarker(scope.Active),
			inventoryStatusDisplay(scope.Status),
			displayScopeRoot(scope),
		})
	}
	renderTable(w, []string{"NAME", "ACTIVE", "STATUS", "ROOT"}, rows)
	writeLine(w, "")
	writeLine(w, "%s", summarizeWorkspaces(scopes))
	writeLine(w, "Active workspace: %s", summaryValue(opts.ActiveName))
}

func renderProviderScope(w io.Writer, scope inventoryScope) {
	writeLine(w, "Scope: %s", scope.Name)
	writeLine(w, "Root: %s", displayScopeRoot(scope))
	writeLine(w, "Status: %s", inventoryStatusDisplay(scope.Status))
	if scope.Detail != "" {
		writeLine(w, "Detail: %s", scope.Detail)
	}
	if scope.Status != "ready" {
		return
	}
	writeLine(w, "")
	renderProviderTable(w, scope.Providers)
	writeLine(w, "")
	writeLine(w, "%s", summarizeProviders(scope.Providers))
}

func renderProviderTable(w io.Writer, providers []providerInventory) {
	if len(providers) == 0 {
		writeLine(w, "no providers installed")
		return
	}
	rows := make([][]string, 0, len(providers))
	for _, provider := range providers {
		rows = append(rows, []string{
			displayProviderName(provider),
			inventoryStatusDisplay(provider.Status),
			provider.Ref,
			fallbackDisplay(provider.Version),
		})
	}
	renderTable(w, []string{"NAME", "STATUS", "PROVIDER", "VERSION"}, rows)
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

func providerInvokeHint(meta state.ProviderMetadata, aliases []string, workspaceScope bool) string {
	target := providerReference(meta)
	if len(aliases) > 0 {
		target = aliases[0]
	}
	if workspaceScope {
		return "tinx -- " + target + " ..."
	}
	addTarget := providerAddTarget(meta)
	if len(aliases) == 0 {
		return "tinx add " + addTarget
	}
	return "tinx add " + addTarget + " as " + aliases[0]
}

func missingProviderInvokeHint(ref string, aliases []string, workspaceScope bool) string {
	if workspaceScope {
		if len(aliases) > 0 {
			return "tinx -- " + aliases[0] + " ..."
		}
		return "tinx -- " + ref + " ..."
	}
	if len(aliases) > 0 {
		return "tinx add " + ref + " as " + aliases[0]
	}
	return "tinx add " + ref
}

func providerAddTarget(meta state.ProviderMetadata) string {
	if ref := strings.TrimSpace(meta.Source.Ref); ref != "" {
		return ref
	}
	return providerReference(meta)
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

func displayProviderName(provider providerInventory) string {
	if len(provider.Aliases) > 0 {
		return strings.Join(provider.Aliases, ",")
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

func filterWorkspaceScopes(scopes []inventoryScope, statusFilter string, activeOnly bool) []inventoryScope {
	if statusFilter == "" && !activeOnly {
		return scopes
	}
	filtered := make([]inventoryScope, 0, len(scopes))
	for _, scope := range scopes {
		if activeOnly && !scope.Active {
			continue
		}
		if statusFilter != "" && !strings.EqualFold(scope.Status, statusFilter) {
			continue
		}
		filtered = append(filtered, scope)
	}
	return filtered
}

func activeWorkspaceScopeName(scopes []inventoryScope) string {
	for _, scope := range scopes {
		if scope.Active {
			return scope.Name
		}
	}
	return ""
}

func renderTable(w io.Writer, headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for index, header := range headers {
		widths[index] = textWidth(header)
	}
	for _, row := range rows {
		for index, cell := range row {
			if width := textWidth(cell); width > widths[index] {
				widths[index] = width
			}
		}
	}
	headerLine := formatTableRow(headers, widths)
	writeLine(w, "%s", headerLine)
	writeLine(w, "%s", strings.Repeat("-", len(headerLine)))
	for _, row := range rows {
		writeLine(w, "%s", formatTableRow(row, widths))
	}
}

func formatTableRow(cells []string, widths []int) string {
	parts := make([]string, 0, len(cells))
	for index, cell := range cells {
		if index == len(cells)-1 {
			parts = append(parts, cell)
			continue
		}
		parts = append(parts, fmt.Sprintf("%-*s", widths[index], cell))
	}
	return strings.Join(parts, "  ")
}

func textWidth(value string) int {
	return utf8.RuneCountInString(value)
}

func displayScopeRoot(scope inventoryScope) string {
	if scope.Type == defaultInventoryScopeName {
		return "(global)"
	}
	return displayInventoryPath(scope.Root)
}

func displayInventoryPath(path string) string {
	absPath := normalizeInventoryPath(path)
	if absPath == "" {
		return "-"
	}
	if cwd, err := os.Getwd(); err == nil {
		normalizedCWD := normalizeInventoryPath(cwd)
		if pathWithinBase(absPath, normalizedCWD) {
			return displayRelativePath(absPath)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		normalizedHome := normalizeInventoryPath(home)
		if pathWithinBase(absPath, normalizedHome) {
			rel, err := filepath.Rel(normalizedHome, absPath)
			if err == nil {
				rel = filepath.Clean(rel)
				if rel == "." {
					return "~"
				}
				return "~" + string(os.PathSeparator) + rel
			}
		}
	}
	return compactAbsolutePath(absPath)
}

func pathWithinBase(target, base string) bool {
	if target == "" || base == "" {
		return false
	}
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	rel = filepath.Clean(rel)
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func compactAbsolutePath(path string) string {
	cleaned := filepath.Clean(path)
	volume := filepath.VolumeName(cleaned)
	trimmed := strings.TrimPrefix(cleaned, volume)
	trimmed = strings.TrimPrefix(trimmed, string(os.PathSeparator))
	if trimmed == "" {
		if volume != "" {
			return volume + string(os.PathSeparator)
		}
		return string(os.PathSeparator)
	}
	parts := strings.Split(trimmed, string(os.PathSeparator))
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			filtered = append(filtered, part)
		}
	}
	if len(filtered) <= 3 {
		return cleaned
	}
	prefix := string(os.PathSeparator)
	if volume != "" {
		prefix = volume + string(os.PathSeparator)
	}
	return filepath.Join(prefix, filtered[0], "...", filtered[len(filtered)-1])
}

func activeMarker(active bool) string {
	if active {
		return "*"
	}
	return " "
}

func inventoryStatusDisplay(status string) string {
	label := strings.TrimSpace(status)
	if label == "" {
		label = "unknown"
	}
	return inventoryStatusSymbol(label) + " " + label
}

func inventoryStatusSymbol(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "ready":
		return "✓"
	case "missing":
		return "✗"
	case "partial":
		return "~"
	case "invalid", "error":
		return "!"
	default:
		return "?"
	}
}

func summarizeWorkspaces(scopes []inventoryScope) string {
	counts := make(map[string]int, len(scopes))
	for _, scope := range scopes {
		counts[strings.ToLower(strings.TrimSpace(scope.Status))]++
	}
	return summarizeInventory(len(scopes), "workspace", "workspaces", counts)
}

func summarizeProviders(providers []providerInventory) string {
	counts := make(map[string]int, len(providers))
	for _, provider := range providers {
		counts[strings.ToLower(strings.TrimSpace(provider.Status))]++
	}
	return summarizeInventory(len(providers), "provider", "providers", counts)
}

func summarizeInventory(total int, singular, plural string, counts map[string]int) string {
	summary := countLabel(total, singular, plural)
	breakdown := statusBreakdown(counts)
	if breakdown == "" {
		return summary
	}
	return summary + " (" + breakdown + ")"
}

func countLabel(total int, singular, plural string) string {
	if total == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %s", total, plural)
}

func statusBreakdown(counts map[string]int) string {
	ordered := []string{"ready", "missing", "invalid", "partial", "unknown"}
	parts := make([]string, 0, len(ordered))
	for _, status := range ordered {
		if count := counts[status]; count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", count, status))
		}
	}
	return strings.Join(parts, ", ")
}

func summaryValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return value
}
