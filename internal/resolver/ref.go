package resolver

import "strings"

const DefaultRegistry = "ghcr.io"

func ResolveProviderSource(ref string) string {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "://") {
		return trimmed
	}
	if !isShortProviderRef(trimmed) {
		return trimmed
	}
	return DefaultRegistry + "/" + trimmed
}

func isShortProviderRef(ref string) bool {
	if strings.HasPrefix(ref, "/") || strings.Count(ref, "/") != 1 {
		return false
	}
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	if parts[0] == "localhost" {
		return false
	}
	return !strings.ContainsAny(parts[0], ".:")
}
