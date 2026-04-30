package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type DataSource struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Count   int    `json:"count"`
	Visible bool   `json:"visible"`
}

const sourceFileName = "source.json"

func sourceFilePath() string {
	return filepath.Join(ConfigDir(), sourceFileName)
}

func loadSources() ([]DataSource, error) {
	data, err := os.ReadFile(sourceFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read source.json: %w", err)
	}
	var src []DataSource
	if err := json.Unmarshal(data, &src); err != nil {
		return nil, fmt.Errorf("parse source.json: %w", err)
	}
	return src, nil
}

func saveSources(src []DataSource) error {
	if err := ensureConfigDir(); err != nil {
		return err
	}
	if src == nil {
		src = []DataSource{}
	}
	data, err := json.MarshalIndent(src, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sourceFilePath(), data, 0644)
}
