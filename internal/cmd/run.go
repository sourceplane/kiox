package cmd

import (
	"fmt"
	"os"
	goruntime "runtime"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/oci"
	tinxruntime "github.com/sourceplane/tinx/internal/runtime"
	"github.com/sourceplane/tinx/internal/state"
	"github.com/sourceplane/tinx/pkg/version"
)

func newRunCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <provider-or-alias> <capability> [args...]",
		Short: "Execute an installed provider capability",
		FParseErrWhitelist: cobra.FParseErrWhitelist{
			UnknownFlags: true,
		},
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := ensureHome(root.Home)
			if err != nil {
				return err
			}
			ref := args[0]
			if aliases, err := state.LoadAliases(home); err == nil {
				if aliased, ok := aliases[ref]; ok {
					ref = aliased
				}
			}
			providerMeta, err := resolveInstalledProvider(home, ref)
			if err != nil {
				return err
			}
			if !contains(providerMeta.Capabilities, args[1]) {
				return fmt.Errorf("provider %s does not expose capability %q", ref, args[1])
			}
			binaryPath, err := oci.MaterializeRuntime(home, providerMeta, goruntime.GOOS, goruntime.GOARCH)
			if err != nil {
				return err
			}
			providerHome := providerHomeFromBinary(binaryPath)
			return tinxruntime.Execute(tinxruntime.ExecOptions{
				BinaryPath:   binaryPath,
				Args:         args[1:],
				WorkingDir:   mustGetwd(),
				ProviderHome: providerHome,
				TinxVersion:  version.String(),
				Stdout:       cmd.OutOrStdout(),
				Stderr:       cmd.ErrOrStderr(),
				Stdin:        os.Stdin,
			})
		},
	}
	return cmd
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
