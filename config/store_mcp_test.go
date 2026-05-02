package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// newStoreInTemp returns a Store rooted in a fresh tempdir HOME so each test
// gets isolated mcp-data.json / mcp-agents.json files.
func newStoreInTemp(t *testing.T) *Store {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	s := NewStore()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	return s
}

func mustAddMCP(t *testing.T, s *Store, name, gh, cfg string) {
	t.Helper()
	if err := s.AddMCP(MCP{Name: name, GithubURL: gh, Config: json.RawMessage(cfg)}); err != nil {
		t.Fatalf("AddMCP(%q): %v", name, err)
	}
}

func mustAddAgent(t *testing.T, s *Store, name, path string) {
	t.Helper()
	if err := s.AddMCPAgent(MCPAgent{
		Name: name, Type: MCPWriterClaudeJSONType, Path: path, Visible: true,
	}); err != nil {
		t.Fatalf("AddMCPAgent(%q): %v", name, err)
	}
}

// makeAgentFile creates a .claude.json with the given content under tempDir
// and returns its absolute path.
func makeAgentFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, ".claude.json")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestStore_AddMCP_RejectsDuplicateName(t *testing.T) {
	s := newStoreInTemp(t)
	mustAddMCP(t, s, "n1", "", `{"command":"x"}`)
	err := s.AddMCP(MCP{Name: "n1", Config: json.RawMessage(`{"command":"y"}`)})
	if !errors.Is(err, ErrMCPNameExists) {
		t.Errorf("expected ErrMCPNameExists, got %v", err)
	}
}

func TestStore_AddMCP_RejectsDuplicateGithub_AllowsEmpty(t *testing.T) {
	s := newStoreInTemp(t)
	mustAddMCP(t, s, "n1", "https://github.com/a/b", `{"command":"x"}`)
	err := s.AddMCP(MCP{Name: "n2", GithubURL: "https://github.com/a/b", Config: json.RawMessage(`{"command":"y"}`)})
	if !errors.Is(err, ErrMCPGithubExists) {
		t.Errorf("expected ErrMCPGithubExists, got %v", err)
	}
	// Empty github should NOT collide.
	mustAddMCP(t, s, "n3", "", `{"command":"z"}`)
	mustAddMCP(t, s, "n4", "", `{"command":"z2"}`)
}

func TestStore_AddMCP_RejectsNonObjectConfig(t *testing.T) {
	s := newStoreInTemp(t)
	for _, bad := range []string{`[1,2]`, `"string"`, `42`, `null`, ``} {
		err := s.AddMCP(MCP{Name: "x", Config: json.RawMessage(bad)})
		if !errors.Is(err, ErrMCPInvalidConfig) {
			t.Errorf("AddMCP(%q): expected ErrMCPInvalidConfig, got %v", bad, err)
		}
	}
}

func TestStore_UpdateMCP_BlocksWhenAssigned(t *testing.T) {
	s := newStoreInTemp(t)
	mustAddMCP(t, s, "x", "", `{"command":"a"}`)
	mustAddAgent(t, s, "ag", makeAgentFile(t, `{"mcpServers":{}}`))
	if err := s.AssignMCP("x", "ag", false); err != nil {
		t.Fatal(err)
	}
	err := s.UpdateMCP(0, MCP{Name: "x2", Config: json.RawMessage(`{"command":"b"}`)})
	if !errors.Is(err, ErrMCPHasAssignments) {
		t.Errorf("expected ErrMCPHasAssignments, got %v", err)
	}
}

func TestStore_RemoveMCP_BlocksWhenAssigned(t *testing.T) {
	s := newStoreInTemp(t)
	mustAddMCP(t, s, "x", "", `{"command":"a"}`)
	mustAddAgent(t, s, "ag", makeAgentFile(t, `{"mcpServers":{}}`))
	if err := s.AssignMCP("x", "ag", false); err != nil {
		t.Fatal(err)
	}
	err := s.RemoveMCP(0)
	if !errors.Is(err, ErrMCPHasAssignments) {
		t.Errorf("expected ErrMCPHasAssignments, got %v", err)
	}
}

func TestStore_RemoveMCPAgent_BlocksWhenInUse(t *testing.T) {
	s := newStoreInTemp(t)
	mustAddMCP(t, s, "x", "", `{"command":"a"}`)
	mustAddAgent(t, s, "ag", makeAgentFile(t, `{"mcpServers":{}}`))
	if err := s.AssignMCP("x", "ag", false); err != nil {
		t.Fatal(err)
	}
	err := s.RemoveMCPAgent(0)
	if !errors.Is(err, ErrMCPAgentInUse) {
		t.Errorf("expected ErrMCPAgentInUse, got %v", err)
	}
}

func TestStore_AssignMCP_WritesAgentFile(t *testing.T) {
	s := newStoreInTemp(t)
	cfg := `{"command":"bunx","args":["-y","figma"]}`
	mustAddMCP(t, s, "Framelink", "", cfg)
	agentPath := makeAgentFile(t, `{"misc":1,"mcpServers":{}}`)
	mustAddAgent(t, s, "claude-code", agentPath)

	if err := s.AssignMCP("Framelink", "claude-code", false); err != nil {
		t.Fatal(err)
	}
	if !s.IsMCPAssigned("Framelink", "claude-code") {
		t.Error("Store missed the assignment record")
	}

	data, _ := os.ReadFile(agentPath)
	var top map[string]any
	json.Unmarshal(data, &top)
	if top["misc"].(float64) != 1 {
		t.Errorf("misc clobbered")
	}
	servers := top["mcpServers"].(map[string]any)
	got, ok := servers["Framelink"].(map[string]any)
	if !ok {
		t.Fatalf("Framelink missing from agent file: %v", servers)
	}
	if got["command"] != "bunx" {
		t.Errorf("command lost: %v", got)
	}
}

