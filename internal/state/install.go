package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type PackageMetadata struct {
	Namespace              string            `json:"namespace"`
	Name                   string            `json:"name"`
	Version                string            `json:"version"`
	StoreID                string            `json:"storeID,omitempty"`
	StorePath              string            `json:"storePath,omitempty"`
	Description            string            `json:"description,omitempty"`
	Homepage               string            `json:"homepage,omitempty"`
	License                string            `json:"license,omitempty"`
	Runtime                string            `json:"runtime"`
	Entrypoint             string            `json:"entrypoint"`
	Capabilities           []string          `json:"capabilities,omitempty"`
	CapabilityDescriptions map[string]string `json:"capabilityDescriptions,omitempty"`
	Platforms              []PlatformSummary `json:"platforms,omitempty"`
	Dependencies           map[string]string `json:"dependencies,omitempty"`
	Source                 Source            `json:"source"`
	InstalledAt            time.Time         `json:"installedAt"`
}

type ProviderMetadata = PackageMetadata

type PlatformSummary struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

type Source struct {
	LayoutPath string `json:"layoutPath,omitempty"`
	Tag        string `json:"tag,omitempty"`
	Ref        string `json:"ref,omitempty"`
	Resolved   string `json:"resolved,omitempty"`
	Digest     string `json:"digest,omitempty"`
	PlainHTTP  bool   `json:"plainHTTP,omitempty"`
}

func PackageRoot(home, namespace, name string) string {
	return filepath.Join(home, "packages", namespace, name)
}

func ProviderRoot(home, namespace, name string) string {
	return PackageRoot(home, namespace, name)
}

func PackageKey(namespace, name, version string) string {
	return namespace + "/" + name + "@" + version
}

func ProviderKey(namespace, name, version string) string {
	return PackageKey(namespace, name, version)
}

func MetadataKey(meta PackageMetadata) string {
	return PackageKey(meta.Namespace, meta.Name, meta.Version)
}

func VersionRoot(home, namespace, name, version string) string {
	return filepath.Join(PackageRoot(home, namespace, name), version)
}

func PackageMetadataPath(home, namespace, name, version string) string {
	return filepath.Join(VersionRoot(home, namespace, name, version), "metadata.json")
}

func ProviderMetadataPath(home, namespace, name, version string) string {
	return PackageMetadataPath(home, namespace, name, version)
}

func InstallStatePath(home, namespace, name, version string) string {
	return filepath.Join(VersionRoot(home, namespace, name, version), "install.json")
}

func StoreRoot(home, storeID string) string {
	return filepath.Join(home, "store", "oci", storeID)
}

func StoreLayoutPath(home, storeID string) string {
	return StoreRoot(home, storeID)
}

func RuntimeRoot(home, storeID string) string {
	return filepath.Join(home, "runtimes", storeID)
}

func MetadataStoreRoot(meta PackageMetadata) string {
	if path := strings.TrimSpace(meta.StorePath); path != "" {
		return filepath.Clean(path)
	}
	if layoutPath := strings.TrimSpace(meta.Source.LayoutPath); layoutPath != "" && strings.TrimSpace(meta.StoreID) != "" {
		return runtimeRootFromLayoutPath(layoutPath, meta.StoreID)
	}
	if layoutPath := strings.TrimSpace(meta.Source.LayoutPath); layoutPath != "" {
		return filepath.Clean(filepath.Dir(layoutPath))
	}
	return ""
}

func runtimeRootFromLayoutPath(layoutPath, storeID string) string {
	home := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Clean(layoutPath))))
	return RuntimeRoot(home, storeID)
}

func SplitPackageKey(key string) (string, string, string, error) {
	trimmed := strings.TrimSpace(key)
	parts := strings.SplitN(trimmed, "@", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return "", "", "", fmt.Errorf("package key must be <namespace>/<name>@<version>: %q", key)
	}
	refParts := strings.SplitN(parts[0], "/", 2)
	if len(refParts) != 2 || strings.TrimSpace(refParts[0]) == "" || strings.TrimSpace(refParts[1]) == "" {
		return "", "", "", fmt.Errorf("package key must be <namespace>/<name>@<version>: %q", key)
	}
	return strings.TrimSpace(refParts[0]), strings.TrimSpace(refParts[1]), strings.TrimSpace(parts[1]), nil
}

func SplitProviderKey(key string) (string, string, string, error) {
	return SplitPackageKey(key)
}

func PackageRefFromKey(key string) string {
	namespace, name, _, err := SplitPackageKey(key)
	if err != nil {
		return strings.TrimSpace(key)
	}
	return namespace + "/" + name
}

func ProviderRefFromKey(key string) string {
	return PackageRefFromKey(key)
}

func SavePackageMetadata(home string, meta PackageMetadata) error {
	if err := os.MkdirAll(VersionRoot(home, meta.Namespace, meta.Name, meta.Version), 0o755); err != nil {
		return fmt.Errorf("create package version root: %w", err)
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode package metadata: %w", err)
	}
	return os.WriteFile(PackageMetadataPath(home, meta.Namespace, meta.Name, meta.Version), data, 0o644)
}

func SaveProviderMetadata(home string, meta ProviderMetadata) error {
	return SavePackageMetadata(home, meta)
}

func LoadPackageMetadata(home, namespace, name, version string) (PackageMetadata, error) {
	var meta PackageMetadata
	data, err := os.ReadFile(PackageMetadataPath(home, namespace, name, version))
	if err != nil {
		return meta, fmt.Errorf("read package metadata: %w", err)
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return meta, fmt.Errorf("decode package metadata: %w", err)
	}
	return meta, nil
}

func LoadProviderMetadata(home, namespace, name, version string) (ProviderMetadata, error) {
	return LoadPackageMetadata(home, namespace, name, version)
}

func LoadPackageMetadataByKey(home, key string) (PackageMetadata, error) {
	namespace, name, version, err := SplitPackageKey(key)
	if err != nil {
		return PackageMetadata{}, err
	}
	return LoadPackageMetadata(home, namespace, name, version)
}

func LoadProviderMetadataByKey(home, key string) (ProviderMetadata, error) {
	return LoadPackageMetadataByKey(home, key)
}

func SaveInstallSource(home, namespace, name, version string, source Source) error {
	if err := os.MkdirAll(VersionRoot(home, namespace, name, version), 0o755); err != nil {
		return fmt.Errorf("create version root: %w", err)
	}
	data, err := json.MarshalIndent(source, "", "  ")
	if err != nil {
		return fmt.Errorf("encode install source: %w", err)
	}
	return os.WriteFile(InstallStatePath(home, namespace, name, version), data, 0o644)
}
