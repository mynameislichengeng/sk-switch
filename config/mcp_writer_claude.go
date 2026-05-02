package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// MCPWriterClaudeJSON handles Claude Code's config file
// (`~/.claude.json` or `~/.claude/.claude.json`).
//
// The file is a single JSON object whose top-level "mcpServers" key holds a
// map of mcp-name → config payload. We MUST preserve every other top-level
// field — the file also contains conversation history, project state, etc.
// — so we round-trip through json.RawMessage map and only touch the
// mcpServers entry.
type MCPWriterClaudeJSON struct{}

// MCPWriterClaudeJSONType is the registered type tag.
const MCPWriterClaudeJSONType = "claude-json"

// init registers the claude-json writer so package consumers don't need to.
// New writers should follow the same pattern.
func init() {
	RegisterMCPWriter(MCPWriterClaudeJSONType, MCPWriterClaudeJSON{})
}

// claudeServersKey is the mcpServers field name inside the agent's JSON.
const claudeServersKey = "mcpServers"

// readClaudeFile loads the agent's JSON file as an ordered-ish top-level map.
// Returns ErrMCPFileMissing when the file is absent.
func readClaudeFile(path string) (map[string]json.RawMessage, os.FileMode, error) {
	abs := ExpandPath(path)
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, ErrMCPFileMissing
		}
		return nil, 0, fmt.Errorf("stat %s: %w", abs, err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, info.Mode(), fmt.Errorf("read %s: %w", abs, err)
	}
	if len(data) == 0 {
		return map[string]json.RawMessage{}, info.Mode(), nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, info.Mode(), fmt.Errorf("parse %s: %w", abs, err)
	}
	if top == nil {
		top = map[string]json.RawMessage{}
	}
	return top, info.Mode(), nil
}

// writeClaudeFile serializes top with indented output and atomically replaces
// path. Mode is preserved (or 0644 for new files).
func writeClaudeFile(path string, top map[string]json.RawMessage, mode os.FileMode) error {
	if mode == 0 {
		mode = 0644
	}
	data, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	// Trailing newline mirrors what most editors leave behind.
	data = append(data, '\n')
	return atomicWriteFile(ExpandPath(path), data, mode)
}

func extractServers(top map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	raw, ok := top[claudeServersKey]
	if !ok {
		return map[string]json.RawMessage{}, nil
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(raw, &servers); err != nil {
		return nil, fmt.Errorf("parse mcpServers: %w", err)
	}
	if servers == nil {
		servers = map[string]json.RawMessage{}
	}
	return servers, nil
}

func putServers(top map[string]json.RawMessage, servers map[string]json.RawMessage) error {
	if len(servers) == 0 {
		// Keep mcpServers as an empty object rather than deleting the key —
		// surprises tooling less, and the key is conventionally present.
		top[claudeServersKey] = json.RawMessage("{}")
		return nil
	}
	raw, err := json.Marshal(servers)
	if err != nil {
		return err
	}
	top[claudeServersKey] = raw
	return nil
}

// Read implements MCPWriter.
func (MCPWriterClaudeJSON) Read(path, name string) (json.RawMessage, error) {
	top, _, err := readClaudeFile(path)
	if err != nil {
		return nil, err
	}
	servers, err := extractServers(top)
	if err != nil {
		return nil, err
	}
	v, ok := servers[name]
	if !ok {
		return nil, nil
	}
	return v, nil
}

// Write implements MCPWriter.
func (MCPWriterClaudeJSON) Write(path, name string, config json.RawMessage) error {
	top, mode, err := readClaudeFile(path)
	if err != nil {
		return err
	}
	servers, err := extractServers(top)
	if err != nil {
		return err
	}
	servers[name] = config
	if err := putServers(top, servers); err != nil {
		return err
	}
	return writeClaudeFile(path, top, mode)
}

// Delete implements MCPWriter.
func (MCPWriterClaudeJSON) Delete(path, name string) error {
	top, mode, err := readClaudeFile(path)
	if err != nil {
		return err
	}
	servers, err := extractServers(top)
	if err != nil {
		return err
	}
	if _, ok := servers[name]; !ok {
		// nothing to delete; skip the rewrite to avoid touching mtime
		return nil
	}
	delete(servers, name)
	if err := putServers(top, servers); err != nil {
		return err
	}
	return writeClaudeFile(path, top, mode)
}
