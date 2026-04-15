package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/pkg/version"
)

type rootOptions struct {
	Home      string
	Workspace string
}

func Execute(ctx context.Context) error {
	return executeCLI(ctx, os.Args[1:], os.Stdout, os.Stderr)
}

func NewRootCommand() *cobra.Command {
	return newRootCommand(&rootOptions{})
}

func newRootCommand(opts *rootOptions) *cobra.Command {
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
			return cmd.Help()
		},
	}
	cmd.PersistentFlags().StringVar(&opts.Home, "tinx-home", opts.Home, "override the tinx home directory")
	cmd.PersistentFlags().StringVarP(&opts.Workspace, "workspace", "w", opts.Workspace, "select the workspace for workspace-shell commands")
	cmd.SetVersionTemplate("tinx {{.Version}}\n")
	cmd.AddCommand(newInitCommand(opts))
	cmd.AddCommand(newStatusCommand(opts))
	cmd.AddCommand(newWorkspaceCommand(opts))
	cmd.AddCommand(newProviderCommand(opts))
	cmd.AddCommand(newAddCommand(opts))
	cmd.AddCommand(newRemoveCommand(opts))
	cmd.AddCommand(newSyncCommand(opts))
	cmd.AddCommand(newUpdateCommand(opts))
	cmd.AddCommand(newUseCommand(opts))
	cmd.AddCommand(newInstallCommand(opts))
	cmd.AddCommand(newListCommand(opts))
	cmd.AddCommand(newShellCommand(opts))
	cmd.AddCommand(newShimCommand(opts))
	cmd.AddCommand(newExecCommand(opts))
	cmd.AddCommand(newRunCommand(opts))
	cmd.AddCommand(newPackCommand())
	cmd.AddCommand(newReleaseCommand())
	cmd.AddCommand(newVersionCommand())
	return cmd
}

func unsupportedProviderSourceError(ref, detail string) error {
	return fmt.Errorf("unsupported provider source %q: %s", ref, detail)
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

func extractRootArgs(args []string) (rootOptions, []string, error) {
	opts := rootOptions{}
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			remaining = append(remaining, args[i:]...)
			break
		}
		switch {
		case strings.HasPrefix(arg, "--tinx-home="):
			opts.Home = strings.TrimPrefix(arg, "--tinx-home=")
		case arg == "--tinx-home":
			if i+1 >= len(args) {
				return rootOptions{}, nil, fmt.Errorf("missing value for --tinx-home")
			}
			opts.Home = args[i+1]
			i++
		case strings.HasPrefix(arg, "--workspace="):
			opts.Workspace = strings.TrimPrefix(arg, "--workspace=")
		case strings.HasPrefix(arg, "-w="):
			opts.Workspace = strings.TrimPrefix(arg, "-w=")
		case arg == "--workspace" || arg == "-w":
			if i+1 >= len(args) {
				return rootOptions{}, nil, fmt.Errorf("missing value for --workspace")
			}
			opts.Workspace = args[i+1]
			i++
		default:
			remaining = append(remaining, arg)
		}
	}
	return opts, remaining, nil
}
