package oci

import (
	"context"
	"errors"
	goruntime "runtime"
	"testing"

	"oras.land/oras-go/v2/registry/remote/auth"
)

func clearRegistryAuthEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TINX_REGISTRY_USERNAME", "")
	t.Setenv("TINX_REGISTRY_PASSWORD", "")
	t.Setenv("ORAS_USERNAME", "")
	t.Setenv("ORAS_PASSWORD", "")
	t.Setenv("GITHUB_ACTOR", "")
	t.Setenv("GITHUB_TOKEN", "")
}

func TestDockerCredentialLookupEnabledDefaultsByPlatform(t *testing.T) {
	t.Setenv(registryDockerAuthEnv, "")
	if got, want := dockerCredentialLookupEnabled(), goruntime.GOOS != "darwin"; got != want {
		t.Fatalf("dockerCredentialLookupEnabled() = %v, want %v", got, want)
	}
}

func TestDockerCredentialLookupEnabledHonorsOverride(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "enabled", value: "1", want: true},
		{name: "disabled", value: "0", want: false},
		{name: "invalid falls back", value: "maybe", want: goruntime.GOOS != "darwin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(registryDockerAuthEnv, tt.value)
			if got := dockerCredentialLookupEnabled(); got != tt.want {
				t.Fatalf("dockerCredentialLookupEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveRegistryCredentialPrefersEnvironment(t *testing.T) {
	clearRegistryAuthEnv(t)
	t.Setenv("TINX_REGISTRY_USERNAME", "env-user")
	t.Setenv("TINX_REGISTRY_PASSWORD", "env-pass")
	dockerCalls := 0
	cred, err := resolveRegistryCredential(context.Background(), "ghcr.io", func(context.Context, string) (auth.Credential, error) {
		dockerCalls++
		return auth.Credential{Username: "docker-user", Password: "docker-pass"}, nil
	})
	if err != nil {
		t.Fatalf("resolveRegistryCredential() error = %v", err)
	}
	if dockerCalls != 0 {
		t.Fatalf("expected docker credential lookup to be skipped, got %d calls", dockerCalls)
	}
	if cred.Username != "env-user" || cred.Password != "env-pass" {
		t.Fatalf("resolveRegistryCredential() = %#v, want environment credentials", cred)
	}
}

func TestResolveRegistryCredentialFallsBackToDockerResolver(t *testing.T) {
	clearRegistryAuthEnv(t)
	cred, err := resolveRegistryCredential(context.Background(), "registry.example.com", func(context.Context, string) (auth.Credential, error) {
		return auth.Credential{Username: "docker-user", Password: "docker-pass"}, nil
	})
	if err != nil {
		t.Fatalf("resolveRegistryCredential() error = %v", err)
	}
	if cred.Username != "docker-user" || cred.Password != "docker-pass" {
		t.Fatalf("resolveRegistryCredential() = %#v, want docker credentials", cred)
	}
}

func TestResolveRegistryCredentialAllowsAnonymousFallback(t *testing.T) {
	clearRegistryAuthEnv(t)
	cred, err := resolveRegistryCredential(context.Background(), "ghcr.io", func(context.Context, string) (auth.Credential, error) {
		return auth.EmptyCredential, errors.New("docker helper unavailable")
	})
	if err != nil {
		t.Fatalf("resolveRegistryCredential() error = %v", err)
	}
	if cred != auth.EmptyCredential {
		t.Fatalf("resolveRegistryCredential() = %#v, want empty credential", cred)
	}
}
