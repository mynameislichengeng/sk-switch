package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mynameislichengeng/sk-switch/config"
	"github.com/mynameislichengeng/sk-switch/internal/jsontree"
)

// mcpAgentView is a full-tab read-only viewer for an MCP agent's config
// file (e.g. ~/.claude/.claude.json). Mirrors skillView in shape: while
// it's active the parent treats this as a "special state" so global
// Tab/Ctrl+P/r hotkeys are suppressed.
//
// Two display modes share one viewport:
//
//   1. Tree mode (preferred when the file parses as JSON): a jsontree.Viewer
//      from internal/jsontree owns cursor + expanded state. ↑↓ moves the
//      cursor between nodes, Space/Enter toggles fold, the surrounding
//      viewport handles scroll.
//   2. Text mode (everything else — read errors, empty files, malformed
//      JSON): the body is glamour-rendered once and dumped into the
//      viewport, same as the SKILL.md viewer. Cursor / fold keys do
//      nothing in this mode.
//
// File reading and (in text mode) glamour rendering happen in a goroutine
// via tea.Cmd so the UI doesn't freeze. Tree parsing happens there too —
// the result is sent over via mcpAgentRenderedMsg.
type mcpAgentView struct {
	active   bool
	agent    config.MCPAgent
	viewport viewport.Model
	err      string

	// Tree mode state (nil in text mode).
	tree *jsontree.Viewer

	// Tree-mode search state (vim-style "/" + Enter, then `n` for next).
	// searching=true means the user is currently typing the query in
	// the input bar; once Enter commits, searching flips back to false
	// and searchQuery sticks around so `n` can jump to subsequent hits.
	// searchMessage carries a one-shot status line (e.g. "未找到 …") that
	// the next tree-mode keystroke clears.
	searching         bool
	searchQuery       string
	searchInputCursor int
	searchMessage     string

	// Text mode payload (kept so Resize can re-render at new width).
	textRaw    string
	textIsJSON bool
}

// mcpAgentRenderedMsg carries the file contents back from the async reader.
// agentName lets the parent drop a stale result if the user already closed
// the viewer or opened a different agent before the read completed.
//
// When `treeRoot` is non-nil the viewer enters tree mode and `content` is
// ignored. Otherwise `content` is shown verbatim (already glamour-styled).
type mcpAgentRenderedMsg struct {
	agentName string
	treeRoot  *jsontree.Node // non-nil → tree mode
	textRaw   string         // raw body for text mode (for re-render on resize)
	isJSON    bool           // text mode: true if body is pretty-printed JSON
	content   string         // text mode: pre-rendered output for SetContent
	err       string
}

func (v *mcpAgentView) Open(ag config.MCPAgent, w, h int) tea.Cmd {
	v.active = true
	v.agent = ag
	v.err = ""
	v.tree = nil
	v.textRaw = ""
	v.textIsJSON = false
	v.searching = false
	v.searchQuery = ""
	v.searchInputCursor = 0
	v.searchMessage = ""
	v.viewport = viewport.New(w, h)
	v.viewport.SetContent(lipgloss.NewStyle().Faint(true).Render("加载中…"))
	return readMCPAgentFileCmd(ag, w)
}

func (v *mcpAgentView) Close() {
	v.active = false
	v.tree = nil
	v.textRaw = ""
	v.textIsJSON = false
	v.searching = false
	v.searchQuery = ""
	v.searchInputCursor = 0
	v.searchMessage = ""
	v.err = ""
}

// IsSearching tells the parent whether the viewer is currently consuming
// keystrokes for its search input bar — when true the parent must NOT
// treat Esc / q as "close view", because those keys belong to the input
// (Esc cancels search; q is just a typed character).
func (v mcpAgentView) IsSearching() bool { return v.searching }

func (v *mcpAgentView) Resize(w, h int) {
	v.viewport.Width = w
	v.viewport.Height = h
	switch {
	case v.tree != nil:
		v.refreshTreeContent()
	case v.textRaw != "":
		v.viewport.SetContent(renderJSONForViewer(v.textRaw, v.textIsJSON, w))
	}
}

func (v *mcpAgentView) ApplyRender(msg mcpAgentRenderedMsg) {
	if !v.active || v.agent.Name != msg.agentName {
		return
	}
	if msg.err != "" {
		v.err = msg.err
		v.tree = nil
		return
	}
	if msg.treeRoot != nil {
		v.tree = jsontree.NewViewer(msg.treeRoot, jsonTreeStyleForApp())
		v.textRaw = ""
		v.textIsJSON = false
		v.refreshTreeContent()
		return
	}
	v.tree = nil
	v.textRaw = msg.textRaw
	v.textIsJSON = msg.isJSON
	v.viewport.SetContent(msg.content)
}

