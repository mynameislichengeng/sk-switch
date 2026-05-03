package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mynameislichengeng/sk-switch/config"
)

// mcpFormMode says whether the form is creating a new MCP or editing an
// existing one. The form lives inside MCPListModel and is mutually exclusive
// with the view/delete/blocked popups.
type mcpFormMode int

const (
	mcpFormAdd mcpFormMode = iota
	mcpFormEdit
)

const (
	mcpFormFieldName = iota
	mcpFormFieldGithub
	mcpFormFieldJSON
	mcpFormFieldCount
)

// mcpForm wraps the three input fields + edit-target index. Single-line
// fields use the existing rune-cursor helpers; the JSON field uses a
// bubbles/textarea so multi-line paste from the user's clipboard works.
type mcpForm struct {
	mode    mcpFormMode
	editIdx int

	name         string
	nameCursor   int
	github       string
	githubCursor int

	jsonArea textarea.Model

	field int // mcpFormFieldName .. mcpFormFieldJSON
	err   string

	width  int
	height int
}

// newMCPForm builds a fresh form. For edit mode call seedEdit() afterwards.
func newMCPForm() mcpForm {
	ta := textarea.New()
	ta.Placeholder = `{"command": "bunx", "args": ["-y", "..."]}`
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.MaxHeight = 12
	ta.SetHeight(8)
	// Strip the textarea's own Tab/Shift+Tab bindings so our parent can
	// reliably grab them as field-switch keys; otherwise textarea inserts
	// a tab character and the form gets stuck.
	ta.KeyMap.InsertNewline.SetKeys("enter")
	// Disable the textarea's Ctrl+S binding (none by default, but explicit
	// note: Ctrl+S is owned by the parent form for "save").
	ta.Focus()
	return mcpForm{
		mode:     mcpFormAdd,
		field:    mcpFormFieldName,
		jsonArea: ta,
	}
}

func (f *mcpForm) SetSize(w, h int) {
	f.width = w
	f.height = h
	// Leave room for borders, labels, hint line, error line. Conservative.
	taW := w - 18
	if taW < 30 {
		taW = 30
	}
	if taW > 100 {
		taW = 100
	}
	f.jsonArea.SetWidth(taW)
	taH := h - 14
	if taH < 4 {
		taH = 4
	}
	if taH > 12 {
		taH = 12
	}
	f.jsonArea.SetHeight(taH)
}

// seedEdit fills the form from an existing MCP for the edit flow. The JSON
// payload is re-indented for readability since the on-disk form may have
// been minified by an earlier save.
func (f *mcpForm) seedEdit(idx int, mcp config.MCP) {
	f.mode = mcpFormEdit
	f.editIdx = idx
	f.name = mcp.Name
	f.nameCursor = runeCount(f.name)
	f.github = mcp.GithubURL
	f.githubCursor = runeCount(f.github)
	f.jsonArea.SetValue(prettyJSON(mcp.Config))
	f.field = mcpFormFieldName
	f.err = ""
}

func (f *mcpForm) seedAdd() {
	f.mode = mcpFormAdd
	f.editIdx = -1
	f.name = ""
	f.nameCursor = 0
	f.github = ""
	f.githubCursor = 0
	f.jsonArea.Reset()
	f.field = mcpFormFieldName
	f.err = ""
}

// activeText returns a pointer to the rune-buffer for the currently focused
// single-line field (name/github), or nil when the JSON textarea is focused.
func (f *mcpForm) activeText() (*string, *int) {
	switch f.field {
	case mcpFormFieldName:
		return &f.name, &f.nameCursor
	case mcpFormFieldGithub:
		return &f.github, &f.githubCursor
	}
	return nil, nil
}

// updateField responds to a key event for the active field. Returns the
// model unchanged + nil cmd when the key is unhandled at this layer.
func (f mcpForm) updateField(km tea.KeyMsg) (mcpForm, tea.Cmd) {
	if f.field == mcpFormFieldJSON {
		var cmd tea.Cmd
		f.jsonArea, cmd = f.jsonArea.Update(km)
		return f, cmd
	}
	target, cursor := f.activeText()
	if target == nil {
		return f, nil
	}
	switch {
	case keyPress(km, "left"):
		if *cursor > 0 {
			*cursor--
		}
	case keyPress(km, "right"):
		if n := runeCount(*target); *cursor < n {
			*cursor++
		}
	case keyPress(km, "home", "ctrl+a"):
		*cursor = 0
	case keyPress(km, "end", "ctrl+e"):
		*cursor = runeCount(*target)
	case keyPress(km, "backspace"):
		*target, *cursor = deleteBefore(*target, *cursor)
	case keyPress(km, "delete"):
		*target, *cursor = deleteAfter(*target, *cursor)
	default:
		if r := insertableRunes(km); r != nil {
			*target, *cursor = insertAt(*target, *cursor, string(r))
		}
	}
	return f, nil
}

