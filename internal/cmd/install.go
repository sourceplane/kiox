package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/oci"
	"github.com/sourceplane/tinx/internal/state"
)

func newInstallCommand(root *rootOptions) *cobra.Command {
	var source string
	var tag string
	var plainHTTP bool

	cmd := &cobra.Command{
		Use:   "install [alias] <ref>",
		Short: "Install provider metadata from an OCI layout or registry reference",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := ensureHome(root.Home)
			if err != nil {
				return err
			}

			alias := ""
			installTarget := args[0]
			if len(args) == 2 {
				alias = args[0]
				installTarget = args[1]
			}

			if source == "" {
				installed, err := oci.InstallRemote(cmd.Context(), home, installTarget, alias, plainHTTP)
				if err != nil {
					return err
				}
				if alias != "" {
					aliases, err := state.LoadAliases(home)
					if err != nil {
						return err
					}
					aliases[alias] = installTarget
					if err := state.SaveAliases(home, aliases); err != nil {
						return err
					}
				}
				writeLine(cmd.OutOrStdout(), "installed %s/%s@%s", installed.Namespace, installed.Name, installed.Version)
				return nil
			}

			if _, _, err := splitProviderRef(installTarget); err != nil {
				return fmt.Errorf("when using --source, ref must be <namespace>/<name>")
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
			if installed.Namespace+"/"+installed.Name != installTarget {
				return fmt.Errorf("installed provider %s/%s does not match requested ref %s", installed.Namespace, installed.Name, installTarget)
			}
			writeLine(cmd.OutOrStdout(), "installed %s/%s@%s", installed.Namespace, installed.Name, installed.Version)
			return nil
		},
	}
	cmd.Flags().StringVar(&source, "source", "", "path to a local OCI image layout")
	cmd.Flags().StringVar(&tag, "tag", "", "OCI tag inside the local layout")
	cmd.Flags().BoolVar(&plainHTTP, "plain-http", false, "use plain HTTP for registry pull/install")
	return cmd
}
