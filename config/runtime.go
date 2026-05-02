package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Runtime captures session-spanning UI state and one-shot bootstrap flags.
//
// LastModule remembers which top-level module ("skills"/"mcp"/"settings") the
// user last entered, so subsequent launches skip the entry modal. Empty value
// (default) plus FirstRun=true signals the entry modal must be shown.
type Runtime struct {
	FirstRun   bool   `yaml:"first_run"`
	LastModule string `yaml:"last_module,omitempty"`
}

const runtimeFileName = "runtime-config.yaml"

func runtimeFilePath() string {
	return filepath.Join(ConfigDir(), runtimeFileName)
}

func LoadRuntime() (*Runtime, error) { return loadRuntime() }

func loadRuntime() (*Runtime, error) {
	data, err := os.ReadFile(runtimeFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return &Runtime{FirstRun: true}, nil
		}
		return nil, fmt.Errorf("read runtime-config.yaml: %w", err)
	}
	var r Runtime
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse runtime-config.yaml: %w", err)
	}
	return &r, nil
}

func SaveRuntime(r *Runtime) error { return saveRuntime(r) }

func saveRuntime(r *Runtime) error {
	if err := ensureConfigDir(); err != nil {
		return err
	}
	data, err := yaml.Marshal(r)
	if err != nil {
		return err
	}
	return os.WriteFile(runtimeFilePath(), data, 0644)
}
