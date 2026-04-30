package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Skill represents one skill discovered under a data source.
//
// Identity rule: a directory at depth=1 below the data source root that
// contains a SKILL.md file. Anything deeper is ignored.
type Skill struct {
	Name        string
	Source      string // GitHub repo from .skill-lock.json (optional)
	SourceURL   string // full GitHub URL (optional)
	DataSource  string // data source name
	SourcePath  string // absolute path to the data source root
	SkillDir    string // absolute path to <root>/<name>
	Description string // short description from SKILL.md frontmatter
}

// Key uniquely identifies a skill across data sources.
func (s Skill) Key() string { return s.DataSource + "/" + s.Name }

// scanOneSource scans the data source at depth=1 only.
func scanOneSource(ds DataSource) []Skill {
	abs := ExpandPath(ds.Path)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil
	}
	lock := loadSkillLock(abs)

	var out []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(abs, e.Name())
		if !hasSkillMD(dir) {
			continue
		}
		sk := Skill{
			Name:        e.Name(),
			DataSource:  ds.Name,
			SourcePath:  abs,
			SkillDir:    dir,
			Description: extractDescription(dir),
		}
		if entry, ok := lock[e.Name()]; ok {
			sk.Source = entry.Source
			sk.SourceURL = entry.SourceURL
		}
		out = append(out, sk)
	}
	return out
}

func hasSkillMD(dir string) bool {
	for _, n := range []string{"SKILL.md", "skill.md", "Skill.md"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			return true
		}
	}
	return false
}

// isAssigned: a skill is considered assigned to an agent when the agent's skill
// directory contains a same-named subdirectory holding a SKILL.md.
func isAssigned(ag Agent, sk Skill) bool {
	return hasSkillMD(filepath.Join(ExpandPath(ag.Path), sk.Name))
}

type skillLockEntry struct {
	Source    string `json:"source"`
	SourceURL string `json:"sourceURL"`
}

func loadSkillLock(dsPath string) map[string]skillLockEntry {
	home, _ := os.UserHomeDir()
	paths := []string{
		filepath.Join(home, ".local", "state", "skills", ".skill-lock.json"),
		filepath.Join(dsPath, ".skill-lock.json"),
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var raw struct {
			Skills map[string]skillLockEntry `json:"skills"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			continue
		}
		return raw.Skills
	}
	return nil
}

var descRegex = regexp.MustCompile(`(?i)^description:\s*["']?(.+?)["']?\s*$`)

func extractDescription(dir string) string {
	for _, name := range []string{"SKILL.md", "skill.md", "Skill.md"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		content := string(data)
		inFrontmatter := false
		for _, line := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "---" {
				if !inFrontmatter {
					inFrontmatter = true
					continue
				}
				break
			}
			if inFrontmatter {
				if m := descRegex.FindStringSubmatch(trimmed); m != nil {
					desc := m[1]
					if len(desc) > 40 {
						return desc[:37] + "..."
					}
					return desc
				}
			}
		}
		for _, line := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "---") {
				continue
			}
			if len(trimmed) > 40 {
				return trimmed[:37] + "..."
			}
			return trimmed
		}
	}
	return ""
}
