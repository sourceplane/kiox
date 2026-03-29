package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/gha"
	"github.com/sourceplane/tinx/internal/oci"
	"github.com/sourceplane/tinx/internal/state"
	"github.com/sourceplane/tinx/pkg/version"
)

func newInstallCommand(root *rootOptions) *cobra.Command {
	var source string
	var tag string
	var plainHTTP bool
	var inputValues []string

	cmd := &cobra.Command{
		Use:   "install [alias] <ref>",
		Short: "Install provider metadata from an OCI layout, registry reference, or GitHub Action",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := ensureHome(root.Home)
			if err != nil {
				return err
			}
			installInputs, err := parseInstallInputs(inputValues)
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
				var installed state.ProviderMetadata
				if gha.IsReference(installTarget) {
					if len(installInputs) > 0 && alias == "" {
						return fmt.Errorf("GitHub Action install-time inputs require an alias so the configured provider can be stored")
					}
					if alias != "" {
						installed, err = gha.InstallAlias(cmd.Context(), home, installTarget, gha.InstallOptions{
							Alias:       alias,
							Inputs:      installInputs,
							WorkingDir:  mustGetwd(),
							TinxVersion: version.String(),
							Stdout:      cmd.OutOrStdout(),
							Stderr:      cmd.ErrOrStderr(),
							Stdin:       os.Stdin,
						}, cmd.ErrOrStderr())
					} else {
						installed, err = gha.Install(cmd.Context(), home, installTarget, cmd.ErrOrStderr())
					}
				} else {
					if len(installInputs) > 0 {
						return fmt.Errorf("--input is only supported for GitHub Action installs")
					}
					installed, err = oci.InstallRemote(cmd.Context(), home, installTarget, alias, plainHTTP, cmd.ErrOrStderr())
				}
				if err != nil {
					return err
				}
				writeLine(cmd.OutOrStdout(), "installed %s/%s@%s", installed.Namespace, installed.Name, installed.Version)
				return nil
			}
			if len(installInputs) > 0 {
				return fmt.Errorf("--input is only supported for GitHub Action installs")
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
			installed, err := oci.InstallMetadata(absSource, tag, home, alias, cmd.ErrOrStderr())
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
	cmd.Flags().StringArrayVar(&inputValues, "input", nil, "configure a GitHub Action input at install time (name=value)")
	return cmd
}

func parseInstallInputs(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	inputs := make(map[string]string, len(values))
	for _, raw := range values {
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("GitHub Action inputs must use name=value syntax, got %q", raw)
		}
		inputs[strings.TrimSpace(parts[0])] = parts[1]
	}
	return inputs, nil
}
