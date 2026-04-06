package oci

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	ocistore "oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"

	"github.com/sourceplane/tinx/internal/state"
	"github.com/sourceplane/tinx/internal/ui/progress"
)

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

func InstallRemote(ctx context.Context, activationHome, storeHome, ref, alias string, plainHTTP bool, out io.Writer) (state.ProviderMetadata, error) {
	tracker := progress.New(out)
	defer tracker.Finish()
	tracker.Step("lookup", fmt.Sprintf("checking local cache for %s", ref))
	if cached, ok := cachedRemoteInstall(activationHome, ref, false); ok {
		tracker.Cached("cache", fmt.Sprintf("using cached metadata %s/%s@%s", cached.Namespace, cached.Name, cached.Version))
		if err := updateAlias(activationHome, alias, cached); err != nil {
			return state.ProviderMetadata{}, err
		}
		return cached, nil
	}
	return installRemote(ctx, activationHome, storeHome, ref, alias, plainHTTP, true, tracker)
}

func InstallRemoteFull(ctx context.Context, activationHome, storeHome, ref, alias string, plainHTTP bool, out io.Writer) (state.ProviderMetadata, error) {
	tracker := progress.New(out)
	defer tracker.Finish()
	tracker.Step("lookup", fmt.Sprintf("checking local cache for %s", ref))
	if cached, ok := cachedRemoteInstall(activationHome, ref, true); ok {
		tracker.Cached("cache", fmt.Sprintf("using cached runtime %s/%s@%s", cached.Namespace, cached.Name, cached.Version))
		if err := updateAlias(activationHome, alias, cached); err != nil {
			return state.ProviderMetadata{}, err
		}
		return cached, nil
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

	store, err := ocistore.New(tempDir)
	if err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("create local OCI store: %w", err)
	}

	repository, tag := parseReference(ref)
	repo, err := remote.NewRepository(repository)
	if err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("create remote repository: %w", err)
	}
	repo.PlainHTTP = plainHTTP
	configureRepositoryAuth(repo)
	tracker.Step("resolve", fmt.Sprintf("resolved %s:%s", repository, tag))
	if metadataOnly {
		tracker.Info("download", "pulling metadata layers")
	} else {
		tracker.Info("download", "pulling full runtime layers")
	}

	var mu sync.Mutex
	var downloadedCount int
	var downloadedBytes int64
	copyOptions := oras.DefaultCopyOptions
	if metadataOnly {
		copyOptions.PreCopy = func(_ context.Context, descriptor ocispec.Descriptor) error {
			switch descriptor.MediaType {
			case ocispec.MediaTypeImageManifest, MediaTypeConfig, MediaTypeManifest, MediaTypeMetadata:
				mu.Lock()
				downloadedCount++
				downloadedBytes += descriptor.Size
				count := downloadedCount
				bytes := downloadedBytes
				mu.Unlock()
				tracker.Info("download", fmt.Sprintf("%d blobs • %s", count, formatBytes(bytes)))
				return nil
			default:
				return oras.SkipNode
			}
		}
	} else {
		copyOptions.PreCopy = func(_ context.Context, descriptor ocispec.Descriptor) error {
			mu.Lock()
			downloadedCount++
			downloadedBytes += descriptor.Size
			count := downloadedCount
			bytes := downloadedBytes
			mu.Unlock()
			tracker.Info("download", fmt.Sprintf("%d blobs • %s", count, formatBytes(bytes)))
			return nil
		}
	}
	if _, err := oras.Copy(ctx, repo, tag, store, tag, copyOptions); err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("pull artifact: %w", err)
	}
	tracker.Done("download", "artifact pull complete")
	meta, err := installMetadata(tempDir, tag, activationHome, storeHome, alias, ref, plainHTTP, tracker)
	if err != nil {
		return state.ProviderMetadata{}, err
	}
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

func cachedRemoteInstall(home, ref string, requireRuntimeBlobs bool) (state.ProviderMetadata, bool) {
	providers, err := state.ListInstalledProviders(home)
	if err != nil {
		return state.ProviderMetadata{}, false
	}
	requestedRepository, requestedTag := parseReference(ref)
	for _, meta := range providers {
		matchesRequestedRef := strings.TrimSpace(meta.Source.Ref) == ref || strings.TrimSpace(meta.Source.Resolved) == ref
		if !matchesRequestedRef {
			resolvedRepository, resolvedTag := parseReference(meta.Source.Resolved)
			requestedTag = strings.TrimSpace(requestedTag)
			switch {
			case requestedRepository == resolvedRepository && requestedTag != "" && requestedTag == strings.TrimSpace(meta.Source.Digest):
			case requestedRepository == resolvedRepository && requestedTag != "" && requestedTag == strings.TrimSpace(resolvedTag):
			default:
				resolvedRepository, _ = parseReference(meta.Source.Ref)
				if requestedRepository != resolvedRepository || requestedTag == "" || requestedTag != strings.TrimSpace(meta.Version) {
					continue
				}
			}
		}
		if requireRuntimeBlobs && !layoutHasRuntimeBlobs(meta.Source.LayoutPath, meta.Source.Tag) {
			continue
		}
		return meta, true
	}
	return state.ProviderMetadata{}, false
}

func layoutHasRuntimeBlobs(layoutPath, tag string) bool {
	_, view, _, _, _, _, err := readLayout(layoutPath, tag)
	if err != nil {
		return false
	}
	if view.AssetsDescriptor != nil {
		if _, err := readBlob(layoutPath, *view.AssetsDescriptor); err != nil {
			return false
		}
	}
	for _, descriptor := range view.BinaryDescriptors {
		if _, err := readBlob(layoutPath, descriptor); err != nil {
			return false
		}
	}
	return true
}

func configureRepositoryAuth(repo *remote.Repository) {
	var dockerCredResolver auth.CredentialFunc
	if credStore, err := credentials.NewStoreFromDocker(credentials.StoreOptions{}); err == nil {
		dockerCredResolver = credentials.Credential(credStore)
	}

	repo.Client = &auth.Client{
		Credential: func(ctx context.Context, hostport string) (auth.Credential, error) {
			if dockerCredResolver != nil {
				cred, err := dockerCredResolver(ctx, hostport)
				if err == nil && !isEmptyCredential(cred) {
					return cred, nil
				}
			}
			if cred, ok := credentialFromEnv(hostport); ok {
				return cred, nil
			}
			return auth.EmptyCredential, nil
		},
	}
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

func hydrateStoreFromRemote(ctx context.Context, layoutPath, ref string, plainHTTP bool) error {
	tempDir, err := os.MkdirTemp("", "tinx-oci-hydrate-*")
	if err != nil {
		return fmt.Errorf("create temp OCI layout: %w", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := ocistore.New(tempDir)
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
	if _, err := oras.Copy(ctx, repo, tag, store, tag, oras.DefaultCopyOptions); err != nil {
		return fmt.Errorf("pull artifact: %w", err)
	}
	if err := copyDirectory(tempDir, layoutPath); err != nil {
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
