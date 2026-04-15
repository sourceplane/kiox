package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/oci"
)

func newPackCommand() *cobra.Command {
	var manifestPath string
	var artifactRoot string
	var outputDir string
	var tag string

	cmd := &cobra.Command{
		Use:   "pack",
		Short: "Package a provider into an OCI image layout",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			absManifest, err := resolveProviderManifestPath(manifestPath)
			if err != nil {
				return err
			}
			root := artifactRoot
			if root == "" {
				root = filepath.Dir(absManifest)
			}
			absRoot, err := filepath.Abs(root)
			if err != nil {
				return fmt.Errorf("resolve artifact root: %w", err)
			}
			absOutput, err := filepath.Abs(outputDir)
			if err != nil {
				return fmt.Errorf("resolve output dir: %w", err)
			}
			result, err := oci.Pack(oci.PackOptions{ManifestPath: absManifest, ArtifactRoot: absRoot, OutputDir: absOutput, Tag: tag})
			if err != nil {
				return err
			}
			writeLine(cmd.OutOrStdout(), "packed %s@%s -> %s", result.ProviderRef, result.Tag, result.LayoutDir)
			return nil
		},
	}
	cmd.Flags().StringVar(&manifestPath, "manifest", preferredProviderManifestName, "path to provider.yaml (legacy tinx.yaml also supported)")
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "root containing built binaries and assets")
	cmd.Flags().StringVar(&outputDir, "output", "oci", "output OCI image layout directory")
	cmd.Flags().StringVar(&tag, "tag", "", "tag to write into the OCI layout index")
	return cmd
}
