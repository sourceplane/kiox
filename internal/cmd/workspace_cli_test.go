package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	gcrregistry "github.com/google/go-containerregistry/pkg/registry"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	internaloci "github.com/sourceplane/kiox/internal/oci"
	"github.com/sourceplane/kiox/internal/state"
	workspacepkg "github.com/sourceplane/kiox/internal/workspace"
)

func TestInitWorkspaceFromFlagsAutoSelectsWorkspaceAndDispatches(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	liteCIProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	nodeProject := createNodeProviderProject(t, filepath.Join(tempDir, "node-provider"))
	liteCILayout := releaseProviderLayout(t, globalHome, liteCIProject)
	nodeLayout := releaseProviderLayout(t, globalHome, nodeProject)
	workspaceRoot := filepath.Join(tempDir, "my-workspace")

	initBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceRoot})
	if !bytes.Contains(initBuf.Bytes(), []byte("initialized workspace my-workspace")) {
		t.Fatalf("unexpected init output: %s", initBuf.String())
	}
	if !bytes.Contains(initBuf.Bytes(), []byte("active workspace: my-workspace")) {
		t.Fatalf("expected init to select the workspace, got: %s", initBuf.String())
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "kiox.yaml")); err != nil {
		t.Fatalf("expected workspace manifest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "kiox.lock")); err != nil {
		t.Fatalf("expected workspace lock file: %v", err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(workspaceRoot, "kiox.yaml"))
	if err != nil {
		t.Fatalf("read workspace manifest: %v", err)
	}
	manifestContent := string(manifestBytes)
	for _, expected := range []string{"kind: Workspace", "providers: {}", "name: my-workspace"} {
		if !strings.Contains(manifestContent, expected) {
			t.Fatalf("expected %q in workspace manifest, got:\n%s", expected, manifestContent)
		}
	}
	if strings.Contains(manifestContent, "workspace:") {
		t.Fatalf("expected workspace manifest to omit legacy workspace field, got:\n%s", manifestContent)
	}
	lockBytes, err := os.ReadFile(filepath.Join(workspaceRoot, "kiox.lock"))
	if err != nil {
		t.Fatalf("read workspace lock: %v", err)
	}
	lockContent := string(lockBytes)
	for _, expected := range []string{"kind: WorkspaceLock", "metadata:", "name: my-workspace"} {
		if !strings.Contains(lockContent, expected) {
			t.Fatalf("expected %q in workspace lock, got:\n%s", expected, lockContent)
		}
	}
	if strings.Contains(lockContent, "workspace:") {
		t.Fatalf("expected workspace lock to omit legacy workspace field, got:\n%s", lockContent)
	}

	runRootCommand(t, []string{"--kiox-home", globalHome, "add", liteCILayout, "as", "lite-ci"})
	runRootCommand(t, []string{"--kiox-home", globalHome, "add", nodeLayout, "as", "node"})

	runBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "exec", "lite-ci", "plan", "--", "node", "build"})
	assertWorkspaceShellOutput(t, runBuf)
	for _, path := range []string{
		filepath.Join(workspaceRoot, ".workspace", "env"),
		filepath.Join(workspaceRoot, ".workspace", "path"),
		filepath.Join(workspaceRoot, ".workspace", "providers"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected workspace runtime artifact %s: %v", path, err)
		}
	}

	activeWorkspace, err := state.LoadActiveWorkspace(globalHome)
	if err != nil {
		t.Fatalf("load active workspace: %v", err)
	}
	if activeWorkspace != workspaceRoot {
		t.Fatalf("expected active workspace %q, got %q", workspaceRoot, activeWorkspace)
	}
}

func TestWorkspaceSupportsNormalizedMultiToolProvider(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	providerDir := filepath.Join(tempDir, "normalized-provider")
	if err := copyTree(filepath.Join("..", "..", "testdata", "multi-tool-provider"), providerDir); err != nil {
		t.Fatalf("copy normalized provider: %v", err)
	}
	layoutPath := releaseProviderLayout(t, globalHome, providerDir)
	workspaceRoot := filepath.Join(tempDir, "normalized-workspace")

	runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceRoot})
	runRootCommand(t, []string{"--kiox-home", globalHome, "add", layoutPath, "as", "echo"})

	workspaceHome := workspacepkg.Home(workspaceRoot)
	aliases, err := state.LoadAliases(workspaceHome)
	if err != nil {
		t.Fatalf("load workspace aliases: %v", err)
	}
	meta, err := state.LoadProviderMetadataByKey(workspaceHome, aliases["echo"])
	if err != nil {
		t.Fatalf("load provider metadata: %v", err)
	}
	scriptToolPath := filepath.Join(state.MetadataStoreRoot(meta), "tools", "echo-tool", "bin", "echo-tool")
	setupToolPath := filepath.Join(state.MetadataStoreRoot(meta), "bin", goruntime.GOOS, goruntime.GOARCH, "setup-echo")
	if _, err := os.Stat(scriptToolPath); !os.IsNotExist(err) {
		t.Fatalf("expected script tool to be lazy-installed, stat=%v", err)
	}
	if _, err := os.Stat(setupToolPath); !os.IsNotExist(err) {
		t.Fatalf("expected setup tool to be lazy-installed, stat=%v", err)
	}
	aliasBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "exec", "echo", "one", "two"})
	if !strings.Contains(aliasBuf.String(), "normalized-env=hello-normalized") {
		t.Fatalf("unexpected alias output: %s", aliasBuf.String())
	}
	if !strings.Contains(aliasBuf.String(), "normalized-args=one two") {
		t.Fatalf("unexpected alias args output: %s", aliasBuf.String())
	}
	for _, shimName := range []string{"echo", "echo-tool"} {
		if _, err := os.Stat(filepath.Join(workspaceRoot, ".workspace", "bin", shimName)); err != nil {
			t.Fatalf("expected shim %s: %v", shimName, err)
		}
	}
	toolBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "exec", "echo-tool", "alpha", "beta"})
	if !strings.Contains(toolBuf.String(), "normalized-args=alpha beta") {
		t.Fatalf("unexpected tool output: %s", toolBuf.String())
	}
	if _, err := os.Stat(scriptToolPath); err != nil {
		t.Fatalf("expected lazy-installed script tool %s: %v", scriptToolPath, err)
	}
	if _, err := os.Stat(setupToolPath); err != nil {
		t.Fatalf("expected materialized setup tool %s: %v", setupToolPath, err)
	}
	if strings.Contains(aliasBuf.String(), "install no longer executes commands") {
		t.Fatalf("unexpected legacy execution path: %s", aliasBuf.String())
	}
}

