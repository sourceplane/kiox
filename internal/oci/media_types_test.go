package oci

import (
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestLegacyMediaTypesAreRecognizedAsMetadata(t *testing.T) {
	tests := []string{
		legacyMediaTypeConfig,
		legacyMediaTypeManifest,
		legacyMediaTypeMetadata,
	}
	for _, mediaType := range tests {
		descriptor := ocispec.Descriptor{MediaType: mediaType}
		if !isMetadataDescriptor(descriptor) {
			t.Fatalf("isMetadataDescriptor(%q) = false, want true", mediaType)
		}
	}
}

func TestDescriptorPlatformSupportsLegacyAnnotationsAndMediaTypes(t *testing.T) {
	t.Run("legacy annotation", func(t *testing.T) {
		descriptor := ocispec.Descriptor{Annotations: map[string]string{"io.tinx.platform": "darwin/arm64"}}
		goos, goarch, ok := descriptorPlatform(descriptor)
		if !ok || goos != "darwin" || goarch != "arm64" {
			t.Fatalf("descriptorPlatform() = (%q, %q, %v), want (darwin, arm64, true)", goos, goarch, ok)
		}
	})

	t.Run("legacy media type", func(t *testing.T) {
		descriptor := ocispec.Descriptor{MediaType: "application/vnd.tinx.provider.binary.linux.amd64.v1"}
		goos, goarch, ok := descriptorPlatform(descriptor)
		if !ok || goos != "linux" || goarch != "amd64" {
			t.Fatalf("descriptorPlatform() = (%q, %q, %v), want (linux, amd64, true)", goos, goarch, ok)
		}
	})
}

func TestDescriptorFromLayerSupportsLegacyAnnotations(t *testing.T) {
	descriptor := ocispec.Descriptor{
		MediaType: "application/vnd.tinx.provider.binary.linux.amd64.v1",
		Annotations: map[string]string{
			"io.tinx.bundle":                 "gluon",
			"io.tinx.platform":               "linux/amd64",
			"io.tinx.source":                 "bin/linux/amd64/entrypoint",
			"org.opencontainers.image.title": "bin/linux/amd64/entrypoint",
		},
	}
	view := descriptorFromLayer(descriptor)
	if view.Bundle != "gluon" {
		t.Fatalf("descriptorFromLayer().Bundle = %q, want gluon", view.Bundle)
	}
	if view.Platform.OS != "linux" || view.Platform.Arch != "amd64" {
		t.Fatalf("descriptorFromLayer().Platform = %#v, want linux/amd64", view.Platform)
	}
	if view.Source != "bin/linux/amd64/entrypoint" {
		t.Fatalf("descriptorFromLayer().Source = %q, want bin/linux/amd64/entrypoint", view.Source)
	}
}
