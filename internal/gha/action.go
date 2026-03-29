package gha

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Action struct {
	Name        string                  `yaml:"name"`
	Description string                  `yaml:"description"`
	Inputs      map[string]ActionInput  `yaml:"inputs"`
	Outputs     map[string]ActionOutput `yaml:"outputs"`
	Runs        ActionRuns              `yaml:"runs"`
}

type ActionInput struct {
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
	Default     any    `yaml:"default"`
}

type ActionOutput struct {
	Description string `yaml:"description"`
	Value       string `yaml:"value"`
}

type ActionRuns struct {
	Using string       `yaml:"using"`
	Main  string       `yaml:"main"`
	Image string       `yaml:"image"`
	Steps []ActionStep `yaml:"steps"`
}

type ActionStep struct {
	ID               string            `yaml:"id"`
	Name             string            `yaml:"name"`
	Run              string            `yaml:"run"`
	Shell            string            `yaml:"shell"`
	WorkingDirectory string            `yaml:"working-directory"`
	Env              map[string]string `yaml:"env"`
	Uses             string            `yaml:"uses"`
	With             map[string]any    `yaml:"with"`
}

func LoadAction(actionDir string) (Action, string, error) {
	var action Action
	for _, candidate := range []string{"action.yml", "action.yaml"} {
		manifestPath := filepath.Join(actionDir, candidate)
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return action, "", fmt.Errorf("read action manifest: %w", err)
		}
		if err := yaml.Unmarshal(data, &action); err != nil {
			return action, "", fmt.Errorf("decode action manifest: %w", err)
		}
		if strings.TrimSpace(action.Runs.Using) == "" {
			return action, "", fmt.Errorf("action manifest %s is missing runs.using", manifestPath)
		}
		return action, candidate, nil
	}
	return action, "", fmt.Errorf("action manifest not found in %s", actionDir)
}

func (a Action) Summary() string {
	if description := strings.TrimSpace(a.Description); description != "" {
		return description
	}
	return strings.TrimSpace(a.Name)
}

func scalarString(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func ProviderRuntime(using string) (string, error) {
	switch {
	case IsCompositeRuntime(using):
		return RuntimeComposite, nil
	case IsNodeRuntime(using):
		return RuntimeNode, nil
	default:
		return "", fmt.Errorf("GitHub Action runtime %q is not supported yet", strings.TrimSpace(using))
	}
}

func IsCompositeRuntime(using string) bool {
	return normalizeRuntime(using) == "composite"
}

func IsNodeRuntime(using string) bool {
	return strings.HasPrefix(normalizeRuntime(using), "node")
}

func MainEntrypoint(action Action) (string, error) {
	return cleanActionRelativePath(action.Runs.Main, "runs.main")
}

func cleanActionRelativePath(value, field string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("GitHub Action %s is required", field)
	}
	cleaned := path.Clean(trimmed)
	switch {
	case cleaned == ".":
		return "", fmt.Errorf("GitHub Action %s must point to a file", field)
	case cleaned == "..", strings.HasPrefix(cleaned, "../"), strings.HasPrefix(cleaned, "/"):
		return "", fmt.Errorf("GitHub Action %s must stay within the repository", field)
	default:
		return cleaned, nil
	}
}

func normalizeRuntime(using string) string {
	return strings.ToLower(strings.TrimSpace(using))
}
