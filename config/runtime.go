package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Runtime struct {
	FirstRun bool `yaml:"first_run"`
}

const runtimeFileName = "runtime-config.yaml"

func runtimeFilePath() string {
	return filepath.Join(ConfigDir(), runtimeFileName)
}

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
