package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writerScratchFile creates a tempfile with the given top-level JSON content
// and returns its absolute path; cleanup is registered via t.Cleanup.
func writerScratchFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, ".claude.json")
	if content != "" {
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// loadTopLevel re-reads `path` and returns the parsed top-level JSON map for
// assertions. The test harness round-trips on its own to avoid coupling
// assertions to the exact byte output produced by the writer.
func loadTopLevel(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]any
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatal(err)
	}
	return top
}

func TestClaudeWriterWrite_FileMissing_ReturnsErr(t *testing.T) {
	w := MCPWriterClaudeJSON{}
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope", ".claude.json")
	err := w.Write(missing, "test", json.RawMessage(`{"command":"x"}`))
	if err != ErrMCPFileMissing {
		t.Fatalf("expected ErrMCPFileMissing, got %v", err)
	}
}

func TestClaudeWriterWrite_AddsToEmptyFile_PreservesOtherKeys(t *testing.T) {
	w := MCPWriterClaudeJSON{}
	const initial = `{
  "history": ["one", "two"],
  "userId": "abc",
  "mcpServers": {}
}`
	path := writerScratchFile(t, initial)
	cfg := json.RawMessage(`{"command":"bunx","args":["-y","figma-developer-mcp@latest"]}`)
	if err := w.Write(path, "Framelink MCP for Figma", cfg); err != nil {
		t.Fatal(err)
	}

	top := loadTopLevel(t, path)
	if got, want := top["userId"], "abc"; got != want {
		t.Errorf("userId clobbered: got %v want %v", got, want)
	}
	hist, ok := top["history"].([]any)
	if !ok || len(hist) != 2 {
		t.Errorf("history clobbered: %v", top["history"])
	}
	servers, ok := top["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers missing/wrong type: %v", top["mcpServers"])
	}
	got, ok := servers["Framelink MCP for Figma"].(map[string]any)
	if !ok {
		t.Fatalf("entry missing: %v", servers)
	}
	if got["command"] != "bunx" {
		t.Errorf("command lost: %v", got)
	}
}

func TestClaudeWriterWrite_NoMcpServersKey_CreatesIt(t *testing.T) {
	w := MCPWriterClaudeJSON{}
	const initial = `{"unrelated": 1}`
	path := writerScratchFile(t, initial)
	cfg := json.RawMessage(`{"command":"x"}`)
	if err := w.Write(path, "n1", cfg); err != nil {
		t.Fatal(err)
	}
	top := loadTopLevel(t, path)
	if top["unrelated"].(float64) != 1 {
		t.Errorf("unrelated clobbered")
	}
	servers := top["mcpServers"].(map[string]any)
	if servers["n1"].(map[string]any)["command"] != "x" {
		t.Errorf("entry not present")
	}
}

func TestClaudeWriterWrite_OverwritesExistingKey(t *testing.T) {
	w := MCPWriterClaudeJSON{}
	const initial = `{"mcpServers": {"n1": {"command":"old"}, "n2": {"command":"keep"}}}`
	path := writerScratchFile(t, initial)
	cfg := json.RawMessage(`{"command":"new"}`)
	if err := w.Write(path, "n1", cfg); err != nil {
		t.Fatal(err)
	}
	top := loadTopLevel(t, path)
	servers := top["mcpServers"].(map[string]any)
	if servers["n1"].(map[string]any)["command"] != "new" {
		t.Errorf("not overwritten")
	}
	if servers["n2"].(map[string]any)["command"] != "keep" {
		t.Errorf("other entry clobbered")
	}
}

func TestClaudeWriterRead_PresentAndAbsent(t *testing.T) {
	w := MCPWriterClaudeJSON{}
	const initial = `{"mcpServers": {"a": {"command":"X"}}}`
	path := writerScratchFile(t, initial)

	got, err := w.Read(path, "a")
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	json.Unmarshal(got, &v)
	if v["command"] != "X" {
		t.Errorf("Read returned wrong payload: %v", v)
	}

	missing, err := w.Read(path, "b")
	if err != nil {
		t.Fatal(err)
	}
	if missing != nil {
		t.Errorf("expected nil for missing key, got %v", missing)
	}
}

func TestClaudeWriterRead_FileMissing(t *testing.T) {
	w := MCPWriterClaudeJSON{}
	_, err := w.Read("/nonexistent/path/.claude.json", "x")
	if err != ErrMCPFileMissing {
		t.Errorf("expected ErrMCPFileMissing, got %v", err)
	}
}

func TestClaudeWriterDelete_RemovesOnly_Target(t *testing.T) {
	w := MCPWriterClaudeJSON{}
	const initial = `{"misc":42,"mcpServers":{"a":{"command":"A"},"b":{"command":"B"}}}`
	path := writerScratchFile(t, initial)
	if err := w.Delete(path, "a"); err != nil {
		t.Fatal(err)
	}
	top := loadTopLevel(t, path)
	if top["misc"].(float64) != 42 {
		t.Errorf("unrelated clobbered")
	}
	servers := top["mcpServers"].(map[string]any)
	if _, present := servers["a"]; present {
		t.Errorf("a still present")
	}
	if _, present := servers["b"]; !present {
		t.Errorf("b should remain")
	}
}