func TestWorkspaceSupportsInlineNormalizedProvider(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	providerDir := filepath.Join(tempDir, "inline-provider")
	if err := copyTree(filepath.Join("..", "..", "testdata", "inline-tool-provider"), providerDir); err != nil {
		t.Fatalf("copy inline provider: %v", err)
	}
	layoutPath := releaseProviderLayout(t, globalHome, providerDir)
	workspaceRoot := filepath.Join(tempDir, "inline-workspace")

	runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceRoot})
	runRootCommand(t, []string{"--kiox-home", globalHome, "add", layoutPath, "as", "inline"})

	workspaceHome := workspacepkg.Home(workspaceRoot)
	aliases, err := state.LoadAliases(workspaceHome)
	if err != nil {
		t.Fatalf("load workspace aliases: %v", err)
	}
	meta, err := state.LoadProviderMetadataByKey(workspaceHome, aliases["inline"])
	if err != nil {
		t.Fatalf("load provider metadata: %v", err)
	}
	scriptToolPath := filepath.Join(state.MetadataStoreRoot(meta), "tools", "inline-tool", "bin", "inline-tool")
	setupToolPath := filepath.Join(state.MetadataStoreRoot(meta), "bin", goruntime.GOOS, goruntime.GOARCH, "setup-inline")
	assetPath := filepath.Join(state.MetadataStoreRoot(meta), "assets", "templates", "message.txt")
	for _, path := range []string{scriptToolPath, setupToolPath, assetPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be lazy, stat=%v", path, err)
		}
	}

	aliasBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "exec", "inline", "red", "blue"})
	for _, expected := range []string{
		"inline-env=hello-inline",
		"inline-mode=inline",
		"inline-asset=hello-from-inline-assets",
		"inline-args=red blue",
	} {
		if !strings.Contains(aliasBuf.String(), expected) {
			t.Fatalf("expected %q in alias output, got: %s", expected, aliasBuf.String())
		}
	}
	for _, shimName := range []string{"inline", "inline-tool"} {
		if _, err := os.Stat(filepath.Join(workspaceRoot, ".workspace", "bin", shimName)); err != nil {
			t.Fatalf("expected shim %s: %v", shimName, err)
		}
	}
	toolBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "exec", "inline-tool", "green", "gold"})
	if !strings.Contains(toolBuf.String(), "inline-args=green gold") {
		t.Fatalf("unexpected inline tool output: %s", toolBuf.String())
	}
	for _, path := range []string{scriptToolPath, setupToolPath, assetPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected materialized path %s: %v", path, err)
		}
	}
	assetBytes, err := os.ReadFile(assetPath)
	if err != nil {
		t.Fatalf("read inline asset: %v", err)
	}
	if strings.TrimSpace(string(assetBytes)) != "hello-from-inline-assets" {
		t.Fatalf("unexpected inline asset content: %q", string(assetBytes))
	}
}

func TestInitDefaultsToCurrentDirectory(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	projectRoot := filepath.Join(tempDir, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(projectRoot)

	initBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "init"})
	workspaceName := filepath.Base(projectRoot)
	if !bytes.Contains(initBuf.Bytes(), []byte("initialized workspace "+workspaceName)) {
		t.Fatalf("unexpected init output: %s", initBuf.String())
	}
	if _, err := os.Stat(filepath.Join(projectRoot, "kiox.yaml")); err != nil {
		t.Fatalf("expected workspace manifest: %v", err)
	}
	currentBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "ws", "current"})
	if !bytes.Contains(currentBuf.Bytes(), []byte("workspace: "+workspaceName)) {
		t.Fatalf("unexpected current workspace output: %s", currentBuf.String())
	}
}

func TestInitUsesExistingWorkspaceManifest(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	workspaceRoot := filepath.Join(tempDir, "project")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	existingManifest := strings.Join([]string{
		"apiVersion: kiox.io/v1",
		"kind: Workspace",
		"metadata:",
		"  name: custom-space",
		"providers: {}",
		"",
	}, "\n")
	manifestPath := filepath.Join(workspaceRoot, "kiox.yaml")
	if err := os.WriteFile(manifestPath, []byte(existingManifest), 0o644); err != nil {
		t.Fatal(err)
	}

	initBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceRoot})
	if !bytes.Contains(initBuf.Bytes(), []byte("initialized workspace custom-space")) {
		t.Fatalf("unexpected init output: %s", initBuf.String())
	}
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read workspace manifest: %v", err)
	}
	manifestContent := string(manifestBytes)
	for _, expected := range []string{"kind: Workspace", "name: custom-space", "providers: {}"} {
		if !strings.Contains(manifestContent, expected) {
			t.Fatalf("expected %q in initialized manifest, got:\n%s", expected, manifestContent)
		}
	}
	if strings.Contains(manifestContent, "name: project") {
		t.Fatalf("expected init to preserve the existing workspace name, got:\n%s", manifestContent)
	}
}

func TestAddProviderDoesNotMutateManifestOnFailure(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	workspaceRoot := filepath.Join(tempDir, "workspace")

	runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceRoot})
	manifestPath := filepath.Join(workspaceRoot, "kiox.yaml")
	beforeManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read workspace manifest before add: %v", err)
	}

	buf := new(bytes.Buffer)
	err = executeCLI(context.Background(), []string{"--kiox-home", globalHome, "add", "custom://broken-provider"}, buf, buf)
	if err == nil {
		t.Fatal("expected add to fail for unsupported provider source")
	}
	if !strings.Contains(err.Error(), `unsupported provider source "custom://broken-provider"`) {
		t.Fatalf("unexpected add error: %v", err)
	}
	afterManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read workspace manifest after add failure: %v", err)
	}
	if !bytes.Equal(beforeManifest, afterManifest) {
		t.Fatalf("expected add failure to leave manifest unchanged\nbefore:\n%s\nafter:\n%s", string(beforeManifest), string(afterManifest))
	}
	lock, err := workspacepkg.LoadLock(workspaceRoot)
	if err != nil {
		t.Fatalf("load workspace lock: %v", err)
	}
	if len(lock.Providers) != 0 {
		t.Fatalf("expected add failure to leave lock unchanged, got %#v", lock.Providers)
	}
}

