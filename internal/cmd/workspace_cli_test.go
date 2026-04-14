package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	gcrregistry "github.com/google/go-containerregistry/pkg/registry"

	"github.com/sourceplane/tinx/internal/state"
	workspacepkg "github.com/sourceplane/tinx/internal/workspace"
)

func TestInitWorkspaceFromFlagsAutoSelectsWorkspaceAndDispatches(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".tinx-global")
	liteCIProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	nodeProject := createNodeProviderProject(t, filepath.Join(tempDir, "node-provider"))
	liteCILayout := releaseProviderLayout(t, globalHome, liteCIProject)
	nodeLayout := releaseProviderLayout(t, globalHome, nodeProject)
	workspaceRoot := filepath.Join(tempDir, "my-workspace")

	initBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "init", workspaceRoot})
	if !bytes.Contains(initBuf.Bytes(), []byte("initialized workspace my-workspace")) {
		t.Fatalf("unexpected init output: %s", initBuf.String())
	}
	if !bytes.Contains(initBuf.Bytes(), []byte("active workspace: my-workspace")) {
		t.Fatalf("expected init to select the workspace, got: %s", initBuf.String())
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "tinx.yaml")); err != nil {
		t.Fatalf("expected workspace manifest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "tinx.lock")); err != nil {
		t.Fatalf("expected workspace lock file: %v", err)
	}

	runRootCommand(t, []string{"--tinx-home", globalHome, "add", liteCILayout, "as", "lite-ci"})
	runRootCommand(t, []string{"--tinx-home", globalHome, "add", nodeLayout, "as", "node"})

	runBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "exec", "lite-ci", "plan", "--", "node", "build"})
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
	globalHome := filepath.Join(tempDir, ".tinx-global")
	providerDir := filepath.Join(tempDir, "normalized-provider")
	if err := copyTree(filepath.Join("..", "..", "testdata", "multi-tool-provider"), providerDir); err != nil {
		t.Fatalf("copy normalized provider: %v", err)
	}
	layoutPath := releaseProviderLayout(t, globalHome, providerDir)
	workspaceRoot := filepath.Join(tempDir, "normalized-workspace")

	runRootCommand(t, []string{"--tinx-home", globalHome, "init", workspaceRoot})
	runRootCommand(t, []string{"--tinx-home", globalHome, "add", layoutPath, "as", "echo"})

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
	aliasBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "exec", "echo", "one", "two"})
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
	toolBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "exec", "echo-tool", "alpha", "beta"})
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
	globalHome := filepath.Join(tempDir, ".tinx-global")
	providerDir := filepath.Join(tempDir, "inline-provider")
	if err := copyTree(filepath.Join("..", "..", "testdata", "inline-tool-provider"), providerDir); err != nil {
		t.Fatalf("copy inline provider: %v", err)
	}
	layoutPath := releaseProviderLayout(t, globalHome, providerDir)
	workspaceRoot := filepath.Join(tempDir, "inline-workspace")

	runRootCommand(t, []string{"--tinx-home", globalHome, "init", workspaceRoot})
	runRootCommand(t, []string{"--tinx-home", globalHome, "add", layoutPath, "as", "inline"})

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

	aliasBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "exec", "inline", "red", "blue"})
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
	toolBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "exec", "inline-tool", "green", "gold"})
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
	globalHome := filepath.Join(tempDir, ".tinx-global")
	projectRoot := filepath.Join(tempDir, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(projectRoot)

	initBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "init"})
	workspaceName := filepath.Base(projectRoot)
	if !bytes.Contains(initBuf.Bytes(), []byte("initialized workspace "+workspaceName)) {
		t.Fatalf("unexpected init output: %s", initBuf.String())
	}
	if _, err := os.Stat(filepath.Join(projectRoot, "tinx.yaml")); err != nil {
		t.Fatalf("expected workspace manifest: %v", err)
	}
	currentBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "ws", "current"})
	if !bytes.Contains(currentBuf.Bytes(), []byte("workspace: "+workspaceName)) {
		t.Fatalf("unexpected current workspace output: %s", currentBuf.String())
	}
}

