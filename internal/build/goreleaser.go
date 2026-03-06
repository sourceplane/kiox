package build

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
)

type GoReleaserOptions struct {
	ModuleRoot  string
	ConfigPath  string
	DistDir     string
	ExtraArgs   []string
	Environment []string
}

func BuildWithGoReleaser(ctx context.Context, opts GoReleaserOptions) error {
	if _, err := osexec.LookPath("goreleaser"); err != nil {
		return fmt.Errorf("goreleaser not found in PATH: %w", err)
	}
	configPath, err := resolveGoReleaserConfig(opts.ModuleRoot, opts.ConfigPath)
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

func resolveGoReleaserConfig(moduleRoot, explicitPath string) (string, error) {
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
	return "", fmt.Errorf("goreleaser config not found in %s", moduleRoot)
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
