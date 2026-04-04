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

	"github.com/sourceplane/tinx/internal/gha"
	"github.com/sourceplane/tinx/internal/oci"
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
	if providerMeta.InvocationStyle == state.InvocationStylePassthrough {
		binaryPath, err := resolveBinaryPath(home, providerMeta, cmd.ErrOrStderr())
		if err != nil {
			return err
		}
		providerHome := providerHomeFromBinary(binaryPath)
		envOverrides := map[string]string(nil)
		if providerMeta.Source.Driver == gha.DriverName {
			envOverrides, err = gha.PassthroughEnvironment(home, providerMeta, version.String())
			if err != nil {
				return err
			}
		}
		return tinxruntime.Execute(tinxruntime.ExecOptions{
			BinaryPath:   binaryPath,
			Args:         args,
			WorkingDir:   mustGetwd(),
			ProviderHome: providerHome,
			TinxVersion:  version.String(),
			EnvOverrides: envOverrides,
			Stdout:       cmd.OutOrStdout(),
			Stderr:       cmd.ErrOrStderr(),
			Stdin:        os.Stdin,
		})
	}
	if len(args) == 0 {
		return fmt.Errorf("capability is required")
	}
	if !contains(providerMeta.Capabilities, args[0]) {
		return fmt.Errorf("provider %s does not expose capability %q", providerRef, args[0])
	}
	if providerMeta.Runtime == gha.RuntimeComposite || providerMeta.Runtime == gha.RuntimeNode {
		return gha.Execute(gha.ExecuteOptions{
			Home:        home,
			Metadata:    providerMeta,
			Capability:  args[0],
			Args:        args[1:],
			WorkingDir:  mustGetwd(),
			TinxVersion: version.String(),
			Stdout:      cmd.OutOrStdout(),
			Stderr:      cmd.ErrOrStderr(),
			Stdin:       os.Stdin,
		})
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
	if gha.IsReference(ref) {
		return gha.ResolveForRun(context.Background(), home, ref, progressOut)
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
	if providerMeta.InvocationStyle == state.InvocationStylePassthrough {
		writeConfiguredInputs(w, providerMeta)
		writeLine(w, "")
		writeLine(w, "Usage:")
		writeLine(w, "  tinx %s [args...]", alias)
		writeLine(w, "  tinx run %s [args...]", providerRef)
		return
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
	writeProviderInputs(w, providerMeta)
	writeLine(w, "")
	writeLine(w, "Usage:")
	if providerMeta.DefaultCapability != "" {
		writeLine(w, "  tinx %s [--input name=value]", alias)
		writeLine(w, "  tinx run %s [--input name=value]", providerRef)
		writeLine(w, "  tinx %s %s [--input name=value]", alias, providerMeta.DefaultCapability)
		return
	}
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
	writeProviderInputs(w, providerMeta)
	writeLine(w, "")
	writeLine(w, "Usage:")
	if providerMeta.DefaultCapability == capability {
		writeLine(w, "  tinx %s [--input name=value]", alias)
		writeLine(w, "  tinx run %s [--input name=value]", providerRef)
		writeLine(w, "  tinx %s %s [--input name=value]", alias, capability)
		return
	}
	writeLine(w, "  tinx %s %s [flags]", alias, capability)
}

type providerInvocationPlan struct {
	showProviderHelp   bool
	showCapabilityHelp bool
	capability         string
	args               []string
}

func planProviderInvocation(providerMeta state.ProviderMetadata, args []string) providerInvocationPlan {
	if providerMeta.InvocationStyle == state.InvocationStylePassthrough {
		if len(args) > 0 && isHelpToken(args[0]) {
			return providerInvocationPlan{showProviderHelp: true}
		}
		return providerInvocationPlan{args: args}
	}
	if providerMeta.DefaultCapability == "" {
		if len(args) == 0 || isHelpToken(args[0]) {
			return providerInvocationPlan{showProviderHelp: true}
		}
		if len(args) >= 2 && isHelpToken(args[1]) {
			return providerInvocationPlan{showCapabilityHelp: true, capability: args[0]}
		}
		return providerInvocationPlan{capability: args[0], args: args}
	}
	if len(args) == 0 {
		return providerInvocationPlan{capability: providerMeta.DefaultCapability, args: []string{providerMeta.DefaultCapability}}
	}
	if isHelpToken(args[0]) {
		return providerInvocationPlan{showProviderHelp: true}
	}
	if contains(providerMeta.Capabilities, args[0]) {
		if len(args) >= 2 && isHelpToken(args[1]) {
			return providerInvocationPlan{showCapabilityHelp: true, capability: args[0]}
		}
		return providerInvocationPlan{capability: args[0], args: args}
	}
	invocationArgs := append([]string{providerMeta.DefaultCapability}, args...)
	return providerInvocationPlan{capability: providerMeta.DefaultCapability, args: invocationArgs}
}

func writeProviderInputs(w io.Writer, providerMeta state.ProviderMetadata) {
	if len(providerMeta.Inputs) == 0 {
		return
	}
	writeLine(w, "")
	writeLine(w, "Inputs:")
	names := make([]string, 0, len(providerMeta.Inputs))
	for name := range providerMeta.Inputs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		input := providerMeta.Inputs[name]
		details := make([]string, 0, 2)
		if input.Required {
			details = append(details, "required")
		}
		if configured := providerMeta.Source.Inputs[name]; configured != "" {
			details = append(details, "configured: "+configured)
		} else if input.Default != "" {
			details = append(details, "default: "+input.Default)
		}
		trailer := ""
		if len(details) > 0 {
			trailer = " (" + strings.Join(details, ", ") + ")"
		}
		if input.Description == "" {
			writeLine(w, "  %s%s", name, trailer)
			continue
		}
		writeLine(w, "  %-12s %s%s", name, input.Description, trailer)
	}
}

func writeConfiguredInputs(w io.Writer, providerMeta state.ProviderMetadata) {
	if len(providerMeta.Source.Inputs) == 0 {
		return
	}
	writeLine(w, "")
	writeLine(w, "Configured Inputs:")
	names := make([]string, 0, len(providerMeta.Source.Inputs))
	for name := range providerMeta.Source.Inputs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		writeLine(w, "  %-12s %s", name, providerMeta.Source.Inputs[name])
	}
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
