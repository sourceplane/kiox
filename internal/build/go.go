package build

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"

	"github.com/sourceplane/tinx/internal/manifest"
)

type GoBuildOptions struct {
	Provider   manifest.Provider
	ModuleRoot string
	MainPkg    string
	OutputRoot string
	Version    string
}

func BuildProvider(opts GoBuildOptions) error {
	mainPkg := opts.MainPkg
	if mainPkg == "" {
		mainPkg = detectMainPackage(opts.ModuleRoot, opts.Provider.Metadata.Name)
	}
	for _, platform := range opts.Provider.Spec.Platforms {
		outputPath := filepath.Join(opts.OutputRoot, filepath.FromSlash(platform.Binary))
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
		cmd := osexec.Command("go", "build", "-trimpath", "-ldflags", ldflags(opts.Version), "-o", outputPath, mainPkg)
		cmd.Dir = opts.ModuleRoot
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+platform.OS, "GOARCH="+platform.Arch)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("go build %s/%s: %w\n%s", platform.OS, platform.Arch, err, string(out))
		}
	}
	return nil
}

func detectMainPackage(moduleRoot, providerName string) string {
	candidate := filepath.Join(moduleRoot, "cmd", providerName)
	if stat, err := os.Stat(candidate); err == nil && stat.IsDir() {
		return "./cmd/" + providerName
	}
	entries, err := os.ReadDir(filepath.Join(moduleRoot, "cmd"))
	if err == nil {
		dirs := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				dirs = append(dirs, entry.Name())
			}
		}
		if len(dirs) == 1 {
			return "./cmd/" + dirs[0]
		}
	}
	return "."
}

func ldflags(version string) string {
	if version == "" {
		version = "dev"
	}
	return "-s -w -X github.com/sourceplane/tinx/pkg/version.Version=" + version
}
