package oci

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	ocistore "oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"

	"github.com/sourceplane/tinx/internal/core"
	"github.com/sourceplane/tinx/internal/state"
	"github.com/sourceplane/tinx/internal/ui/progress"
)

const registryDockerAuthEnv = "TINX_REGISTRY_DOCKER_AUTH"

func PushLayout(ctx context.Context, layoutPath, tag, ref string, plainHTTP bool) error {
	store, err := ocistore.New(layoutPath)
	if err != nil {
		return fmt.Errorf("open OCI layout: %w", err)
	}
	repository, remoteTag := parseReference(ref)
	if tag == "" {
		tag = remoteTag
	}
	repo, err := remote.NewRepository(repository)
	if err != nil {
		return fmt.Errorf("create remote repository: %w", err)
	}
	repo.PlainHTTP = plainHTTP
	configureRepositoryAuth(repo)
	if _, err := oras.Copy(ctx, store, tag, repo, remoteTag, oras.DefaultCopyOptions); err != nil {
		return fmt.Errorf("push artifact: %w", err)
	}
	return nil
}

type remoteInstallCandidate struct {
	Metadata        state.ProviderMetadata
	NeedsActivation bool
}

type RemoteInstallCache struct {
	exactRefs    map[string][]remoteInstallCandidate
	repoTags     map[string][]remoteInstallCandidate
	runtimeBlobs map[string]bool
}

func LoadRemoteInstallCache(activationHome, storeHome string) (*RemoteInstallCache, error) {
	cache := &RemoteInstallCache{
		exactRefs:    map[string][]remoteInstallCandidate{},
		repoTags:     map[string][]remoteInstallCandidate{},
		runtimeBlobs: map[string]bool{},
	}
	seen := map[string]struct{}{}
	providers, err := state.ListInstalledProviders(activationHome)
	if err != nil {
		return nil, err
	}
	for _, meta := range providers {
		cache.addCandidate(seen, remoteInstallCandidate{Metadata: meta})
	}
	if strings.TrimSpace(storeHome) != "" {
		stored, err := listStoredProviders(storeHome)
		if err != nil {
			return nil, err
		}
		for _, meta := range stored {
			cache.addCandidate(seen, remoteInstallCandidate{Metadata: meta, NeedsActivation: true})
		}
	}
	return cache, nil
}

func (cache *RemoteInstallCache) Activate(activationHome, alias, ref string, requireRuntimeBlobs, plainHTTP bool) (state.ProviderMetadata, bool, error) {
	candidate, ok := cache.lookupCandidate(ref, requireRuntimeBlobs)
	if !ok {
		return state.ProviderMetadata{}, false, nil
	}
	meta, err := activateCachedProvider(activationHome, alias, candidate.Metadata, candidate.NeedsActivation, plainHTTP)
	if err != nil {
		return state.ProviderMetadata{}, false, err
	}
	return meta, true, nil
}

func (cache *RemoteInstallCache) addCandidate(seen map[string]struct{}, candidate remoteInstallCandidate) {
	key := storeCandidateKey(candidate.Metadata)
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	ref := strings.TrimSpace(candidate.Metadata.Source.Ref)
	if ref != "" {
		cache.exactRefs[ref] = append(cache.exactRefs[ref], candidate)
	}
	repository, _ := parseReference(ref)
	tag := strings.TrimSpace(candidate.Metadata.Source.Tag)
	if tag == "" {
		tag = strings.TrimSpace(candidate.Metadata.Version)
	}
	if repository == "" || tag == "" {
		return
	}
	cache.repoTags[repository+"@"+tag] = append(cache.repoTags[repository+"@"+tag], candidate)
}

func (cache *RemoteInstallCache) lookupCandidate(ref string, requireRuntimeBlobs bool) (remoteInstallCandidate, bool) {
	if cache == nil {
		return remoteInstallCandidate{}, false
	}
	trimmedRef := strings.TrimSpace(ref)
	if trimmedRef == "" {
		return remoteInstallCandidate{}, false
	}
	if cached, ok := cache.lookupCandidates(cache.exactRefs[trimmedRef], requireRuntimeBlobs); ok {
		return cached, true
	}
	repository, tag := parseReference(trimmedRef)
	if strings.TrimSpace(repository) == "" || strings.TrimSpace(tag) == "" {
		return remoteInstallCandidate{}, false
	}
	return cache.lookupCandidates(cache.repoTags[repository+"@"+strings.TrimSpace(tag)], requireRuntimeBlobs)
}

