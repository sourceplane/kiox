package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gcrregistry "github.com/google/go-containerregistry/pkg/registry"
)

func TestReleaseInstallAndRun(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, ".tinx-home")
	providerDir := copyTestProvider(t, workspace)

	releaseBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"release",
		"--manifest", filepath.Join(providerDir, "tinx.yaml"),
		"--dist", filepath.Join(providerDir, "dist"),
		"--output", filepath.Join(providerDir, "oci"),
	})
	if !bytes.Contains(releaseBuf.Bytes(), []byte("released sourceplane/echo-provider")) {
		t.Fatalf("unexpected release output: %s", releaseBuf.String())
	}

	runInstallAndRunAssertions(t, home, providerDir, filepath.Join(providerDir, "oci"), "")
}

func TestReleaseDelegatesToGoReleaser(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, ".tinx-home")
	providerDir := copyTestProvider(t, workspace)
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(workspace, "goreleaser.log")
	script := fmt.Sprintf(`#!/bin/sh
set -eu
printf '%%s\n' "$*" > %q
mkdir -p dist/bin/darwin/amd64 dist/bin/darwin/arm64 dist/bin/linux/amd64 dist/bin/linux/arm64
go build -o dist/bin/darwin/amd64/echo-provider ./cmd/echo-provider
cp dist/bin/darwin/amd64/echo-provider dist/bin/darwin/arm64/echo-provider
cp dist/bin/darwin/amd64/echo-provider dist/bin/linux/amd64/echo-provider
cp dist/bin/darwin/amd64/echo-provider dist/bin/linux/arm64/echo-provider
`, logPath)
	if err := os.WriteFile(filepath.Join(binDir, "goreleaser"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath)

	releaseBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"release",
		"--manifest", filepath.Join(providerDir, "tinx.yaml"),
		"--dist", filepath.Join(providerDir, "artifacts"),
		"--output", filepath.Join(providerDir, "oci"),
		"--delegate-goreleaser",
	})
	if !bytes.Contains(releaseBuf.Bytes(), []byte("released sourceplane/echo-provider")) {
		t.Fatalf("unexpected release output: %s", releaseBuf.String())
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logged), "build") {
		t.Fatalf("expected goreleaser invocation, got %q", string(logged))
	}
}

func TestReleasePushAndInstallFromRegistry(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, ".tinx-home")
	providerDir := copyTestProvider(t, workspace)
	server := httptest.NewServer(gcrregistry.New())
	defer server.Close()
	registryHost := strings.TrimPrefix(server.URL, "http://")
	ref := registryHost + "/sourceplane/echo-provider:v0.1.0"

	releaseBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"release",
		"--manifest", filepath.Join(providerDir, "tinx.yaml"),
		"--dist", filepath.Join(providerDir, "dist"),
		"--output", filepath.Join(providerDir, "oci"),
		"--push-ref", ref,
		"--plain-http",
	})
	if !bytes.Contains(releaseBuf.Bytes(), []byte("pushed "+ref)) {
		t.Fatalf("unexpected push output: %s", releaseBuf.String())
	}

	installBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"install", "sourceplane/echo-provider",
		"--ref", ref,
		"--alias", "echo",
		"--plain-http",
	})
	if !bytes.Contains(installBuf.Bytes(), []byte("installed sourceplane/echo-provider@v0.1.0")) {
		t.Fatalf("unexpected install output: %s", installBuf.String())
	}

	runBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"run", "echo", "plan", "--intent", "intent.yaml",
	})
	if !bytes.Contains(runBuf.Bytes(), []byte("capability=plan")) {
		t.Fatalf("unexpected run output: %s", runBuf.String())
	}
}

func runInstallAndRunAssertions(t *testing.T, home, providerDir, layoutPath, ref string) {
	t.Helper()
	installArgs := []string{"--tinx-home", home, "install", "sourceplane/echo-provider", "--alias", "echo"}
	if ref != "" {
		installArgs = append(installArgs, "--ref", ref, "--plain-http")
	} else {
		installArgs = append(installArgs, "--source", layoutPath)
	}
	installBuf := runRootCommand(t, installArgs)
	if !bytes.Contains(installBuf.Bytes(), []byte("installed sourceplane/echo-provider@v0.1.0")) {
		t.Fatalf("unexpected install output: %s", installBuf.String())
	}

	runBuf := runRootCommand(t, []string{"--tinx-home", home, "run", "echo", "plan", "--intent", "intent.yaml"})
	if !bytes.Contains(runBuf.Bytes(), []byte("capability=plan")) {
		t.Fatalf("unexpected run output: %s", runBuf.String())
	}

	cachedAsset := filepath.Join(home, "providers", "sourceplane", "echo-provider", "v0.1.0", "assets", "templates", "intent.json")
	if _, err := os.Stat(cachedAsset); err != nil {
		t.Fatalf("expected cached asset at %s: %v", cachedAsset, err)
	}
	_ = providerDir
}

func copyTestProvider(t *testing.T, workspace string) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "echo-provider")
	dst := filepath.Join(workspace, "provider")
	if err := copyTree(src, dst); err != nil {
		t.Fatalf("copy test provider: %v", err)
	}
	return dst
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

func runRootCommand(t *testing.T, args []string) *bytes.Buffer {
	t.Helper()
	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("command %v failed: %v\n%s", args, err, buf.String())
	}
	return buf
}