func TestInitWorkspaceFromConfigFileAndUseOneShotCommand(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	liteCIProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	nodeProject := createNodeProviderProject(t, filepath.Join(tempDir, "node-provider"))
	liteCILayout := releaseProviderLayout(t, globalHome, liteCIProject)
	nodeLayout := releaseProviderLayout(t, globalHome, nodeProject)
	workspaceRoot := filepath.Join(tempDir, "dev")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(workspaceRoot, "providers.tx.yaml")
	manifest := fmt.Sprintf(`kind: workspace
workspace: dev

providers:
  lite-ci: %s
  node: %s
`, liteCILayout, nodeLayout)
	if err := os.WriteFile(configPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	initBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "init", configPath})
	if !bytes.Contains(initBuf.Bytes(), []byte("initialized workspace dev")) {
		t.Fatalf("unexpected init output: %s", initBuf.String())
	}
	if !bytes.Contains(initBuf.Bytes(), []byte("active workspace: dev")) {
		t.Fatalf("expected init to select the workspace, got: %s", initBuf.String())
	}
	materializedManifest := filepath.Join(workspaceRoot, "kiox.yaml")
	if _, err := os.Stat(materializedManifest); err != nil {
		t.Fatalf("expected materialized workspace manifest: %v", err)
	}
	loadedConfig, err := workspacepkg.Load(materializedManifest)
	if err != nil {
		t.Fatalf("load materialized workspace manifest: %v", err)
	}
	if !loadedConfig.HasProviderAlias("lite-ci") || !loadedConfig.HasProviderAlias("node") {
		t.Fatalf("expected materialized manifest aliases, got %#v", loadedConfig.ProviderAliases())
	}
	activeWorkspace, err := state.LoadActiveWorkspace(globalHome)
	if err != nil {
		t.Fatalf("load active workspace: %v", err)
	}
	if activeWorkspace != workspaceRoot {
		t.Fatalf("expected active workspace %q, got %q", workspaceRoot, activeWorkspace)
	}

	runBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "--workspace", workspaceRoot, "--", "lite-ci", "plan", "--", "node", "build"})
	assertWorkspaceShellOutput(t, runBuf)
}

func TestSyncReconcilesManualManifestEdits(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	liteCIProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	liteCILayout := releaseProviderLayout(t, globalHome, liteCIProject)
	workspaceRoot := filepath.Join(tempDir, "dev")

	runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceRoot})
	runRootCommand(t, []string{"--kiox-home", globalHome, "add", liteCILayout, "as", "lite-ci"})

	if err := workspacepkg.Save(filepath.Join(workspaceRoot, "kiox.yaml"), workspacepkg.Config{
		APIVersion: workspacepkg.APIVersionV1,
		Kind:       workspacepkg.KindWorkspace,
		Metadata:   workspacepkg.Metadata{Name: "dev"},
		Providers:  map[string]workspacepkg.Provider{},
	}); err != nil {
		t.Fatalf("save reconciled workspace manifest: %v", err)
	}

	syncBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "sync"})
	if !strings.Contains(syncBuf.String(), "synced workspace dev") {
		t.Fatalf("unexpected sync output: %s", syncBuf.String())
	}
	aliases, err := state.LoadAliases(workspacepkg.Home(workspaceRoot))
	if err != nil {
		t.Fatalf("load workspace aliases after sync: %v", err)
	}
	if len(aliases) != 0 {
		t.Fatalf("expected sync to remove workspace aliases, got %#v", aliases)
	}
	lock, err := workspacepkg.LoadLock(workspaceRoot)
	if err != nil {
		t.Fatalf("load workspace lock after sync: %v", err)
	}
	if len(lock.Providers) != 0 {
		t.Fatalf("expected sync to clear lock providers, got %#v", lock.Providers)
	}
	providers, err := state.ListInstalledProviders(workspacepkg.Home(workspaceRoot))
	if err != nil {
		t.Fatalf("list workspace providers after sync: %v", err)
	}
	if len(providers) != 0 {
		t.Fatalf("expected sync to remove workspace-installed providers, got %#v", providers)
	}
}

func TestInteractiveWorkspaceShellUsesWorkspaceEnvironment(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	liteCIProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	liteCILayout := releaseProviderLayout(t, globalHome, liteCIProject)
	workspaceRoot := filepath.Join(tempDir, "interactive-workspace")

	runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceRoot})
	runRootCommand(t, []string{"--kiox-home", globalHome, "workspace", "use", workspaceRoot})
	runRootCommand(t, []string{"--kiox-home", globalHome, "add", liteCILayout, "as", "lite-ci"})

	fakeShell := filepath.Join(tempDir, "fake-shell")
	script := "#!/bin/sh\nset -eu\nprintf 'shell-root=%s\\n' \"$KIOX_WORKSPACE_ROOT\"\nprintf 'shell-env-file=%s\\n' \"$KIOX_WORKSPACE_ENV_FILE\"\nprintf 'shell-path=%s\\n' \"$PATH\"\n"
	if err := os.WriteFile(fakeShell, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", fakeShell)

	shellBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "shell"})
	if !bytes.Contains(shellBuf.Bytes(), []byte("shell-root="+workspaceRoot)) {
		t.Fatalf("expected interactive shell to inherit workspace root, got: %s", shellBuf.String())
	}
	if !bytes.Contains(shellBuf.Bytes(), []byte(filepath.Join(workspaceRoot, ".workspace", "env"))) {
		t.Fatalf("expected interactive shell env file, got: %s", shellBuf.String())
	}
	if !bytes.Contains(shellBuf.Bytes(), []byte(filepath.Join(workspaceRoot, ".workspace", "bin"))) {
		t.Fatalf("expected interactive shell path to include workspace shims, got: %s", shellBuf.String())
	}
}

