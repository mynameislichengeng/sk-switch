package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Store is the in-memory runtime state.
//
// UI reads exclusively from the Store; disk is only touched by Refresh()
// (re-scan) or by the mutation methods below (which call Refresh themselves).
//
// Two domains live side-by-side: SKILLS (sources/agents/skills/assignments,
// scanned from filesystem) and MCP (mcps/mcpAgents, fully driven by
// mcp-data.json + mcp-agents.json — the actual per-agent files are written
// reactively by AssignMCP/UnassignMCP).
type Store struct {
	mu sync.RWMutex

	// SKILLS
	sources     []DataSource
	agents      []Agent
	skills      []Skill
	assignments map[string]map[string]bool // skill.Key() -> agent.Name -> assigned

	// MCP
	mcps      []MCP
	mcpAgents []MCPAgent

	runtime *Runtime
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
	mcps, err := loadMCPs()
	if err != nil {
		return err
	}
	mcpAgents, err := loadMCPAgents()
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.runtime = rt
	s.sources = sources
	s.agents = agents
	s.mcps = mcps
	s.mcpAgents = mcpAgents
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

// Runtime returns a snapshot of the runtime state. Callers must not mutate the
// returned pointer's fields directly; use SetLastModule etc.
func (s *Store) Runtime() Runtime {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.runtime == nil {
		return Runtime{}
	}
	return *s.runtime
}

// SetLastModule persists the module the user last entered so the next launch
// skips the entry modal. Empty string clears it.
func (s *Store) SetLastModule(name string) error {
	s.mu.Lock()
	if s.runtime == nil {
		s.runtime = &Runtime{}
	}
	if s.runtime.LastModule == name {
		s.mu.Unlock()
		return nil
	}
	s.runtime.LastModule = name
	snap := *s.runtime
	s.mu.Unlock()
	return saveRuntime(&snap)
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

// ───── MCP read API ─────

func (s *Store) MCPs() []MCP {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MCP, len(s.mcps))
	for i, m := range s.mcps {
		out[i] = cloneMCP(m)
	}
	return out
}

func (s *Store) MCPAgents() []MCPAgent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MCPAgent, len(s.mcpAgents))
	copy(out, s.mcpAgents)
	return out
}

func (s *Store) VisibleMCPAgents() []MCPAgent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []MCPAgent
	for _, a := range s.mcpAgents {
		if a.Visible {
			out = append(out, a)
		}
	}
	return out
}

// MCPByName returns the MCP with the given name and its index, or
// (MCP{}, -1) when not found.
func (s *Store) MCPByName(name string) (MCP, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i, m := range s.mcps {
		if m.Name == name {
			return cloneMCP(m), i
		}
	}
	return MCP{}, -1
}

// IsMCPAssigned consults mcp-data.json (the source of truth), not the
// agent's actual file. See AssignMCP for the reconcile rules.
func (s *Store) IsMCPAssigned(mcpName, agentName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.mcps {
		if m.Name != mcpName {
			continue
		}
		for _, a := range m.Assignments {
			if a == agentName {
				return true
			}
		}
		return false
	}
	return false
}

// MCPHasAssignments reports whether the named MCP has at least one
// assignment recorded — gates editing/deletion in the strict mode.
func (s *Store) MCPHasAssignments(mcpName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.mcps {
		if m.Name == mcpName {
			return len(m.Assignments) > 0
		}
	}
	return false
}

// MCPAgentInUse reports whether any MCP is assigned to the named agent.
// Used to gate MCPAgent removal.
func (s *Store) MCPAgentInUse(agentName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.mcps {
		for _, a := range m.Assignments {
			if a == agentName {
				return true
			}
		}
	}
	return false
}

// ───── MCP mutation API ─────

