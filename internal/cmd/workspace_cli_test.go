package cmd

import (
	"bytes"
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

	initBuf := runRootCommand(t, []string{
		"--tinx-home", globalHome,
		"init", workspaceRoot,
		"-p", liteCILayout, "as", "lite-ci",
		"-p", nodeLayout, "as", "node",
	})
	if !bytes.Contains(initBuf.Bytes(), []byte("initialized workspace my-workspace")) {
		t.Fatalf("unexpected init output: %s", initBuf.String())
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "tinx.yaml")); err != nil {
		t.Fatalf("expected workspace manifest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "tinx.lock")); err != nil {
		t.Fatalf("expected workspace lock file: %v", err)
	}

	useBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "use", workspaceRoot})
	if !bytes.Contains(useBuf.Bytes(), []byte("active workspace: my-workspace")) {
		t.Fatalf("unexpected use output: %s", useBuf.String())
	}

	runBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "--", "lite-ci", "run", "plan", "--", "node", "deploy"})
	assertWorkspaceDispatchOutput(t, runBuf)

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

	runBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "use", workspaceRoot, "--", "lite-ci", "run", "plan", "--", "node", "deploy"})
	assertWorkspaceDispatchOutput(t, runBuf)
}

func TestInstallRefAsAliasDispatchesProviderCommand(t *testing.T) {
	tempDir := t.TempDir()
	home := filepath.Join(tempDir, ".tinx-home")
	providerProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	server := httptest.NewServer(gcrregistry.New())
	defer server.Close()
	registryHost := strings.TrimPrefix(server.URL, "http://")
	ref := registryHost + "/acme/lite-ci:v0.1.0"
	releaseProviderRef(t, home, providerProject, ref)

	runBuf := runRootCommand(t, []string{"--tinx-home", home, "install", ref, "as", "lite-ci", "--plain-http", "--", "lite-ci", "run", "plan"})
	if !bytes.Contains(runBuf.Bytes(), []byte("installed acme/lite-ci@v0.1.0")) {
		t.Fatalf("unexpected install output: %s", runBuf.String())
	}
	if !bytes.Contains(runBuf.Bytes(), []byte("lite-ci-args=run plan")) {
		t.Fatalf("expected dispatched provider execution, got: %s", runBuf.String())
	}
}

func TestRunRefDispatchesUsingBinaryName(t *testing.T) {
	tempDir := t.TempDir()
	home := filepath.Join(tempDir, ".tinx-home")
	providerProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	server := httptest.NewServer(gcrregistry.New())
	defer server.Close()
	registryHost := strings.TrimPrefix(server.URL, "http://")
	ref := registryHost + "/acme/lite-ci:v0.1.0"
	releaseProviderRef(t, home, providerProject, ref)

	runBuf := runRootCommand(t, []string{"--tinx-home", home, "run", ref, "--plain-http", "--", "lite-ci", "run", "plan"})
	if !bytes.Contains(runBuf.Bytes(), []byte("lite-ci-args=run plan")) {
		t.Fatalf("expected dispatched provider execution, got: %s", runBuf.String())
	}
}

func assertWorkspaceDispatchOutput(t *testing.T, buf *bytes.Buffer) {
	t.Helper()
	if !bytes.Contains(buf.Bytes(), []byte("lite-ci-args=run plan -- node deploy")) {
		t.Fatalf("expected lite-ci provider output, got: %s", buf.String())
	}
	if !bytes.Contains(buf.Bytes(), []byte("node-args=deploy")) {
		t.Fatalf("expected node provider output, got: %s", buf.String())
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
	manifest := fmt.Sprintf(`apiVersion: tinx.io/v1
kind: Provider

metadata:
  namespace: %s
  name: %s
  version: v0.1.0

spec:
  runtime: binary
  entrypoint: %s
  platforms:
    - os: %s
      arch: %s
      binary: bin/%s/%s/%s
  capabilities:
    %s:
      description: test capability
`, namespace, name, name, goruntime.GOOS, goruntime.GOARCH, goruntime.GOOS, goruntime.GOARCH, name, capability)
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