func TestInitWorkspaceFromConfigFileAndUseOneShotCommand(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".tinx-global")
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

	initBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "init", configPath})
	if !bytes.Contains(initBuf.Bytes(), []byte("initialized workspace dev")) {
		t.Fatalf("unexpected init output: %s", initBuf.String())
	}
	if !bytes.Contains(initBuf.Bytes(), []byte("active workspace: dev")) {
		t.Fatalf("expected init to select the workspace, got: %s", initBuf.String())
	}
	materializedManifest := filepath.Join(workspaceRoot, "tinx.yaml")
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

	runBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "--workspace", workspaceRoot, "--", "lite-ci", "plan", "--", "node", "build"})
	assertWorkspaceShellOutput(t, runBuf)
}

func TestInteractiveWorkspaceShellUsesWorkspaceEnvironment(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".tinx-global")
	liteCIProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	liteCILayout := releaseProviderLayout(t, globalHome, liteCIProject)
	workspaceRoot := filepath.Join(tempDir, "interactive-workspace")

	runRootCommand(t, []string{"--tinx-home", globalHome, "init", workspaceRoot})
	runRootCommand(t, []string{"--tinx-home", globalHome, "workspace", "use", workspaceRoot})
	runRootCommand(t, []string{"--tinx-home", globalHome, "add", liteCILayout, "as", "lite-ci"})

	fakeShell := filepath.Join(tempDir, "fake-shell")
	script := "#!/bin/sh\nset -eu\nprintf 'shell-root=%s\\n' \"$TINX_WORKSPACE_ROOT\"\nprintf 'shell-env-file=%s\\n' \"$TINX_WORKSPACE_ENV_FILE\"\nprintf 'shell-path=%s\\n' \"$PATH\"\n"
	if err := os.WriteFile(fakeShell, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", fakeShell)

	shellBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "shell"})
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
	globalHome := filepath.Join(tempDir, ".tinx-global")
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
	if err := workspacepkg.Save(filepath.Join(workspaceRoot, "tinx.yaml"), workspacepkg.Config{
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
	script := "#!/bin/sh\nset -eu\nprintf 'shell-root=%s\\n' \"$TINX_WORKSPACE_ROOT\"\nprintf 'shell-env-file=%s\\n' \"$TINX_WORKSPACE_ENV_FILE\"\nprintf 'shell-path=%s\\n' \"$PATH\"\n"
	if err := os.WriteFile(fakeShell, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", fakeShell)

	shellBuf := new(bytes.Buffer)
	if err := executeCLI(context.Background(), []string{"--tinx-home", globalHome, "--"}, shellBuf, shellBuf); err != nil {
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
	globalHome := filepath.Join(tempDir, ".tinx-global")
	liteCIProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	server := httptest.NewServer(gcrregistry.New())
	registryHost := strings.TrimPrefix(server.URL, "http://")
	ref := registryHost + "/acme/lite-ci:0.1.1"
	releaseProviderRef(t, globalHome, liteCIProject, ref)

	workspaceRoot := filepath.Join(tempDir, "cached-tagged-workspace")
	runRootCommand(t, []string{"--tinx-home", globalHome, "init", workspaceRoot})
	runRootCommand(t, []string{"--tinx-home", globalHome, "add", ref, "as", "lite-ci", "--plain-http"})

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

	server.Close()

	runBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "--workspace", workspaceRoot, "--", "lite-ci", "plan"})
	if !strings.Contains(runBuf.String(), "lite-ci-env=acme/lite-ci") {
		t.Fatalf("expected cached provider output, got: %s", runBuf.String())
	}
	if !strings.Contains(runBuf.String(), "lite-ci-args=plan") {
		t.Fatalf("expected cached command args, got: %s", runBuf.String())
	}
}

func TestProviderCommandAliasesAndStatus(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".tinx-global")
	liteCIProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	liteCILayout := releaseProviderLayout(t, globalHome, liteCIProject)
	workspaceRoot := filepath.Join(tempDir, "dev")

	runRootCommand(t, []string{"--tinx-home", globalHome, "init", workspaceRoot})
	runRootCommand(t, []string{"--tinx-home", globalHome, "p", "add", liteCILayout, "as", "lite-ci"})

	statusBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "status"})
	for _, expected := range []string{
		"tinx workspace: dev",
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
	statusShortBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "status", "--short"})
	if !strings.Contains(statusShortBuf.String(), "dev | 1 providers | shims active") {
		t.Fatalf("unexpected short status output: %s", statusShortBuf.String())
	}
	statusVerboseBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "status", "--verbose"})
	for _, expected := range []string{
		"details:",
		"env file: " + displayRelativePath(filepath.Join(workspaceRoot, ".workspace", "env")),
		"path file: " + displayRelativePath(filepath.Join(workspaceRoot, ".workspace", "path")),
	} {
		if !strings.Contains(statusVerboseBuf.String(), expected) {
			t.Fatalf("expected %q in verbose status output, got:\n%s", expected, statusVerboseBuf.String())
		}
	}
	listBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "list"})
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

	updateBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "update", "lite-ci"})
	if !strings.Contains(updateBuf.String(), "updated providers: lite-ci") {
		t.Fatalf("unexpected provider update output: %s", updateBuf.String())
	}

	removeBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "remove", "lite-ci"})
	if !strings.Contains(removeBuf.String(), "removed provider lite-ci") {
		t.Fatalf("unexpected provider remove output: %s", removeBuf.String())
	}
	config, err := workspacepkg.Load(filepath.Join(workspaceRoot, "tinx.yaml"))
	if err != nil {
		t.Fatalf("load workspace after provider remove: %v", err)
	}
	if config.HasProviderAlias("lite-ci") {
		t.Fatalf("expected provider alias to be removed from workspace manifest")
	}
	providersListBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "p", "list"})
	if !strings.Contains(providersListBuf.String(), "no providers installed") {
		t.Fatalf("expected provider list to be empty after removal, got:\n%s", providersListBuf.String())
	}
}

