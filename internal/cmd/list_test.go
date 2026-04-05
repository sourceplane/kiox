package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sourceplane/tinx/internal/state"
)

func TestListWorkspacesShowsCompactWorkspaceScopes(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".tinx-global")

	liteCIProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	nodeProject := createNodeProviderProject(t, filepath.Join(tempDir, "node-provider"))
	liteCILayout := releaseProviderLayout(t, globalHome, liteCIProject)
	nodeLayout := releaseProviderLayout(t, globalHome, nodeProject)
	workspaceRoot := filepath.Join(tempDir, "my-workspace")

	runRootCommand(t, []string{
		"--tinx-home", globalHome,
		"init", workspaceRoot,
		"-p", liteCILayout, "as", "lite-ci",
		"-p", nodeLayout, "as", "node",
	})
	runRootCommand(t, []string{"--tinx-home", globalHome, "use", workspaceRoot})

	standaloneProvider := copyTestProvider(t, filepath.Join(tempDir, "standalone-provider"))
	standaloneLayout := releaseStandaloneProviderLayout(t, globalHome, standaloneProvider)
	runRootCommand(t, []string{
		"--tinx-home", globalHome,
		"install", "sourceplane/echo-provider",
		"--source", standaloneLayout,
	})
	missingRoot := filepath.Join(tempDir, "missing-workspace")
	if err := state.RememberWorkspace(globalHome, "missing-workspace", missingRoot); err != nil {
		t.Fatalf("remember missing workspace: %v", err)
	}

	listBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "ws", "list"})
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

	shortBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "ws", "list", "--short"})
	shortOutput := shortBuf.String()
	for _, expected := range []string{"* my-workspace", "  default"} {
		if !strings.Contains(shortOutput, expected) {
			t.Fatalf("expected %q in short workspace output, got:\n%s", expected, shortOutput)
		}
	}
	if strings.Contains(shortOutput, "ROOT") {
		t.Fatalf("did not expect table header in short workspace output, got:\n%s", shortOutput)
	}

	readyBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "ws", "list", "--ready"})
	readyOutput := readyBuf.String()
	if strings.Contains(readyOutput, "missing-workspace") {
		t.Fatalf("did not expect missing workspace in ready output, got:\n%s", readyOutput)
	}

	missingBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "ws", "list", "--missing"})
	missingOutput := missingBuf.String()
	if !strings.Contains(missingOutput, "missing-workspace") || !strings.Contains(missingOutput, "1 workspace (1 missing)") {
		t.Fatalf("unexpected missing workspace output:\n%s", missingOutput)
	}

	activeBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "ws", "list", "--active"})
	activeOutput := activeBuf.String()
	if !strings.Contains(activeOutput, "my-workspace") || strings.Contains(activeOutput, "missing-workspace") {
		t.Fatalf("unexpected active workspace output:\n%s", activeOutput)
	}
}

func TestListProvidersSupportsWorkspaceAndDefaultScopes(t *testing.T) {
	tempDir := t.TempDir()
	globalHome := filepath.Join(tempDir, ".tinx-global")

	liteCIProject := createLiteCIProviderProject(t, filepath.Join(tempDir, "lite-ci-provider"))
	liteCILayout := releaseProviderLayout(t, globalHome, liteCIProject)
	workspaceRoot := filepath.Join(tempDir, "team-workspace")

	runRootCommand(t, []string{
		"--tinx-home", globalHome,
		"init", workspaceRoot,
		"-p", liteCILayout, "as", "lite-ci",
	})

	standaloneProvider := copyTestProvider(t, filepath.Join(tempDir, "standalone-provider"))
	standaloneLayout := releaseStandaloneProviderLayout(t, globalHome, standaloneProvider)
	runRootCommand(t, []string{
		"--tinx-home", globalHome,
		"install", "sourceplane/echo-provider",
		"--source", standaloneLayout,
	})

	workspaceBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "p", "list", "team-workspace"})
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

	defaultBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "p", "list", "default"})
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

func releaseStandaloneProviderLayout(t *testing.T, home, providerDir string) string {
	t.Helper()
	layoutPath := filepath.Join(providerDir, "oci")
	buf := runRootCommand(t, []string{
		"--tinx-home", home,
		"release",
		"--manifest", filepath.Join(providerDir, "tinx.yaml"),
		"--dist", filepath.Join(providerDir, "dist"),
		"--output", layoutPath,
	})
	if !strings.Contains(buf.String(), "released sourceplane/echo-provider@v0.1.0") {
		t.Fatalf("unexpected release output: %s", buf.String())
	}
	return layoutPath
}
