package oci

import (
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	ArtifactTypeProvider = "application/vnd.tinx.provider.v1"
	MediaTypeConfig      = "application/vnd.tinx.provider.config.v1+json"
	MediaTypeManifest    = "application/vnd.tinx.provider.manifest.v1+yaml"
	MediaTypeMetadata    = "application/vnd.tinx.provider.metadata.v1+json"
	MediaTypeAssets      = "application/vnd.tinx.provider.assets.v1+tar"
)

type ProviderConfig struct {
	APIVersion  string `json:"apiVersion"`
	Kind        string `json:"kind"`
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	Homepage    string `json:"homepage,omitempty"`
	License     string `json:"license,omitempty"`
	Runtime     string `json:"runtime"`
	Entrypoint  string `json:"entrypoint"`
}

type ProviderMetadata struct {
	Namespace    string             `json:"namespace"`
	Name         string             `json:"name"`
	Version      string             `json:"version"`
	Description  string             `json:"description,omitempty"`
	Entrypoint   string             `json:"entrypoint"`
	Runtime      string             `json:"runtime"`
	Capabilities []string           `json:"capabilities,omitempty"`
	Platforms    []ProviderPlatform `json:"platforms,omitempty"`
}

type ProviderPlatform struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Binary string `json:"binary"`
}

type ImageManifest struct {
	SchemaVersion int                  `json:"schemaVersion"`
	MediaType     string               `json:"mediaType,omitempty"`
	ArtifactType  string               `json:"artifactType,omitempty"`
	Config        ocispec.Descriptor   `json:"config"`
	Layers        []ocispec.Descriptor `json:"layers"`
	Annotations   map[string]string    `json:"annotations,omitempty"`
}

type PackOptions struct {
	ManifestPath string
	ArtifactRoot string
	OutputDir    string
	Tag          string
}

type PackResult struct {
	ProviderRef string
	Version     string
	Tag         string
	LayoutDir   string
}

type ProviderManifestView struct {
	ConfigDescriptor   ocispec.Descriptor
	ManifestDescriptor ocispec.Descriptor
	MetadataDescriptor ocispec.Descriptor
	AssetsDescriptor   *ocispec.Descriptor
	BinaryDescriptors  map[string]ocispec.Descriptor
}
