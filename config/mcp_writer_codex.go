package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// MCPWriterCodexTOML handles Codex's config file (e.g. ~/.codex/config.toml).
//
// The file is a TOML document where each MCP is a section under [mcp_servers].
// For example:
//
//	[mcp_servers.chrome_devtools]
//	url = "http://localhost:3000/mcp"
//	enabled_tools = ["open", "screenshot"]
//
// The key passed to Write/Read/Delete is the section name (e.g. "chrome_devtools").
// The value is the section body (everything after [mcp_servers.key]).
type MCPWriterCodexTOML struct{}

// MCPWriterCodexTOMLType is the registered type tag.
const MCPWriterCodexTOMLType = "codex-toml"

func init() {
	RegisterMCPWriter(MCPWriterCodexTOMLType, MCPWriterCodexTOML{})
}

const codexServersSection = "mcp_servers"

// readCodexFile reads the TOML file and returns its raw bytes.
// Returns ErrMCPFileMissing when the file is absent.
func readCodexFile(path string) ([]byte, error) {
	abs := ExpandPath(path)
	data, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrMCPFileMissing
		}
		return nil, fmt.Errorf("read %s: %w", abs, err)
	}
	return data, nil
}

// writeCodexFile writes data to path atomically. Preserves the original
// file mode when the file exists; falls back to 0644 for new files.
func writeCodexFile(path string, data []byte) error {
	abs := ExpandPath(path)
	mode := os.FileMode(0644)
	if info, err := os.Stat(abs); err == nil {
		mode = info.Mode()
	}
	return atomicWriteFile(abs, data, mode)
}

// extractSection extracts the body of a [mcp_servers.key] section from the
// TOML data, returning the raw text between the section header and the next
// section (or EOF). Returns empty string when the section is not found.
func extractSection(data []byte, key string) string {
	lines := strings.Split(string(data), "\n")
	header := fmt.Sprintf("[%s.%s]", codexServersSection, key)
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == header {
			start = i
			break
		}
	}
	if start == -1 {
		return ""
	}
	// Collect lines until the next section header (line starting with "[")
	// or end of file.
	var body []string
	for i := start + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "[") {
			break
		}
		if trimmed != "" {
			body = append(body, lines[i])
		}
	}
	return strings.Join(body, "\n")
}

// deleteSection removes the [mcp_servers.key] section (header + body) from
// the TOML data and returns the remaining text.
func deleteSection(data []byte, key string) []byte {
	lines := strings.Split(string(data), "\n")
	header := fmt.Sprintf("[%s.%s]", codexServersSection, key)
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == header {
			start = i
			break
		}
	}
	if start == -1 {
		return data
	}
	// Find the end of this section (next section header or EOF).
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "[") {
			end = i
			break
		}
	}
	// Remove lines [start, end).
	var out []string
	out = append(out, lines[:start]...)
	out = append(out, lines[end:]...)
	result := strings.Join(out, "\n")
	// Clean up trailing blank lines.
	result = strings.TrimRight(result, "\n")
	if result == "" {
		return nil
	}
	return []byte(result + "\n")
}

// Validate implements MCPWriter.
func (MCPWriterCodexTOML) Validate(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("路径不能为空")
	}
	abs := ExpandPath(path)
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("文件不存在: %s", path)
		}
		return fmt.Errorf("无法访问 %s: %v", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("路径必须是文件，不是目录: %s", path)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Errorf("无法读取 %s: %v", path, err)
	}
	// Empty file is OK.
	if len(data) == 0 {
		return nil
	}
	var v any
	if err := toml.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("文件不是合法的 TOML: %v", err)
	}
	return nil
}

// ValidateConfig validates that the value is valid TOML by wrapping it in a
// temporary section header and parsing.
func (MCPWriterCodexTOML) ValidateConfig(value string) error {
	if len(value) == 0 {
		return fmt.Errorf("%w: 配置为空", ErrMCPInvalidConfig)
	}
	// Wrap in a temporary section so toml.Unmarshal can parse it.
	testData := fmt.Sprintf("[%s]\n%s", codexServersSection, value)
	var v any
	if err := toml.Unmarshal([]byte(testData), &v); err != nil {
		return fmt.Errorf("%w: %v", ErrMCPInvalidConfig, err)
	}
	return nil
}

// Read implements MCPWriter.
func (MCPWriterCodexTOML) Read(path, key string) (string, error) {
	data, err := readCodexFile(path)
	if err != nil {
		return "", err
	}
	return extractSection(data, key), nil
}

// Write implements MCPWriter.
func (MCPWriterCodexTOML) Write(path, key string, value string) error {
	data, err := readCodexFile(path)
	if err != nil && err != ErrMCPFileMissing {
		return err
	}
	// Delete old section if present.
	if len(data) > 0 {
		data = deleteSection(data, key)
	}
	// Append new section.
	newSection := fmt.Sprintf("[%s.%s]\n%s\n", codexServersSection, key, value)
	if len(data) == 0 {
		data = []byte(newSection)
	} else {
		data = append(data, []byte(newSection)...)
	}
	return writeCodexFile(path, data)
}

// Delete implements MCPWriter.
func (MCPWriterCodexTOML) Delete(path, key string) error {
	data, err := readCodexFile(path)
	if err != nil {
		return err
	}
	newData := deleteSection(data, key)
	if string(newData) == string(data) {
		// Section not found; nothing to do.
		return nil
	}
	if len(newData) == 0 {
		// File is now empty; write an empty file.
		return writeCodexFile(path, []byte{})
	}
	return writeCodexFile(path, newData)
}
