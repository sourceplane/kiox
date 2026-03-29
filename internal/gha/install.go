package gha

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/sourceplane/tinx/internal/state"
	"github.com/sourceplane/tinx/internal/ui/progress"
)

func ResolveForRun(ctx context.Context, home, ref string, out io.Writer) (state.ProviderMetadata, error) {
	tracker := progress.New(out)
	defer tracker.Finish()
	tracker.Step("lookup", fmt.Sprintf("checking local cache for %s", ref))
	if cached, ok := cachedInstall(home, ref); ok {
		tracker.Cached("cache", fmt.Sprintf("using cached action %s/%s@%s", cached.Namespace, cached.Name, cached.Version))
		return cached, nil
	}
	return install(ctx, home, ref, tracker)
}

func Install(ctx context.Context, home, ref string, out io.Writer) (state.ProviderMetadata, error) {
	tracker := progress.New(out)
	defer tracker.Finish()
	tracker.Step("lookup", fmt.Sprintf("checking local cache for %s", ref))
	if cached, ok := cachedInstall(home, ref); ok {
		tracker.Cached("cache", fmt.Sprintf("using cached action %s/%s@%s", cached.Namespace, cached.Name, cached.Version))
		return cached, nil
	}
	return install(ctx, home, ref, tracker)
}

func cachedInstall(home, ref string) (state.ProviderMetadata, bool) {
	spec, err := ParseReference(ref)
	if err != nil {
		return state.ProviderMetadata{}, false
	}
	meta, err := state.LoadProviderMetadata(home, DriverName, spec.ProviderName())
	if err != nil {
		return state.ProviderMetadata{}, false
	}
	if meta.Runtime != RuntimeComposite && meta.Runtime != RuntimeNode {
		return state.ProviderMetadata{}, false
	}
	if meta.Source.Driver != DriverName || meta.Source.Ref != ref || strings.TrimSpace(meta.Source.SourcePath) == "" {
		return state.ProviderMetadata{}, false
	}
	if _, err := os.Stat(meta.Source.SourcePath); err != nil {
		return state.ProviderMetadata{}, false
	}
	return meta, true
}

func install(ctx context.Context, home, ref string, tracker *progress.Tracker) (state.ProviderMetadata, error) {
	spec, err := ParseReference(ref)
	if err != nil {
		return state.ProviderMetadata{}, err
	}

	tempDir, err := os.MkdirTemp("", "tinx-gha-*")
	if err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("create temp action workspace: %w", err)
	}
	defer os.RemoveAll(tempDir)

	tracker.Step("fetch", fmt.Sprintf("cloning %s@%s", spec.Repository(), spec.Version))
	repo, err := git.PlainCloneContext(ctx, tempDir, false, &git.CloneOptions{
		URL:  repositoryURL(spec),
		Tags: git.AllTags,
	})
	if err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("clone action repository: %w", err)
	}
	hash, err := resolveRevision(repo, spec.Version)
	if err != nil {
		return state.ProviderMetadata{}, err
	}
	worktree, err := repo.Worktree()
	if err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("open action worktree: %w", err)
	}
	if err := worktree.Checkout(&git.CheckoutOptions{Hash: *hash, Force: true}); err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("checkout action ref %q: %w", spec.Version, err)
	}
	tracker.Done("fetch", fmt.Sprintf("checked out %s", shortHash(*hash)))

	actionDir := spec.ActionDir(tempDir)
	action, manifestName, err := LoadAction(actionDir)
	if err != nil {
		return state.ProviderMetadata{}, err
	}
	runtimeKind, err := ProviderRuntime(action.Runs.Using)
	if err != nil {
		return state.ProviderMetadata{}, err
	}
	entrypoint, err := actionEntrypoint(actionDir, spec, manifestName, action, runtimeKind)
	if err != nil {
		return state.ProviderMetadata{}, err
	}

	versionRoot := state.VersionRoot(home, DriverName, spec.ProviderName(), spec.Version)
	if err := os.MkdirAll(versionRoot, 0o755); err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("create provider cache: %w", err)
	}
	sourcePath := filepath.Join(versionRoot, "source")
	if err := os.RemoveAll(sourcePath); err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("reset cached action source: %w", err)
	}
	tracker.Step("cache", "caching action source")
	if err := copyTree(tempDir, sourcePath); err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("cache action source: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(sourcePath, ".git")); err != nil {
		return state.ProviderMetadata{}, fmt.Errorf("trim cached git metadata: %w", err)
	}

	meta := state.ProviderMetadata{
		Namespace:         DriverName,
		Name:              spec.ProviderName(),
		Version:           spec.Version,
		Description:       action.Summary(),
		Runtime:           runtimeKind,
		Entrypoint:        entrypoint,
		DefaultCapability: DefaultCapability,
		Capabilities:      []string{DefaultCapability},
		CapabilityDescriptions: map[string]string{
			DefaultCapability: capabilityDescription(action),
		},
		Inputs:      toInputMetadata(action.Inputs),
		InstalledAt: time.Now().UTC(),
		Source: state.Source{
			Driver:     DriverName,
			SourcePath: sourcePath,
			Ref:        ref,
			Repo:       spec.Repository(),
			Subpath:    spec.Subpath,
			Revision:   hash.String(),
		},
	}
	if err := state.SaveProviderMetadata(home, meta); err != nil {
		return state.ProviderMetadata{}, err
	}
	if err := state.SaveInstallSource(home, meta.Namespace, meta.Name, meta.Version, meta.Source); err != nil {
		return state.ProviderMetadata{}, err
	}
	tracker.Done("cache", "action source cached locally")
	tracker.Done("install", fmt.Sprintf("installed %s/%s@%s", meta.Namespace, meta.Name, meta.Version))
	return meta, nil
}

