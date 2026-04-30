package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mynameislichengeng/sk-switch/config"
)

// listMode controls how skills are ordered and rendered in 技能列表 tab.
//
// The mode lives only on ListModel; switching does not touch filter, draft,
// scroll position semantics, or cursor identity (the focused skill stays the
// same — see ListModel.toggleListMode).
type listMode int

const (
	listModeGroup listMode = iota // 按 sk.Source 分组，每组一个表格 (default)
	listModeAlpha                 // 按 sk.Name 字母升序，单表平铺
)

func (m listMode) label() string {
	switch m {
	case listModeAlpha:
		return "(字母)"
	default:
		return "(分组)"
	}
}

// orderSkills returns a copy of `all` ordered for the given listMode.
// Filter is *not* applied here — that stays in ListModel.applyFilter.
func orderSkills(all []config.Skill, mode listMode) []config.Skill {
	if mode == listModeAlpha {
		return alphaSorted(all)
	}
	return groupOrdered(all)
}

// groupOrdered preserves first-occurrence Source order, with each source's
// skills in the order they were scanned.
func groupOrdered(all []config.Skill) []config.Skill {
	groups := groupBySource(all)
	out := make([]config.Skill, 0, len(all))
	for _, g := range groups {
		out = append(out, g.skills...)
	}
	return out
}

// alphaSorted: case-insensitive Name asc, DataSource asc tiebreak.
func alphaSorted(all []config.Skill) []config.Skill {
	out := make([]config.Skill, len(all))
	copy(out, all)
	sort.SliceStable(out, func(i, j int) bool {
		ni := strings.ToLower(out[i].Name)
		nj := strings.ToLower(out[j].Name)
		if ni != nj {
			return ni < nj
		}
		return out[i].DataSource < out[j].DataSource
	})
	return out
}

// listViewRenderer renders an ordered skill list and locates the cursor row.
// Each implementation matches one listMode. Implementations are stateless;
// they receive the already-ordered skills slice.
type listViewRenderer interface {
	renderLines(skills []config.Skill, agents []config.Agent, cursor int, linkState map[[2]int]bool, viewWidth int) []string
	cursorLineNumber(skills []config.Skill, cursor int) int
}

func rendererFor(mode listMode) listViewRenderer {
	if mode == listModeAlpha {
		return alphaRenderer{}
	}
	return groupRenderer{}
}

// ───── group renderer ──────────────────────────────────────────────────

type groupRenderer struct{}

func (groupRenderer) renderLines(skills []config.Skill, agents []config.Agent, cursor int, linkState map[[2]int]bool, viewWidth int) []string {
	groupStyle := lipgloss.NewStyle().Foreground(theme.PopupBorder)

	var allLines []string
	groups := groupBySource(skills)
	rowIdx := 0
	for _, g := range groups {
		allLines = append(allLines, "")
		allLines = append(allLines, groupStyle.Render(fmt.Sprintf("%s（%d）", g.source, len(g.skills))))
		localCur := -1
		if cursor >= rowIdx && cursor < rowIdx+len(g.skills) {
			localCur = cursor - rowIdx
		}
		allLines = append(allLines, renderTableLines(g.skills, agents, localCur, rowIdx, linkState, viewWidth)...)
		rowIdx += len(g.skills)
	}
	allLines = append(allLines, "", "")
	return allLines
}

func (groupRenderer) cursorLineNumber(skills []config.Skill, cursor int) int {
	line := 1
	groups := groupBySource(skills)
	rowIdx := 0
	for gi, g := range groups {
		if gi > 0 {
			line += 1
		}
		line += 1 + 2 // title + header + sep
		if cursor >= rowIdx && cursor < rowIdx+len(g.skills) {
			return line + (cursor - rowIdx)
		}
		line += len(g.skills)
		rowIdx += len(g.skills)
	}
	return line
}

// ───── alpha renderer ──────────────────────────────────────────────────

type alphaRenderer struct{}

func (alphaRenderer) renderLines(skills []config.Skill, agents []config.Agent, cursor int, linkState map[[2]int]bool, viewWidth int) []string {
	var allLines []string
	allLines = append(allLines, "")
	allLines = append(allLines, renderTableLines(skills, agents, cursor, 0, linkState, viewWidth)...)
	allLines = append(allLines, "", "")
	return allLines
}

func (alphaRenderer) cursorLineNumber(skills []config.Skill, cursor int) int {
	// lines[0]="", lines[1]=header, lines[2]=sep, lines[3]=skill 0
	return 3 + cursor
}
