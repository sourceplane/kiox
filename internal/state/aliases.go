package state

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type AliasFile struct {
	Aliases map[string]string `yaml:"aliases"`
}

func aliasPath(home string) string {
	return filepath.Join(home, "config.yaml")
}

func legacyAliasPath(home string) string {
	return filepath.Join(home, "aliases.yaml")
}

func LoadAliases(home string) (map[string]string, error) {
	data, err := os.ReadFile(aliasPath(home))
	if err != nil {
		if os.IsNotExist(err) {
			legacyData, legacyErr := os.ReadFile(legacyAliasPath(home))
			if legacyErr != nil {
				if os.IsNotExist(legacyErr) {
					return map[string]string{}, nil
				}
				return nil, fmt.Errorf("read aliases: %w", legacyErr)
			}
			data = legacyData
		} else {
			return nil, fmt.Errorf("read aliases: %w", err)
		}
	}
	var file AliasFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("decode aliases: %w", err)
	}
	if file.Aliases == nil {
		file.Aliases = map[string]string{}
	}
	return file.Aliases, nil
}

func SaveAliases(home string, aliases map[string]string) error {
	data, err := yaml.Marshal(AliasFile{Aliases: aliases})
	if err != nil {
		return fmt.Errorf("encode aliases: %w", err)
	}
	return os.WriteFile(aliasPath(home), data, 0o644)
}
