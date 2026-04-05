package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestListWorkspacesShowsWorkspaceAndDefaultProviders(t *testing.T) {
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

	listBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "list", "workspaces"})
	output := listBuf.String()
	for _, expected := range []string{
		"my-workspace",
		"default",
		"Providers in my-workspace:",
		"Providers in default:",
		"lite-ci",
		"node",
		"sourceplane/echo-provider",
		"echo-provider",
		"tinx lite-ci <capability>",
		"tinx run sourceplane/echo-provider <capability>",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in list output, got:\n%s", expected, output)
		}
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

	workspaceBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "list", "providers", "team-workspace"})
	workspaceOutput := workspaceBuf.String()
	for _, expected := range []string{
		"Scope: team-workspace",
		"Type: workspace",
		"lite-ci",
		"acme/lite-ci",
	} {
		if !strings.Contains(workspaceOutput, expected) {
			t.Fatalf("expected %q in workspace provider output, got:\n%s", expected, workspaceOutput)
		}
	}
	if strings.Contains(workspaceOutput, "sourceplane/echo-provider") {
		t.Fatalf("workspace provider output should not include default-home installs:\n%s", workspaceOutput)
	}

	defaultBuf := runRootCommand(t, []string{"--tinx-home", globalHome, "list", "providers", "default"})
	defaultOutput := defaultBuf.String()
	for _, expected := range []string{
		"Scope: default",
		"Type: default",
		"sourceplane/echo-provider",
		"tinx run sourceplane/echo-provider <capability>",
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
