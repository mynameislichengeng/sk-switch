package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// TypeConfig holds the key-value pair for a single MCP writer type.
// Key is the identifier written into the agent's config file.
// Value is the raw config payload as a string (JSON, TOML, etc.).
type TypeConfig struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// MCP represents a single Model Context Protocol server registration managed
// by sk-switch.
//
// mcp-data.json is the source of truth for both the MCP itself and the set
// of agents it has been assigned to (Agents). The actual per-agent config
// file is only touched at assign/unassign time; "is this assigned" queries
// always answer from Agents here, not by re-scanning agent files.
//
// Configs is a map of writer type tag → TypeConfig, allowing different
// payloads for different agent config formats (claude-json vs codex-toml).
type MCP struct {
	Name      string                 `json:"name"`
	GithubURL string                 `json:"github,omitempty"`
	Configs   map[string]TypeConfig  `json:"configs"`
	Agents    []string               `json:"agents,omitempty"`
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
	ErrMCPInvalidConfig    = errors.New("config 格式不正确")
	ErrMCPAgentTypeUnknown = errors.New("未注册的 MCP agent 类型")
)

// MCPConflict is returned by AssignMCP when the agent's existing config file
// already declares the MCP with a different payload. The caller (TUI / CLI)
// can show ExistingRaw vs NewRaw side-by-side and call AssignMCP again with
// force=true to overwrite.
type MCPConflict struct {
	MCPName     string
	AgentName   string
	ExistingRaw string
	NewRaw      string
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

// ValidateMCPConfigs validates every TypeConfig in the map by delegating to
// the corresponding MCPWriter's ValidateConfig. Returns ErrMCPInvalidConfig
// when any entry fails validation or when the map is empty.
func ValidateMCPConfigs(configs map[string]TypeConfig) error {
	if len(configs) == 0 {
		return fmt.Errorf("%w: 至少需要配置一个类型", ErrMCPInvalidConfig)
	}
	for typ, tc := range configs {
		if tc.Key == "" {
			return fmt.Errorf("%w: %s 的 key 不能为空", ErrMCPInvalidConfig, typ)
		}
		writer, err := MCPWriterFor(typ)
		if err != nil {
			return fmt.Errorf("%w: 未找到 writer %q", ErrMCPInvalidConfig, typ)
		}
		if err := writer.ValidateConfig(tc.Value); err != nil {
			return fmt.Errorf("%w (%s): %v", ErrMCPInvalidConfig, typ, err)
		}
	}
	return nil
}

// equivalentString reports whether two string payloads are structurally
// identical. For JSON payloads it uses json.Unmarshal+Marshal for deep
// comparison; for TOML it uses toml.Unmarshal and re-serializes; for other
// formats it falls back to direct string comparison.
func equivalentString(a, b string) bool {
	// Try JSON deep comparison first.
	var av, bv any
	if err := json.Unmarshal([]byte(a), &av); err == nil {
		if err := json.Unmarshal([]byte(b), &bv); err == nil {
			aOut, _ := json.Marshal(av)
			bOut, _ := json.Marshal(bv)
			return string(aOut) == string(bOut)
		}
	}
	// Try TOML deep comparison.
	var at, bt any
	if err := toml.Unmarshal([]byte(a), &at); err == nil {
		if err := toml.Unmarshal([]byte(b), &bt); err == nil {
			// TOML doesn't have a standard Marshal, so compare the parsed
			// structures via JSON serialization (both represent maps/arrays).
			aOut, _ := json.Marshal(at)
			bOut, _ := json.Marshal(bt)
			return string(aOut) == string(bOut)
		}
	}
	// Fall back to direct string comparison for non-JSON/non-TOML.
	return a == b
}

// normalizeMCPName trims surrounding whitespace; empty result rejected by
// callers. We deliberately allow internal spaces ("Framelink MCP for Figma")
// because mcpServers keys are arbitrary strings.
func normalizeMCPName(name string) string { return strings.TrimSpace(name) }

func normalizeGithub(url string) string { return strings.TrimSpace(url) }
