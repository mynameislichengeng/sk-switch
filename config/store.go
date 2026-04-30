package config

import (
	"fmt"
	"strings"
	"sync"
)

// Store is the in-memory runtime state.
//
// UI reads exclusively from the Store; disk is only touched by Refresh()
// (re-scan) or by the mutation methods below (which call Refresh themselves).
type Store struct {
	mu          sync.RWMutex
	sources     []DataSource
	agents      []Agent
	skills      []Skill
	assignments map[string]map[string]bool // skill.Key() -> agent.Name -> assigned
	runtime     *Runtime
}

func NewStore() *Store {
	return &Store{assignments: map[string]map[string]bool{}}
}

// Init loads runtime/source/agents from disk, applies first-run defaults if
// needed, then performs a full Refresh.
func (s *Store) Init() error {
	rt, err := loadRuntime()
	if err != nil {
		return err
	}

	if rt.FirstRun {
		defaults := []DataSource{
			{Name: "default", Path: "~/.agents/skills", Visible: true},
		}
		if err := saveSources(defaults); err != nil {
			return err
		}
		defaultAgents := []Agent{
			{Name: "claude-code", Path: "~/.claude/skills", Visible: true},
			{Name: "opencode-code", Path: "~/.config/opencode/skills", Visible: true},
		}
		if err := saveAgents(defaultAgents); err != nil {
			return err
		}
		rt.FirstRun = false
		if err := saveRuntime(rt); err != nil {
			return err
		}
	}

	sources, err := loadSources()
	if err != nil {
		return err
	}
	agents, err := loadAgents()
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.runtime = rt
	s.sources = sources
	s.agents = agents
	s.mu.Unlock()

	return s.Refresh()
}

// Refresh re-scans every data source, recomputes assignments, refreshes the
// per-source count, and persists source.json.
func (s *Store) Refresh() error {
	s.mu.Lock()

	// Drop sources whose path is no longer a valid directory; they will be
	// removed from source.json on the save below.
	valid := s.sources[:0]
	for _, ds := range s.sources {
		if IsDirPath(ds.Path) {
			valid = append(valid, ds)
		}
	}
	s.sources = valid

	var allSkills []Skill
	for i, ds := range s.sources {
		skills := scanOneSource(ds)
		s.sources[i].Count = len(skills)
		allSkills = append(allSkills, skills...)
	}
	s.skills = allSkills

	assignments := make(map[string]map[string]bool, len(allSkills))
	for _, sk := range allSkills {
		m := make(map[string]bool, len(s.agents))
		for _, ag := range s.agents {
			m[ag.Name] = isAssigned(ag, sk)
		}
		assignments[sk.Key()] = m
	}
	s.assignments = assignments

	srcSnap := make([]DataSource, len(s.sources))
	copy(srcSnap, s.sources)
	s.mu.Unlock()

	return saveSources(srcSnap)
}

// ───── Read API (returns snapshots; safe for concurrent readers) ─────

func (s *Store) Sources() []DataSource {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]DataSource, len(s.sources))
	copy(out, s.sources)
	return out
}

func (s *Store) Agents() []Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Agent, len(s.agents))
	copy(out, s.agents)
	return out
}

func (s *Store) VisibleAgents() []Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Agent
	for _, a := range s.agents {
		if a.Visible {
			out = append(out, a)
		}
	}
	return out
}

func (s *Store) Skills() []Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Skill, len(s.skills))
	copy(out, s.skills)
	return out
}

// IsAssigned returns the cached assignment state for (skill, agent).
func (s *Store) IsAssigned(skillKey, agentName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.assignments[skillKey]
	return m != nil && m[agentName]
}

// SourceHasAssigned returns true if any skill belonging to sourceName is
// currently assigned to any agent.
func (s *Store) SourceHasAssigned(sourceName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sk := range s.skills {
		if sk.DataSource != sourceName {
			continue
		}
		for _, v := range s.assignments[sk.Key()] {
			if v {
				return true
			}
		}
	}
	return false
}

// ───── Mutation API ─────