func InstallRemote(ctx context.Context, activationHome, storeHome, ref, alias string, plainHTTP bool, out io.Writer) (state.ProviderMetadata, error) {
	tracker := progress.New(out)
	defer tracker.Finish()
	tracker.Step("lookup", fmt.Sprintf("checking local cache for %s", ref))
	if cached, ok, err := cachedRemoteInstall(activationHome, storeHome, alias, ref, false, plainHTTP); err != nil {
		return state.ProviderMetadata{}, err
	} else if ok {
		tracker.Cached("cache", fmt.Sprintf("using cached metadata %s/%s@%s", cached.Namespace, cached.Name, cached.Version))
		return cached, nil
	}
	return installRemote(ctx, activationHome, storeHome, ref, alias, plainHTTP, true, tracker)
}

func InstallRemoteFull(ctx context.Context, activationHome, storeHome, ref, alias string, plainHTTP, allowCache bool, out io.Writer) (state.ProviderMetadata, error) {
	tracker := progress.New(out)
	defer tracker.Finish()
	if allowCache {
		tracker.Step("lookup", fmt.Sprintf("checking local cache for %s", ref))
		if cached, ok, err := cachedRemoteInstall(activationHome, storeHome, alias, ref, true, plainHTTP); err != nil {
			return state.ProviderMetadata{}, err
		} else if ok {
			tracker.Cached("cache", fmt.Sprintf("using cached runtime %s/%s@%s", cached.Namespace, cached.Name, cached.Version))
			return cached, nil
		}
	}
	return installRemote(ctx, activationHome, storeHome, ref, alias, plainHTTP, false, tracker)
}

func installRemote(ctx context.Context, activationHome, storeHome, ref, alias string, plainHTTP, metadataOnly bool, tracker *progress.Tracker) (state.ProviderMetadata, error) {
	tempDir, err := os.MkdirTemp("", "tinx-oci-*")
	if err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("create temp OCI layout: %w", err)
	}
	defer os.RemoveAll(tempDir)
	tracker.Step("prepare", "created temporary OCI workspace")
	repository, tag := parseReference(ref)
	layoutTag := normalizedLayoutTag(ref, tag)
	tracker.Step("resolve", fmt.Sprintf("resolved %s:%s", repository, tag))
	if err := pullRemoteLayout(ctx, tempDir, ref, plainHTTP, remoteCopySelection{metadataOnly: true}, tracker, "pulling metadata layers"); err != nil {
		return state.ProviderMetadata{}, err
	}
	tracker.Done("download", "metadata pull complete")
	meta, err := installMetadata(tempDir, layoutTag, activationHome, storeHome, alias, ref, plainHTTP, tracker)
	if err != nil {
		return state.ProviderMetadata{}, err
	}
	if metadataOnly {
		tracker.Done("install", fmt.Sprintf("installed %s/%s@%s", meta.Namespace, meta.Name, meta.Version))
		return meta, nil
	}
	if layoutHasRuntimeBlobs(meta.Source.LayoutPath, meta.Source.Tag) {
		tracker.Cached("cache", fmt.Sprintf("reusing shared provider store %s/%s@%s", meta.Namespace, meta.Name, meta.Version))
		tracker.Done("install", fmt.Sprintf("installed %s/%s@%s", meta.Namespace, meta.Name, meta.Version))
		return meta, nil
	}
	if err := hydrateStoreFromRemote(ctx, meta.Source.LayoutPath, meta.Source.Ref, meta.Source.PlainHTTP, remoteCopySelection{goos: goruntime.GOOS, goarch: goruntime.GOARCH}, tracker, fmt.Sprintf("pulling %s/%s runtime layers", goruntime.GOOS, goruntime.GOARCH)); err != nil {
		return state.ProviderMetadata{}, err
	}
	tracker.Done("download", "runtime pull complete")
	tracker.Done("install", fmt.Sprintf("installed %s/%s@%s", meta.Namespace, meta.Name, meta.Version))
	return meta, nil
}

func formatBytes(size int64) string {
	const unit = int64(1024)
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := unit, 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}

func parseReference(ref string) (repository, tag string) {
	repository = ref
	tag = "latest"
	if at := strings.Index(ref, "@"); at >= 0 {
		return ref[:at], ref[at+1:]
	}
	if idx := strings.LastIndex(ref, ":"); idx > 0 {
		after := ref[idx+1:]
		if !strings.Contains(after, "/") {
			return ref[:idx], after
		}
	}
	return repository, tag
}