func TestDashShortcutUsesContextWhenSyncingRemoteProviders(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	liteCIProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	server := httptest.NewServer(gcrregistry.New())
	defer server.Close()
	registryHost := strings.TrimPrefix(server.URL, "http://")
	ref := registryHost + "/acme/lite-ci:v0.1.0"
	releaseProviderRef(t, globalHome, liteCIProject, ref)

	workspaceRoot := filepath.Join(tempDir, "dash-workspace")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := workspacepkg.Save(filepath.Join(workspaceRoot, "kiox.yaml"), workspacepkg.Config{
		APIVersion: workspacepkg.APIVersionV1,
		Kind:       workspacepkg.KindWorkspace,
		Workspace:  "dash-workspace",
		Metadata:   workspacepkg.Metadata{Name: "dash-workspace"},
		Providers: map[string]workspacepkg.Provider{
			"lite-ci": {Source: ref, PlainHTTP: true},
		},
	}); err != nil {
		t.Fatalf("save workspace manifest: %v", err)
	}
	if err := state.SaveActiveWorkspace(globalHome, workspaceRoot); err != nil {
		t.Fatalf("save active workspace: %v", err)
	}

	fakeShell := filepath.Join(tempDir, "fake-shell")
	script := "#!/bin/sh\nset -eu\nprintf 'shell-root=%s\\n' \"$KIOX_WORKSPACE_ROOT\"\nprintf 'shell-env-file=%s\\n' \"$KIOX_WORKSPACE_ENV_FILE\"\nprintf 'shell-path=%s\\n' \"$PATH\"\n"
	if err := os.WriteFile(fakeShell, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", fakeShell)

	shellBuf := new(bytes.Buffer)
	if err := executeCLI(context.Background(), []string{"--kiox-home", globalHome, "--"}, shellBuf, shellBuf); err != nil {
		t.Fatalf("dash shortcut failed: %v\n%s", err, shellBuf.String())
	}
	if !bytes.Contains(shellBuf.Bytes(), []byte("shell-root="+workspaceRoot)) {
		t.Fatalf("expected dash shortcut shell to inherit workspace root, got: %s", shellBuf.String())
	}
	if !bytes.Contains(shellBuf.Bytes(), []byte(filepath.Join(workspaceRoot, ".workspace", "bin"))) {
		t.Fatalf("expected dash shortcut shell path to include workspace shims, got: %s", shellBuf.String())
	}
}

func TestDashShortcutReusesCachedTaggedRegistryProvider(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	liteCIProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	server := httptest.NewServer(gcrregistry.New())
	registryHost := strings.TrimPrefix(server.URL, "http://")
	ref := registryHost + "/acme/lite-ci:0.1.1"
	releaseProviderRef(t, globalHome, liteCIProject, ref)

	workspaceRoot := filepath.Join(tempDir, "cached-tagged-workspace")
	runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceRoot})
	runRootCommand(t, []string{"--kiox-home", globalHome, "add", ref, "as", "lite-ci", "--plain-http"})

	lock, err := workspacepkg.LoadLock(workspaceRoot)
	if err != nil {
		t.Fatalf("load workspace lock: %v", err)
	}
	if len(lock.Providers) != 1 {
		t.Fatalf("expected one locked provider, got %#v", lock.Providers)
	}
	if !strings.HasPrefix(lock.Providers[0].Resolved, registryHost+"/acme/lite-ci@sha256:") {
		t.Fatalf("expected tagged provider to be pinned in the lockfile, got %q", lock.Providers[0].Resolved)
	}
	aliases, err := state.LoadAliases(workspacepkg.Home(workspaceRoot))
	if err != nil {
		t.Fatalf("load workspace aliases: %v", err)
	}
	meta, err := state.LoadProviderMetadataByKey(workspacepkg.Home(workspaceRoot), aliases["lite-ci"])
	if err != nil {
		t.Fatalf("load workspace provider metadata: %v", err)
	}
	for _, path := range []string{filepath.Join(meta.StorePath, "package.json"), filepath.Join(meta.StorePath, "kiox.yaml")} {
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove cached package file %s: %v", path, err)
		}
	}

	server.Close()

	runBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "--workspace", workspaceRoot, "--", "lite-ci", "plan"})
	if !strings.Contains(runBuf.String(), "lite-ci-env=acme/lite-ci") {
		t.Fatalf("expected cached provider output, got: %s", runBuf.String())
	}
	if !strings.Contains(runBuf.String(), "lite-ci-args=plan") {
		t.Fatalf("expected cached command args, got: %s", runBuf.String())
	}
	if strings.Contains(runBuf.String(), "checking local cache") {
		t.Fatalf("expected cached workspace execution to stay quiet, got: %s", runBuf.String())
	}
	if strings.Contains(runBuf.String(), "using cached runtime") {
		t.Fatalf("expected cached workspace execution to avoid repeated runtime cache logs, got: %s", runBuf.String())
	}
	if strings.Contains(runBuf.String(), "Installing providers (") {
		t.Fatalf("expected prepared workspace execution to avoid provider sync output, got: %s", runBuf.String())
	}
	if strings.Contains(runBuf.String(), "Installed 1 providers") {
		t.Fatalf("expected prepared workspace execution to avoid provider sync summary, got: %s", runBuf.String())
	}
}

func TestProviderCommandAliasesAndStatus(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	liteCIProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	liteCILayout := releaseProviderLayout(t, globalHome, liteCIProject)
	workspaceRoot := filepath.Join(tempDir, "dev")

	runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceRoot})
	runRootCommand(t, []string{"--kiox-home", globalHome, "p", "add", liteCILayout, "as", "lite-ci"})

	statusBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "status"})
	for _, expected := range []string{
		"kiox workspace: dev",
		"path: " + displayRelativePath(workspaceRoot),
		"shims: active",
		"providers:",
		"tools:",
		"lite-ci",
		"acme/lite-ci",
		"v0.1.0",
	} {
		if !strings.Contains(statusBuf.String(), expected) {
			t.Fatalf("expected %q in status output, got:\n%s", expected, statusBuf.String())
		}
	}
	statusShortBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "status", "--short"})
	if !strings.Contains(statusShortBuf.String(), "dev | 1 providers | shims active") {
		t.Fatalf("unexpected short status output: %s", statusShortBuf.String())
	}
	statusVerboseBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "status", "--verbose"})
	for _, expected := range []string{
		"details:",
		"env file: " + displayRelativePath(filepath.Join(workspaceRoot, ".workspace", "env")),
		"path file: " + displayRelativePath(filepath.Join(workspaceRoot, ".workspace", "path")),
	} {
		if !strings.Contains(statusVerboseBuf.String(), expected) {
			t.Fatalf("expected %q in verbose status output, got:\n%s", expected, statusVerboseBuf.String())
		}
	}
	listBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "list"})
	for _, expected := range []string{
		"Scope: dev",
		"acme/lite-ci",
		"lite-ci",
		"1 provider (1 ready)",
	} {
		if !strings.Contains(listBuf.String(), expected) {
			t.Fatalf("expected %q in list output, got:\n%s", expected, listBuf.String())
		}
	}

	updateBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "update", "lite-ci"})
	if !strings.Contains(updateBuf.String(), "updated providers: lite-ci") {
		t.Fatalf("unexpected provider update output: %s", updateBuf.String())
	}

	removeBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "remove", "lite-ci"})
	if !strings.Contains(removeBuf.String(), "removed provider lite-ci") {
		t.Fatalf("unexpected provider remove output: %s", removeBuf.String())
	}
	config, err := workspacepkg.Load(filepath.Join(workspaceRoot, "kiox.yaml"))
	if err != nil {
		t.Fatalf("load workspace after provider remove: %v", err)
	}
	if config.HasProviderAlias("lite-ci") {
		t.Fatalf("expected provider alias to be removed from workspace manifest")
	}
	providersListBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "p", "list"})
	if !strings.Contains(providersListBuf.String(), "no providers installed") {
		t.Fatalf("expected provider list to be empty after removal, got:\n%s", providersListBuf.String())
	}
}

