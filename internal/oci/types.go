package oci

import (
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	ArtifactTypePackage  = "application/vnd.tinx.package.v2"
	ArtifactTypeProvider = ArtifactTypePackage

	LegacyArtifactTypeProvider = "application/vnd.tinx.provider.v1"

	MediaTypeConfigPackage   = "application/vnd.tinx.package.config.v2+json"
	MediaTypeManifestPackage = "application/vnd.tinx.package.manifest.v2+yaml"
	MediaTypeMetadataPackage = "application/vnd.tinx.package.metadata.v2+json"
	MediaTypeAssetsPackage   = "application/vnd.tinx.package.assets.v2+tar"

	LegacyMediaTypeConfig   = "application/vnd.tinx.provider.config.v1+json"
	LegacyMediaTypeManifest = "application/vnd.tinx.provider.manifest.v1+yaml"
	LegacyMediaTypeMetadata = "application/vnd.tinx.provider.metadata.v1+json"
	LegacyMediaTypeAssets   = "application/vnd.tinx.provider.assets.v1+tar"

	MediaTypeConfig   = MediaTypeConfigPackage
	MediaTypeManifest = MediaTypeManifestPackage
	MediaTypeMetadata = MediaTypeMetadataPackage
	MediaTypeAssets   = MediaTypeAssetsPackage
)

type PackageConfig struct {
	APIVersion  string `json:"apiVersion"`
	Kind        string `json:"kind"`
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	Homepage    string `json:"homepage,omitempty"`
	License     string `json:"license,omitempty"`
	Runtime     string `json:"runtime"`
	Entrypoint  string `json:"entrypoint,omitempty"`
	Image       string `json:"image,omitempty"`
	Module      string `json:"module,omitempty"`
	Interpreter string `json:"interpreter,omitempty"`
}

type ProviderConfig = PackageConfig

type PackageMetadata struct {
	Namespace              string             `json:"namespace"`
	Name                   string             `json:"name"`
	Version                string             `json:"version"`
	Description            string             `json:"description,omitempty"`
	Entrypoint             string             `json:"entrypoint,omitempty"`
	Runtime                string             `json:"runtime"`
	Image                  string             `json:"image,omitempty"`
	Module                 string             `json:"module,omitempty"`
	Interpreter            string             `json:"interpreter,omitempty"`
	Capabilities           []string           `json:"capabilities,omitempty"`
	CapabilityDescriptions map[string]string  `json:"capabilityDescriptions,omitempty"`
	Platforms              []ProviderPlatform `json:"platforms,omitempty"`
	Dependencies           map[string]string  `json:"dependencies,omitempty"`
}

type ProviderMetadata = PackageMetadata

type ProviderPlatform struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Binary string `json:"binary,omitempty"`
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
	PackageRef  string
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
