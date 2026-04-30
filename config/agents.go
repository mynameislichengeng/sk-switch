package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Agent struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Visible bool   `json:"visible"`
}

const agentsFileName = "agents.json"

func agentsFilePath() string {
	return filepath.Join(ConfigDir(), agentsFileName)
}

func loadAgents() ([]Agent, error) {
	data, err := os.ReadFile(agentsFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read agents.json: %w", err)
	}
	var ag []Agent
	if err := json.Unmarshal(data, &ag); err != nil {
		return nil, fmt.Errorf("parse agents.json: %w", err)
	}
	return ag, nil
}

func saveAgents(ag []Agent) error {
	if err := ensureConfigDir(); err != nil {
		return err
	}
	if ag == nil {
		ag = []Agent{}
	}
	data, err := json.MarshalIndent(ag, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(agentsFilePath(), data, 0644)
}
