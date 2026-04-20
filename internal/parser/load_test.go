package parser

import (
	"strings"
	"testing"

	"github.com/sourceplane/kiox/internal/core"
)

func TestLoadBytesNormalizesLegacyProvider(t *testing.T) {
	manifest := strings.Join([]string{
		"apiVersion: kiox.io/v1",
		"kind: Provider",
		"metadata:",
		"  namespace: sourceplane",
		"  name: legacy-demo",
		"  version: v0.1.0",
		"spec:",
		"  runtime: binary",
		"  entrypoint: legacy-demo",
		"  env:",
		"    DEMO_REF: ${provider_ref}",
		"  path:",
		"    - assets/bin",
		"  platforms:",
		"    - os: darwin",
		"      arch: arm64",
		"      binary: bin/darwin/arm64/legacy-demo",
		"  layers:",
		"    assets:",
		"      root: assets",
		"",
	}, "\n")
	pkg, err := LoadBytes([]byte(manifest))
	if err != nil {
		t.Fatalf("LoadBytes() error = %v", err)
	}
	if pkg.ProviderRef() != "sourceplane/legacy-demo" {
		t.Fatalf("ProviderRef() = %q", pkg.ProviderRef())
	}
	tool, ok := pkg.DefaultTool()
	if !ok {
		t.Fatal("expected default tool")
	}
	if tool.Spec.Runtime.Type != core.RuntimeOCI {
		t.Fatalf("expected oci runtime, got %q", tool.Spec.Runtime.Type)
	}
	if tool.PrimaryProvide() != "legacy-demo" {
		t.Fatalf("expected primary provide legacy-demo, got %q", tool.PrimaryProvide())
	}
	if _, ok := pkg.Environments["legacy-demo"]; !ok {
		t.Fatal("expected normalized environment")
	}
	if _, ok := pkg.Bundles["legacy-demo"]; !ok {
		t.Fatal("expected normalized bundle")
	}
	if _, ok := pkg.Assets["legacy-demo-assets"]; !ok {
		t.Fatal("expected normalized asset")
	}
}

func TestLoadBytesParsesMultiDocumentProvider(t *testing.T) {
	manifest := strings.Join([]string{
		"apiVersion: kiox.io/v1",
		"kind: Provider",
		"metadata:",
		"  namespace: acme",
		"  name: multi-doc",
		"  version: v0.1.0",
		"spec:",
		"  contents:",
		"    - Tool: setup-tool",
		"    - Tool: default-tool",
		"    - Bundle: setup-tool",
		"    - Environment: demo-env",
		"---",
		"apiVersion: kiox.io/v1",
		"kind: Bundle",
		"metadata:",
		"  name: setup-tool",
		"spec:",
		"  layers:",
		"    - platform:",
		"        os: darwin",
		"        arch: arm64",
		"      source: bin/darwin/arm64/setup-tool",
		"      mediaType: application/vnd.kiox.tool.binary",
		"---",
		"apiVersion: kiox.io/v1",
		"kind: Tool",
		"metadata:",
		"  name: setup-tool",
		"spec:",
		"  runtime:",
		"    type: oci",
		"  source:",
		"    type: bundle",
		"    ref: setup-tool",
		"  provides:",
		"    - setup-tool",
		"---",
		"apiVersion: kiox.io/v1",
		"kind: Tool",
		"metadata:",
		"  name: default-tool",
		"spec:",
		"  default: true",
		"  runtime:",
		"    type: script",
		"  source:",
		"    type: script",
		"    script: setup-tool \"$KIOX_TOOL_BIN\"",
		"  dependsOn:",
		"    - tool: setup-tool",
		"  provides:",
		"    - default-tool",
		"  environments:",
		"    - demo-env",
		"---",
		"apiVersion: kiox.io/v1",
		"kind: Environment",
		"metadata:",
		"  name: demo-env",
		"spec:",
		"  variables:",
		"    DEMO_GREETING: hello",
		"  export:",
		"    - DEMO_GREETING",
		"",
	}, "\n")
	pkg, err := LoadBytes([]byte(manifest))
	if err != nil {
		t.Fatalf("LoadBytes() error = %v", err)
	}
	tool, ok := pkg.DefaultTool()
	if !ok {
		t.Fatal("expected default tool")
	}
	if tool.Metadata.Name != "default-tool" {
		t.Fatalf("expected default-tool, got %q", tool.Metadata.Name)
	}
	if tool.Spec.Runtime.Type != core.RuntimeScript {
		t.Fatalf("expected script runtime, got %q", tool.Spec.Runtime.Type)
	}
	if len(tool.Spec.DependsOn) != 1 || tool.Spec.DependsOn[0].Tool != "setup-tool" {
		t.Fatalf("unexpected dependencies: %#v", tool.Spec.DependsOn)
	}
	if _, ok := pkg.Environments["demo-env"]; !ok {
		t.Fatal("expected demo environment")
	}
	if _, ok := pkg.Tools["setup-tool"]; !ok {
		t.Fatal("expected setup tool")
	}
}

func TestLoadBytesParsesManagedInstallTool(t *testing.T) {
	manifest := strings.Join([]string{
		"apiVersion: kiox.io/v1",
		"kind: Provider",
		"metadata:",
		"  namespace: acme",
		"  name: setup-kubectl",
		"  version: v0.1.0",
		"spec:",
		"  tools:",
		"    - name: setup-kubectl",
		"      runtime:",
		"        type: oci",
		"      source:",
		"        type: bundle",
		"        ref: setup-kubectl",
		"      provides:",
		"        - setup-kubectl",
		"    - name: kubectl",
		"      default: true",
		"      runtime:",
		"        type: local",
		"      install:",
		"        strategy: lazy",
		"        tool: setup-kubectl",
		"        path: bin/kubectl",
		"      dependsOn:",
		"        - tool: setup-kubectl",
		"      provides:",
		"        - kubectl",
		"  bundles:",
		"    - name: setup-kubectl",
		"      layers:",
		"        - platform:",
		"            os: darwin",
		"            arch: arm64",
		"          source: bin/darwin/arm64/setup-kubectl",
		"          mediaType: application/vnd.kiox.tool.binary",
		"",
	}, "\n")
	pkg, err := LoadBytes([]byte(manifest))
	if err != nil {
		t.Fatalf("LoadBytes() error = %v", err)
	}
	tool, ok := pkg.Tool("kubectl")
	if !ok {
		t.Fatal("expected kubectl tool")
	}
	if tool.Spec.Runtime.Type != core.RuntimeLocal {
		t.Fatalf("expected local runtime, got %q", tool.Spec.Runtime.Type)
	}
	if tool.Spec.Install.Tool != "setup-kubectl" {
		t.Fatalf("expected install.tool setup-kubectl, got %q", tool.Spec.Install.Tool)
	}
	if tool.InstallPath() != "bin/kubectl" {
		t.Fatalf("expected install path bin/kubectl, got %q", tool.InstallPath())
	}
}
