package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/oci"
)

func newInstallCommand(root *rootOptions) *cobra.Command {
	var source string
	var ref string
	var tag string
	var alias string
	var plainHTTP bool

	cmd := &cobra.Command{
		Use:   "install <namespace/name>",
		Short: "Install provider metadata from an OCI layout or registry reference",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := ensureHome(root.Home)
			if err != nil {
				return err
			}
			switch {
			case source != "" && ref != "":
				return fmt.Errorf("--source and --ref are mutually exclusive")
			case source == "" && ref == "":
				return fmt.Errorf("either --source or --ref is required")
			}
			expectedRef := args[0]
			if ref != "" {
				installed, err := oci.InstallRemote(cmd.Context(), home, ref, alias, plainHTTP)
				if err != nil {
					return err
				}
				if installed.Namespace+"/"+installed.Name != expectedRef {
					return fmt.Errorf("installed provider %s/%s does not match requested ref %s", installed.Namespace, installed.Name, expectedRef)
				}
				writeLine(cmd.OutOrStdout(), "installed %s/%s@%s", installed.Namespace, installed.Name, installed.Version)
				return nil
			}
			absSource, err := filepath.Abs(source)
			if err != nil {
				return fmt.Errorf("resolve source path: %w", err)
			}
			if _, err := os.Stat(absSource); err != nil {
				return fmt.Errorf("open source layout: %w", err)
			}
			installed, err := oci.InstallMetadata(absSource, tag, home, alias)
			if err != nil {
				return err
			}
			if installed.Namespace+"/"+installed.Name != expectedRef {
				return fmt.Errorf("installed provider %s/%s does not match requested ref %s", installed.Namespace, installed.Name, expectedRef)
			}
			writeLine(cmd.OutOrStdout(), "installed %s/%s@%s", installed.Namespace, installed.Name, installed.Version)
			return nil
		},
	}
	cmd.Flags().StringVar(&source, "source", "", "path to a local OCI image layout")
	cmd.Flags().StringVar(&ref, "ref", "", "OCI registry reference to pull and install")
	cmd.Flags().StringVar(&tag, "tag", "", "OCI tag inside the local layout")
	cmd.Flags().StringVar(&alias, "alias", "", "optional alias to register for shorthand execution")
	cmd.Flags().BoolVar(&plainHTTP, "plain-http", false, "use plain HTTP for registry pull/install")
	return cmd
}
