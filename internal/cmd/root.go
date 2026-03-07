package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
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
		} else {
			return aliasErr
		}
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
	providerMeta, err := resolveProviderForRun(home, ref, false, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	if len(args) == 1 || isHelpToken(args[1]) {
		writeProviderHelp(cmd.OutOrStdout(), args[0], ref, providerMeta)
		return nil
	}
	if len(args) >= 3 && isHelpToken(args[2]) {
		writeCapabilityHelp(cmd.OutOrStdout(), args[0], args[1], providerMeta)
		return nil
	}
	return executeProviderCapability(cmd, home, ref, providerMeta, args[1:])
}

func executeProviderCapability(cmd *cobra.Command, home, providerRef string, providerMeta state.ProviderMetadata, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("capability is required")
	}
	if !contains(providerMeta.Capabilities, args[0]) {
		return fmt.Errorf("provider %s does not expose capability %q", providerRef, args[0])
	}
	binaryPath, err := oci.MaterializeRuntime(home, providerMeta, goruntime.GOOS, goruntime.GOARCH, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	providerHome := providerHomeFromBinary(binaryPath)
	return tinxruntime.Execute(tinxruntime.ExecOptions{
		BinaryPath:   binaryPath,
		Args:         args,
		WorkingDir:   mustGetwd(),
		ProviderHome: providerHome,
		TinxVersion:  version.String(),
		Stdout:       cmd.OutOrStdout(),
		Stderr:       cmd.ErrOrStderr(),
		Stdin:        os.Stdin,
	})
}

func resolveProviderForRun(home, ref string, plainHTTP bool, progressOut io.Writer) (state.ProviderMetadata, error) {
	providerMeta, err := resolveInstalledProvider(home, ref)
	if err == nil {
		return providerMeta, nil
	}
	if !isOCIReference(ref) {
		return state.ProviderMetadata{}, err
	}
	if installedRef, ok := providerRefFromOCIReference(ref); ok {
		if installedMeta, installedErr := resolveInstalledProvider(home, installedRef); installedErr == nil {
			return installedMeta, nil
		}
	}
	installed, installErr := oci.InstallRemoteFull(context.Background(), home, ref, "", plainHTTP, progressOut)
	if installErr != nil {
		return state.ProviderMetadata{}, err
	}
	return installed, nil
}

func writeProviderHelp(w io.Writer, alias, providerRef string, providerMeta state.ProviderMetadata) {
	writeLine(w, "Provider: %s/%s %s", providerMeta.Namespace, providerMeta.Name, providerMeta.Version)
	if providerMeta.Description != "" {
		writeLine(w, "")
		writeLine(w, "%s", providerMeta.Description)
	}
	writeLine(w, "")
	writeLine(w, "Capabilities:")
	capabilities := append([]string(nil), providerMeta.Capabilities...)
	sort.Strings(capabilities)
	for _, capability := range capabilities {
		description := ""
		if providerMeta.CapabilityDescriptions != nil {
			description = providerMeta.CapabilityDescriptions[capability]
		}
		if description == "" {
			writeLine(w, "  %s", capability)
			continue
		}
		writeLine(w, "  %-12s %s", capability, description)
	}
	writeLine(w, "")
	writeLine(w, "Usage:")
	writeLine(w, "  tinx %s <capability> [flags]", alias)
	writeLine(w, "  tinx run %s <capability> [flags]", providerRef)
}

func writeCapabilityHelp(w io.Writer, alias, capability string, providerMeta state.ProviderMetadata) {
	writeLine(w, "Capability: %s", capability)
	if providerMeta.CapabilityDescriptions != nil {
		if description := providerMeta.CapabilityDescriptions[capability]; description != "" {
			writeLine(w, "")
			writeLine(w, "Description:")
			writeLine(w, "  %s", description)
		}
	}
	writeLine(w, "")
	writeLine(w, "Usage:")
	writeLine(w, "  tinx %s %s [flags]", alias, capability)
}

func isHelpToken(value string) bool {
	return value == "-h" || value == "--help" || value == "help"
}

func isOCIReference(ref string) bool {
	if strings.Contains(ref, "@") {
		return true
	}
	if strings.Count(ref, "/") >= 2 {
		return true
	}
	if idx := strings.LastIndex(ref, ":"); idx > 0 {
		after := ref[idx+1:]
		if !strings.Contains(after, "/") {
			return true
		}
	}
	if slash := strings.Index(ref, "/"); slash > 0 {
		return strings.Contains(ref[:slash], ".")
	}
	return false
}

func providerRefFromOCIReference(ref string) (string, bool) {
	repository := ref
	if at := strings.Index(repository, "@"); at >= 0 {
		repository = repository[:at]
	}
	if idx := strings.LastIndex(repository, ":"); idx > 0 {
		after := repository[idx+1:]
		if !strings.Contains(after, "/") {
			repository = repository[:idx]
		}
	}
	segments := strings.Split(repository, "/")
	if len(segments) < 3 {
		return "", false
	}
	return segments[len(segments)-2] + "/" + segments[len(segments)-1], true
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