func capabilityDescription(action Action) string {
	if summary := action.Summary(); summary != "" {
		return summary
	}
	return "Execute GitHub Action"
}

func actionEntrypoint(actionDir string, spec Reference, manifestName string, action Action, runtimeKind string) (string, error) {
	relativePath := manifestName
	if runtimeKind == RuntimeNode {
		mainEntrypoint, err := MainEntrypoint(action)
		if err != nil {
			return "", err
		}
		absoluteEntrypoint := filepath.Join(actionDir, filepath.FromSlash(mainEntrypoint))
		if _, err := os.Stat(filepath.Clean(absoluteEntrypoint)); err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("GitHub Action node entrypoint %q does not exist", mainEntrypoint)
			}
			return "", fmt.Errorf("stat GitHub Action node entrypoint: %w", err)
		}
		relativePath = mainEntrypoint
	}
	if spec.Subpath != "" {
		return filepath.ToSlash(filepath.Join("source", spec.Subpath, relativePath)), nil
	}
	return filepath.ToSlash(filepath.Join("source", relativePath)), nil
}

func toInputMetadata(inputs map[string]ActionInput) map[string]state.Input {
	if len(inputs) == 0 {
		return nil
	}
	metadata := make(map[string]state.Input, len(inputs))
	for name, input := range inputs {
		metadata[name] = state.Input{
			Description: strings.TrimSpace(input.Description),
			Required:    input.Required,
			Default:     scalarString(input.Default),
		}
	}
	return metadata
}

func repositoryURL(spec Reference) string {
	if root := strings.TrimSpace(os.Getenv("TINX_GHA_REPO_ROOT")); root != "" {
		return filepath.Join(root, spec.Owner, spec.Repo)
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("TINX_GHA_GIT_BASE_URL")), "/")
	if base == "" {
		base = "https://github.com"
	}
	return base + "/" + spec.Owner + "/" + spec.Repo + ".git"
}

func resolveRevision(repo *git.Repository, revision string) (*plumbing.Hash, error) {
	candidates := []string{
		revision,
		"refs/tags/" + revision,
		"refs/heads/" + revision,
		"refs/remotes/origin/" + revision,
		"origin/" + revision,
	}
	for _, candidate := range candidates {
		hash, err := repo.ResolveRevision(plumbing.Revision(candidate))
		if err == nil && hash != nil {
			return hash, nil
		}
	}
	return nil, fmt.Errorf("resolve GitHub Action git ref %q", revision)
}

func shortHash(hash plumbing.Hash) string {
	value := hash.String()
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}

func copyTree(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
