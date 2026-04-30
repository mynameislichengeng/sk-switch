package config

import (
	"os"
	"path/filepath"
	"strings"
)

func ConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "sk-switch")
}

func ensureConfigDir() error {
	return os.MkdirAll(ConfigDir(), 0755)
}

func ExpandPath(p string) string {
	if len(p) >= 2 && p[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

// NormalizePath trims surrounding whitespace and any trailing path separator,
// so "~/.agents/skills" and "~/.agents/skills/" persist identically.
func NormalizePath(p string) string {
	p = strings.TrimSpace(p)
	for len(p) > 1 && (p[len(p)-1] == '/' || p[len(p)-1] == '\\') {
		p = p[:len(p)-1]
	}
	return p
}

// IsDirPath reports whether the path (with ~ expanded and symlinks resolved)
// refers to an existing directory.
func IsDirPath(p string) bool {
	abs := ExpandPath(p)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	info, err := os.Stat(abs)
	return err == nil && info.IsDir()
}
