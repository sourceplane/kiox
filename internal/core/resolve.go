package core

import (
	"fmt"
	"sort"
	"strings"
)

func ResolveToolPlan(pkg Package, toolName string) ([]Tool, error) {
	trimmed := strings.TrimSpace(toolName)
	if trimmed == "" {
		return nil, fmt.Errorf("tool is required")
	}
	if _, ok := pkg.Tools[trimmed]; !ok {
		return nil, fmt.Errorf("tool %q was not found", trimmed)
	}
	ordered := make([]Tool, 0, len(pkg.Tools))
	permanent := map[string]struct{}{}
	temporary := map[string]struct{}{}
	stack := make([]string, 0, len(pkg.Tools))

	var visit func(name string) error
	visit = func(name string) error {
		if _, ok := permanent[name]; ok {
			return nil
		}
		if _, ok := temporary[name]; ok {
			cycle := append(append([]string(nil), stack...), name)
			return fmt.Errorf("dependency cycle detected: %s", strings.Join(cycle, " -> "))
		}
		tool := pkg.Tools[name]
		temporary[name] = struct{}{}
		stack = append(stack, name)
		deps := make([]string, 0, len(tool.Spec.DependsOn))
		for _, dependency := range tool.Spec.DependsOn {
			deps = append(deps, dependency.Tool)
		}
		sort.Strings(deps)
		for _, dependency := range deps {
			if _, ok := pkg.Tools[dependency]; !ok {
				return fmt.Errorf("tool %s depends on unknown tool %q", name, dependency)
			}
			if err := visit(dependency); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		delete(temporary, name)
		permanent[name] = struct{}{}
		ordered = append(ordered, tool)
		return nil
	}

	if err := visit(trimmed); err != nil {
		return nil, err
	}
	return ordered, nil
}
