package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const (
	defaultStableVersion = "v1.15.0"
	defaultReleaseBase   = "https://dl.k8s.io/release"
)

var versionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)(?:\.(\d+))?$`)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: setup-kubectl <target>")
		os.Exit(1)
	}
	target := os.Args[1]
	version, err := resolveKubectlVersion(strings.TrimSpace(os.Getenv("KUBECTL_VERSION")))
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve kubectl version: %v\n", err)
		os.Exit(1)
	}
	downloadURL, err := kubectlDownloadURL(version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build kubectl download URL: %v\n", err)
		os.Exit(1)
	}
	response, err := http.Get(downloadURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "download kubectl: %v\n", err)
		os.Exit(1)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "download kubectl: unexpected status %d\n", response.StatusCode)
		os.Exit(1)
	}
	binary, err := io.ReadAll(response.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read kubectl download: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create tool dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(target, binary, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "write tool shim: %v\n", err)
		os.Exit(1)
	}
}

func resolveKubectlVersion(version string) (string, error) {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" || strings.EqualFold(trimmed, "latest") {
		return stableVersion()
	}
	match := versionPattern.FindStringSubmatch(trimmed)
	if len(match) == 0 {
		return "", fmt.Errorf("invalid version format %q", version)
	}
	major, minor, patch := match[1], match[2], match[3]
	if patch != "" {
		if strings.HasPrefix(trimmed, "v") {
			return trimmed, nil
		}
		return "v" + trimmed, nil
	}
	return latestPatchVersion(major, minor)
}

func stableVersion() (string, error) {
	version, err := readVersionFile(releaseBaseURL() + "/stable.txt")
	if err != nil || strings.TrimSpace(version) == "" {
		return defaultStableVersion, nil
	}
	return version, nil
}

func latestPatchVersion(major, minor string) (string, error) {
	version, err := readVersionFile(fmt.Sprintf("%s/stable-%s.%s.txt", releaseBaseURL(), major, minor))
	if err != nil {
		return "", fmt.Errorf("failed to get latest patch version for %s.%s", major, minor)
	}
	if strings.TrimSpace(version) == "" {
		return "", fmt.Errorf("failed to get latest patch version for %s.%s", major, minor)
	}
	return version, nil
}

func readVersionFile(url string) (string, error) {
	response, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", response.StatusCode)
	}
	content, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}

func kubectlDownloadURL(version string) (string, error) {
	goos := runtime.GOOS
	switch goos {
	case "linux", "darwin", "windows":
	default:
		return "", fmt.Errorf("unsupported os %q", goos)
	}
	arch := runtime.GOARCH
	if arch == "amd64" || arch == "arm64" || arch == "arm" {
		return fmt.Sprintf("%s/%s/bin/%s/%s/kubectl%s", releaseBaseURL(), version, goos, arch, executableExtension()), nil
	}
	return "", fmt.Errorf("unsupported arch %q", arch)
}

func releaseBaseURL() string {
	if value := strings.TrimSpace(os.Getenv("KUBECTL_RELEASE_BASE_URL")); value != "" {
		return strings.TrimRight(value, "/")
	}
	return defaultReleaseBase
}

func executableExtension() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