func TestStatusShowsToolInventoryForSetupProviderFlow(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	providerDir := filepath.Join(tempDir, "setup-kubectl-provider")
	if err := copyTree(filepath.Join("..", "..", "testdata", "setup-kubectl"), providerDir); err != nil {
		t.Fatalf("copy setup-kubectl provider: %v", err)
	}
	layoutPath := releaseProviderLayout(t, globalHome, providerDir)
	workspaceRoot := filepath.Join(tempDir, "kubectl-workspace")

	runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceRoot})
	runRootCommand(t, []string{"--kiox-home", globalHome, "add", layoutPath, "as", "setup-kubectl"})
	server, version := newFakeKubectlReleaseServer(t)
	defer server.Close()
	t.Setenv("KUBECTL_RELEASE_BASE_URL", server.URL)
	t.Setenv("KUBECTL_VERSION", "1.27")

	beforeBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "--workspace", workspaceRoot, "status"})
	beforeOutput := beforeBuf.String()
	for _, expected := range []string{
		"kiox workspace: kubectl-workspace",
		"tools:",
		"TOOL",
		"COMMANDS",
		"setup-kubectl",
		"kubectl",
		"~ lazy",
	} {
		if !strings.Contains(beforeOutput, expected) {
			t.Fatalf("expected %q in pre-install status output, got:\n%s", expected, beforeOutput)
		}
	}

	execBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "--workspace", workspaceRoot, "--", "kubectl", "version", "--client"})
	for _, expected := range []string{
		"kubectl-version=" + version,
		"kubectl-args=version --client",
	} {
		if !strings.Contains(execBuf.String(), expected) {
			t.Fatalf("expected %q in kubectl output, got:\n%s", expected, execBuf.String())
		}
	}

	afterBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "--workspace", workspaceRoot, "status"})
	afterOutput := afterBuf.String()
	for _, expected := range []string{
		"tools:",
		"setup-kubectl",
		"kubectl",
		"✓ ready",
	} {
		if !strings.Contains(afterOutput, expected) {
			t.Fatalf("expected %q in post-install status output, got:\n%s", expected, afterOutput)
		}
	}
	if strings.Contains(afterOutput, "~ lazy") {
		t.Fatalf("expected lazy status to be cleared after setup, got:\n%s", afterOutput)
	}
}

func TestWorkspaceProvidersReuseGlobalStoreAcrossWorkspaces(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	providerProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	layoutPath := releaseProviderLayout(t, globalHome, providerProject)
	workspaceOne := filepath.Join(tempDir, "workspace-one")
	workspaceTwo := filepath.Join(tempDir, "workspace-two")

	runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceOne})
	runRootCommand(t, []string{"--kiox-home", globalHome, "add", layoutPath, "as", "lite-ci"})
	runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceTwo})
	runRootCommand(t, []string{"--kiox-home", globalHome, "add", layoutPath, "as", "lite-ci"})

	metaOne, err := state.LoadProviderMetadata(filepath.Join(workspaceOne, ".workspace"), "acme", "lite-ci", "v0.1.0")
	if err != nil {
		t.Fatalf("load workspace one provider metadata: %v", err)
	}
	metaTwo, err := state.LoadProviderMetadata(filepath.Join(workspaceTwo, ".workspace"), "acme", "lite-ci", "v0.1.0")
	if err != nil {
		t.Fatalf("load workspace two provider metadata: %v", err)
	}
	if metaOne.StoreID == "" || metaOne.StorePath == "" {
		t.Fatalf("expected workspace one provider to reference the global store, got %#v", metaOne)
	}
	if metaOne.StoreID != metaTwo.StoreID {
		t.Fatalf("expected shared store id, got %q and %q", metaOne.StoreID, metaTwo.StoreID)
	}
	if metaOne.StorePath != metaTwo.StorePath {
		t.Fatalf("expected shared store path, got %q and %q", metaOne.StorePath, metaTwo.StorePath)
	}
	if _, err := os.Stat(filepath.Join(metaOne.StorePath, "oci", "index.json")); err != nil {
		t.Fatalf("expected shared OCI store layout: %v", err)
	}
	storeEntries, err := os.ReadDir(filepath.Join(globalHome, "store"))
	if err != nil {
		t.Fatalf("read global store: %v", err)
	}
	if len(storeEntries) != 1 {
		t.Fatalf("expected one shared store entry, got %d", len(storeEntries))
	}
}

func TestWorkspaceRegistryProvidersReuseGlobalStoreAcrossWorkspaces(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	providerProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	workspaceOne := filepath.Join(tempDir, "workspace-one")
	workspaceTwo := filepath.Join(tempDir, "workspace-two")
	server := httptest.NewServer(gcrregistry.New())
	registryHost := strings.TrimPrefix(server.URL, "http://")
	ref := registryHost + "/acme/lite-ci:v0.1.0"
	releaseProviderRef(t, globalHome, providerProject, ref)

	runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceOne})
	runRootCommand(t, []string{"--kiox-home", globalHome, "add", ref, "as", "lite-ci", "--plain-http"})

	server.Close()

	runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceTwo})
	secondBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "add", ref, "as", "lite-ci", "--plain-http"})
	if strings.Contains(secondBuf.String(), "pulling metadata layers") {
		t.Fatalf("expected second workspace to reuse the global store without a registry pull, got: %s", secondBuf.String())
	}

	metaOne, err := state.LoadProviderMetadata(filepath.Join(workspaceOne, ".workspace"), "acme", "lite-ci", "v0.1.0")
	if err != nil {
		t.Fatalf("load workspace one provider metadata: %v", err)
	}
	metaTwo, err := state.LoadProviderMetadata(filepath.Join(workspaceTwo, ".workspace"), "acme", "lite-ci", "v0.1.0")
	if err != nil {
		t.Fatalf("load workspace two provider metadata: %v", err)
	}
	if metaOne.StoreID == "" || metaOne.StorePath == "" {
		t.Fatalf("expected workspace one provider to reference the global store, got %#v", metaOne)
	}
	if metaOne.StoreID != metaTwo.StoreID {
		t.Fatalf("expected shared store id, got %q and %q", metaOne.StoreID, metaTwo.StoreID)
	}
	if metaOne.StorePath != metaTwo.StorePath {
		t.Fatalf("expected shared store path, got %q and %q", metaOne.StorePath, metaTwo.StorePath)
	}
}

