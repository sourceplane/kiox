package cmd

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/sourceplane/kiox/internal/state"
)

func TestListWorkspacesShowsCompactWorkspaceScopes(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")

	liteCIProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	nodeProject := createNodeProviderProject(t, filepath.Join(tempDir, "node-provider"))
	liteCILayout := releaseProviderLayout(t, globalHome, liteCIProject)
	nodeLayout := releaseProviderLayout(t, globalHome, nodeProject)
	workspaceRoot := filepath.Join(tempDir, "my-workspace")

	runRootCommand(t, []string{
		"--kiox-home", globalHome,
		"init", workspaceRoot,
		"-p", liteCILayout, "as", "lite-ci",
		"-p", nodeLayout, "as", "node",
	})
	runRootCommand(t, []string{"--kiox-home", globalHome, "use", workspaceRoot})

	standaloneProvider := copyTestProvider(t, filepath.Join(tempDir, "standalone-provider"))
	standaloneLayout := releaseStandaloneProviderLayout(t, globalHome, standaloneProvider)
	runRootCommand(t, []string{
		"--kiox-home", globalHome,
		"install", "sourceplane/echo-provider",
		"--source", standaloneLayout,
	})
	missingRoot := filepath.Join(tempDir, "missing-workspace")
	if err := state.RememberWorkspace(globalHome, "missing-workspace", missingRoot); err != nil {
		t.Fatalf("remember missing workspace: %v", err)
	}

	listBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "ws", "list"})
	output := listBuf.String()
	for _, expected := range []string{
		"NAME",
		"ACTIVE",
		"STATUS",
		"ROOT",
		"*",
		"✓ ready",
		"✗ missing",
		"my-workspace",
		displayInventoryPath(workspaceRoot),
		"missing-workspace",
		compactAbsolutePath(missingRoot),
		"default",
		"(global)",
		"3 workspaces",
		"Active workspace: my-workspace",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in list output, got:\n%s", expected, output)
		}
	}
	for _, unexpected := range []string{
		"Providers in my-workspace:",
		"Providers in default:",
		"acme/lite-ci",
		"sourceplane/echo-provider",
		"TYPE",
	} {
		if strings.Contains(output, unexpected) {
			t.Fatalf("did not expect %q in compact workspace output, got:\n%s", unexpected, output)
		}
	}

	shortBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "ws", "list", "--short"})
	shortOutput := shortBuf.String()
	for _, expected := range []string{"* my-workspace", "  default"} {
		if !strings.Contains(shortOutput, expected) {
			t.Fatalf("expected %q in short workspace output, got:\n%s", expected, shortOutput)
		}
	}
	if strings.Contains(shortOutput, "ROOT") {
		t.Fatalf("did not expect table header in short workspace output, got:\n%s", shortOutput)
	}

	readyBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "ws", "list", "--ready"})
	readyOutput := readyBuf.String()
	if strings.Contains(readyOutput, "missing-workspace") {
		t.Fatalf("did not expect missing workspace in ready output, got:\n%s", readyOutput)
	}

	missingBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "ws", "list", "--missing"})
	missingOutput := missingBuf.String()
	if !strings.Contains(missingOutput, "missing-workspace") || !strings.Contains(missingOutput, "1 workspace (1 missing)") {
		t.Fatalf("unexpected missing workspace output:\n%s", missingOutput)
	}

	activeBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "ws", "list", "--active"})
	activeOutput := activeBuf.String()
	if !strings.Contains(activeOutput, "my-workspace") || strings.Contains(activeOutput, "missing-workspace") {
		t.Fatalf("unexpected active workspace output:\n%s", activeOutput)
	}
}

