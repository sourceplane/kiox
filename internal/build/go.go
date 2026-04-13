package build

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sourceplane/tinx/internal/core"
)

type GoBuildOptions struct {
	Package    core.Package
	ModuleRoot string
	MainPkg    string
	OutputRoot string
	Version    string
}

func BuildProvider(opts GoBuildOptions) error {
	targets := bundleBuildTargets(opts.Package)
	for _, target := range targets {
		mainPkg := opts.MainPkg
		if mainPkg == "" {
			mainPkg = detectMainPackage(opts.ModuleRoot, target.BinaryName)
		}
		outputPath := filepath.Join(opts.OutputRoot, filepath.FromSlash(target.Source))
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
		cmd := osexec.Command("go", "build", "-trimpath", "-ldflags", ldflags(opts.Version), "-o", outputPath, mainPkg)
		cmd.Dir = opts.ModuleRoot
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+target.OS, "GOARCH="+target.Arch)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("go build %s/%s for %s: %w\n%s", target.OS, target.Arch, target.BinaryName, err, string(out))
		}
	}
	return nil
}

type bundleBuildTarget struct {
	OS         string
	Arch       string
	Source     string
	BinaryName string
}

func bundleBuildTargets(pkg core.Package) []bundleBuildTarget {
	seen := map[string]struct{}{}
	targets := make([]bundleBuildTarget, 0)
	bundleNames := make([]string, 0, len(pkg.Bundles))
	for name := range pkg.Bundles {
		bundleNames = append(bundleNames, name)
	}
	sort.Strings(bundleNames)
	for _, bundleName := range bundleNames {
		bundle := pkg.Bundles[bundleName]
		for _, layer := range bundle.Spec.Layers {
			if strings.HasSuffix(strings.TrimSpace(layer.MediaType), "+tar") {
				continue
			}
			if layer.Platform.OS == "any" || layer.Platform.Arch == "any" {
				continue
			}
			source := strings.TrimSpace(layer.Source)
			if source == "" {
				continue
			}
			key := layer.Platform.OS + "/" + layer.Platform.Arch + ":" + source
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			targets = append(targets, bundleBuildTarget{OS: layer.Platform.OS, Arch: layer.Platform.Arch, Source: source, BinaryName: filepath.Base(source)})
		}
	}
	return targets
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