// nextField / prevField cycle through the three form fields and (re-)focus
// the JSON textarea as needed.
func (f *mcpForm) nextField() {
	f.field = (f.field + 1) % mcpFormFieldCount
	f.syncFocus()
}

func (f *mcpForm) prevField() {
	f.field = (f.field - 1 + mcpFormFieldCount) % mcpFormFieldCount
	f.syncFocus()
}

func (f *mcpForm) syncFocus() {
	if f.field == mcpFormFieldJSON {
		f.jsonArea.Focus()
	} else {
		f.jsonArea.Blur()
	}
}

// commit attempts to validate inputs and call the right Store method. On
// success, returns nil; on validation failure, sets f.err and returns the
// non-nil error so the caller knows to keep the form open.
func (f *mcpForm) commit(store *config.Store) error {
	name := strings.TrimSpace(f.name)
	gh := strings.TrimSpace(f.github)
	jsonStr := strings.TrimSpace(f.jsonArea.Value())

	if name == "" {
		f.err = "名称不能为空"
		return fmt.Errorf("name empty")
	}
	if jsonStr == "" {
		f.err = "config JSON 不能为空"
		return fmt.Errorf("json empty")
	}
	raw := json.RawMessage(jsonStr)
	if err := config.ValidateMCPConfig(raw); err != nil {
		f.err = err.Error()
		return err
	}
	mcp := config.MCP{
		Name:      name,
		GithubURL: gh,
		Config:    raw,
	}
	var err error
	if f.mode == mcpFormAdd {
		err = store.AddMCP(mcp)
	} else {
		err = store.UpdateMCP(f.editIdx, mcp)
	}
	if err != nil {
		f.err = err.Error()
		return err
	}
	return nil
}

// View renders the form as a centered popup. Caller is responsible for
// placing the result inside the surrounding tab content.
func (f mcpForm) View() string {
	titleStyle := popupTitleStyle
	keyStyle := lipgloss.NewStyle().Faint(true)
	activeStyle := popupActiveStyle
	errStyle := lipgloss.NewStyle().Foreground(theme.ModifiedFg)

	title := "新增 MCP"
	if f.mode == mcpFormEdit {
		title = "编辑 MCP"
	}

	var lines []string
	lines = append(lines, titleStyle.Render(title), "")

	// Name field
	lines = append(lines, renderSingleLine("名称  :", f.name, f.nameCursor,
		f.field == mcpFormFieldName, keyStyle, activeStyle))
	lines = append(lines, renderSingleLine("GitHub:", f.github, f.githubCursor,
		f.field == mcpFormFieldGithub, keyStyle, activeStyle))

	// JSON field — multi-line textarea
	jsonHeader := keyStyle.Render("JSON  :")
	if f.field == mcpFormFieldJSON {
		jsonHeader = activeStyle.Render("JSON  :")
	}
	lines = append(lines, jsonHeader)
	// Indent the textarea body by 2 spaces so it visually aligns with the
	// other field bodies.
	taLines := strings.Split(f.jsonArea.View(), "\n")
	for _, l := range taLines {
		lines = append(lines, "  "+l)
	}

	if f.err != "" {
		lines = append(lines, "", errStyle.Render("❌ "+f.err))
	}

	hint := popupHintLine(lines, "↑↓ 切字段 | Ctrl+S 保存 | Esc 取消")
	lines = append(lines, "", hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(popupBorderColor).
		Padding(1, 3).
		Render(strings.Join(lines, "\n"))

	if f.width > 0 && f.height > 0 {
		boxLines := strings.Count(box, "\n") + 1
		topPad := (f.height - boxLines) / 6
		if topPad < 0 {
			topPad = 0
		}
		return lipgloss.Place(f.width, f.height, lipgloss.Center, lipgloss.Top,
			strings.Repeat("\n", topPad)+box)
	}
	return box
}

// renderSingleLine formats one labeled single-line field with optional
// cursor block when active. Centralized so name/github render identically.
func renderSingleLine(label, value string, cursor int, active bool,
	labelStyle, activeStyle lipgloss.Style) string {
	display := value
	if active {
		display = renderWithCursor(value, cursor)
	}
	if active {
		return fmt.Sprintf("%s %s", activeStyle.Render(label), activeStyle.Render(display))
	}
	return fmt.Sprintf("%s %s", labelStyle.Render(label), display)
}

// prettyJSON re-indents bytes for editor display. On parse failure returns
// the raw input — the user may have left the previous value mid-edit.
func prettyJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(out)
}
