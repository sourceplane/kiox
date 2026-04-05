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

func TestInitWorkspaceFromFlagsAndDispatchWithActiveWorkspace(t *testing.T) {
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
	if _, err := os.Stat(filepath.Join(workspaceRoot, "tinx.yaml")); err != nil {
		t.Fatalf("expected workspace manifest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "tinx.lock")); err != nil {
		t.Fatalf("expected workspace lock file: %v", err)
	}

	activateBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "workspace", "activate", workspaceRoot})
	if !bytes.Contains(activateBuf.Bytes(), []byte("active workspace: my-workspace")) {
		t.Fatalf("unexpected activate output: %s", activateBuf.String())
	}

	runRootCommand(t, []string{"--tinx-home", globalHome, "add", liteCILayout, "as", "lite-ci"})
	runRootCommand(t, []string{"--tinx-home", globalHome, "add", nodeLayout, "as", "node"})

	runBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "--", "lite-ci", "plan", "--", "node", "build"})
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
	runRootCommand(t, []string{"--tinx-home", globalHome, "workspace", "activate", workspaceRoot})
	runRootCommand(t, []string{"--tinx-home", globalHome, "add", liteCILayout, "as", "lite-ci"})

	fakeShell := filepath.Join(tempDir, "fake-shell")
	script := "#!/bin/sh\nset -eu\nprintf 'shell-root=%s\\n' \"$TINX_WORKSPACE_ROOT\"\nprintf 'shell-env-file=%s\\n' \"$TINX_WORKSPACE_ENV_FILE\"\nprintf 'shell-path=%s\\n' \"$PATH\"\n"
	if err := os.WriteFile(fakeShell, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", fakeShell)

	shellBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "--"})
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
