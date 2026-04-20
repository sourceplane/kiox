package workspace

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/sourceplane/kiox/internal/oci"
	"github.com/sourceplane/kiox/internal/resolver"
	"github.com/sourceplane/kiox/internal/state"
	"github.com/sourceplane/kiox/internal/ui/progress"
	"golang.org/x/sync/errgroup"
)

const (
	syncInstallConcurrencyEnv     = "KIOX_SYNC_INSTALL_CONCURRENCY"
	defaultSyncInstallConcurrency = 4
	defaultSyncRuntimeConcurrency = 2
)

type SyncOptions struct {
	Out            io.Writer
	GlobalHome     string
	RefreshAliases []string
	Verbose        bool
}

type SyncResult struct {
	Home      string
	Aliases   map[string]string
	Providers []LockedProvider
}

type workspaceInstallRequest struct {
	Alias         string
	InstallSource string
	Provider      Provider
	AllowCache    bool
}

type workspaceInstallTask struct {
	Request workspaceInstallRequest
	Aliases []string
}

func (task workspaceInstallTask) Key() string {
	return workspaceInstallTaskKey(task.Request)
}

func (task workspaceInstallTask) DisplayName() string {
	return displayProviderSourceName(task.Request.InstallSource)
}

type workspaceInstallPrepared struct {
	Task         workspaceInstallTask
	Metadata     state.ProviderMetadata
	Cached       bool
	NeedsRuntime bool
}

type workspacePendingRuntime struct {
	Task     workspaceInstallTask
	Metadata state.ProviderMetadata
}

type workspaceInstallOutcome struct {
	Metadata state.ProviderMetadata
	Cached   bool
}

func LoadPreparedState(root string, config Config) (SyncResult, bool, error) {
	if err := config.Normalize(); err != nil {
		return SyncResult{}, false, err
	}
	home := Home(root)
	providers := config.ProviderMap()
	result := SyncResult{
		Home:      home,
		Aliases:   make(map[string]string, len(providers)),
		Providers: make([]LockedProvider, 0, len(providers)),
	}
	aliases, err := state.LoadAliases(home)
	if err != nil {
		return SyncResult{}, false, err
	}
	lock, err := LoadLock(root)
	if err != nil {
		return SyncResult{}, false, err
	}
	if len(aliases) != len(providers) || len(lock.Providers) != len(providers) {
		return result, false, nil
	}
	lockedByAlias := make(map[string]LockedProvider, len(lock.Providers))
	for _, provider := range lock.Providers {
		lockedByAlias[provider.Alias] = provider
	}
	if len(lockedByAlias) != len(lock.Providers) {
		return result, false, nil
	}
	for _, alias := range config.ProviderAliases() {
		provider := providers[alias]
		lockedProvider, ok := lockedByAlias[alias]
		if !ok {
			return result, false, nil
		}
		source := resolveWorkspaceSource(root, provider.Source)
		if strings.TrimSpace(lockedProvider.Source) != source {
			return result, false, nil
		}
		providerKey := strings.TrimSpace(aliases[alias])
		if providerKey == "" {
			return result, false, nil
		}
		meta, err := state.LoadProviderMetadataByKey(home, providerKey)
		if err != nil {
			if os.IsNotExist(err) {
				return result, false, nil
			}
			return SyncResult{}, false, err
		}
		if strings.TrimSpace(meta.Source.LayoutPath) == "" {
			return result, false, nil
		}
		if _, err := os.Stat(meta.Source.LayoutPath); err != nil {
			if os.IsNotExist(err) {
				return result, false, nil
			}
			return SyncResult{}, false, err
		}
		if expectedProvider := strings.TrimSpace(lockedProvider.Provider); expectedProvider != "" && state.ProviderRefFromKey(providerKey) != expectedProvider {
			return result, false, nil
		}
		if expectedVersion := strings.TrimSpace(lockedProvider.Version); expectedVersion != "" && strings.TrimSpace(meta.Version) != expectedVersion {
			return result, false, nil
		}
		if expectedStore := strings.TrimSpace(lockedProvider.Store); expectedStore != "" && strings.TrimSpace(meta.StoreID) != expectedStore {
			return result, false, nil
		}
		if expectedResolved := strings.TrimSpace(lockedProvider.Resolved); expectedResolved != "" && strings.TrimSpace(meta.Source.Ref) != expectedResolved {
			return result, false, nil
		}
		if !preparedWorkspaceProviderMatchesTransport(source, provider, meta) {
			return result, false, nil
		}
		result.Aliases[alias] = providerKey
		result.Providers = append(result.Providers, lockedProvider)
	}
	return result, true, nil
}