func TestStatusShowsToolInventoryForSetupProviderFlow(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".tinx-global")
	providerDir := filepath.Join(tempDir, "setup-kubectl-provider")
	if err := copyTree(filepath.Join("..", "..", "testdata", "setup-kubectl"), providerDir); err != nil {
		t.Fatalf("copy setup-kubectl provider: %v", err)
	}
	layoutPath := releaseProviderLayout(t, globalHome, providerDir)
	workspaceRoot := filepath.Join(tempDir, "kubectl-workspace")

	runRootCommand(t, []string{"--tinx-home", globalHome, "init", workspaceRoot})
	runRootCommand(t, []string{"--tinx-home", globalHome, "add", layoutPath, "as", "setup-kubectl"})
	server, version := newFakeKubectlReleaseServer(t)
	defer server.Close()
	t.Setenv("KUBECTL_RELEASE_BASE_URL", server.URL)
	t.Setenv("KUBECTL_VERSION", "1.27")

	beforeBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "--workspace", workspaceRoot, "status"})
	beforeOutput := beforeBuf.String()
	for _, expected := range []string{
		"tinx workspace: kubectl-workspace",
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

	execBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "--workspace", workspaceRoot, "--", "kubectl", "version", "--client"})
	for _, expected := range []string{
		"kubectl-version=" + version,
		"kubectl-args=version --client",
	} {
		if !strings.Contains(execBuf.String(), expected) {
			t.Fatalf("expected %q in kubectl output, got:\n%s", expected, execBuf.String())
		}
	}

	afterBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "--workspace", workspaceRoot, "status"})
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
	globalHome := filepath.Join(tempDir, ".tinx-global")
	providerProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	layoutPath := releaseProviderLayout(t, globalHome, providerProject)
	workspaceOne := filepath.Join(tempDir, "workspace-one")
	workspaceTwo := filepath.Join(tempDir, "workspace-two")

	runRootCommand(t, []string{"--tinx-home", globalHome, "init", workspaceOne})
	runRootCommand(t, []string{"--tinx-home", globalHome, "add", layoutPath, "as", "lite-ci"})
	runRootCommand(t, []string{"--tinx-home", globalHome, "init", workspaceTwo})
	runRootCommand(t, []string{"--tinx-home", globalHome, "add", layoutPath, "as", "lite-ci"})

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