func TestAddRemoteProviderUsesConciseSyncOutput(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	providerProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	workspaceRoot := filepath.Join(tempDir, "workspace")
	server := httptest.NewServer(gcrregistry.New())
	defer server.Close()
	registryHost := strings.TrimPrefix(server.URL, "http://")
	ref := registryHost + "/acme/lite-ci:v0.1.0"
	releaseProviderRef(t, globalHome, providerProject, ref)

	runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceRoot})
	addBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "add", ref, "as", "lite-ci", "--plain-http"})
	output := addBuf.String()
	for _, unexpected := range []string{
		"checking local cache",
		"pulling metadata layers",
		"runtime pull complete",
		"provider cached locally",
		"queued ",
		"copied ",
	} {
		if strings.Contains(output, unexpected) {
			t.Fatalf("expected concise sync output without %q, got:\n%s", unexpected, output)
		}
	}
	for _, expected := range []string{
		"Installing providers (1)",
		"acme/lite-ci",
		"ready",
		"Installed 1 providers",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in concise sync output, got:\n%s", expected, output)
		}
	}
}

func TestInitWithLocalProviderShowsCompactProgressSurface(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	providerProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	layoutPath := releaseProviderLayout(t, globalHome, providerProject)
	workspaceRoot := filepath.Join(tempDir, "workspace")

	initBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceRoot, "-p", layoutPath, "as", "lite-ci"})
	output := initBuf.String()
	for _, expected := range []string{
		"Installing providers (1)",
		"acme/lite-ci",
		"ready",
		"Installed 1 providers",
		"initialized workspace workspace",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in init progress output, got:\n%s", expected, output)
		}
	}
	for _, unexpected := range []string{"checking local cache", "pulling metadata layers", "queued ", "copied "} {
		if strings.Contains(output, unexpected) {
			t.Fatalf("expected compact init output without %q, got:\n%s", unexpected, output)
		}
	}
}

func TestSyncWithCachedRemoteProviderShowsCompactProgressSurface(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	providerProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	workspaceRoot := filepath.Join(tempDir, "workspace")
	server := httptest.NewServer(gcrregistry.New())
	defer server.Close()
	registryHost := strings.TrimPrefix(server.URL, "http://")
	ref := registryHost + "/acme/lite-ci:v0.1.0"
	releaseProviderRef(t, globalHome, providerProject, ref)

	runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceRoot})
	runRootCommand(t, []string{"--kiox-home", globalHome, "add", ref, "as", "lite-ci", "--plain-http"})
	syncBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "sync"})
	output := syncBuf.String()
	for _, expected := range []string{
		"Installing providers (1)",
		"acme/lite-ci",
		"cached",
		"Installed 1 providers",
		"synced workspace workspace",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in sync progress output, got:\n%s", expected, output)
		}
	}
	for _, unexpected := range []string{"checking local cache", "pulling metadata layers", "queued ", "copied "} {
		if strings.Contains(output, unexpected) {
			t.Fatalf("expected compact sync output without %q, got:\n%s", unexpected, output)
		}
	}
}

func TestSyncSummaryUsesShortWorkspacePaths(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	workspaceRoot := filepath.Join(tempDir, "workspace")

	runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceRoot})
	t.Chdir(workspaceRoot)

	syncBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "sync"})
	output := syncBuf.String()
	for _, expected := range []string{
		"synced workspace workspace",
		"manifest: kiox.yaml",
		"home: workspace/",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in sync summary output, got:\n%s", expected, output)
		}
	}
	if strings.Contains(output, ".workspace") {
		t.Fatalf("expected sync summary to hide internal workspace home, got:\n%s", output)
	}
}

func TestRegistryInstallCachesCurrentPlatformBlobsOnly(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	providerDir := filepath.Join(tempDir, "setup-kubectl-provider")
	if err := copyTree(filepath.Join("..", "..", "testdata", "setup-kubectl"), providerDir); err != nil {
		t.Fatalf("copy setup-kubectl provider: %v", err)
	}
	workspaceRoot := filepath.Join(tempDir, "workspace")
	server := httptest.NewServer(gcrregistry.New())
	defer server.Close()
	registryHost := strings.TrimPrefix(server.URL, "http://")
	ref := registryHost + "/acme/setup-kubectl:v0.1.0"
	releaseProviderRef(t, globalHome, providerDir, ref)

	runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceRoot})
	runRootCommand(t, []string{"--kiox-home", globalHome, "add", ref, "as", "setup-kubectl", "--plain-http"})

	meta, err := state.LoadProviderMetadata(filepath.Join(workspaceRoot, ".workspace"), "acme", "setup-kubectl", "v0.1.0")
	if err != nil {
		t.Fatalf("load workspace provider metadata: %v", err)
	}
	manifest := loadStoreImageManifest(t, filepath.Join(meta.StorePath, "oci"))
	hostPlatform := goruntime.GOOS + "/" + goruntime.GOARCH
	foundHostBinary := false
	foundSkippedBinary := false
	for _, layer := range manifest.Layers {
		platform := strings.TrimSpace(layer.Annotations["io.kiox.platform"])
		if platform == "" {
			continue
		}
		blobPath := filepath.Join(meta.StorePath, "oci", "blobs", "sha256", layer.Digest.Encoded())
		_, err := os.Stat(blobPath)
		if platform == hostPlatform {
			foundHostBinary = true
			if err != nil {
				t.Fatalf("expected host platform blob %s to be cached: %v", platform, err)
			}
			continue
		}
		foundSkippedBinary = true
		if !os.IsNotExist(err) {
			t.Fatalf("expected non-host platform blob %s to be absent from the local cache, stat=%v", platform, err)
		}
	}
	if !foundHostBinary {
		t.Fatalf("expected a cached host platform binary for %s", hostPlatform)
	}
	if !foundSkippedBinary {
		t.Fatal("expected at least one non-host platform blob to validate selective caching")
	}
}