func normalizedLayoutTag(ref, tag string) string {
	if strings.Contains(strings.TrimSpace(ref), "@") {
		return ""
	}
	return strings.TrimSpace(tag)
}

type remoteCopySelection struct {
	metadataOnly bool
	goos         string
	goarch       string
}

func pullRemoteLayout(ctx context.Context, layoutPath, ref string, plainHTTP bool, selection remoteCopySelection, tracker *progress.Tracker, detail string) error {
	store, err := ocistore.New(layoutPath)
	if err != nil {
		return fmt.Errorf("create local OCI store: %w", err)
	}
	repository, tag := parseReference(ref)
	repo, err := remote.NewRepository(repository)
	if err != nil {
		return fmt.Errorf("create remote repository: %w", err)
	}
	repo.PlainHTTP = plainHTTP
	configureRepositoryAuth(repo)
	if tracker != nil && strings.TrimSpace(detail) != "" {
		tracker.Info("download", detail)
	}
	var mu sync.Mutex
	var downloadedCount int
	var downloadedBytes int64
	copyOptions := oras.DefaultCopyOptions
	copyOptions.PreCopy = func(_ context.Context, descriptor ocispec.Descriptor) error {
		if !shouldCopyDescriptor(descriptor, selection) {
			return oras.SkipNode
		}
		if tracker != nil {
			mu.Lock()
			downloadedCount++
			downloadedBytes += descriptor.Size
			count := downloadedCount
			bytes := downloadedBytes
			mu.Unlock()
			tracker.Info("download", fmt.Sprintf("%d blobs • %s", count, formatBytes(bytes)))
		}
		return nil
	}
	if _, err := oras.Copy(ctx, repo, tag, store, tag, copyOptions); err != nil {
		return fmt.Errorf("pull artifact: %w", err)
	}
	return nil
}

func shouldCopyDescriptor(descriptor ocispec.Descriptor, selection remoteCopySelection) bool {
	if isMetadataDescriptor(descriptor) {
		return true
	}
	if selection.metadataOnly {
		return false
	}
	if isArchiveMediaType(descriptor.MediaType) {
		return true
	}
	platformOS, platformArch, ok := descriptorPlatform(descriptor)
	if !ok {
		return true
	}
	return platformMatches(core.PlatformSpec{OS: platformOS, Arch: platformArch}, selection.goos, selection.goarch)
}

func isMetadataDescriptor(descriptor ocispec.Descriptor) bool {
	switch descriptor.MediaType {
	case ocispec.MediaTypeImageManifest, MediaTypeConfig, MediaTypeManifest, MediaTypeMetadata:
		return true
	default:
		return false
	}
}