func (s *Store) AddMCP(mcp MCP) error {
	mcp.Name = normalizeMCPName(mcp.Name)
	mcp.GithubURL = normalizeGithub(mcp.GithubURL)
	if mcp.Name == "" {
		return fmt.Errorf("名称不能为空")
	}
	if err := ValidateMCPConfig(mcp.Config); err != nil {
		return err
	}

	s.mu.Lock()
	for _, existing := range s.mcps {
		if existing.Name == mcp.Name {
			s.mu.Unlock()
			return fmt.Errorf("%w: %q", ErrMCPNameExists, mcp.Name)
		}
		if mcp.GithubURL != "" && existing.GithubURL == mcp.GithubURL {
			s.mu.Unlock()
			return fmt.Errorf("%w: %q (已被 %q 占用)", ErrMCPGithubExists, mcp.GithubURL, existing.Name)
		}
	}
	mcp.Assignments = nil // never trust caller-supplied state
	s.mcps = append(s.mcps, mcp)
	snap := cloneMCPs(s.mcps)
	s.mu.Unlock()
	return saveMCPs(snap)
}

// UpdateMCP replaces the MCP at idx. Strict-edit policy: caller must check
// MCPHasAssignments() first; this method will reject any change to a
// currently-assigned MCP regardless.
func (s *Store) UpdateMCP(idx int, mcp MCP) error {
	mcp.Name = normalizeMCPName(mcp.Name)
	mcp.GithubURL = normalizeGithub(mcp.GithubURL)
	if mcp.Name == "" {
		return fmt.Errorf("名称不能为空")
	}
	if err := ValidateMCPConfig(mcp.Config); err != nil {
		return err
	}

	s.mu.Lock()
	if idx < 0 || idx >= len(s.mcps) {
		s.mu.Unlock()
		return fmt.Errorf("%w: 索引 %d 越界", ErrMCPNotFound, idx)
	}
	current := s.mcps[idx]
	if len(current.Assignments) > 0 {
		s.mu.Unlock()
		return ErrMCPHasAssignments
	}
	for i, existing := range s.mcps {
		if i == idx {
			continue
		}
		if existing.Name == mcp.Name {
			s.mu.Unlock()
			return fmt.Errorf("%w: %q", ErrMCPNameExists, mcp.Name)
		}
		if mcp.GithubURL != "" && existing.GithubURL == mcp.GithubURL {
			s.mu.Unlock()
			return fmt.Errorf("%w: %q (已被 %q 占用)", ErrMCPGithubExists, mcp.GithubURL, existing.Name)
		}
	}
	mcp.Assignments = current.Assignments // preserve (should be empty per check above)
	s.mcps[idx] = mcp
	snap := cloneMCPs(s.mcps)
	s.mu.Unlock()
	return saveMCPs(snap)
}

// RemoveMCP removes the MCP at idx. Returns ErrMCPHasAssignments if any
// agent has it assigned.
func (s *Store) RemoveMCP(idx int) error {
	s.mu.Lock()
	if idx < 0 || idx >= len(s.mcps) {
		s.mu.Unlock()
		return fmt.Errorf("%w: 索引 %d 越界", ErrMCPNotFound, idx)
	}
	if len(s.mcps[idx].Assignments) > 0 {
		s.mu.Unlock()
		return ErrMCPHasAssignments
	}
	s.mcps = append(s.mcps[:idx], s.mcps[idx+1:]...)
	snap := cloneMCPs(s.mcps)
	s.mu.Unlock()
	return saveMCPs(snap)
}

// ───── MCP agent mutation API ─────

