package state

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type ConfigFile struct {
	Aliases         map[string]string `yaml:"aliases,omitempty"`
	ActiveWorkspace string            `yaml:"activeWorkspace,omitempty"`
	Workspaces      map[string]string `yaml:"workspaces,omitempty"`
}

func aliasPath(home string) string {
	return filepath.Join(home, "config.yaml")
}

func legacyAliasPath(home string) string {
	return filepath.Join(home, "aliases.yaml")
}

func LoadConfig(home string) (ConfigFile, error) {
	var config ConfigFile
	data, err := os.ReadFile(aliasPath(home))
	if err != nil {
		if os.IsNotExist(err) {
			legacyData, legacyErr := os.ReadFile(legacyAliasPath(home))
			if legacyErr != nil {
				if os.IsNotExist(legacyErr) {
					config.Aliases = map[string]string{}
					config.Workspaces = map[string]string{}
					return config, nil
				}
				return config, fmt.Errorf("read aliases: %w", legacyErr)
			}
			data = legacyData
		} else {
			return config, fmt.Errorf("read aliases: %w", err)
		}
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("decode aliases: %w", err)
	}
	if config.Aliases == nil {
		config.Aliases = map[string]string{}
	}
	if config.Workspaces == nil {
		config.Workspaces = map[string]string{}
	}
	return config, nil
}

func SaveConfig(home string, config ConfigFile) error {
	if config.Aliases == nil {
		config.Aliases = map[string]string{}
	}
	if config.Workspaces == nil {
		config.Workspaces = map[string]string{}
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return fmt.Errorf("create config root: %w", err)
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode aliases: %w", err)
	}
	return os.WriteFile(aliasPath(home), data, 0o644)
}

func LoadAliases(home string) (map[string]string, error) {
	config, err := LoadConfig(home)
	if err != nil {
		return nil, err
	}
	return config.Aliases, nil
}

func SaveAliases(home string, aliases map[string]string) error {
	config, err := LoadConfig(home)
	if err != nil {
		return err
	}
	config.Aliases = aliases
	return SaveConfig(home, config)
}

func LoadActiveWorkspace(home string) (string, error) {
	config, err := LoadConfig(home)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(config.ActiveWorkspace), nil
}

func SaveActiveWorkspace(home, workspaceRoot string) error {
	config, err := LoadConfig(home)
	if err != nil {
		return err
	}
	config.ActiveWorkspace = strings.TrimSpace(workspaceRoot)
	return SaveConfig(home, config)
}

func LoadWorkspaces(home string) (map[string]string, error) {
	config, err := LoadConfig(home)
	if err != nil {
		return nil, err
	}
	return config.Workspaces, nil
}

func RememberWorkspace(home, name, workspaceRoot string) error {
	config, err := LoadConfig(home)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if name == "" || workspaceRoot == "" {
		return nil
	}
	config.Workspaces[name] = workspaceRoot
	return SaveConfig(home, config)
}

func ForgetWorkspace(home, workspaceRoot string) error {
	config, err := LoadConfig(home)
	if err != nil {
		return err
	}
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return nil
	}
	for name, root := range config.Workspaces {
		if strings.TrimSpace(root) == workspaceRoot {
			delete(config.Workspaces, name)
		}
	}
	if strings.TrimSpace(config.ActiveWorkspace) == workspaceRoot {
		config.ActiveWorkspace = ""
	}
	return SaveConfig(home, config)
}

func RegisteredWorkspaceNames(home string) ([]string, error) {
	workspaces, err := LoadWorkspaces(home)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(workspaces))
	for name := range workspaces {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