func TestWorkspaceLockPinsUnversionedRegistrySourceUntilUpdate(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	providerProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	workspaceRoot := filepath.Join(tempDir, "workspace")
	server := httptest.NewServer(gcrregistry.New())
	defer server.Close()
	registryHost := strings.TrimPrefix(server.URL, "http://")
	latestRef := registryHost + "/acme/lite-ci:latest"
	untaggedRef := registryHost + "/acme/lite-ci"

	releaseProviderRef(t, globalHome, providerProject, latestRef)
	runRootCommand(t, []string{"--kiox-home", globalHome, "init", workspaceRoot})
	runRootCommand(t, []string{"--kiox-home", globalHome, "add", untaggedRef, "as", "lite-ci", "--plain-http"})

	lock, err := workspacepkg.LoadLock(workspaceRoot)
	if err != nil {
		t.Fatalf("load initial lock: %v", err)
	}
	if len(lock.Providers) != 1 {
		t.Fatalf("expected one locked provider, got %#v", lock.Providers)
	}
	if !strings.HasPrefix(lock.Providers[0].Resolved, registryHost+"/acme/lite-ci@sha256:") {
		t.Fatalf("expected initial resolved ref to pin an immutable digest, got %q", lock.Providers[0].Resolved)
	}
	initialResolved := lock.Providers[0].Resolved

	setProviderVersion(t, providerProject, "v0.2.0")
	releaseProviderRef(t, globalHome, providerProject, latestRef)

	statusBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "status"})
	if !strings.Contains(statusBuf.String(), "v0.1.0") {
		t.Fatalf("expected locked workspace status to remain on v0.1.0, got:\n%s", statusBuf.String())
	}

	runRootCommand(t, []string{"--kiox-home", globalHome, "provider", "update", "lite-ci"})
	updatedLock, err := workspacepkg.LoadLock(workspaceRoot)
	if err != nil {
		t.Fatalf("load updated lock: %v", err)
	}
	if !strings.HasPrefix(updatedLock.Providers[0].Resolved, registryHost+"/acme/lite-ci@sha256:") {
		t.Fatalf("expected updated resolved ref to pin an immutable digest, got %q", updatedLock.Providers[0].Resolved)
	}
	if updatedLock.Providers[0].Resolved == initialResolved {
		t.Fatalf("expected provider update to move to a new immutable digest")
	}
	updatedStatusBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "status"})
	if !strings.Contains(updatedStatusBuf.String(), "v0.2.0") {
		t.Fatalf("expected updated workspace status to report v0.2.0, got:\n%s", updatedStatusBuf.String())
	}
}

func TestWorkspaceCreateListCurrentAndDelete(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	workspaceRoot := filepath.Join(tempDir, "team")

	createBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "ws", "create", workspaceRoot})
	if !strings.Contains(createBuf.String(), "initialized workspace team") {
		t.Fatalf("unexpected workspace create output: %s", createBuf.String())
	}
	listBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "ws", "list"})
	if !strings.Contains(listBuf.String(), "team") {
		t.Fatalf("expected workspace list to include team, got:\n%s", listBuf.String())
	}
	workspacesBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "workspaces", "list"})
	if !strings.Contains(workspacesBuf.String(), "team") {
		t.Fatalf("expected plural workspace alias to include team, got:\n%s", workspacesBuf.String())
	}
	currentBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "ws", "current"})
	if !strings.Contains(currentBuf.String(), "workspace: team") {
		t.Fatalf("unexpected workspace current output: %s", currentBuf.String())
	}
	deleteBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "ws", "delete", workspaceRoot})
	if !strings.Contains(deleteBuf.String(), "deleted workspace team") {
		t.Fatalf("unexpected workspace delete output: %s", deleteBuf.String())
	}
	for _, path := range []string{
		filepath.Join(workspaceRoot, "kiox.yaml"),
		filepath.Join(workspaceRoot, "kiox.lock"),
		filepath.Join(workspaceRoot, ".workspace"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected workspace artifact %s to be removed", path)
		}
	}
	postDeleteCurrentBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "ws", "current"})
	if !strings.Contains(postDeleteCurrentBuf.String(), "workspace: none") {
		t.Fatalf("expected no active workspace after delete, got: %s", postDeleteCurrentBuf.String())
	}
}

func TestWorkspaceUseFailsCleanlyWhenWorkspaceRootIsMissing(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	workspaceRoot := filepath.Join(tempDir, "interactive-workspace")

	runRootCommand(t, []string{"--kiox-home", globalHome, "ws", "create", workspaceRoot})
	if err := os.RemoveAll(workspaceRoot); err != nil {
		t.Fatalf("remove workspace root: %v", err)
	}

	buf := new(bytes.Buffer)
	err := executeCLI(context.Background(), []string{"--kiox-home", globalHome, "ws", "use", "interactive-workspace"}, buf, buf)
	if err == nil {
		t.Fatal("expected workspace use to fail for missing workspace root")
	}
	for _, expected := range []string{
		"workspace \"interactive-workspace\" is missing",
		"run kiox workspace delete \"interactive-workspace\" to unregister it",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("expected %q in error, got: %v", expected, err)
		}
	}
	if buf.Len() != 0 {
		t.Fatalf("did not expect command output on error, got: %s", buf.String())
	}
	activeWorkspace, err := state.LoadActiveWorkspace(globalHome)
	if err != nil {
		t.Fatalf("load active workspace: %v", err)
	}
	if activeWorkspace != workspaceRoot {
		t.Fatalf("expected missing workspace to remain registered as active until deleted, got %q", activeWorkspace)
	}
}

func TestWorkspaceDeleteUnregistersMissingWorkspaceRoot(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")
	workspaceRoot := filepath.Join(tempDir, "interactive-workspace")

	runRootCommand(t, []string{"--kiox-home", globalHome, "ws", "create", workspaceRoot})
	if err := os.RemoveAll(workspaceRoot); err != nil {
		t.Fatalf("remove workspace root: %v", err)
	}

	deleteBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "ws", "delete", "interactive-workspace"})
	for _, expected := range []string{
		"unregistered missing workspace interactive-workspace",
		"root: " + displayInventoryPath(workspaceRoot),
	} {
		if !strings.Contains(deleteBuf.String(), expected) {
			t.Fatalf("expected %q in delete output, got:\n%s", expected, deleteBuf.String())
		}
	}

	workspaces, err := state.LoadWorkspaces(globalHome)
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}
	if _, ok := workspaces["interactive-workspace"]; ok {
		t.Fatalf("expected missing workspace registration to be removed")
	}
	activeWorkspace, err := state.LoadActiveWorkspace(globalHome)
	if err != nil {
		t.Fatalf("load active workspace: %v", err)
	}
	if activeWorkspace != "" {
		t.Fatalf("expected active workspace to be cleared, got %q", activeWorkspace)
	}
	currentBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "ws", "current"})
	if !strings.Contains(currentBuf.String(), "workspace: none") {
		t.Fatalf("expected no active workspace after unregistering missing root, got: %s", currentBuf.String())
	}
}

func TestInstallRejectsExecutionAfterDash(t *testing.T) {
	tempDir := t.TempDir()
	home := filepath.Join(tempDir, ".kiox-home")
	providerProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	server := httptest.NewServer(gcrregistry.New())
	defer server.Close()
	registryHost := strings.TrimPrefix(server.URL, "http://")
	ref := registryHost + "/acme/lite-ci:v0.1.0"
	releaseProviderRef(t, home, providerProject, ref)

	buf := new(bytes.Buffer)
	err := executeCLI(context.Background(), []string{"--kiox-home", home, "install", ref, "as", "lite-ci", "--plain-http", "--", "lite-ci", "plan"}, buf, buf)
	if err == nil {
		t.Fatal("expected install to reject standalone execution")
	}
	if !strings.Contains(err.Error(), "install no longer executes commands") {
		t.Fatalf("unexpected install error: %v", err)
	}
}