func TestWorkspaceLockPinsUnversionedRegistrySourceUntilUpdate(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".tinx-global")
	providerProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	workspaceRoot := filepath.Join(tempDir, "workspace")
	server := httptest.NewServer(gcrregistry.New())
	defer server.Close()
	registryHost := strings.TrimPrefix(server.URL, "http://")
	latestRef := registryHost + "/acme/lite-ci:latest"
	untaggedRef := registryHost + "/acme/lite-ci"

	releaseProviderRef(t, globalHome, providerProject, latestRef)
	runRootCommand(t, []string{"--tinx-home", globalHome, "init", workspaceRoot})
	runRootCommand(t, []string{"--tinx-home", globalHome, "add", untaggedRef, "as", "lite-ci", "--plain-http"})

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

	statusBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "status"})
	if !strings.Contains(statusBuf.String(), "v0.1.0") {
		t.Fatalf("expected locked workspace status to remain on v0.1.0, got:\n%s", statusBuf.String())
	}

	runRootCommand(t, []string{"--tinx-home", globalHome, "provider", "update", "lite-ci"})
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
	updatedStatusBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "status"})
	if !strings.Contains(updatedStatusBuf.String(), "v0.2.0") {
		t.Fatalf("expected updated workspace status to report v0.2.0, got:\n%s", updatedStatusBuf.String())
	}
}

func TestWorkspaceCreateListCurrentAndDelete(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".tinx-global")
	workspaceRoot := filepath.Join(tempDir, "team")

	createBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "ws", "create", workspaceRoot})
	if !strings.Contains(createBuf.String(), "initialized workspace team") {
		t.Fatalf("unexpected workspace create output: %s", createBuf.String())
	}
	listBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "ws", "list"})
	if !strings.Contains(listBuf.String(), "team") {
		t.Fatalf("expected workspace list to include team, got:\n%s", listBuf.String())
	}
	workspacesBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "workspaces", "list"})
	if !strings.Contains(workspacesBuf.String(), "team") {
		t.Fatalf("expected plural workspace alias to include team, got:\n%s", workspacesBuf.String())
	}
	currentBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "ws", "current"})
	if !strings.Contains(currentBuf.String(), "workspace: team") {
		t.Fatalf("unexpected workspace current output: %s", currentBuf.String())
	}
	deleteBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "ws", "delete", workspaceRoot})
	if !strings.Contains(deleteBuf.String(), "deleted workspace team") {
		t.Fatalf("unexpected workspace delete output: %s", deleteBuf.String())
	}
	for _, path := range []string{
		filepath.Join(workspaceRoot, "tinx.yaml"),
		filepath.Join(workspaceRoot, "tinx.lock"),
		filepath.Join(workspaceRoot, ".workspace"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected workspace artifact %s to be removed", path)
		}
	}
	postDeleteCurrentBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "ws", "current"})
	if !strings.Contains(postDeleteCurrentBuf.String(), "workspace: none") {
		t.Fatalf("expected no active workspace after delete, got: %s", postDeleteCurrentBuf.String())
	}
}

