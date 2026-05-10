package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexWriterValidateConfig(t *testing.T) {
	w := MCPWriterCodexTOML{}

	t.Run("valid TOML", func(t *testing.T) {
		if err := w.ValidateConfig(`url = "http://localhost:3000/mcp"`); err != nil {
			t.Errorf("expected valid, got %v", err)
		}
	})

	t.Run("empty string rejected", func(t *testing.T) {
		if err := w.ValidateConfig(""); err == nil {
			t.Errorf("expected error for empty string")
		}
	})

	t.Run("malformed TOML rejected", func(t *testing.T) {
		if err := w.ValidateConfig(`[invalid`); err == nil {
			t.Errorf("expected error for malformed TOML")
		}
	})
}

func TestCodexWriterReadWriteDelete(t *testing.T) {
	w := MCPWriterCodexTOML{}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Write a section.
	if err := w.Write(path, "chrome_devtools", `url = "http://localhost:3000/mcp"`); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read it back.
	got, err := w.Read(path, "chrome_devtools")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if !strings.Contains(got, "url = \"http://localhost:3000/mcp\"") {
		t.Errorf("Read returned wrong content: %q", got)
	}

	// Read missing key.
	missing, err := w.Read(path, "nonexistent")
	if err != nil {
		t.Fatalf("Read missing failed: %v", err)
	}
	if missing != "" {
		t.Errorf("expected empty for missing key, got %q", missing)
	}

	// Delete the section.
	if err := w.Delete(path, "chrome_devtools"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if strings.Contains(string(data), "[mcp_servers.chrome_devtools]") {
		t.Errorf("section still present after delete")
	}
}

func TestCodexWriterOverwrite(t *testing.T) {
	w := MCPWriterCodexTOML{}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Write initial section.
	if err := w.Write(path, "key1", `url = "old"`); err != nil {
		t.Fatal(err)
	}

	// Overwrite with new content.
	if err := w.Write(path, "key1", `url = "new"`); err != nil {
		t.Fatal(err)
	}

	// Read back.
	got, err := w.Read(path, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `url = "new"`) {
		t.Errorf("overwrite failed, got: %q", got)
	}
	if strings.Contains(got, `url = "old"`) {
		t.Errorf("old content still present")
	}
}

func TestCodexWriterMultipleSections(t *testing.T) {
	w := MCPWriterCodexTOML{}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Write multiple sections.
	if err := w.Write(path, "a", `url = "http://a"`); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(path, "b", `url = "http://b"`); err != nil {
		t.Fatal(err)
	}

	// Delete middle section.
	if err := w.Delete(path, "a"); err != nil {
		t.Fatal(err)
	}

	// Verify b still exists.
	got, err := w.Read(path, "b")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `url = "http://b"`) {
		t.Errorf("b lost after deleting a")
	}

	// Verify a is gone.
	missing, _ := w.Read(path, "a")
	if missing != "" {
		t.Errorf("a still present after delete")
	}
}

func TestCodexWriterDeleteMissingKey(t *testing.T) {
	w := MCPWriterCodexTOML{}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Write a section.
	if err := w.Write(path, "exists", `url = "http://exists"`); err != nil {
		t.Fatal(err)
	}

	// Delete a missing key - should not error and not modify file.
	mtimeBefore := mtime(t, path)
	if err := w.Delete(path, "missing"); err != nil {
		t.Fatalf("Delete missing key failed: %v", err)
	}
	if mtime(t, path) != mtimeBefore {
		t.Errorf("file modified when deleting missing key")
	}
}