func Sync(ctx context.Context, root string, config Config, opts SyncOptions) (SyncResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := config.Normalize(); err != nil {
		return SyncResult{}, err
	}
	home := Home(root)
	if err := state.EnsureHome(home); err != nil {
		return SyncResult{}, err
	}
	existingAliases, err := state.LoadAliases(home)
	if err != nil {
		return SyncResult{}, err
	}
	result := SyncResult{
		Home:      home,
		Aliases:   make(map[string]string, len(config.ProviderMap())),
		Providers: make([]LockedProvider, 0, len(config.ProviderMap())),
	}
	storeHome := strings.TrimSpace(opts.GlobalHome)
	if storeHome == "" {
		storeHome = home
	}
	remoteCache, err := oci.LoadRemoteInstallCache(home, storeHome)
	if err != nil {
		return SyncResult{}, err
	}
	locked, err := LoadLock(root)
	if err != nil {
		return SyncResult{}, err
	}
	lockedByAlias := make(map[string]LockedProvider, len(locked.Providers))
	for _, provider := range locked.Providers {
		lockedByAlias[provider.Alias] = provider
	}
	refreshAliases := make(map[string]struct{}, len(opts.RefreshAliases))
	for _, alias := range opts.RefreshAliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		refreshAliases[alias] = struct{}{}
	}
	requests := make([]workspaceInstallRequest, 0, len(config.ProviderMap()))
	for _, alias := range config.ProviderAliases() {
		provider := config.ProviderMap()[alias]
		source := resolveWorkspaceSource(root, provider.Source)
		installSource := resolvedWorkspaceInstallSource(source, lockedByAlias[alias], refreshAliases, alias)
		_, refreshRequested := refreshAliases[alias]
		requests = append(requests, workspaceInstallRequest{
			Alias:         alias,
			InstallSource: installSource,
			Provider:      provider,
			AllowCache:    !refreshRequested,
		})
	}
	tasks := groupWorkspaceInstallRequests(requests)
	surface := progress.NewProviderSyncSurface(opts.Out, len(tasks), opts.Verbose)
	installedByAlias, err := executeWorkspaceInstallRequests(ctx, requests, surface, func(ctx context.Context, task workspaceInstallTask) (workspaceInstallPrepared, error) {
		return prepareWorkspaceInstallTask(ctx, home, storeHome, task, remoteCache, surface)
	}, func(ctx context.Context, pending workspacePendingRuntime) error {
		if surface != nil {
			surface.Update(pending.Task.Key(), displayProviderMetadataName(pending.Metadata), progress.ProviderSyncStatePulling, "")
		}
		_, err := oci.EnsureRemoteRuntime(ctx, pending.Metadata, nil)
		return err
	})
	if surface != nil {
		surface.Finish(err)
	}
	if err != nil {
		return SyncResult{}, err
	}
	for _, request := range requests {
		alias := request.Alias
		provider := request.Provider
		source := resolveWorkspaceSource(root, provider.Source)
		installSource := request.InstallSource
		installed := installedByAlias[alias].Metadata
		providerRef := installed.Namespace + "/" + installed.Name
		providerKey := state.MetadataKey(installed)
		result.Aliases[alias] = providerKey
		result.Providers = append(result.Providers, LockedProvider{
			Alias:    alias,
			Provider: providerRef,
			Source:   source,
			Version:  installed.Version,
			Resolved: lockedProviderResolvedSource(installed, installSource),
			Store:    installed.StoreID,
		})
	}
	if err := removeStaleWorkspaceProviders(home, existingAliases, result.Aliases); err != nil {
		return SyncResult{}, err
	}
	if err := state.SaveAliases(home, result.Aliases); err != nil {
		return SyncResult{}, err
	}
	if err := SaveLock(root, config.Name(), result.Providers); err != nil {
		return SyncResult{}, err
	}
	return result, nil
}

