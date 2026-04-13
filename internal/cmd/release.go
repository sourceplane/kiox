package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/build"
	"github.com/sourceplane/tinx/internal/core"
	"github.com/sourceplane/tinx/internal/oci"
	"github.com/sourceplane/tinx/internal/parser"
)

func newReleaseCommand() *cobra.Command {
	var manifestPath string
	var outputDir string
	var distDir string
	var mainPkg string
	var skipBuild bool
	var tag string
	var push string
	var plainHTTP bool
	var delegateGoReleaser bool
	var goreleaserConfig string

	cmd := &cobra.Command{
		Use:   "release",
		Short: "Build, package, and optionally push a provider artifact",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			absManifest, err := filepath.Abs(manifestPath)
			if err != nil {
				return fmt.Errorf("resolve manifest path: %w", err)
			}
			pkg, err := parser.Load(absManifest)
			if err != nil {
				return err
			}
			moduleRoot := filepath.Dir(absManifest)
			absOutput, err := filepath.Abs(outputDir)
			if err != nil {
				return fmt.Errorf("resolve output dir: %w", err)
			}
			absDist, err := filepath.Abs(distDir)
			if err != nil {
				return fmt.Errorf("resolve dist dir: %w", err)
			}
			if !skipBuild {
				if err := os.RemoveAll(absDist); err != nil {
					return fmt.Errorf("reset dist dir: %w", err)
				}
				if delegateGoReleaser {
					if err := build.BuildWithGoReleaser(context.Background(), build.GoReleaserOptions{
						ModuleRoot:   moduleRoot,
						ConfigPath:   goreleaserConfig,
						ManifestPath: absManifest,
						DistDir:      absDist,
					}); err != nil {
						return err
					}
				} else {
					if err := build.BuildProvider(build.GoBuildOptions{Package: pkg, ModuleRoot: moduleRoot, MainPkg: mainPkg, OutputRoot: absDist, Version: pkg.Provider.Metadata.Version}); err != nil {
						return err
					}
				}
			}
			if err := stageBundleSources(pkg, moduleRoot, absDist); err != nil {
				return err
			}
			result, err := oci.Pack(oci.PackOptions{ManifestPath: absManifest, ArtifactRoot: absDist, OutputDir: absOutput, Tag: tag})
			if err != nil {
				return err
			}
			if push != "" {
				if err := oci.PushLayout(cmd.Context(), result.LayoutDir, result.Tag, push, plainHTTP); err != nil {
					return err
				}
				writeLine(cmd.OutOrStdout(), "pushed %s", push)
			}
			writeLine(cmd.OutOrStdout(), "released %s@%s -> %s", result.ProviderRef, result.Tag, result.LayoutDir)
			return nil
		},
	}
	cmd.Flags().StringVar(&manifestPath, "manifest", "tinx.yaml", "path to tinx.yaml")
	cmd.Flags().StringVar(&outputDir, "output", "oci", "output OCI image layout directory")
	cmd.Flags().StringVar(&distDir, "dist", "dist", "build output directory used before packaging")
	cmd.Flags().StringVar(&mainPkg, "main", "", "Go main package to build")
	cmd.Flags().BoolVar(&skipBuild, "skip-build", false, "skip building and package the existing dist directory")
	cmd.Flags().StringVar(&tag, "tag", "", "tag to write into the OCI layout index")
	cmd.Flags().StringVar(&push, "push", "", "optional OCI reference to push after packaging")
	cmd.Flags().BoolVar(&plainHTTP, "plain-http", false, "use plain HTTP for registry push/install flows")
	cmd.Flags().BoolVar(&delegateGoReleaser, "delegate-goreleaser", false, "delegate provider builds to goreleaser")
	cmd.Flags().BoolVar(&delegateGoReleaser, "delegate-gorelaser", false, "delegate provider builds to goreleaser")
	cmd.Flags().StringVar(&goreleaserConfig, "goreleaser-config", "", "path to .goreleaser.yml/.yaml when goreleaser delegation is enabled")
	return cmd
}

func stageBundleSources(pkg core.Package, moduleRoot, distRoot string) error {
	for _, bundle := range pkg.Bundles {
		for _, layer := range bundle.Spec.Layers {
			if !strings.HasSuffix(strings.TrimSpace(layer.MediaType), "+tar") {
				continue
			}
			source := filepath.FromSlash(strings.TrimSpace(layer.Source))
			if source == "" {
				continue
			}
			srcPath := filepath.Join(moduleRoot, source)
			dstPath := filepath.Join(distRoot, source)
			if _, err := os.Stat(srcPath); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return fmt.Errorf("stat bundle source %s: %w", srcPath, err)
			}
			if err := copyReleaseTree(srcPath, dstPath); err != nil {
				return fmt.Errorf("stage bundle source %s: %w", layer.Source, err)
			}
		}
	}
	return nil
}

func copyReleaseTree(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
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
