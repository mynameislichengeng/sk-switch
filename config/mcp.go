package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MCP represents a single Model Context Protocol server registration managed
// by sk-switch.
//
// mcp-data.json is the source of truth for both the MCP itself and the set
// of agents it has been assigned to (Assignments). The actual
// per-agent config file (e.g. ~/.claude/.claude.json) is only touched at
// assign/unassign time; "is this assigned" queries always answer from
// Assignments here, not by re-scanning agent files.
//
// Config is stored as a raw JSON value (the object that gets written under
// mcpServers[name]). It is validated to be a JSON object on add/update.
type MCP struct {
	Name        string          `json:"name"`
	GithubURL   string          `json:"github,omitempty"`
	Config      json.RawMessage `json:"config"`
	Assignments []string        `json:"assignments,omitempty"`
}

const mcpDataFileName = "mcp-data.json"

func mcpDataFilePath() string {
	return filepath.Join(ConfigDir(), mcpDataFileName)
}

// MCP-domain sentinel errors — exported so both the TUI and a future CLI can
// branch on them via errors.Is().
var (
	ErrMCPNameExists       = errors.New("MCP 名称已存在")
	ErrMCPGithubExists     = errors.New("MCP github 地址已存在")
	ErrMCPNotFound         = errors.New("MCP 不存在")
	ErrMCPAgentNotFound    = errors.New("MCP agent 不存在")
	ErrMCPHasAssignments   = errors.New("MCP 已被分配到 agent，无法操作")
	ErrMCPAgentInUse       = errors.New("MCP agent 上仍有 MCP 分配，无法删除")
	ErrMCPInvalidConfig    = errors.New("config 必须是 JSON 对象")
	ErrMCPAgentTypeUnknown = errors.New("未注册的 MCP agent 类型")
)

// MCPConflict is returned by AssignMCP when the agent's existing config file
// already declares the MCP with a different payload. The caller (TUI / CLI)
// can show ExistingRaw vs NewRaw side-by-side and call AssignMCP again with
// force=true to overwrite.
type MCPConflict struct {
	MCPName     string
	AgentName   string
	ExistingRaw json.RawMessage
	NewRaw      json.RawMessage
}

func (e *MCPConflict) Error() string {
	return fmt.Sprintf("MCP %q 在 agent %q 的配置文件中已存在但内容不同", e.MCPName, e.AgentName)
}

func loadMCPs() ([]MCP, error) {
	data, err := os.ReadFile(mcpDataFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read mcp-data.json: %w", err)
	}
	var list []MCP
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse mcp-data.json: %w", err)
	}
	return list, nil
}

func saveMCPs(list []MCP) error {
	if err := ensureConfigDir(); err != nil {
		return err
	}
	if list == nil {
		list = []MCP{}
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(mcpDataFilePath(), data, 0644)
}

// ValidateMCPConfig parses raw bytes and returns ErrMCPInvalidConfig when the
// payload is not a JSON object (array / scalar / malformed). Used by AddMCP /
// UpdateMCP to reject bad input early.
func ValidateMCPConfig(raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("%w: 配置为空", ErrMCPInvalidConfig)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("%w: %v", ErrMCPInvalidConfig, err)
	}
	if _, ok := v.(map[string]any); !ok {
		return fmt.Errorf("%w: 顶层必须是对象 {}", ErrMCPInvalidConfig)
	}
	return nil
}

// equivalentJSON reports whether two raw JSON payloads are structurally
// identical (same keys / values regardless of whitespace or key order).
// Used to decide whether an "existing key" in an agent file conflicts with
// the user's stored config or merely differs in formatting.
func equivalentJSON(a, b json.RawMessage) bool {
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	// Marshal back with sorted keys via the standard encoder (Go sorts map
	// keys deterministically) and compare byte-wise.
	aOut, _ := json.Marshal(av)
	bOut, _ := json.Marshal(bv)
	return string(aOut) == string(bOut)
}

// normalizeMCPName trims surrounding whitespace; empty result rejected by
// callers. We deliberately allow internal spaces ("Framelink MCP for Figma")
// because mcpServers keys are arbitrary strings.
func normalizeMCPName(name string) string { return strings.TrimSpace(name) }

func normalizeGithub(url string) string { return strings.TrimSpace(url) }