func TestListProvidersSupportsWorkspaceAndDefaultScopes(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".kiox-global")

	liteCIProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	liteCILayout := releaseProviderLayout(t, globalHome, liteCIProject)
	workspaceRoot := filepath.Join(tempDir, "team-workspace")

	runRootCommand(t, []string{
		"--kiox-home", globalHome,
		"init", workspaceRoot,
		"-p", liteCILayout, "as", "lite-ci",
	})

	standaloneProvider := copyTestProvider(t, filepath.Join(tempDir, "standalone-provider"))
	standaloneLayout := releaseStandaloneProviderLayout(t, globalHome, standaloneProvider)
	runRootCommand(t, []string{
		"--kiox-home", globalHome,
		"install", "sourceplane/echo-provider",
		"--source", standaloneLayout,
	})

	workspaceBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "p", "list", "team-workspace"})
	workspaceOutput := workspaceBuf.String()
	for _, expected := range []string{
		"Scope: team-workspace",
		"Root: " + displayInventoryPath(workspaceRoot),
		"Status: ✓ ready",
		"NAME",
		"STATUS",
		"PROVIDER",
		"VERSION",
		"lite-ci",
		"acme/lite-ci",
		"1 provider (1 ready)",
	} {
		if !strings.Contains(workspaceOutput, expected) {
			t.Fatalf("expected %q in workspace provider output, got:\n%s", expected, workspaceOutput)
		}
	}
	if strings.Contains(workspaceOutput, "sourceplane/echo-provider") {
		t.Fatalf("workspace provider output should not include default-home installs:\n%s", workspaceOutput)
	}

	defaultBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "p", "list", "default"})
	defaultOutput := defaultBuf.String()
	for _, expected := range []string{
		"Scope: default",
		"Root: (global)",
		"Status: ✓ ready",
		"sourceplane/echo-provider",
		"1 provider (1 ready)",
	} {
		if !strings.Contains(defaultOutput, expected) {
			t.Fatalf("expected %q in default provider output, got:\n%s", expected, defaultOutput)
		}
	}
	if strings.Contains(defaultOutput, "acme/lite-ci") {
		t.Fatalf("default provider output should not include workspace-local installs:\n%s", defaultOutput)
	}
}

func TestListShowsToolInventoryForSetupProviderFlow(t *testing.T) {
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

	beforeBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "ls", "kubectl-workspace"})
	beforeOutput := beforeBuf.String()
	for _, expected := range []string{
		"Scope: kubectl-workspace",
		"Tools:",
		"TOOL",
		"COMMANDS",
		"setup-kubectl",
		"kubectl",
		"2 tools (2 lazy)",
	} {
		if !strings.Contains(beforeOutput, expected) {
			t.Fatalf("expected %q in pre-install list output, got:\n%s", expected, beforeOutput)
		}
	}

	execBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "--workspace", workspaceRoot, "--", "kubectl", "version", "--client"})
	execOutput := execBuf.String()
	for _, expected := range []string{
		"kubectl-version=" + version,
		"kubectl-args=version --client",
	} {
		if !strings.Contains(execOutput, expected) {
			t.Fatalf("expected %q in kubectl output, got:\n%s", expected, execOutput)
		}
	}

	afterBuf := runRootCommand(t, []string{"--kiox-home", globalHome, "ls", "kubectl-workspace"})
	afterOutput := afterBuf.String()
	for _, expected := range []string{
		"setup-kubectl",
		"kubectl",
		"2 tools (2 ready)",
	} {
		if !strings.Contains(afterOutput, expected) {
			t.Fatalf("expected %q in post-install list output, got:\n%s", expected, afterOutput)
		}
	}
	if strings.Contains(afterOutput, "2 tools (2 lazy)") {
		t.Fatalf("expected lazy tool statuses to clear after install, got:\n%s", afterOutput)
	}
}

func newFakeKubectlReleaseServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	version := "v1.27.15"
	arch := goruntime.GOARCH
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stable.txt":
			_, _ = w.Write([]byte(version))
		case "/stable-1.27.txt":
			_, _ = w.Write([]byte(version))
		case "/" + version + "/bin/" + goruntime.GOOS + "/" + arch + "/kubectl":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte(strings.Join([]string{
				"#!/bin/sh",
				"set -eu",
				"printf 'kubectl-version=%s\\n' '" + version + "'",
				"printf 'kubectl-args=%s\\n' \"$*\"",
				"",
			}, "\n")))
		default:
			http.NotFound(w, r)
		}
	}))
	return server, version
}

func releaseStandaloneProviderLayout(t *testing.T, home, providerDir string) string {
	t.Helper()
	layoutPath := filepath.Join(providerDir, "oci")
	buf := runRootCommand(t, []string{
		"--kiox-home", home,
		"release",
		"--manifest", filepath.Join(providerDir, "kiox.yaml"),
		"--dist", filepath.Join(providerDir, "dist"),
		"--output", layoutPath,
	})
	if !strings.Contains(buf.String(), "released sourceplane/echo-provider@v0.1.0") {
		t.Fatalf("unexpected release output: %s", buf.String())
	}
	return layoutPath
}
