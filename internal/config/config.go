package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

var cliWorkspace string

// SetCLIWorkspace records the --workspace global flag value.
func SetCLIWorkspace(slug string) {
	cliWorkspace = slug
}

// CLIWorkspace returns the --workspace global flag value, if set.
func CLIWorkspace() string {
	return cliWorkspace
}

// projectConfig holds keys loaded from the nearest project .linear.toml / linear.toml.
var projectConfig map[string]any

// LoadProjectConfig searches cwd (and optional explicit paths) for project config.
// Callers in tests may pass paths; production should call LoadProjectConfigFromCwd.
func LoadProjectConfig(paths ...string) error {
	projectConfig = nil
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		var parsed map[string]any
		if _, err := toml.Decode(string(data), &parsed); err != nil {
			return err
		}
		projectConfig = parsed
		return nil
	}
	return nil
}

// LoadProjectConfigFromCwd loads ./linear.toml or ./.linear.toml if present.
func LoadProjectConfigFromCwd() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	return LoadProjectConfig(
		filepath.Join(cwd, "linear.toml"),
		filepath.Join(cwd, ".linear.toml"),
	)
}

// GetOption returns a string option from project config (team_id, workspace, api_key, …).
func GetOption(key string) (string, bool) {
	if projectConfig == nil {
		return "", false
	}
	v, ok := projectConfig[key]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}