func TestWorkspaceUseFailsCleanlyWhenWorkspaceRootIsMissing(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".tinx-global")
	workspaceRoot := filepath.Join(tempDir, "interactive-workspace")

	runRootCommand(t, []string{"--tinx-home", globalHome, "ws", "create", workspaceRoot})
	if err := os.RemoveAll(workspaceRoot); err != nil {
		t.Fatalf("remove workspace root: %v", err)
	}

	buf := new(bytes.Buffer)
	err := executeCLI(context.Background(), []string{"--tinx-home", globalHome, "ws", "use", "interactive-workspace"}, buf, buf)
	if err == nil {
		t.Fatal("expected workspace use to fail for missing workspace root")
	}
	for _, expected := range []string{
		"workspace \"interactive-workspace\" is missing",
		"run tinx workspace delete \"interactive-workspace\" to unregister it",
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
	globalHome := filepath.Join(tempDir, ".tinx-global")
	workspaceRoot := filepath.Join(tempDir, "interactive-workspace")

	runRootCommand(t, []string{"--tinx-home", globalHome, "ws", "create", workspaceRoot})
	if err := os.RemoveAll(workspaceRoot); err != nil {
		t.Fatalf("remove workspace root: %v", err)
	}

	deleteBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "ws", "delete", "interactive-workspace"})
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
	currentBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "ws", "current"})
	if !strings.Contains(currentBuf.String(), "workspace: none") {
		t.Fatalf("expected no active workspace after unregistering missing root, got: %s", currentBuf.String())
	}
}

func TestInstallRejectsExecutionAfterDash(t *testing.T) {
	tempDir := t.TempDir()
	home := filepath.Join(tempDir, ".tinx-home")
	providerProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	server := httptest.NewServer(gcrregistry.New())
	defer server.Close()
	registryHost := strings.TrimPrefix(server.URL, "http://")
	ref := registryHost + "/acme/lite-ci:v0.1.0"
	releaseProviderRef(t, home, providerProject, ref)

	buf := new(bytes.Buffer)
	err := executeCLI(context.Background(), []string{"--tinx-home", home, "install", ref, "as", "lite-ci", "--plain-http", "--", "lite-ci", "plan"}, buf, buf)
	if err == nil {
		t.Fatal("expected install to reject standalone execution")
	}
	if !strings.Contains(err.Error(), "install no longer executes commands") {
		t.Fatalf("unexpected install error: %v", err)
	}
}

func TestRunCommandExplainsWorkspaceMigration(t *testing.T) {
	tempDir := t.TempDir()
	home := filepath.Join(tempDir, ".tinx-home")
	providerProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	server := httptest.NewServer(gcrregistry.New())
	defer server.Close()
	registryHost := strings.TrimPrefix(server.URL, "http://")
	ref := registryHost + "/acme/lite-ci:v0.1.0"
	releaseProviderRef(t, home, providerProject, ref)

	buf := new(bytes.Buffer)
	err := executeCLI(context.Background(), []string{"--tinx-home", home, "run", ref, "plan", "--plain-http"}, buf, buf)
	if err == nil {
		t.Fatal("expected run to be rejected")
	}
	if !strings.Contains(err.Error(), "'tinx run' has been removed") {
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
		"apiVersion: tinx.io/v1",
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
	if err := os.WriteFile(filepath.Join(dir, "tinx.yaml"), []byte(manifest), 0o644); err != nil {
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
		"--tinx-home", home,
		"release",
		"--manifest", filepath.Join(providerDir, "tinx.yaml"),
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
	manifestPath := filepath.Join(providerDir, "tinx.yaml")
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
		"--tinx-home", home,
		"release",
		"--manifest", filepath.Join(providerDir, "tinx.yaml"),
		"--dist", filepath.Join(providerDir, "dist"),
		"--output", filepath.Join(providerDir, "oci"),
		"--push", ref,
		"--plain-http",
	})
	if !bytes.Contains(buf.Bytes(), []byte("pushed "+ref)) {
		t.Fatalf("unexpected release output: %s", buf.String())
	}
}

func manifestEnvName(name string) string {
	upper := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	return upper + "_PROVIDER_REF"
}
