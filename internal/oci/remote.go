package oci

import (
	"context"
	"fmt"
	"os"
	"strings"

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
	if _, err := oras.Copy(ctx, repo, tag, store, tag, oras.DefaultCopyOptions); err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("pull artifact: %w", err)
	}
	meta, err := installMetadata(tempDir, tag, home, alias, ref)
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
