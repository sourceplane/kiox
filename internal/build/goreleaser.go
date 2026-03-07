package build

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/sourceplane/tinx/internal/manifest"
)

type GoReleaserOptions struct {
	ModuleRoot   string
	ConfigPath   string
	ManifestPath string
	DistDir      string
	ExtraArgs    []string
	Environment  []string
}

func BuildWithGoReleaser(ctx context.Context, opts GoReleaserOptions) error {
	if _, err := osexec.LookPath("goreleaser"); err != nil {
		return fmt.Errorf("goreleaser not found in PATH: %w", err)
	}
	configPath, err := resolveGoReleaserConfig(opts.ModuleRoot, opts.ConfigPath, opts.ManifestPath)
	if err != nil {
		return err
	}
	args := []string{"build", "--clean", "--skip=validate", "--config", configPath}
	args = append(args, opts.ExtraArgs...)
	cmd := osexec.CommandContext(ctx, "goreleaser", args...)
	cmd.Dir = opts.ModuleRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), opts.Environment...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("goreleaser build: %w", err)
	}
	defaultDist := filepath.Join(opts.ModuleRoot, "dist")
	if opts.DistDir != "" && filepath.Clean(opts.DistDir) != filepath.Clean(defaultDist) {
		if err := os.RemoveAll(opts.DistDir); err != nil {
			return fmt.Errorf("reset delegated dist dir: %w", err)
		}
		if err := copyBuildOutput(defaultDist, opts.DistDir); err != nil {
			return fmt.Errorf("copy goreleaser dist output: %w", err)
		}
	}
	return nil
}

func resolveGoReleaserConfig(moduleRoot, explicitPath, manifestPath string) (string, error) {
	if explicitPath != "" {
		if !filepath.IsAbs(explicitPath) {
			explicitPath = filepath.Join(moduleRoot, explicitPath)
		}
		if _, err := os.Stat(explicitPath); err != nil {
			return "", fmt.Errorf("open goreleaser config: %w", err)
		}
		return explicitPath, nil
	}
	for _, candidate := range []string{".goreleaser.yml", ".goreleaser.yaml"} {
		path := filepath.Join(moduleRoot, candidate)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	generatedPath, err := generateGoReleaserConfigFromManifest(moduleRoot, manifestPath)
	if err != nil {
		return "", fmt.Errorf("goreleaser config not found in %s and fallback generation failed: %w", moduleRoot, err)
	}
	return generatedPath, nil
}

type generatedGoReleaserConfig struct {
	ProjectName string                     `yaml:"project_name"`
	Dist        string                     `yaml:"dist"`
	Builds      []generatedGoReleaserBuild `yaml:"builds"`
	Archives    []generatedArchive         `yaml:"archives"`
	Checksum    generatedToggle            `yaml:"checksum"`
	Release     generatedToggle            `yaml:"release"`
	Changelog   generatedToggle            `yaml:"changelog"`
}

type generatedGoReleaserBuild struct {
	ID      string                       `yaml:"id"`
	Main    string                       `yaml:"main"`
	Binary  string                       `yaml:"binary"`
	Env     []string                     `yaml:"env"`
	Goos    []string                     `yaml:"goos"`
	Goarch  []string                     `yaml:"goarch"`
	Ignore  []generatedPlatformCombo     `yaml:"ignore,omitempty"`
	Ldflags []string                     `yaml:"ldflags,omitempty"`
	Hooks   generatedGoReleaserBuildHook `yaml:"hooks,omitempty"`
}

type generatedGoReleaserBuildHook struct {
	Post []string `yaml:"post,omitempty"`
}

type generatedPlatformCombo struct {
	Goos   string `yaml:"goos"`
	Goarch string `yaml:"goarch"`
}

type generatedArchive struct {
	Formats      []string `yaml:"formats"`
	NameTemplate string   `yaml:"name_template,omitempty"`
}

type generatedToggle struct {
	Disable bool `yaml:"disable"`
}

func generateGoReleaserConfigFromManifest(moduleRoot, manifestPath string) (string, error) {
	resolvedManifest := manifestPath
	if resolvedManifest == "" {
		resolvedManifest = filepath.Join(moduleRoot, "tinx.yaml")
	} else if !filepath.IsAbs(resolvedManifest) {
		resolvedManifest = filepath.Join(moduleRoot, resolvedManifest)
	}

	provider, err := manifest.Load(resolvedManifest)
	if err != nil {
		return "", err
	}

	mainPkg := detectMainPackage(moduleRoot, provider.Metadata.Name)
	platformPairs := make(map[string]map[string]struct{})
	goosValues := make(map[string]struct{})
	goarchValues := make(map[string]struct{})
	for _, platform := range provider.Spec.Platforms {
		goosValues[platform.OS] = struct{}{}
		goarchValues[platform.Arch] = struct{}{}
		arches, ok := platformPairs[platform.OS]
		if !ok {
			arches = map[string]struct{}{}
			platformPairs[platform.OS] = arches
		}
		arches[platform.Arch] = struct{}{}
	}

	goos := sortedKeys(goosValues)
	goarch := sortedKeys(goarchValues)
	ignore := make([]generatedPlatformCombo, 0)
	for _, osValue := range goos {
		for _, archValue := range goarch {
			arches := platformPairs[osValue]
			if _, ok := arches[archValue]; ok {
				continue
			}
			ignore = append(ignore, generatedPlatformCombo{Goos: osValue, Goarch: archValue})
		}
	}

	cfg := generatedGoReleaserConfig{
		ProjectName: provider.Metadata.Name,
		Dist:        "dist",
		Builds: []generatedGoReleaserBuild{{
			ID:      provider.Metadata.Name,
			Main:    mainPkg,
			Binary:  provider.Spec.Entrypoint,
			Env:     []string{"CGO_ENABLED=0"},
			Goos:    goos,
			Goarch:  goarch,
			Ignore:  ignore,
			Ldflags: []string{ldflags(provider.Metadata.Version)},
			Hooks: generatedGoReleaserBuildHook{Post: []string{
				"mkdir -p dist/bin/{{ .Os }}/{{ .Arch }}",
				fmt.Sprintf("cp {{ .Path }} dist/bin/{{ .Os }}/{{ .Arch }}/%s", provider.Spec.Entrypoint),
			}},
		}},
		Archives:  []generatedArchive{{Formats: []string{"binary"}, NameTemplate: "{{ .Binary }}-{{ .Os }}-{{ .Arch }}"}},
		Checksum:  generatedToggle{Disable: true},
		Release:   generatedToggle{Disable: true},
		Changelog: generatedToggle{Disable: true},
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("encode generated goreleaser config: %w", err)
	}

	path := filepath.Join(moduleRoot, ".goreleaser.tinx.generated.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write generated goreleaser config: %w", err)
	}
	return path, nil
}

func sortedKeys(values map[string]struct{}) []string {
	items := make([]string, 0, len(values))
	for value := range values {
		items = append(items, value)
	}
	sort.Strings(items)
	return items
}

func copyBuildOutput(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dstDir, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
