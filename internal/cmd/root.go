package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/oci"
	tinxruntime "github.com/sourceplane/tinx/internal/runtime"
	"github.com/sourceplane/tinx/internal/state"
	"github.com/sourceplane/tinx/pkg/version"
)

type rootOptions struct {
	Home string
}

func Execute(ctx context.Context) error {
	root := NewRootCommand()
	if err := root.ExecuteContext(ctx); err != nil {
		if !strings.Contains(err.Error(), "unknown command") {
			return err
		}
		home, args, parseErr := extractHomeArg(os.Args[1:])
		if parseErr != nil {
			return err
		}
		fallback := &cobra.Command{}
		fallback.SetOut(os.Stdout)
		fallback.SetErr(os.Stderr)
		if aliasErr := runAlias(fallback, &rootOptions{Home: home}, args); aliasErr == nil {
			return nil
		}
		return err
	}
	return nil
}

func NewRootCommand() *cobra.Command {
	opts := &rootOptions{}
	cmd := &cobra.Command{
		Use:           "tinx",
		Short:         "OCI-native provider runtime and packager",
		SilenceUsage:  true,
		SilenceErrors: true,
		FParseErrWhitelist: cobra.FParseErrWhitelist{
			UnknownFlags: true,
		},
		Version: version.String(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return runAlias(cmd, opts, args)
		},
	}
	cmd.PersistentFlags().StringVar(&opts.Home, "tinx-home", "", "override the tinx home directory")
	cmd.SetVersionTemplate("tinx {{.Version}}\n")
	cmd.AddCommand(newInstallCommand(opts))
	cmd.AddCommand(newRunCommand(opts))
	cmd.AddCommand(newPackCommand())
	cmd.AddCommand(newReleaseCommand())
	cmd.AddCommand(newVersionCommand())
	return cmd
}

func runAlias(cmd *cobra.Command, opts *rootOptions, args []string) error {
	home, err := ensureHome(opts.Home)
	if err != nil {
		return err
	}
	aliases, err := state.LoadAliases(home)
	if err != nil {
		return err
	}
	ref, ok := aliases[args[0]]
	if !ok {
		return fmt.Errorf("unknown command or alias %q", args[0])
	}
	if len(args) < 2 {
		return fmt.Errorf("capability is required for alias %q", args[0])
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
}

func providerHomeFromBinary(binaryPath string) string {
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(binaryPath))))
}

func ensureHome(override string) (string, error) {
	home, err := state.ResolveHome(override)
	if err != nil {
		return "", err
	}
	if err := state.EnsureHome(home); err != nil {
		return "", err
	}
	return home, nil
}

func resolveInstalledProvider(home, ref string) (state.ProviderMetadata, error) {
	namespace, name, err := splitProviderRef(ref)
	if err != nil {
		return state.ProviderMetadata{}, err
	}
	return state.LoadProviderMetadata(home, namespace, name)
}

func splitProviderRef(ref string) (string, string, error) {
	for i := 0; i < len(ref); i++ {
		if ref[i] == '/' {
			return ref[:i], ref[i+1:], nil
		}
	}
	return "", "", fmt.Errorf("provider ref must be <namespace>/<name>: %q", ref)
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func writeLine(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, format+"\n", args...)
}

func extractHomeArg(args []string) (string, []string, error) {
	remaining := make([]string, 0, len(args))
	home := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case strings.HasPrefix(arg, "--tinx-home="):
			home = strings.TrimPrefix(arg, "--tinx-home=")
		case arg == "--tinx-home":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("missing value for --tinx-home")
			}
			home = args[i+1]
			i++
		default:
			remaining = append(remaining, arg)
		}
	}
	return home, remaining, nil
}