func descriptorPlatform(descriptor ocispec.Descriptor) (string, string, bool) {
	if raw := strings.TrimSpace(descriptor.Annotations["io.tinx.platform"]); raw != "" {
		parts := strings.SplitN(raw, "/", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
			return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
		}
	}
	const prefix = "application/vnd.tinx.provider.binary."
	const suffix = ".v1"
	mediaType := strings.TrimSpace(descriptor.MediaType)
	if !strings.HasPrefix(mediaType, prefix) || !strings.HasSuffix(mediaType, suffix) {
		return "", "", false
	}
	trimmed := strings.TrimSuffix(strings.TrimPrefix(mediaType, prefix), suffix)
	parts := strings.SplitN(trimmed, ".", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func cachedRemoteInstall(activationHome, storeHome, alias, ref string, requireRuntimeBlobs, plainHTTP bool) (state.ProviderMetadata, bool, error) {
	cache, err := LoadRemoteInstallCache(activationHome, storeHome)
	if err != nil {
		return state.ProviderMetadata{}, false, err
	}
	return cache.Activate(activationHome, alias, ref, requireRuntimeBlobs, plainHTTP)
}

func (cache *RemoteInstallCache) lookupCandidates(candidates []remoteInstallCandidate, requireRuntimeBlobs bool) (remoteInstallCandidate, bool) {
	for _, meta := range candidates {
		if requireRuntimeBlobs && !cache.hasRuntimeBlobs(meta.Metadata) {
			continue
		}
		return meta, true
	}
	return remoteInstallCandidate{}, false
}

func (cache *RemoteInstallCache) hasRuntimeBlobs(meta state.ProviderMetadata) bool {
	if cache == nil {
		return layoutHasRuntimeBlobs(meta.Source.LayoutPath, meta.Source.Tag)
	}
	key := storeCandidateKey(meta)
	if ready, ok := cache.runtimeBlobs[key]; ok {
		return ready
	}
	ready := layoutHasRuntimeBlobs(meta.Source.LayoutPath, meta.Source.Tag)
	cache.runtimeBlobs[key] = ready
	return ready
}

func layoutHasRuntimeBlobs(layoutPath, tag string) bool {
	_, view, _, _, _, err := readLayout(layoutPath, tag)
	if err != nil {
		return false
	}
	for _, layer := range view.BundleLayers {
		if !isArchiveMediaType(layer.MediaType) && !platformMatches(layer.Platform, goruntime.GOOS, goruntime.GOARCH) {
			continue
		}
		if _, err := readBlob(layoutPath, layer.Descriptor); err != nil {
			return false
		}
	}
	return true
}

func configureRepositoryAuth(repo *remote.Repository) {
	dockerCredResolver := newDockerCredentialResolver()
	repo.Client = &auth.Client{
		Credential: func(ctx context.Context, hostport string) (auth.Credential, error) {
			return resolveRegistryCredential(ctx, hostport, dockerCredResolver)
		},
	}
}

func newDockerCredentialResolver() auth.CredentialFunc {
	if !dockerCredentialLookupEnabled() {
		return nil
	}
	credStore, err := credentials.NewStoreFromDocker(credentials.StoreOptions{})
	if err != nil {
		return nil
	}
	return credentials.Credential(credStore)
}

func dockerCredentialLookupEnabled() bool {
	return envBoolDefault(registryDockerAuthEnv, goruntime.GOOS != "darwin")
}

func envBoolDefault(name string, defaultValue bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	switch raw {
	case "":
		return defaultValue
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func resolveRegistryCredential(ctx context.Context, hostport string, dockerCredResolver auth.CredentialFunc) (auth.Credential, error) {
	if cred, ok := credentialFromEnv(hostport); ok {
		return cred, nil
	}
	if dockerCredResolver != nil {
		cred, err := dockerCredResolver(ctx, hostport)
		if err == nil && !isEmptyCredential(cred) {
			return cred, nil
		}
	}
	return auth.EmptyCredential, nil
}

func updateAlias(home, alias string, meta state.ProviderMetadata) error {
	if alias == "" {
		return nil
	}
	aliases, err := state.LoadAliases(home)
	if err != nil {
		return err
	}
	aliases[alias] = state.MetadataKey(meta)
	return state.SaveAliases(home, aliases)
}

func hydrateStoreFromRemote(ctx context.Context, layoutPath, ref string, plainHTTP bool, selection remoteCopySelection, tracker *progress.Tracker, detail string) error {
	tempDir, err := os.MkdirTemp("", "tinx-oci-hydrate-*")
	if err != nil {
		return fmt.Errorf("create temp OCI layout: %w", err)
	}
	defer os.RemoveAll(tempDir)
	if err := pullRemoteLayout(ctx, tempDir, ref, plainHTTP, selection, tracker, detail); err != nil {
		return fmt.Errorf("hydrate cached OCI layout: %w", err)
	}
	if err := copyDirectory(filepath.Join(tempDir, "blobs"), filepath.Join(layoutPath, "blobs")); err != nil {
		return fmt.Errorf("hydrate cached OCI layout: %w", err)
	}
	return nil
}

func credentialFromEnv(hostport string) (auth.Credential, bool) {
	if username, password := os.Getenv("TINX_REGISTRY_USERNAME"), os.Getenv("TINX_REGISTRY_PASSWORD"); username != "" && password != "" {
		return auth.Credential{Username: username, Password: password}, true
	}
	if username, password := os.Getenv("ORAS_USERNAME"), os.Getenv("ORAS_PASSWORD"); username != "" && password != "" {
		return auth.Credential{Username: username, Password: password}, true
	}
	if hostport == "ghcr.io" {
		if actor, token := os.Getenv("GITHUB_ACTOR"), os.Getenv("GITHUB_TOKEN"); actor != "" && token != "" {
			return auth.Credential{Username: actor, Password: token}, true
		}
	}
	return auth.EmptyCredential, false
}

func isEmptyCredential(cred auth.Credential) bool {
	return cred.Username == "" && cred.Password == "" && cred.AccessToken == "" && cred.RefreshToken == ""
}
