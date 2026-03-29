package gha

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/sourceplane/tinx/internal/manifest"
	"github.com/sourceplane/tinx/internal/state"
	"github.com/sourceplane/tinx/internal/ui/progress"
)

type InstallOptions struct {
	Alias       string
	Inputs      map[string]string
	WorkingDir  string
	TinxVersion string
	Stdout      io.Writer
	Stderr      io.Writer
	Stdin       *os.File
}

func InstallAlias(ctx context.Context, home, ref string, opts InstallOptions, out io.Writer) (state.ProviderMetadata, error) {
	tracker := progress.New(out)
	defer tracker.Finish()

	alias := strings.TrimSpace(opts.Alias)
	if alias == "" {
		return state.ProviderMetadata{}, fmt.Errorf("GitHub Action installs require a non-empty alias to create a configured provider")
	}
	if strings.Contains(alias, "/") {
		return state.ProviderMetadata{}, fmt.Errorf("GitHub Action aliases must not contain '/': %q", alias)
	}

	tracker.Step("lookup", fmt.Sprintf("checking local cache for %s", ref))
	base, ok := cachedInstall(home, ref)
	if ok {
		tracker.Cached("cache", fmt.Sprintf("using cached action %s/%s@%s", base.Namespace, base.Name, base.Version))
	} else {
		var err error
		base, err = install(ctx, home, ref, tracker)
		if err != nil {
			return state.ProviderMetadata{}, err
		}
	}

	configured, err := configureInstalledProvider(home, alias, base, opts, tracker)
	if err != nil {
		return state.ProviderMetadata{}, err
	}
	tracker.Done("install", fmt.Sprintf("installed %s/%s@%s", configured.Namespace, configured.Name, configured.Version))
	return configured, nil
}

func configureInstalledProvider(home, alias string, base state.ProviderMetadata, opts InstallOptions, tracker *progress.Tracker) (state.ProviderMetadata, error) {
	configured := base
	configured.Namespace = DriverName
	configured.Name = alias
	configured.InvocationStyle = ""
	configured.DefaultCapability = DefaultCapability
	configured.Capabilities = []string{DefaultCapability}
	configured.CapabilityDescriptions = cloneCapabilityDescriptions(base.CapabilityDescriptions)
	configured.Inputs = cloneInputMetadata(base.Inputs)
	configured.Platforms = nil
	configured.InstalledAt = time.Now().UTC()
	configured.Source = cloneSource(base.Source)
	configured.Source.Inputs = cloneStringMap(opts.Inputs)

	versionRoot := state.VersionRoot(home, configured.Namespace, configured.Name, configured.Version)
	if err := os.RemoveAll(versionRoot); err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("reset configured provider cache: %w", err)
	}
	if err := os.MkdirAll(versionRoot, 0o755); err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("create configured provider cache: %w", err)
	}
	if err := saveConfiguredProvider(home, configured); err != nil {
		return state.ProviderMetadata{}, err
	}
	tracker.Info("configure", fmt.Sprintf("stored provider %s/%s", configured.Namespace, configured.Name))

	actionPath := configured.Source.SourcePath
	if subpath := strings.TrimSpace(configured.Source.Subpath); subpath != "" {
		actionPath = filepath.Join(actionPath, filepath.FromSlash(subpath))
	}
	action, _, err := LoadAction(actionPath)
	if err != nil {
		return state.ProviderMetadata{}, err
	}
	shouldBootstrap, err := shouldBootstrapAction(action, configured.Source.Inputs)
	if err != nil {
		return state.ProviderMetadata{}, err
	}
	if shouldBootstrap {
		configured, err = bootstrapConfiguredProvider(home, configured, opts, tracker)
		if err != nil {
			return state.ProviderMetadata{}, err
		}
	}

	aliases, err := state.LoadAliases(home)
	if err != nil {
		return state.ProviderMetadata{}, err
	}
	aliases[alias] = configured.Namespace + "/" + configured.Name
	if err := state.SaveAliases(home, aliases); err != nil {
		return state.ProviderMetadata{}, err
	}
	tracker.Info("install", fmt.Sprintf("updated alias %s", alias))
	return configured, nil
}