func TestClaudeWriterDelete_MissingKey_NoError_NoChange(t *testing.T) {
	w := MCPWriterClaudeJSON{}
	const initial = `{"mcpServers":{"a":{}}}`
	path := writerScratchFile(t, initial)
	mtimeBefore := mtime(t, path)
	if err := w.Delete(path, "nonexistent"); err != nil {
		t.Fatal(err)
	}
	if mtime(t, path) != mtimeBefore {
		t.Errorf("file rewritten when no change was needed")
	}
}

func TestClaudeWriterAtomicity_FileModePreserved(t *testing.T) {
	w := MCPWriterClaudeJSON{}
	const initial = `{"mcpServers":{}}`
	path := writerScratchFile(t, initial)
	// Set a non-default mode to verify it survives.
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(path, "x", json.RawMessage(`{"command":"y"}`)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0640 {
		t.Errorf("mode lost: got %o", info.Mode().Perm())
	}
}

func TestClaudeWriter_RoundtripPreservesNestedFields(t *testing.T) {
	w := MCPWriterClaudeJSON{}
	const initial = `{
  "projects": {
    "/foo/bar": {
      "history": [{"id":1}, {"id":2}],
      "settings": {"theme":"dark"}
    }
  },
  "mcpServers": {
    "Vibma": {"command":"bunx","args":["-y","@ufira/vibma","--edit"]}
  }
}`
	path := writerScratchFile(t, initial)
	cfg := json.RawMessage(`{"command":"bunx","args":["-y","cursor-talk-to-figma-mcp@latest"]}`)
	if err := w.Write(path, "TalkToFigma", cfg); err != nil {
		t.Fatal(err)
	}
	top := loadTopLevel(t, path)

	// Projects untouched.
	projects, _ := top["projects"].(map[string]any)
	bar, _ := projects["/foo/bar"].(map[string]any)
	hist, _ := bar["history"].([]any)
	if len(hist) != 2 || hist[0].(map[string]any)["id"].(float64) != 1 {
		t.Errorf("nested projects clobbered: %v", projects)
	}

	servers, _ := top["mcpServers"].(map[string]any)
	if _, ok := servers["Vibma"]; !ok {
		t.Errorf("Vibma lost")
	}
	tk, _ := servers["TalkToFigma"].(map[string]any)
	args, _ := tk["args"].([]any)
	if !reflect.DeepEqual(args, []any{"-y", "cursor-talk-to-figma-mcp@latest"}) {
		t.Errorf("TalkToFigma args wrong: %v", args)
	}
}

func TestClaudeWriterValidate(t *testing.T) {
	w := MCPWriterClaudeJSON{}
	dir := t.TempDir()

	t.Run("nonexistent path", func(t *testing.T) {
		err := w.Validate(filepath.Join(dir, "nope.json"))
		if err == nil || !strings.Contains(err.Error(), "不存在") {
			t.Errorf("expected 不存在 error, got %v", err)
		}
	})

	t.Run("empty path", func(t *testing.T) {
		err := w.Validate("")
		if err == nil {
			t.Errorf("expected error for empty path")
		}
	})

	t.Run("path is a directory", func(t *testing.T) {
		err := w.Validate(dir)
		if err == nil || !strings.Contains(err.Error(), "目录") {
			t.Errorf("expected 目录 error, got %v", err)
		}
	})

	t.Run("empty file accepted", func(t *testing.T) {
		p := filepath.Join(dir, "empty.json")
		os.WriteFile(p, nil, 0644)
		if err := w.Validate(p); err != nil {
			t.Errorf("empty file should be valid, got %v", err)
		}
	})

	t.Run("valid JSON object accepted", func(t *testing.T) {
		p := filepath.Join(dir, "ok.json")
		os.WriteFile(p, []byte(`{"mcpServers": {}}`), 0644)
		if err := w.Validate(p); err != nil {
			t.Errorf("valid JSON should pass, got %v", err)
		}
	})

	t.Run("malformed JSON rejected", func(t *testing.T) {
		p := filepath.Join(dir, "bad.json")
		os.WriteFile(p, []byte(`{"unclosed":`), 0644)
		err := w.Validate(p)
		if err == nil || !strings.Contains(err.Error(), "JSON") {
			t.Errorf("expected JSON parse error, got %v", err)
		}
	})

	t.Run("non-object JSON (array) rejected", func(t *testing.T) {
		// Top level must be an object since mcpServers lives there. An
		// array-rooted file could never be usable for this writer.
		p := filepath.Join(dir, "arr.json")
		os.WriteFile(p, []byte(`[1,2]`), 0644)
		if err := w.Validate(p); err == nil {
			t.Errorf("array should be rejected by Validate")
		}
	})
}

func mtime(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.ModTime().UnixNano()
}