// jsonTreeStyleForApp adapts the package's default palette to the host
// app's cursor color so the focus marker matches the popups elsewhere in
// the TUI. Called every time a tree is opened so theme changes pick up
// on next render.
func jsonTreeStyleForApp() jsontree.Style {
	s := jsontree.DefaultStyle()
	s.Cursor = popupActiveStyle
	return s
}

// refreshTreeContent re-renders the tree, pushes it to the viewport, and
// scrolls the viewport so the cursor's line is visible. Called after every
// state change in tree mode (cursor move / fold toggle / resize).
func (v *mcpAgentView) refreshTreeContent() {
	content, _, cursorLine := v.tree.Render()
	v.viewport.SetContent(content)
	h := v.viewport.Height
	if h <= 0 {
		return
	}
	off := v.viewport.YOffset
	switch {
	case cursorLine < off:
		v.viewport.SetYOffset(cursorLine)
	case cursorLine >= off+h:
		v.viewport.SetYOffset(cursorLine - h + 1)
	}
}

// Update routes keys. Tree mode handles ↑↓/space/enter/g/G itself and
// hands PgUp/PgDn-style scroll keys off to the viewport. Text mode is the
// SKILL.md-style viewer: g/G top/bottom, everything else to viewport.
func (v *mcpAgentView) Update(msg tea.Msg) tea.Cmd {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		v.viewport, cmd = v.viewport.Update(msg)
		return cmd
	}
	if v.tree != nil {
		if v.searching {
			return v.handleSearchKey(km)
		}
		return v.handleTreeKey(km)
	}
	switch km.String() {
	case "g":
		v.viewport.GotoTop()
		return nil
	case "G":
		v.viewport.GotoBottom()
		return nil
	}
	var cmd tea.Cmd
	v.viewport, cmd = v.viewport.Update(msg)
	return cmd
}

// handleTreeKey owns cursor + fold state. Anything we don't recognize is
// forwarded to the viewport so PgUp/PgDn / mouse-wheel still scroll.
//
// Any keystroke clears searchMessage — the "未找到 …" hint is one-shot
// and shouldn't linger past the next interaction.
func (v *mcpAgentView) handleTreeKey(km tea.KeyMsg) tea.Cmd {
	v.searchMessage = ""
	switch km.String() {
	case "up", "k":
		v.tree.MoveUp()
	case "down", "j":
		v.tree.MoveDown()
	case "g":
		v.tree.MoveTop()
	case "G":
		v.tree.MoveBottom()
	case " ", "enter":
		v.tree.Toggle()
	case "/":
		v.searching = true
		v.searchQuery = ""
		v.searchInputCursor = 0
		return nil
	case "n":
		// "find next" — only meaningful after a prior committed search.
		if strings.TrimSpace(v.searchQuery) == "" {
			return nil
		}
		if hit := v.tree.FindNext(v.searchQuery); hit == nil {
			v.searchMessage = fmt.Sprintf("未找到「%s」", v.searchQuery)
		}
	default:
		var cmd tea.Cmd
		v.viewport, cmd = v.viewport.Update(km)
		return cmd
	}
	v.refreshTreeContent()
	return nil
}

// handleSearchKey routes keystrokes while the user is typing in the
// search bar. The bar is a single-line rune buffer that supports the
// usual cursor / backspace / delete actions; Enter commits a Find,
// Esc cancels (cursor stays on whatever node it was on before "/").
//
// Esc and q here are NOT "close view" — that gate lives one layer up
// in mcp_agents.go which checks IsSearching() before swallowing them.
func (v *mcpAgentView) handleSearchKey(km tea.KeyMsg) tea.Cmd {
	switch {
	case keyPress(km, "esc"):
		v.searching = false
		v.searchQuery = ""
		v.searchInputCursor = 0
		v.searchMessage = ""
		return nil
	case keyPress(km, "enter"):
		// Vim-style search: jump to the next match strictly after the
		// current cursor (with wrap). This is FindNext, NOT Find from
		// top — that way `/foo<CR>` and the subsequent `n` keys behave
		// identically (each call advances), matching what vim/less
		// users expect. On a fresh open the cursor is on root which
		// never matches, so the first call still lands on the very
		// first match in the tree.
		v.searching = false
		query := strings.TrimSpace(v.searchQuery)
		if query == "" {
			v.searchQuery = ""
			return nil
		}
		if hit := v.tree.FindNext(v.searchQuery); hit == nil {
			v.searchMessage = fmt.Sprintf("未找到「%s」", v.searchQuery)
		} else {
			v.searchMessage = ""
		}
		v.refreshTreeContent()
		return nil
	case keyPress(km, "left"):
		if v.searchInputCursor > 0 {
			v.searchInputCursor--
		}
	case keyPress(km, "right"):
		if v.searchInputCursor < runeCount(v.searchQuery) {
			v.searchInputCursor++
		}
	case keyPress(km, "home", "ctrl+a"):
		v.searchInputCursor = 0
	case keyPress(km, "end", "ctrl+e"):
		v.searchInputCursor = runeCount(v.searchQuery)
	case keyPress(km, "backspace"):
		v.searchQuery, v.searchInputCursor = deleteBefore(v.searchQuery, v.searchInputCursor)
	case keyPress(km, "delete"):
		v.searchQuery, v.searchInputCursor = deleteAfter(v.searchQuery, v.searchInputCursor)
	default:
		if r := insertableRunes(km); r != nil {
			v.searchQuery, v.searchInputCursor = insertAt(v.searchQuery, v.searchInputCursor, string(r))
		}
	}
	return nil
}