func shouldBootstrapAction(action Action, inputs map[string]string) (bool, error) {
	if _, err := resolveInputs(action, inputs); err != nil {
		if isMissingRequiredInput(err) && len(inputs) == 0 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func bootstrapConfiguredProvider(home string, meta state.ProviderMetadata, opts InstallOptions, tracker *progress.Tracker) (state.ProviderMetadata, error) {
	tracker.Step("bootstrap", "executing GitHub Action installer")
	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	stdin := opts.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	if err := Execute(ExecuteOptions{
		Home:        home,
		Metadata:    meta,
		Capability:  DefaultCapability,
		WorkingDir:  workingDirOrDefault(opts.WorkingDir),
		TinxVersion: opts.TinxVersion,
		Stdout:      stdout,
		Stderr:      stderr,
		Stdin:       stdin,
	}); err != nil {
		return state.ProviderMetadata{}, err
	}
	tracker.Done("bootstrap", "installer bootstrap complete")

	promoted, promotedOK, err := promoteInstalledBinary(home, meta)
	if err != nil {
		return state.ProviderMetadata{}, err
	}
	if promotedOK {
		tracker.Info("bootstrap", fmt.Sprintf("promoted local entrypoint %s", promoted.Entrypoint))
		return promoted, nil
	}
	tracker.Info("bootstrap", "kept GitHub Action runtime because no single managed executable was detected")
	return meta, nil
}

func promoteInstalledBinary(home string, meta state.ProviderMetadata) (state.ProviderMetadata, bool, error) {
	providerHome := state.VersionRoot(home, meta.Namespace, meta.Name, meta.Version)
	runtimeState, err := loadRuntimeState(providerHome)
	if err != nil {
		return state.ProviderMetadata{}, false, err
	}
	entrypoint, err := discoverManagedEntrypoint(providerHome, runtimeState.Path)
	if err != nil {
		return state.ProviderMetadata{}, false, err
	}
	if entrypoint == "" {
		return meta, false, nil
	}
	targetPath, err := materializeEntrypoint(providerHome, entrypoint)
	if err != nil {
		return state.ProviderMetadata{}, false, err
	}

	promoted := meta
	promoted.Runtime = manifest.RuntimeBinary
	promoted.InvocationStyle = state.InvocationStylePassthrough
	promoted.Entrypoint = filepath.Base(targetPath)
	promoted.DefaultCapability = ""
	promoted.Capabilities = nil
	promoted.CapabilityDescriptions = nil
	promoted.Inputs = nil
	promoted.Platforms = []state.PlatformSummary{{OS: goruntime.GOOS, Arch: goruntime.GOARCH}}
	promoted.InstalledAt = time.Now().UTC()
	if err := saveConfiguredProvider(home, promoted); err != nil {
		return state.ProviderMetadata{}, false, err
	}
	if err := writeGeneratedManifest(providerHome, promoted); err != nil {
		return state.ProviderMetadata{}, false, err
	}
	return promoted, true, nil
}

func discoverManagedEntrypoint(providerHome string, pathEntries []string) (string, error) {
	candidates := make([]string, 0, 1)
	seen := make(map[string]struct{})
	for _, rawPath := range pathEntries {
		path := filepath.Clean(rawPath)
		if !isWithinDir(providerHome, path) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("stat managed path entry: %w", err)
		}
		if !info.IsDir() {
			continue
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return "", fmt.Errorf("read managed path entry: %w", err)
		}
		for _, entry := range entries {
			candidate := filepath.Join(path, entry.Name())
			candidateInfo, err := os.Stat(candidate)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return "", fmt.Errorf("stat managed executable candidate: %w", err)
			}
			if candidateInfo.IsDir() || candidateInfo.Mode()&0o111 == 0 {
				continue
			}
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) != 1 {
		return "", nil
	}
	return candidates[0], nil
}

func materializeEntrypoint(providerHome, sourcePath string) (string, error) {
	entrypoint := filepath.Base(sourcePath)
	targetPath := filepath.Join(providerHome, "bin", goruntime.GOOS, goruntime.GOARCH, entrypoint)
	if filepath.Clean(sourcePath) == filepath.Clean(targetPath) {
		return targetPath, nil
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", fmt.Errorf("create provider runtime directory: %w", err)
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", fmt.Errorf("read promoted entrypoint: %w", err)
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", fmt.Errorf("stat promoted entrypoint: %w", err)
	}
	if err := os.WriteFile(targetPath, data, info.Mode()); err != nil {
		return "", fmt.Errorf("write provider entrypoint: %w", err)
	}
	return targetPath, nil
}

func writeGeneratedManifest(providerHome string, meta state.ProviderMetadata) error {
	provider := manifest.Provider{
		APIVersion: manifest.APIVersionV1,
		Kind:       manifest.KindProvider,
		Metadata: manifest.Metadata{
			Name:        meta.Name,
			Namespace:   meta.Namespace,
			Version:     meta.Version,
			Description: meta.Description,
			Homepage:    meta.Homepage,
			License:     meta.License,
		},
		Spec: manifest.ProviderSpec{
			Runtime:    manifest.RuntimeBinary,
			Entrypoint: meta.Entrypoint,
			Platforms: []manifest.Platform{{
				OS:     goruntime.GOOS,
				Arch:   goruntime.GOARCH,
				Binary: filepath.ToSlash(filepath.Join("bin", goruntime.GOOS, goruntime.GOARCH, meta.Entrypoint)),
			}},
		},
	}
	data, err := yaml.Marshal(provider)
	if err != nil {
		return fmt.Errorf("encode generated tinx manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(providerHome, "tinx.yaml"), data, 0o644); err != nil {
		return fmt.Errorf("write generated tinx manifest: %w", err)
	}
	return nil
}

func saveConfiguredProvider(home string, meta state.ProviderMetadata) error {
	if err := state.SaveProviderMetadata(home, meta); err != nil {
		return err
	}
	if err := state.SaveInstallSource(home, meta.Namespace, meta.Name, meta.Version, meta.Source); err != nil {
		return err
	}
	return nil
}

func workingDirOrDefault(workingDir string) string {
	if strings.TrimSpace(workingDir) != "" {
		return workingDir
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func isMissingRequiredInput(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "missing required GitHub Action input ")
}

func cloneCapabilityDescriptions(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneInputMetadata(values map[string]state.Input) map[string]state.Input {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]state.Input, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneSource(source state.Source) state.Source {
	cloned := source
	cloned.Inputs = cloneStringMap(source.Inputs)
	return cloned
}

func isWithinDir(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}