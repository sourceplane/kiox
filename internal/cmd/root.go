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

	"github.com/sourceplane/tinx/internal/manifest"
	"github.com/sourceplane/tinx/internal/oci"
	"github.com/sourceplane/tinx/internal/resolver"
	tinxruntime "github.com/sourceplane/tinx/internal/runtime"
	"github.com/sourceplane/tinx/internal/state"
	"github.com/sourceplane/tinx/pkg/version"
)

type rootOptions struct {
	Home string
}

func Execute(ctx context.Context) error {
	return executeCLI(ctx, os.Args[1:], os.Stdout, os.Stderr)
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
	cmd.AddCommand(newInitCommand(opts))
	cmd.AddCommand(newUseCommand(opts))
	cmd.AddCommand(newListCommand(opts))
	cmd.AddCommand(newInstallCommand(opts))
	cmd.AddCommand(newRunCommand(opts))
	cmd.AddCommand(newDispatchProviderCommand(opts))
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
	invocation := planProviderInvocation(providerMeta, args[1:])
	if invocation.showProviderHelp {
		writeProviderHelp(cmd.OutOrStdout(), args[0], ref, providerMeta)
		return nil
	}
	if invocation.showCapabilityHelp {
		writeCapabilityHelp(cmd.OutOrStdout(), args[0], ref, invocation.capability, providerMeta)
		return nil
	}
	return executeProviderCapability(cmd, home, ref, providerMeta, invocation.args)
}

func executeProviderCapability(cmd *cobra.Command, home, providerRef string, providerMeta state.ProviderMetadata, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("capability is required")
	}
	if !contains(providerMeta.Capabilities, args[0]) {
		return fmt.Errorf("provider %s does not expose capability %q", providerRef, args[0])
	}
	if providerMeta.Runtime != manifest.RuntimeBinary || strings.TrimSpace(providerMeta.Source.LayoutPath) == "" {
		return fmt.Errorf("provider %s uses an unsupported runtime %q", providerRef, providerMeta.Runtime)
	}
	binaryPath, err := resolveBinaryPath(home, providerMeta, cmd.ErrOrStderr())
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

func resolveBinaryPath(home string, providerMeta state.ProviderMetadata, out io.Writer) (string, error) {
	binaryPath := oci.CurrentBinaryPath(home, providerMeta)
	if info, err := os.Stat(binaryPath); err == nil && !info.IsDir() {
		return binaryPath, nil
	}
	return oci.MaterializeRuntime(home, providerMeta, goruntime.GOOS, goruntime.GOARCH, out)
}

func resolveProviderForRun(home, ref string, plainHTTP bool, progressOut io.Writer) (state.ProviderMetadata, error) {
	if resolver.HasSourceScheme(ref) {
		return state.ProviderMetadata{}, unsupportedProviderSourceError(ref, "expected an installed provider or OCI registry reference")
	}
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

func writeCapabilityHelp(w io.Writer, alias, providerRef, capability string, providerMeta state.ProviderMetadata) {
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

type providerInvocationPlan struct {
	showProviderHelp   bool
	showCapabilityHelp bool
	capability         string
	args               []string
}

func planProviderInvocation(providerMeta state.ProviderMetadata, args []string) providerInvocationPlan {
	if len(args) == 0 || isHelpToken(args[0]) {
		return providerInvocationPlan{showProviderHelp: true}
	}
	if len(args) >= 2 && isHelpToken(args[1]) {
		return providerInvocationPlan{showCapabilityHelp: true, capability: args[0]}
	}
	return providerInvocationPlan{capability: args[0], args: args}
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

func unsupportedProviderSourceError(ref, detail string) error {
	return fmt.Errorf("unsupported provider source %q: %s", ref, detail)
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
