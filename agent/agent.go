package agent

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mynameislichengeng/sk-switch/config"
)

// Link creates a relative symlink from the agent's skill dir to the data source's
// skill dir. The agent dir is created if it doesn't exist.
func Link(ag config.Agent, sk config.Skill) error {
	agentPath := config.ExpandPath(ag.Path)
	if err := os.MkdirAll(agentPath, 0755); err != nil {
		return err
	}
	target := filepath.Join(agentPath, sk.Name)
	rel, err := filepath.Rel(agentPath, sk.SkillDir)
	if err != nil {
		return err
	}
	return os.Symlink(rel, target)
}

// Unlink removes the symlink in the agent's skill dir. Real directories are
// rejected — if the user really wants to remove them they must use overwrite.
func Unlink(ag config.Agent, sk config.Skill) error {
	target := filepath.Join(config.ExpandPath(ag.Path), sk.Name)
	fi, err := os.Lstat(target)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return os.ErrExist
	}
	link, err := os.Readlink(target)
	if err != nil {
		return err
	}
	abs := link
	if !filepath.IsAbs(link) {
		abs = filepath.Join(filepath.Dir(target), link)
	}
	if !strings.HasSuffix(abs, sk.Name) {
		return os.ErrExist
	}
	return os.Remove(target)
}

// CanLink reports whether Link can proceed safely.
//
// Returns ("", true) if the entry doesn't exist; ("already_linked", true) if
// the entry is already a symlink; ("exists", false) if a non-symlink occupies
// the slot — caller should ask the user before overwriting.
func CanLink(ag config.Agent, sk config.Skill) (ok bool, reason string) {
	target := filepath.Join(config.ExpandPath(ag.Path), sk.Name)
	fi, err := os.Lstat(target)
	if err != nil {
		return true, ""
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return true, "already_linked"
	}
	return false, "exists"
}