func executeWorkspaceInstallRequests(ctx context.Context, requests []workspaceInstallRequest, surface *progress.ProviderSyncSurface, prepare func(context.Context, workspaceInstallTask) (workspaceInstallPrepared, error), hydrate func(context.Context, workspacePendingRuntime) error) (map[string]workspaceInstallOutcome, error) {
	results := make(map[string]workspaceInstallOutcome, len(requests))
	if len(requests) == 0 {
		return results, nil
	}
	tasks := groupWorkspaceInstallRequests(requests)
	for _, task := range tasks {
		if surface != nil {
			surface.Start(task.Key(), task.DisplayName())
		}
	}
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(syncInstallConcurrency(len(tasks)))
	var resultsMu sync.Mutex
	var pendingMu sync.Mutex
	pending := make([]workspacePendingRuntime, 0, len(tasks))
	for _, task := range tasks {
		task := task
		eg.Go(func() error {
			prepared, err := prepare(egCtx, task)
			if err != nil {
				if surface != nil {
					surface.Fail(task.Key(), task.DisplayName(), err)
				}
				return err
			}
			if prepared.NeedsRuntime {
				pendingMu.Lock()
				pending = append(pending, workspacePendingRuntime{Task: prepared.Task, Metadata: prepared.Metadata})
				pendingMu.Unlock()
				return nil
			}
			resultsMu.Lock()
			for _, alias := range task.Aliases {
				results[alias] = workspaceInstallOutcome{Metadata: prepared.Metadata, Cached: prepared.Cached}
			}
			resultsMu.Unlock()
			if surface != nil {
				surface.Complete(task.Key(), displayProviderMetadataName(prepared.Metadata), prepared.Cached)
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	if len(pending) == 0 {
		return results, nil
	}
	runtimeLimit := syncRuntimeConcurrency(len(pending), syncInstallConcurrency(len(tasks)))
	eg, egCtx = errgroup.WithContext(ctx)
	eg.SetLimit(runtimeLimit)
	for _, pendingInstall := range pending {
		pendingInstall := pendingInstall
		eg.Go(func() error {
			if err := hydrate(egCtx, pendingInstall); err != nil {
				if surface != nil {
					surface.Fail(pendingInstall.Task.Key(), displayProviderMetadataName(pendingInstall.Metadata), err)
				}
				return err
			}
			resultsMu.Lock()
			for _, alias := range pendingInstall.Task.Aliases {
				results[alias] = workspaceInstallOutcome{Metadata: pendingInstall.Metadata}
			}
			resultsMu.Unlock()
			if surface != nil {
				surface.Complete(pendingInstall.Task.Key(), displayProviderMetadataName(pendingInstall.Metadata), false)
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return results, nil
}

func prepareWorkspaceInstallTask(ctx context.Context, home, storeHome string, task workspaceInstallTask, remoteCache *oci.RemoteInstallCache, surface *progress.ProviderSyncSurface) (workspaceInstallPrepared, error) {
	request := task.Request
	prepared := workspaceInstallPrepared{Task: task}
	if layoutPath, ok := localLayoutPath(request.InstallSource); ok {
		if surface != nil {
			surface.Update(task.Key(), task.DisplayName(), progress.ProviderSyncStateInstalling, "")
		}
		meta, err := oci.InstallMetadata(layoutPath, "", home, storeHome, "", nil)
		if err != nil {
			return workspaceInstallPrepared{}, err
		}
		prepared.Metadata = meta
		return prepared, nil
	}
	if resolver.HasSourceScheme(request.InstallSource) {
		return workspaceInstallPrepared{}, fmt.Errorf("unsupported provider source %q: expected an OCI registry reference or local OCI layout", request.InstallSource)
	}
	if request.AllowCache {
		if cached, ok, err := remoteCache.Activate(home, "", request.InstallSource, true, request.Provider.PlainHTTP); err != nil {
			return workspaceInstallPrepared{}, err
		} else if ok {
			prepared.Metadata = cached
			prepared.Cached = true
			return prepared, nil
		}
		if cached, ok, err := remoteCache.Activate(home, "", request.InstallSource, false, request.Provider.PlainHTTP); err != nil {
			return workspaceInstallPrepared{}, err
		} else if ok {
			if surface != nil {
				surface.Update(task.Key(), displayProviderMetadataName(cached), progress.ProviderSyncStateInstalling, "")
			}
			prepared.Metadata = cached
			prepared.NeedsRuntime = true
			return prepared, nil
		}
	}
	if surface != nil {
		surface.Update(task.Key(), task.DisplayName(), progress.ProviderSyncStatePulling, "")
	}
	meta, err := oci.InstallRemoteMetadata(ctx, home, storeHome, request.InstallSource, "", request.Provider.PlainHTTP, request.AllowCache, nil)
	if err != nil {
		return workspaceInstallPrepared{}, err
	}
	if surface != nil {
		surface.Update(task.Key(), displayProviderMetadataName(meta), progress.ProviderSyncStateInstalling, "")
	}
	prepared.Metadata = meta
	prepared.NeedsRuntime = true
	return prepared, nil
}

func groupWorkspaceInstallRequests(requests []workspaceInstallRequest) []workspaceInstallTask {
	tasks := make(map[string]*workspaceInstallTask, len(requests))
	orderedKeys := make([]string, 0, len(requests))
	for _, request := range requests {
		key := workspaceInstallTaskKey(request)
		task, ok := tasks[key]
		if !ok {
			task = &workspaceInstallTask{Request: request}
			tasks[key] = task
			orderedKeys = append(orderedKeys, key)
		}
		task.Aliases = append(task.Aliases, request.Alias)
	}
	grouped := make([]workspaceInstallTask, 0, len(orderedKeys))
	for _, key := range orderedKeys {
		grouped = append(grouped, *tasks[key])
	}
	return grouped
}

func workspaceInstallTaskKey(request workspaceInstallRequest) string {
	return fmt.Sprintf("%s|%t|%t", strings.TrimSpace(request.InstallSource), request.Provider.PlainHTTP, request.AllowCache)
}

func syncInstallConcurrency(total int) int {
	if total <= 0 {
		return 0
	}
	concurrency := envPositiveIntDefault(syncInstallConcurrencyEnv, defaultSyncInstallConcurrency)
	if concurrency > total {
		return total
	}
	return concurrency
}

func syncRuntimeConcurrency(total, installConcurrency int) int {
	if total <= 0 {
		return 0
	}
	if installConcurrency <= 0 {
		installConcurrency = defaultSyncInstallConcurrency
	}
	concurrency := installConcurrency
	if concurrency > defaultSyncRuntimeConcurrency {
		concurrency = defaultSyncRuntimeConcurrency
	}
	if concurrency > total {
		return total
	}
	return concurrency
}

func envPositiveIntDefault(name string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultValue
	}
	return value
}

func displayProviderSourceName(source string) string {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return "provider"
	}
	if at := strings.Index(trimmed, "@"); at >= 0 {
		trimmed = trimmed[:at]
	}
	if colon := strings.LastIndex(trimmed, ":"); colon > 0 {
		after := trimmed[colon+1:]
		if !strings.Contains(after, "/") {
			trimmed = trimmed[:colon]
		}
	}
	trimmed = filepath.Clean(trimmed)
	parts := strings.Split(trimmed, "/")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return filepath.Base(trimmed)
}

func displayProviderMetadataName(meta state.ProviderMetadata) string {
	if namespace := strings.TrimSpace(meta.Namespace); namespace != "" && strings.TrimSpace(meta.Name) != "" {
		return namespace + "/" + strings.TrimSpace(meta.Name)
	}
	if name := strings.TrimSpace(meta.Name); name != "" {
		return name
	}
	return "provider"
}

func resolvedWorkspaceInstallSource(source string, locked LockedProvider, refreshAliases map[string]struct{}, alias string) string {
	if _, ok := refreshAliases[alias]; ok {
		return source
	}
	if _, ok := localLayoutPath(source); ok {
		return source
	}
	if workspaceSourcePinned(source) {
		return source
	}
	if strings.TrimSpace(locked.Source) == source && strings.TrimSpace(locked.Resolved) != "" {
		return strings.TrimSpace(locked.Resolved)
	}
	return source
}

func preparedWorkspaceProviderMatchesTransport(source string, provider Provider, meta state.ProviderMetadata) bool {
	if _, ok := localLayoutPath(source); ok {
		return true
	}
	if resolver.HasSourceScheme(source) {
		return true
	}
	return provider.PlainHTTP == meta.Source.PlainHTTP
}

func workspaceSourcePinned(source string) bool {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return false
	}
	return strings.Contains(trimmed, "@")
}

func lockedProviderResolvedSource(meta state.ProviderMetadata, installSource string) string {
	if resolved := strings.TrimSpace(meta.Source.Ref); resolved != "" {
		return resolved
	}
	return strings.TrimSpace(installSource)
}

func resolveWorkspaceSource(root, source string) string {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return ""
	}
	if path, ok := localLayoutPath(trimmed); ok {
		return path
	}
	if filepath.IsAbs(trimmed) {
		return trimmed
	}
	rootRelative := filepath.Join(root, trimmed)
	if path, ok := localLayoutPath(rootRelative); ok {
		return path
	}
	return resolver.ResolveProviderSource(trimmed)
}

func localLayoutPath(source string) (string, bool) {
	path := strings.TrimSpace(source)
	if path == "" {
		return "", false
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(path, "index.json")); err != nil {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(path, "oci-layout")); err != nil {
		return "", false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	return absPath, true
}

func removeStaleWorkspaceProviders(home string, previousAliases, desiredAliases map[string]string) error {
	desiredKeys := make(map[string]struct{}, len(desiredAliases))
	for _, key := range desiredAliases {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		desiredKeys[trimmed] = struct{}{}
	}
	staleKeys := make(map[string]struct{})
	for alias, key := range previousAliases {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		if desiredKey, ok := desiredAliases[alias]; ok && strings.TrimSpace(desiredKey) == trimmedKey {
			continue
		}
		if _, ok := desiredKeys[trimmedKey]; ok {
			continue
		}
		staleKeys[trimmedKey] = struct{}{}
	}
	for key := range staleKeys {
		if err := state.RemoveProviderByKey(home, key); err != nil {
			return err
		}
	}
	return nil
}