func (s *Store) AddMCPAgent(ag MCPAgent) error {
	ag.Name = strings.TrimSpace(ag.Name)
	ag.Path = NormalizePath(ag.Path)
	ag.Type = strings.TrimSpace(ag.Type)
	if ag.Name == "" {
		return fmt.Errorf("名称不能为空")
	}
	if ag.Path == "" {
		return fmt.Errorf("路径不能为空")
	}
	writer, err := MCPWriterFor(ag.Type)
	if err != nil {
		return err
	}
	if err := writer.Validate(ag.Path); err != nil {
		return err
	}

	s.mu.Lock()
	for _, existing := range s.mcpAgents {
		if existing.Name == ag.Name {
			s.mu.Unlock()
			return fmt.Errorf("名称 %q 已存在", ag.Name)
		}
	}
	s.mcpAgents = append(s.mcpAgents, ag)
	snap := make([]MCPAgent, len(s.mcpAgents))
	copy(snap, s.mcpAgents)
	s.mu.Unlock()
	return saveMCPAgents(snap)
}

func (s *Store) UpdateMCPAgent(idx int, ag MCPAgent) error {
	ag.Name = strings.TrimSpace(ag.Name)
	ag.Path = NormalizePath(ag.Path)
	ag.Type = strings.TrimSpace(ag.Type)
	if ag.Name == "" {
		return fmt.Errorf("名称不能为空")
	}
	if ag.Path == "" {
		return fmt.Errorf("路径不能为空")
	}
	writer, err := MCPWriterFor(ag.Type)
	if err != nil {
		return err
	}
	if err := writer.Validate(ag.Path); err != nil {
		return err
	}

	s.mu.Lock()
	if idx < 0 || idx >= len(s.mcpAgents) {
		s.mu.Unlock()
		return fmt.Errorf("%w: 索引 %d 越界", ErrMCPAgentNotFound, idx)
	}
	oldName := s.mcpAgents[idx].Name
	for i, existing := range s.mcpAgents {
		if i == idx {
			continue
		}
		if existing.Name == ag.Name {
			s.mu.Unlock()
			return fmt.Errorf("名称 %q 已存在", ag.Name)
		}
	}
	s.mcpAgents[idx] = ag
	// If the agent was renamed, propagate to mcps' Assignments lists.
	if oldName != ag.Name {
		for i := range s.mcps {
			for j, a := range s.mcps[i].Assignments {
				if a == oldName {
					s.mcps[i].Assignments[j] = ag.Name
				}
			}
		}
	}
	agSnap := make([]MCPAgent, len(s.mcpAgents))
	copy(agSnap, s.mcpAgents)
	mcpSnap := cloneMCPs(s.mcps)
	s.mu.Unlock()
	if err := saveMCPAgents(agSnap); err != nil {
		return err
	}
	if oldName != ag.Name {
		return saveMCPs(mcpSnap)
	}
	return nil
}

// RemoveMCPAgent removes the agent at idx. Returns ErrMCPAgentInUse if any
// MCP is currently assigned to it (the user must Unassign first).
func (s *Store) RemoveMCPAgent(idx int) error {
	s.mu.Lock()
	if idx < 0 || idx >= len(s.mcpAgents) {
		s.mu.Unlock()
		return fmt.Errorf("%w: 索引 %d 越界", ErrMCPAgentNotFound, idx)
	}
	name := s.mcpAgents[idx].Name
	for _, m := range s.mcps {
		for _, a := range m.Assignments {
			if a == name {
				s.mu.Unlock()
				return ErrMCPAgentInUse
			}
		}
	}
	s.mcpAgents = append(s.mcpAgents[:idx], s.mcpAgents[idx+1:]...)
	snap := make([]MCPAgent, len(s.mcpAgents))
	copy(snap, s.mcpAgents)
	s.mu.Unlock()
	return saveMCPAgents(snap)
}

// ───── MCP assignment API ─────

