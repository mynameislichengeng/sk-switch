package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// MCPWriter abstracts a single agent-config-file format. Each implementation
// knows how to read/upsert/delete a single named MCP entry inside the file
// while preserving all other content.
//
// All operations take an absolute or ~-prefixed path; callers should NOT
// pre-expand it — implementations apply ExpandPath internally so the
// on-disk MCPAgent.Path string round-trips untouched.
type MCPWriter interface {
	// Read returns the existing payload registered under `name`, or nil
	// (with no error) if the file lacks that entry. ErrMCPFileMissing is
	// returned when the underlying file does not exist on disk.
	Read(path, name string) (json.RawMessage, error)
	// Write upserts the payload under `name`. If the file does not exist
	// the implementation SHOULD return ErrMCPFileMissing rather than
	// silently creating it — initial-create policy is the caller's call.
	Write(path, name string, config json.RawMessage) error
	// Delete removes `name` from the file; missing entry is a no-op.
	// ErrMCPFileMissing when the file itself is gone.
	Delete(path, name string) error
}

// ErrMCPFileMissing signals the agent's config file is absent, distinct from
// "file present but lacks our key".
var ErrMCPFileMissing = errors.New("MCP agent 的配置文件不存在")

// mcpWriterRegistry maps MCPAgent.Type → writer. Adding a new file format
// (e.g. codex-toml) is one line here + one new writer file.
var mcpWriterRegistry = map[string]MCPWriter{}

// RegisterMCPWriter installs a writer for the given type tag. Panics on
// duplicate registration so misconfigurations fail loudly at init.
func RegisterMCPWriter(typeTag string, w MCPWriter) {
	if _, exists := mcpWriterRegistry[typeTag]; exists {
		panic(fmt.Sprintf("config: duplicate MCPWriter registration for %q", typeTag))
	}
	mcpWriterRegistry[typeTag] = w
}

// MCPWriterFor returns the writer for the given type, or ErrMCPAgentTypeUnknown.
func MCPWriterFor(typeTag string) (MCPWriter, error) {
	w, ok := mcpWriterRegistry[typeTag]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrMCPAgentTypeUnknown, typeTag)
	}
	return w, nil
}

// MCPAgentTypes returns the list of registered type tags. Used by the TUI's
// MCP-agent form to render the type dropdown.
func MCPAgentTypes() []string {
	out := make([]string, 0, len(mcpWriterRegistry))
	for k := range mcpWriterRegistry {
		out = append(out, k)
	}
	return out
}

// atomicWriteFile writes data to path via a tmp file in the same directory
// then renames over the target. The file mode is preserved when target
// exists; otherwise the supplied mode is used.
func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := dirOf(path)
	tmp, err := os.CreateTemp(dir, ".sk-switch-mcp-*.tmp")
	if err != nil {
		return fmt.Errorf("create tmp: %w", err)
	}
	tmpName := tmp.Name()
	// Make sure the tmp is gone if we fail before rename.
	committed := false
	defer func() {
		if !committed {
			os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close tmp: %w", err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("chmod tmp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename tmp: %w", err)
	}
	committed = true
	return nil
}

func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[:i]
		}
	}
	return "."
}