func TestRunCommandExplainsWorkspaceMigration(t *testing.T) {
	tempDir := t.TempDir()
	home := filepath.Join(tempDir, ".kiox-home")
	providerProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	server := httptest.NewServer(gcrregistry.New())
	defer server.Close()
	registryHost := strings.TrimPrefix(server.URL, "http://")
	ref := registryHost + "/acme/lite-ci:v0.1.0"
	releaseProviderRef(t, home, providerProject, ref)

	buf := new(bytes.Buffer)
	err := executeCLI(context.Background(), []string{"--kiox-home", home, "run", ref, "plan", "--plain-http"}, buf, buf)
	if err == nil {
		t.Fatal("expected run to be rejected")
	}
	if !strings.Contains(err.Error(), "'kiox run' has been removed") {
		t.Fatalf("unexpected run error: %v", err)
	}
}

func assertWorkspaceShellOutput(t *testing.T, buf *bytes.Buffer) {
	t.Helper()
	if !bytes.Contains(buf.Bytes(), []byte("lite-ci-args=plan -- node build")) {
		t.Fatalf("expected lite-ci provider output, got: %s", buf.String())
	}
	if !bytes.Contains(buf.Bytes(), []byte("node-args=build")) {
		t.Fatalf("expected node provider output, got: %s", buf.String())
	}
	if !bytes.Contains(buf.Bytes(), []byte("node-env=acme/node")) {
		t.Fatalf("expected provider env to be loaded into workspace shell, got: %s", buf.String())
	}
}

func createLiteCIProviderProject(t *testing.T, dir string) string {
	t.Helper()
	return createCapabilityProviderProject(t, dir, "acme", "lite-ci", "run", `package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	args := os.Args[1:]
	fmt.Printf("lite-ci-env=%s\n", os.Getenv("LITE_CI_PROVIDER_REF"))
	fmt.Printf("lite-ci-args=%s\n", strings.Join(args, " "))
	for index, arg := range args {
		if arg != "--" {
			continue
		}
		if index+1 >= len(args) {
			break
		}
		command := exec.Command(args[index+1], args[index+2:]...)
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		command.Stdin = os.Stdin
		if err := command.Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
}
`)
}

func createNodeProviderProject(t *testing.T, dir string) string {
	t.Helper()
	return createCapabilityProviderProject(t, dir, "acme", "node", "deploy", `package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	fmt.Printf("node-env=%s\n", os.Getenv("NODE_PROVIDER_REF"))
	fmt.Printf("node-args=%s\n", strings.Join(os.Args[1:], " "))
}
`)
}

func createCapabilityProviderProject(t *testing.T, dir, namespace, name, capability, mainSource string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "cmd", name), 0o755); err != nil {
		t.Fatal(err)
	}
	goMod := fmt.Sprintf("module example.com/%s\n\ngo 1.24.0\n", name)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := strings.Join([]string{
		"apiVersion: kiox.io/v1",
		"kind: Provider",
		"",
		"metadata:",
		fmt.Sprintf("  namespace: %s", namespace),
		fmt.Sprintf("  name: %s", name),
		"  version: v0.1.0",
		"",
		"spec:",
		"  runtime: binary",
		fmt.Sprintf("  entrypoint: %s", name),
		"  env:",
		fmt.Sprintf("    %s: ${provider_ref}", manifestEnvName(name)),
		"  platforms:",
		fmt.Sprintf("    - os: %s", goruntime.GOOS),
		fmt.Sprintf("      arch: %s", goruntime.GOARCH),
		fmt.Sprintf("      binary: bin/%s/%s/%s", goruntime.GOOS, goruntime.GOARCH, name),
		"  capabilities:",
		fmt.Sprintf("    %s:", capability),
		"      description: test capability",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "kiox.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmd", name, "main.go"), []byte(mainSource), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func releaseProviderLayout(t *testing.T, home, providerDir string) string {
	t.Helper()
	layoutPath := filepath.Join(providerDir, "oci")
	buf := runRootCommand(t, []string{
		"--kiox-home", home,
		"release",
		"--manifest", filepath.Join(providerDir, "kiox.yaml"),
		"--dist", filepath.Join(providerDir, "dist"),
		"--output", layoutPath,
	})
	if !bytes.Contains(buf.Bytes(), []byte("released acme/")) {
		t.Fatalf("unexpected release output: %s", buf.String())
	}
	return layoutPath
}

func setProviderVersion(t *testing.T, providerDir, version string) {
	t.Helper()
	manifestPath := filepath.Join(providerDir, "kiox.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(data), "  version: v0.1.0", "  version: "+version, 1)
	if updated == string(data) {
		t.Fatalf("provider manifest %s did not contain the default version stanza", manifestPath)
	}
	if err := os.WriteFile(manifestPath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
}

func releaseProviderRef(t *testing.T, home, providerDir, ref string) {
	t.Helper()
	buf := runRootCommand(t, []string{
		"--kiox-home", home,
		"release",
		"--manifest", filepath.Join(providerDir, "kiox.yaml"),
		"--dist", filepath.Join(providerDir, "dist"),
		"--output", filepath.Join(providerDir, "oci"),
		"--push", ref,
		"--plain-http",
	})
	if !bytes.Contains(buf.Bytes(), []byte("pushed "+ref)) {
		t.Fatalf("unexpected release output: %s", buf.String())
	}
}

func loadStoreImageManifest(t *testing.T, layoutPath string) internaloci.ImageManifest {
	t.Helper()
	indexBytes, err := os.ReadFile(filepath.Join(layoutPath, "index.json"))
	if err != nil {
		t.Fatalf("read store index.json: %v", err)
	}
	var index struct {
		Manifests []ocispec.Descriptor `json:"manifests"`
	}
	if err := json.Unmarshal(indexBytes, &index); err != nil {
		t.Fatalf("decode store index.json: %v", err)
	}
	if len(index.Manifests) == 0 {
		t.Fatal("expected cached OCI layout to contain a manifest descriptor")
	}
	manifestBytes, err := os.ReadFile(filepath.Join(layoutPath, "blobs", "sha256", index.Manifests[0].Digest.Encoded()))
	if err != nil {
		t.Fatalf("read cached image manifest: %v", err)
	}
	var manifest internaloci.ImageManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("decode cached image manifest: %v", err)
	}
	return manifest
}

func manifestEnvName(name string) string {
	upper := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	return upper + "_PROVIDER_REF"
}
