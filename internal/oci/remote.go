package oci

import (
	"context"
	"fmt"
	"os"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	ocistore "oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"

	"github.com/sourceplane/tinx/internal/state"
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

func InstallRemote(ctx context.Context, home, ref, alias string, plainHTTP bool) (state.ProviderMetadata, error) {
	if cached, ok := cachedRemoteInstall(home, ref, false); ok {
		return cached, nil
	}
	return installRemote(ctx, home, ref, alias, plainHTTP, true)
}

func InstallRemoteFull(ctx context.Context, home, ref, alias string, plainHTTP bool) (state.ProviderMetadata, error) {
	if cached, ok := cachedRemoteInstall(home, ref, true); ok {
		return cached, nil
	}
	return installRemote(ctx, home, ref, alias, plainHTTP, false)
}

func installRemote(ctx context.Context, home, ref, alias string, plainHTTP, metadataOnly bool) (state.ProviderMetadata, error) {
	tempDir, err := os.MkdirTemp("", "tinx-oci-*")
	if err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("create temp OCI layout: %w", err)
	}
	defer os.RemoveAll(tempDir)

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
	copyOptions := oras.DefaultCopyOptions
	if metadataOnly {
		copyOptions.PreCopy = func(_ context.Context, descriptor ocispec.Descriptor) error {
			switch descriptor.MediaType {
			case ocispec.MediaTypeImageManifest, MediaTypeConfig, MediaTypeManifest, MediaTypeMetadata:
				return nil
			default:
				return oras.SkipNode
			}
		}
	}
	if _, err := oras.Copy(ctx, repo, tag, store, tag, copyOptions); err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("pull artifact: %w", err)
	}
	meta, err := installMetadata(tempDir, tag, home, alias, ref, plainHTTP)
	if err != nil {
		return state.ProviderMetadata{}, err
	}
	return meta, nil
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
	namespace, name, ok := providerRefFromReference(ref)
	if !ok {
		return state.ProviderMetadata{}, false
	}
	meta, err := state.LoadProviderMetadata(home, namespace, name)
	if err != nil {
		return state.ProviderMetadata{}, false
	}
	if meta.Source.Ref != "" && meta.Source.Ref == ref {
		if requireRuntimeBlobs {
			if !layoutHasRuntimeBlobs(meta.Source.LayoutPath, meta.Source.Tag) {
				return state.ProviderMetadata{}, false
			}
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

func providerRefFromReference(ref string) (string, string, bool) {
	repository := ref
	if at := strings.Index(repository, "@"); at >= 0 {
		repository = repository[:at]
	}
	if idx := strings.LastIndex(repository, ":"); idx > 0 {
		after := repository[idx+1:]
		if !strings.Contains(after, "/") {
			repository = repository[:idx]
		}
	}
	segments := strings.Split(repository, "/")
	if len(segments) < 3 {
		return "", "", false
	}
	return segments[len(segments)-2], segments[len(segments)-1], true
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
