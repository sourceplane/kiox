package core

import "testing"

func TestResolveToolPlanOrdersDependencies(t *testing.T) {
	pkg := Package{
		Tools: map[string]Tool{
			"setup": {Metadata: Metadata{Name: "setup"}, Spec: ToolSpec{Runtime: RuntimeSpec{Type: RuntimeOCI}, Source: SourceSpec{Type: SourceBundle, Ref: "setup"}}},
			"echo": {
				Metadata: Metadata{Name: "echo"},
				Spec: ToolSpec{
					Runtime:   RuntimeSpec{Type: RuntimeScript},
					Source:    SourceSpec{Type: SourceScript, Script: "setup"},
					DependsOn: []ToolDependency{{Tool: "setup"}},
				},
			},
		},
		Bundles:  map[string]Bundle{"setup": {Metadata: Metadata{Name: "setup"}, Spec: BundleSpec{Layers: []BundleLayer{{Platform: PlatformSpec{OS: "darwin", Arch: "arm64"}, Source: "bin/darwin/arm64/setup"}}}}},
		Provider: Provider{Metadata: Metadata{Namespace: "acme", Name: "demo", Version: "v0.1.0"}},
	}
	plan, err := ResolveToolPlan(pkg, "echo")
	if err != nil {
		t.Fatalf("ResolveToolPlan() error = %v", err)
	}
	if len(plan) != 2 {
		t.Fatalf("expected 2 tools in plan, got %d", len(plan))
	}
	if plan[0].Metadata.Name != "setup" || plan[1].Metadata.Name != "echo" {
		t.Fatalf("unexpected plan order: %#v", plan)
	}
}

func TestResolveToolPlanDetectsCycles(t *testing.T) {
	pkg := Package{
		Tools: map[string]Tool{
			"a": {Metadata: Metadata{Name: "a"}, Spec: ToolSpec{Runtime: RuntimeSpec{Type: RuntimeScript}, Source: SourceSpec{Type: SourceScript, Script: "true"}, DependsOn: []ToolDependency{{Tool: "b"}}}},
			"b": {Metadata: Metadata{Name: "b"}, Spec: ToolSpec{Runtime: RuntimeSpec{Type: RuntimeScript}, Source: SourceSpec{Type: SourceScript, Script: "true"}, DependsOn: []ToolDependency{{Tool: "a"}}}},
		},
		Provider: Provider{Metadata: Metadata{Namespace: "acme", Name: "cycle", Version: "v0.1.0"}},
	}
	if _, err := ResolveToolPlan(pkg, "a"); err == nil {
		t.Fatal("expected dependency cycle error")
	}
}