// AssignMCP records mcpName as assigned to agentName, writing the agent's
// config file as needed.
//
// Reconcile rules (the user-defined contract):
//   - existing key with same payload → just record in mcp-data.json (no write).
//   - existing key with different payload → return *MCPConflict (caller can
//     ask the user, then call again with force=true to overwrite).
//   - missing key → write it + record.
//
// agent file missing entirely → ErrMCPFileMissing surfaces unchanged.
func (s *Store) AssignMCP(mcpName, agentName string, force bool) error {
	s.mu.Lock()
	mcpIdx := -1
	for i, m := range s.mcps {
		if m.Name == mcpName {
			mcpIdx = i
			break
		}
	}
	if mcpIdx < 0 {
		s.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrMCPNotFound, mcpName)
	}
	var agent MCPAgent
	found := false
	for _, a := range s.mcpAgents {
		if a.Name == agentName {
			agent = a
			found = true
			break
		}
	}
	if !found {
		s.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrMCPAgentNotFound, agentName)
	}
	mcp := cloneMCP(s.mcps[mcpIdx])
	s.mu.Unlock()

	writer, err := MCPWriterFor(agent.Type)
	if err != nil {
		return err
	}
	existing, readErr := writer.Read(agent.Path, mcpName)
	if readErr != nil && readErr != ErrMCPFileMissing {
		return readErr
	}
	if existing != nil && !equivalentJSON(existing, mcp.Config) && !force {
		return &MCPConflict{
			MCPName:     mcpName,
			AgentName:   agentName,
			ExistingRaw: existing,
			NewRaw:      mcp.Config,
		}
	}
	// Write only when the file lacks the key or the user opted to overwrite.
	if existing == nil || (existing != nil && !equivalentJSON(existing, mcp.Config)) {
		if readErr == ErrMCPFileMissing {
			return readErr
		}
		if err := writer.Write(agent.Path, mcpName, mcp.Config); err != nil {
			return err
		}
	}
	return s.recordAssignment(mcpName, agentName, true)
}

// UnassignMCP removes agentName from mcpName's Assignments and deletes the
// matching entry from the agent's config file. Per user spec: if the file
// already lacks the key, just update mcp-data.json (no error).
func (s *Store) UnassignMCP(mcpName, agentName string) error {
	s.mu.Lock()
	var agent MCPAgent
	for _, a := range s.mcpAgents {
		if a.Name == agentName {
			agent = a
			break
		}
	}
	s.mu.Unlock()

	if agent.Name != "" {
		writer, err := MCPWriterFor(agent.Type)
		if err != nil {
			return err
		}
		// Best effort: file missing or key already gone → still update record.
		if err := writer.Delete(agent.Path, mcpName); err != nil && err != ErrMCPFileMissing {
			return err
		}
	}
	return s.recordAssignment(mcpName, agentName, false)
}

// recordAssignment updates mcp-data.json's assignments list. assigned=true
// adds, false removes; both are idempotent.
func (s *Store) recordAssignment(mcpName, agentName string, assigned bool) error {
	s.mu.Lock()
	idx := -1
	for i, m := range s.mcps {
		if m.Name == mcpName {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrMCPNotFound, mcpName)
	}
	cur := s.mcps[idx].Assignments
	out := cur[:0:0]
	already := false
	for _, a := range cur {
		if a == agentName {
			already = true
			if !assigned {
				continue // dropping
			}
		}
		out = append(out, a)
	}
	if assigned && !already {
		out = append(out, agentName)
	}
	s.mcps[idx].Assignments = out
	snap := cloneMCPs(s.mcps)
	s.mu.Unlock()
	return saveMCPs(snap)
}

// cloneMCP makes a deep copy so mutations to the returned MCP don't leak
// into the Store-held slice. Config is a RawMessage (immutable bytes), so
// only Assignments needs explicit copy.
func cloneMCP(m MCP) MCP {
	out := m
	if m.Assignments != nil {
		out.Assignments = make([]string, len(m.Assignments))
		copy(out.Assignments, m.Assignments)
	}
	if m.Config != nil {
		out.Config = make(json.RawMessage, len(m.Config))
		copy(out.Config, m.Config)
	}
	return out
}

func cloneMCPs(in []MCP) []MCP {
	out := make([]MCP, len(in))
	for i, m := range in {
		out[i] = cloneMCP(m)
	}
	return out
}