func TestStore_AssignMCP_FilePresentSamePayload_NoConflict(t *testing.T) {
	s := newStoreInTemp(t)
	cfg := `{"command":"x","args":["a"]}`
	mustAddMCP(t, s, "n", "", cfg)
	// Same payload in different formatting/key order — should NOT conflict.
	pre := `{"mcpServers":{"n":{"args":["a"],"command":"x"}}}`
	agentPath := makeAgentFile(t, pre)
	mustAddAgent(t, s, "ag", agentPath)
	if err := s.AssignMCP("n", "ag", false); err != nil {
		t.Errorf("expected no conflict, got %v", err)
	}
}

func TestStore_AssignMCP_FilePresentDifferentPayload_ReturnsConflict(t *testing.T) {
	s := newStoreInTemp(t)
	mustAddMCP(t, s, "n", "", `{"command":"new"}`)
	agentPath := makeAgentFile(t, `{"mcpServers":{"n":{"command":"old"}}}`)
	mustAddAgent(t, s, "ag", agentPath)

	err := s.AssignMCP("n", "ag", false)
	conflict, ok := err.(*MCPConflict)
	if !ok {
		t.Fatalf("expected *MCPConflict, got %T: %v", err, err)
	}
	if conflict.MCPName != "n" || conflict.AgentName != "ag" {
		t.Errorf("conflict has wrong identity: %+v", conflict)
	}
	// Force overwrite: should succeed now.
	if err := s.AssignMCP("n", "ag", true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(agentPath)
	var top map[string]any
	json.Unmarshal(data, &top)
	if top["mcpServers"].(map[string]any)["n"].(map[string]any)["command"] != "new" {
		t.Errorf("force did not overwrite: %v", top)
	}
}

func TestStore_UnassignMCP_FileMissingKey_StillUpdatesRecord(t *testing.T) {
	s := newStoreInTemp(t)
	mustAddMCP(t, s, "n", "", `{"command":"x"}`)
	agentPath := makeAgentFile(t, `{"mcpServers":{}}`) // file present, key absent
	mustAddAgent(t, s, "ag", agentPath)

	// Manually inject the assignment record to mimic drift (user manually
	// edited the file out from under us).
	if err := s.recordAssignment("n", "ag", true); err != nil {
		t.Fatal(err)
	}
	if !s.IsMCPAssigned("n", "ag") {
		t.Fatal("setup failed")
	}

	if err := s.UnassignMCP("n", "ag"); err != nil {
		t.Errorf("expected silent success, got %v", err)
	}
	if s.IsMCPAssigned("n", "ag") {
		t.Errorf("record not cleared")
	}
}

func TestStore_UnassignMCP_RemovesFromAgentFile(t *testing.T) {
	s := newStoreInTemp(t)
	mustAddMCP(t, s, "n", "", `{"command":"x"}`)
	agentPath := makeAgentFile(t, `{"mcpServers":{}}`)
	mustAddAgent(t, s, "ag", agentPath)

	if err := s.AssignMCP("n", "ag", false); err != nil {
		t.Fatal(err)
	}
	if err := s.UnassignMCP("n", "ag"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(agentPath)
	var top map[string]any
	json.Unmarshal(data, &top)
	servers := top["mcpServers"].(map[string]any)
	if _, present := servers["n"]; present {
		t.Errorf("agent file still has the entry")
	}
}

func TestStore_UpdateMCPAgent_RenamePropagatesAssignments(t *testing.T) {
	s := newStoreInTemp(t)
	mustAddMCP(t, s, "n", "", `{"command":"x"}`)
	agentPath := makeAgentFile(t, `{"mcpServers":{}}`)
	mustAddAgent(t, s, "old-name", agentPath)
	if err := s.AssignMCP("n", "old-name", false); err != nil {
		t.Fatal(err)
	}

	if err := s.UpdateMCPAgent(0, MCPAgent{
		Name: "new-name", Type: MCPWriterClaudeJSONType, Path: agentPath, Visible: true,
	}); err != nil {
		t.Fatal(err)
	}
	if !s.IsMCPAssigned("n", "new-name") {
		t.Errorf("rename did not propagate to assignments")
	}
	if s.IsMCPAssigned("n", "old-name") {
		t.Errorf("old name still recorded")
	}
}

func TestStore_AddMCPAgent_RejectsUnknownType(t *testing.T) {
	s := newStoreInTemp(t)
	err := s.AddMCPAgent(MCPAgent{Name: "x", Type: "bogus", Path: "/tmp/x.json"})
	if !errors.Is(err, ErrMCPAgentTypeUnknown) {
		t.Errorf("expected ErrMCPAgentTypeUnknown, got %v", err)
	}
}

func TestStore_PersistenceRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	s := NewStore()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	mustAddMCP(t, s, "Framelink", "https://github.com/x/y", `{"command":"bunx"}`)
	mustAddAgent(t, s, "claude", makeAgentFile(t, `{"mcpServers":{}}`))
	if err := s.AssignMCP("Framelink", "claude", false); err != nil {
		t.Fatal(err)
	}

	// Reload from disk in a fresh Store.
	s2 := NewStore()
	if err := s2.Init(); err != nil {
		t.Fatal(err)
	}
	mcps := s2.MCPs()
	if len(mcps) != 1 || mcps[0].Name != "Framelink" {
		t.Fatalf("MCP not persisted: %+v", mcps)
	}
	if !s2.IsMCPAssigned("Framelink", "claude") {
		t.Errorf("assignment not persisted")
	}
	agents := s2.MCPAgents()
	if len(agents) != 1 || agents[0].Name != "claude" {
		t.Errorf("agent not persisted: %+v", agents)
	}
}