func (s *Store) AddSource(ds DataSource) error {
	ds.Name = strings.TrimSpace(ds.Name)
	ds.Path = NormalizePath(ds.Path)

	if !IsDirPath(ds.Path) {
		return fmt.Errorf("路径不存在或不是文件夹: %s", ds.Path)
	}

	s.mu.Lock()
	expanded := ExpandPath(ds.Path)
	for _, existing := range s.sources {
		if existing.Name == ds.Name {
			s.mu.Unlock()
			return fmt.Errorf("名称 %q 已存在", ds.Name)
		}
		if ExpandPath(existing.Path) == expanded {
			s.mu.Unlock()
			return fmt.Errorf("路径已存在（数据源 %q）", existing.Name)
		}
	}
	s.sources = append(s.sources, ds)
	s.mu.Unlock()
	return s.Refresh()
}

func (s *Store) UpdateSource(idx int, ds DataSource) error {
	ds.Name = strings.TrimSpace(ds.Name)
	ds.Path = NormalizePath(ds.Path)

	if !IsDirPath(ds.Path) {
		return fmt.Errorf("路径不存在或不是文件夹: %s", ds.Path)
	}

	s.mu.Lock()
	if idx < 0 || idx >= len(s.sources) {
		s.mu.Unlock()
		return fmt.Errorf("索引越界")
	}
	expanded := ExpandPath(ds.Path)
	for i, existing := range s.sources {
		if i == idx {
			continue
		}
		if existing.Name == ds.Name {
			s.mu.Unlock()
			return fmt.Errorf("名称 %q 已存在", ds.Name)
		}
		if ExpandPath(existing.Path) == expanded {
			s.mu.Unlock()
			return fmt.Errorf("路径已存在（数据源 %q）", existing.Name)
		}
	}
	s.sources[idx] = ds
	s.mu.Unlock()
	return s.Refresh()
}

func (s *Store) RemoveSource(idx int) error {
	s.mu.Lock()
	if idx < 0 || idx >= len(s.sources) {
		s.mu.Unlock()
		return nil
	}
	s.sources = append(s.sources[:idx], s.sources[idx+1:]...)
	s.mu.Unlock()
	return s.Refresh()
}

func (s *Store) AddAgent(ag Agent) error {
	ag.Name = strings.TrimSpace(ag.Name)
	ag.Path = NormalizePath(ag.Path)

	s.mu.Lock()
	expanded := ExpandPath(ag.Path)
	for _, existing := range s.agents {
		if existing.Name == ag.Name {
			s.mu.Unlock()
			return fmt.Errorf("名称 %q 已存在", ag.Name)
		}
		if ExpandPath(existing.Path) == expanded {
			s.mu.Unlock()
			return fmt.Errorf("路径已存在（agent %q）", existing.Name)
		}
	}
	s.agents = append(s.agents, ag)
	snap := make([]Agent, len(s.agents))
	copy(snap, s.agents)
	s.mu.Unlock()
	if err := saveAgents(snap); err != nil {
		return err
	}
	return s.Refresh()
}

func (s *Store) UpdateAgent(idx int, ag Agent) error {
	ag.Name = strings.TrimSpace(ag.Name)
	ag.Path = NormalizePath(ag.Path)

	s.mu.Lock()
	if idx < 0 || idx >= len(s.agents) {
		s.mu.Unlock()
		return fmt.Errorf("索引越界")
	}
	expanded := ExpandPath(ag.Path)
	for i, existing := range s.agents {
		if i == idx {
			continue
		}
		if existing.Name == ag.Name {
			s.mu.Unlock()
			return fmt.Errorf("名称 %q 已存在", ag.Name)
		}
		if ExpandPath(existing.Path) == expanded {
			s.mu.Unlock()
			return fmt.Errorf("路径已存在（agent %q）", existing.Name)
		}
	}
	s.agents[idx] = ag
	snap := make([]Agent, len(s.agents))
	copy(snap, s.agents)
	s.mu.Unlock()
	if err := saveAgents(snap); err != nil {
		return err
	}
	return s.Refresh()
}

func (s *Store) RemoveAgent(idx int) error {
	s.mu.Lock()
	if idx < 0 || idx >= len(s.agents) {
		s.mu.Unlock()
		return nil
	}
	s.agents = append(s.agents[:idx], s.agents[idx+1:]...)
	snap := make([]Agent, len(s.agents))
	copy(snap, s.agents)
	s.mu.Unlock()
	if err := saveAgents(snap); err != nil {
		return err
	}
	return s.Refresh()
}
