package oci

import "strings"

const (
	legacyArtifactTypeProvider = "application/vnd.tinx.provider.v1"
	legacyMediaTypeConfig      = "application/vnd.tinx.provider.config.v1+json"
	legacyMediaTypeManifest    = "application/vnd.tinx.provider.manifest.v1+yaml"
	legacyMediaTypeMetadata    = "application/vnd.tinx.provider.metadata.v1+json"
	legacyMediaTypeAssets      = "application/vnd.tinx.provider.assets.v1+tar"
)

var providerBinaryMediaTypePrefixes = []string{
	"application/vnd.kiox.provider.binary.",
	"application/vnd.tinx.provider.binary.",
}

func isProviderConfigMediaType(mediaType string) bool {
	switch strings.TrimSpace(mediaType) {
	case MediaTypeConfig, legacyMediaTypeConfig:
		return true
	default:
		return false
	}
}

func isProviderManifestMediaType(mediaType string) bool {
	switch strings.TrimSpace(mediaType) {
	case MediaTypeManifest, legacyMediaTypeManifest:
		return true
	default:
		return false
	}
}

func isProviderMetadataMediaType(mediaType string) bool {
	switch strings.TrimSpace(mediaType) {
	case MediaTypeMetadata, legacyMediaTypeMetadata:
		return true
	default:
		return false
	}
}

func providerBundleAnnotation(annotations map[string]string) string {
	return firstNonEmpty(annotations["io.kiox.bundle"], annotations["io.tinx.bundle"])
}

func providerPlatformAnnotation(annotations map[string]string) string {
	return firstNonEmpty(annotations["io.kiox.platform"], annotations["io.tinx.platform"])
}

func providerSourceAnnotation(annotations map[string]string) string {
	return firstNonEmpty(
		annotations["io.kiox.source"],
		annotations["io.tinx.source"],
		annotations["org.opencontainers.image.title"],
	)
}

func parsePlatformAnnotation(raw string) (string, string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func parseProviderBinaryMediaType(mediaType string) (string, string, bool) {
	trimmed := strings.TrimSpace(mediaType)
	for _, prefix := range providerBinaryMediaTypePrefixes {
		if !strings.HasPrefix(trimmed, prefix) || !strings.HasSuffix(trimmed, ".v1") {
			continue
		}
		platform := strings.TrimSuffix(strings.TrimPrefix(trimmed, prefix), ".v1")
		parts := strings.SplitN(platform, ".", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return "", "", false
		}
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
	}
	return "", "", false
}
