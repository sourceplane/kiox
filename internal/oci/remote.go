package oci

import (
	"context"
	"fmt"
	"os"
	"strings"

	oras "oras.land/oras-go/v2"
	ocistore "oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/registry/remote"

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
