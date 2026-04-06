package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ProviderMetadata struct {
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
	Source                 Source            `json:"source"`
	InstalledAt            time.Time         `json:"installedAt"`
}

type PlatformSummary struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

type Source struct {
	LayoutPath string `json:"layoutPath"`
	Tag        string `json:"tag"`
	Ref        string `json:"ref,omitempty"`
	PlainHTTP  bool   `json:"plainHTTP,omitempty"`
}

func ProviderRoot(home, namespace, name string) string {
	return filepath.Join(home, "providers", namespace, name)
}

func ProviderKey(namespace, name, version string) string {
	return namespace + "/" + name + "@" + version
}

func MetadataKey(meta ProviderMetadata) string {
	return ProviderKey(meta.Namespace, meta.Name, meta.Version)
}

func VersionRoot(home, namespace, name, version string) string {
	return filepath.Join(ProviderRoot(home, namespace, name), version)
}

func ProviderMetadataPath(home, namespace, name, version string) string {
	return filepath.Join(VersionRoot(home, namespace, name, version), "metadata.json")
}

func InstallStatePath(home, namespace, name, version string) string {
	return filepath.Join(VersionRoot(home, namespace, name, version), "install.json")
}

func StoreRoot(home, storeID string) string {
	return filepath.Join(home, "store", storeID)
}

func StoreLayoutPath(home, storeID string) string {
	return filepath.Join(StoreRoot(home, storeID), "oci")
}

func MetadataStoreRoot(meta ProviderMetadata) string {
	if path := strings.TrimSpace(meta.StorePath); path != "" {
		return filepath.Clean(path)
	}
	if layoutPath := strings.TrimSpace(meta.Source.LayoutPath); layoutPath != "" {
		return filepath.Dir(layoutPath)
	}
	return ""
}

func SplitProviderKey(key string) (string, string, string, error) {
	trimmed := strings.TrimSpace(key)
	parts := strings.SplitN(trimmed, "@", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return "", "", "", fmt.Errorf("provider key must be <namespace>/<name>@<version>: %q", key)
	}
	refParts := strings.SplitN(parts[0], "/", 2)
	if len(refParts) != 2 || strings.TrimSpace(refParts[0]) == "" || strings.TrimSpace(refParts[1]) == "" {
		return "", "", "", fmt.Errorf("provider key must be <namespace>/<name>@<version>: %q", key)
	}
	return strings.TrimSpace(refParts[0]), strings.TrimSpace(refParts[1]), strings.TrimSpace(parts[1]), nil
}

func ProviderRefFromKey(key string) string {
	namespace, name, _, err := SplitProviderKey(key)
	if err != nil {
		return strings.TrimSpace(key)
	}
	return namespace + "/" + name
}

func SaveProviderMetadata(home string, meta ProviderMetadata) error {
	if err := os.MkdirAll(VersionRoot(home, meta.Namespace, meta.Name, meta.Version), 0o755); err != nil {
		return fmt.Errorf("create provider version root: %w", err)
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode provider metadata: %w", err)
	}
	return os.WriteFile(ProviderMetadataPath(home, meta.Namespace, meta.Name, meta.Version), data, 0o644)
}

func LoadProviderMetadata(home, namespace, name, version string) (ProviderMetadata, error) {
	var meta ProviderMetadata
	data, err := os.ReadFile(ProviderMetadataPath(home, namespace, name, version))
	if err != nil {
		return meta, fmt.Errorf("read provider metadata: %w", err)
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return meta, fmt.Errorf("decode provider metadata: %w", err)
	}
	return meta, nil
}

func LoadProviderMetadataByKey(home, key string) (ProviderMetadata, error) {
	namespace, name, version, err := SplitProviderKey(key)
	if err != nil {
		return ProviderMetadata{}, err
	}
	return LoadProviderMetadata(home, namespace, name, version)
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
