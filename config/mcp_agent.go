package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// MCPAgent is an "AI agent" target for MCP assignments — distinct from the
// SKILLS-domain Agent. Type selects which MCPWriter handles its config file
// format ("claude-json" for ~/.claude/.claude.json style files; future
// types like "codex-toml" plug in via the writer registry).
//
// Path is the absolute or ~-prefixed path to the agent's MCP config file
// (NOT a directory, unlike SKILLS Agent.Path).
type MCPAgent struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Path    string `json:"path"`
	Visible bool   `json:"visible"`
}

const mcpAgentsFileName = "mcp-agents.json"

func mcpAgentsFilePath() string {
	return filepath.Join(ConfigDir(), mcpAgentsFileName)
}

func loadMCPAgents() ([]MCPAgent, error) {
	data, err := os.ReadFile(mcpAgentsFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read mcp-agents.json: %w", err)
	}
	var list []MCPAgent
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse mcp-agents.json: %w", err)
	}
	return list, nil
}

func saveMCPAgents(list []MCPAgent) error {
	if err := ensureConfigDir(); err != nil {
		return err
	}
	if list == nil {
		list = []MCPAgent{}
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(mcpAgentsFilePath(), data, 0644)
}