func (v mcpAgentView) View(width int) string {
	titleStyle := lipgloss.NewStyle().Bold(true)
	header := titleStyle.Render(fmt.Sprintf("%s — %s", v.agent.Name, v.agent.Path))
	if v.err != "" {
		errStyle := lipgloss.NewStyle().Foreground(theme.ModifiedFg)
		body := errStyle.Render(v.err)
		return header + "\n\n" + body + "\n\n" + lipgloss.NewStyle().Faint(true).Render(v.hintText())
	}
	return header + "\n" + v.viewport.View() + "\n" + v.renderBottomBar()
}

// renderBottomBar picks the right footer for the current state. Three
// possible faces: search input bar (highest priority — user is typing),
// search status line (one-shot, e.g. "未找到 …"), or the regular hint
// cheat-sheet.
func (v mcpAgentView) renderBottomBar() string {
	if v.searching {
		return v.renderSearchBar()
	}
	if v.searchMessage != "" {
		return lipgloss.NewStyle().Foreground(theme.ModifiedFg).Render(v.searchMessage)
	}
	return lipgloss.NewStyle().Faint(true).Render(v.hintText())
}

// renderSearchBar draws "/ <input>█    Enter 搜索 | Esc 取消" — the input
// uses the shared cursor-render helper so caret behavior matches every
// other text input in the app (popups, theme editor, etc.).
func (v mcpAgentView) renderSearchBar() string {
	label := popupActiveStyle.Render("/")
	body := renderWithCursor(v.searchQuery, v.searchInputCursor)
	tail := lipgloss.NewStyle().Faint(true).Render("    Enter 搜索 | Esc 取消")
	return label + " " + body + tail
}

// hintText picks the appropriate key-cheat-sheet for the active mode.
// Tree mode advertises fold + search keys; text mode is just scroll.
func (v mcpAgentView) hintText() string {
	if v.tree != nil {
		return "↑↓ 选 | Space/Enter 展开收起 | / 搜索 | n 下一个 | g/G 顶/底 | Esc 返回"
	}
	return "↑↓ 行 | PgUp/PgDn 翻页 | g/G 顶/底 | Esc 返回"
}

// readMCPAgentFileCmd reads the agent's config file off the UI thread. It
// builds a foldable jsontree.Node when the file parses, and falls back to
// glamour-rendered text otherwise so users can still inspect a malformed
// or non-JSON file.
//
// Decoding inside jsontree.Build uses json.Number so that large integers
// commonly found in ~/.claude/.claude.json (Unix-nanosecond timestamps,
// session IDs) survive without float64 precision loss.
func readMCPAgentFileCmd(ag config.MCPAgent, width int) tea.Cmd {
	return func() tea.Msg {
		path := config.ExpandPath(ag.Path)
		data, err := os.ReadFile(path)
		if err != nil {
			return mcpAgentRenderedMsg{
				agentName: ag.Name,
				err:       fmt.Sprintf("读取失败：%s", err),
			}
		}
		if len(data) == 0 {
			return mcpAgentRenderedMsg{
				agentName: ag.Name,
				content:   lipgloss.NewStyle().Faint(true).Render("(文件为空)"),
			}
		}
		// Try tree mode first; that's the user-facing happy path.
		if root, terr := jsontree.Build(data); terr == nil {
			return mcpAgentRenderedMsg{agentName: ag.Name, treeRoot: root}
		}
		// Fall back: data isn't valid JSON. Render it through glamour with
		// a plain ``` fence so #/* in error logs / partial JSON aren't
		// interpreted as markdown formatting.
		body := string(data)
		return mcpAgentRenderedMsg{
			agentName: ag.Name,
			textRaw:   body,
			isJSON:    false,
			content:   renderJSONForViewer(body, false, width),
		}
	}
}

// renderJSONForViewer wraps the body in a markdown code fence and runs it
// through the shared glamour renderer. ```json triggers chroma's JSON lexer
// for syntax highlighting; the plain ``` fence is used for fall-back text
// so any markdown-significant characters in the body are NOT interpreted by
// glamour as formatting. Empty-body case is short-circuited so we don't
// emit an empty code block.
func renderJSONForViewer(body string, isJSON bool, width int) string {
	if body == "" {
		return lipgloss.NewStyle().Faint(true).Render("(文件为空)")
	}
	fence := "```\n"
	if isJSON {
		fence = "```json\n"
	}
	return renderMarkdown(fence+body+"\n```", width)
}
