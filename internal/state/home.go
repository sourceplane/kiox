package state

import (
	"fmt"
	"os"
	"path/filepath"
)

func ResolveHome(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if env := os.Getenv("TINX_HOME"); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".tinx"), nil
}

func EnsureHome(root string) error {
	for _, dir := range []string{root, filepath.Join(root, "providers")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}
