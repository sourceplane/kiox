package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/build"
	"github.com/sourceplane/tinx/internal/manifest"
	"github.com/sourceplane/tinx/internal/oci"
)

func newReleaseCommand() *cobra.Command {
	var manifestPath string
	var outputDir string
	var distDir string
	var mainPkg string
	var skipBuild bool
	var tag string
	var pushRef string
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
			provider, err := manifest.Load(absManifest)
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
						ModuleRoot: moduleRoot,
						ConfigPath: goreleaserConfig,
						DistDir:    absDist,
					}); err != nil {
						return err
					}
				} else {
					if err := build.BuildProvider(build.GoBuildOptions{Provider: provider, ModuleRoot: moduleRoot, MainPkg: mainPkg, OutputRoot: absDist, Version: provider.Metadata.Version}); err != nil {
						return err
					}
				}
			}
			if assetsRoot := provider.AssetsRoot(); assetsRoot != "" {
				srcAssets := filepath.Join(moduleRoot, filepath.FromSlash(assetsRoot))
				dstAssets := filepath.Join(absDist, filepath.FromSlash(assetsRoot))
				if _, err := os.Stat(srcAssets); err == nil {
					if err := copyReleaseTree(srcAssets, dstAssets); err != nil {
						return fmt.Errorf("stage assets for packaging: %w", err)
					}
				}
			}
			result, err := oci.Pack(oci.PackOptions{ManifestPath: absManifest, ArtifactRoot: absDist, OutputDir: absOutput, Tag: tag})
			if err != nil {
				return err
			}
			if pushRef != "" {
				if err := oci.PushLayout(cmd.Context(), result.LayoutDir, result.Tag, pushRef, plainHTTP); err != nil {
					return err
				}
				writeLine(cmd.OutOrStdout(), "pushed %s", pushRef)
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
	cmd.Flags().StringVar(&pushRef, "push-ref", "", "optional OCI reference to push after packaging")
	cmd.Flags().BoolVar(&plainHTTP, "plain-http", false, "use plain HTTP for registry push/install flows")
	cmd.Flags().BoolVar(&delegateGoReleaser, "delegate-goreleaser", false, "delegate provider builds to goreleaser")
	cmd.Flags().BoolVar(&delegateGoReleaser, "delegate-gorelaser", false, "delegate provider builds to goreleaser")
	cmd.Flags().StringVar(&goreleaserConfig, "goreleaser-config", "", "path to .goreleaser.yml/.yaml when goreleaser delegation is enabled")
	return cmd
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
