package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type ProviderMetadata struct {
	Namespace              string            `json:"namespace"`
	Name                   string            `json:"name"`
	Version                string            `json:"version"`
	Description            string            `json:"description,omitempty"`
	Homepage               string            `json:"homepage,omitempty"`
	License                string            `json:"license,omitempty"`
	Runtime                string            `json:"runtime"`
	InvocationStyle        string            `json:"invocationStyle,omitempty"`
	Entrypoint             string            `json:"entrypoint"`
	DefaultCapability      string            `json:"defaultCapability,omitempty"`
	Capabilities           []string          `json:"capabilities,omitempty"`
	CapabilityDescriptions map[string]string `json:"capabilityDescriptions,omitempty"`
	Inputs                 map[string]Input  `json:"inputs,omitempty"`
	Platforms              []PlatformSummary `json:"platforms,omitempty"`
	Source                 Source            `json:"source"`
	InstalledAt            time.Time         `json:"installedAt"`
}

type Input struct {
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Default     string `json:"default,omitempty"`
}

type PlatformSummary struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

type Source struct {
	Driver     string `json:"driver,omitempty"`
	LayoutPath string `json:"layoutPath"`
	SourcePath string `json:"sourcePath,omitempty"`
	Tag        string `json:"tag"`
	Ref        string `json:"ref,omitempty"`
	Repo       string `json:"repo,omitempty"`
	Subpath    string `json:"subpath,omitempty"`
	Revision   string `json:"revision,omitempty"`
	Inputs     map[string]string `json:"inputs,omitempty"`
	PlainHTTP  bool   `json:"plainHTTP,omitempty"`
}

const InvocationStylePassthrough = "passthrough"

func ProviderRoot(home, namespace, name string) string {
	return filepath.Join(home, "providers", namespace, name)
}

func VersionRoot(home, namespace, name, version string) string {
	return filepath.Join(ProviderRoot(home, namespace, name), version)
}

func ProviderMetadataPath(home, namespace, name string) string {
	return filepath.Join(ProviderRoot(home, namespace, name), "metadata.json")
}

func InstallStatePath(home, namespace, name, version string) string {
	return filepath.Join(VersionRoot(home, namespace, name, version), "install.json")
}

func SaveProviderMetadata(home string, meta ProviderMetadata) error {
	if err := os.MkdirAll(ProviderRoot(home, meta.Namespace, meta.Name), 0o755); err != nil {
		return fmt.Errorf("create provider root: %w", err)
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode provider metadata: %w", err)
	}
	return os.WriteFile(ProviderMetadataPath(home, meta.Namespace, meta.Name), data, 0o644)
}

func LoadProviderMetadata(home, namespace, name string) (ProviderMetadata, error) {
	var meta ProviderMetadata
	data, err := os.ReadFile(ProviderMetadataPath(home, namespace, name))
	if err != nil {
		return meta, fmt.Errorf("read provider metadata: %w", err)
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return meta, fmt.Errorf("decode provider metadata: %w", err)
	}
	return meta, nil
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
