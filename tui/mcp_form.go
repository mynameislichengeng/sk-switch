package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mynameislichengeng/sk-switch/config"
)

// mcpFormMode says whether the form is creating a new MCP or editing an
// existing one.
type mcpFormMode int

const (
	mcpFormAdd mcpFormMode = iota
	mcpFormEdit
)

// typeConfigForm holds the key/value inputs for one writer type.
type typeConfigForm struct {
	key       string
	keyCursor int
	valueArea textarea.Model
}

// mcpForm supports dynamic fields: name + github + (key/value) per registered
// writer type. fieldIdx is the global cursor across all fields.
type mcpForm struct {
	mode    mcpFormMode
	editIdx int

	name         string
	nameCursor   int
	github       string
	githubCursor int

	typeConfigs map[string]*typeConfigForm // writer type tag → form state
	typeOrder   []string                   // stable iteration order

	fieldIdx int // 0=name, 1=github, then per type: key, value
	err      string

	width  int
	height int
}

// newMCPForm builds a fresh form with one textarea per registered writer type.
func newMCPForm() mcpForm {
	types := config.MCPAgentTypes()
	sort.Strings(types)

	tcf := make(map[string]*typeConfigForm, len(types))
	for _, typ := range types {
		ta := textarea.New()
		ta.Placeholder = fmt.Sprintf("配置 %s 的 value", typ)
		ta.ShowLineNumbers = false
		ta.CharLimit = 0
		ta.MaxHeight = 5
		ta.SetHeight(4)
		ta.KeyMap.InsertNewline.SetKeys("enter")
		ta.Blur()
		tcf[typ] = &typeConfigForm{valueArea: ta}
	}

	return mcpForm{
		mode:        mcpFormAdd,
		fieldIdx:    0,
		typeConfigs: tcf,
		typeOrder:   types,
	}
}

func (f *mcpForm) SetSize(w, h int) {
	f.width = w
	f.height = h

	taW := w - 18
	if taW < 30 {
		taW = 30
	}
	if taW > 100 {
		taW = 100
	}
	for _, tc := range f.typeConfigs {
		tc.valueArea.SetWidth(taW)
		tc.valueArea.SetHeight(4)
	}
}

// totalFieldCount returns the number of focusable fields.
// 0=name, 1=github, then for each type: key, value.
func (f mcpForm) totalFieldCount() int {
	return 2 + len(f.typeOrder)*2
}

// seedEdit fills the form from an existing MCP.
func (f *mcpForm) seedEdit(idx int, mcp config.MCP) {
	f.mode = mcpFormEdit
	f.editIdx = idx
	f.name = mcp.Name
	f.nameCursor = runeCount(f.name)
	f.github = mcp.GithubURL
	f.githubCursor = runeCount(f.github)

	// Reset all type configs then fill from mcp.Configs.
	for _, tc := range f.typeConfigs {
		tc.key = ""
		tc.keyCursor = 0
		tc.valueArea.Reset()
		tc.valueArea.Blur()
	}
	for typ, cfg := range mcp.Configs {
		if tc, ok := f.typeConfigs[typ]; ok {
			tc.key = cfg.Key
			tc.keyCursor = runeCount(cfg.Key)
			tc.valueArea.SetValue(cfg.Value)
		}
	}

	f.fieldIdx = 0
	f.err = ""
	f.syncFocus()
}

func (f *mcpForm) seedAdd() {
	f.mode = mcpFormAdd
	f.editIdx = -1
	f.name = ""
	f.nameCursor = 0
	f.github = ""
	f.githubCursor = 0

	for _, tc := range f.typeConfigs {
		tc.key = ""
		tc.keyCursor = 0
		tc.valueArea.Reset()
		tc.valueArea.Blur()
	}

	f.fieldIdx = 0
	f.err = ""
	f.syncFocus()
}

// fieldKind returns what kind of field the current fieldIdx points to.
func (f mcpForm) fieldKind() (isTextarea bool, typeTag string) {
	switch f.fieldIdx {
	case 0, 1:
		return false, ""
	default:
		off := f.fieldIdx - 2
		if off%2 == 1 {
			// Odd offset → value textarea.
			idx := off / 2
			if idx < len(f.typeOrder) {
				return true, f.typeOrder[idx]
			}
		}
		return false, ""
	}
}

func (f *mcpForm) nextField() {
	n := f.totalFieldCount()
	if n > 0 {
		f.fieldIdx = (f.fieldIdx + 1) % n
	}
	f.syncFocus()
}

func (f *mcpForm) prevField() {
	n := f.totalFieldCount()
	if n > 0 {
		f.fieldIdx = (f.fieldIdx - 1 + n) % n
	}
	f.syncFocus()
}

func (f *mcpForm) syncFocus() {
	// Blur all textareas first.
	for _, tc := range f.typeConfigs {
		tc.valueArea.Blur()
	}
	isTA, typ := f.fieldKind()
	if isTA {
		if tc, ok := f.typeConfigs[typ]; ok {
			tc.valueArea.Focus()
		}
	}
}

// activeSingleLine returns the (text, cursor) for the currently focused
// single-line field, or nil when a textarea is focused.
func (f mcpForm) activeSingleLine() (*string, *int) {
	switch f.fieldIdx {
	case 0:
		return &f.name, &f.nameCursor
	case 1:
		return &f.github, &f.githubCursor
	default:
		off := f.fieldIdx - 2
		if off%2 == 0 {
			// Even offset → key field.
			idx := off / 2
			if idx < len(f.typeOrder) {
				if tc, ok := f.typeConfigs[f.typeOrder[idx]]; ok {
					return &tc.key, &tc.keyCursor
				}
			}
		}
	}
	return nil, nil
}

// updateField routes a key event to the active field.
func (f mcpForm) updateField(km tea.KeyMsg) (mcpForm, tea.Cmd) {
	isTA, typ := f.fieldKind()
	if isTA {
		if tc, ok := f.typeConfigs[typ]; ok {
			var cmd tea.Cmd
			tc.valueArea, cmd = tc.valueArea.Update(km)
			return f, cmd
		}
		return f, nil
	}

	target, cursor := f.activeSingleLine()
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

// commit validates inputs and calls the Store. Returns an error when
// validation fails so the caller keeps the form open.
func (f *mcpForm) commit(store *config.Store) error {
	name := strings.TrimSpace(f.name)
	gh := strings.TrimSpace(f.github)

	if name == "" {
		f.err = "名称不能为空"
		return fmt.Errorf("name empty")
	}

	configs := make(map[string]config.TypeConfig)
	for _, typ := range f.typeOrder {
		tc := f.typeConfigs[typ]
		key := strings.TrimSpace(tc.key)
		value := strings.TrimSpace(tc.valueArea.Value())
		if key != "" && value != "" {
			configs[typ] = config.TypeConfig{Key: key, Value: value}
		}
	}

	if len(configs) == 0 {
		f.err = "至少需要配置一个类型的 key 和 value"
		return fmt.Errorf("no configs")
	}

	if err := config.ValidateMCPConfigs(configs); err != nil {
		f.err = err.Error()
		return err
	}

	mcp := config.MCP{
		Name:      name,
		GithubURL: gh,
		Configs:   configs,
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

// View renders the form as a centered popup.
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

	// Name
	lines = append(lines, renderSingleLine("名称  :", f.name, f.nameCursor,
		f.fieldIdx == 0, keyStyle, activeStyle))
	// GitHub
	lines = append(lines, renderSingleLine("GitHub:", f.github, f.githubCursor,
		f.fieldIdx == 1, keyStyle, activeStyle))

	// Per-type fields
	for i, typ := range f.typeOrder {
		tc := f.typeConfigs[typ]
		keyFieldIdx := 2 + i*2
		valFieldIdx := keyFieldIdx + 1

		lines = append(lines, "")
		lines = append(lines, keyStyle.Render("--- "+typ+" ---"))

		// Key
		keyActive := f.fieldIdx == keyFieldIdx
		keyLabel := "Key   :"
		if keyActive {
			keyLabel = activeStyle.Render(keyLabel)
		}
		keyDisplay := tc.key
		if keyActive {
			keyDisplay = renderWithCursor(tc.key, tc.keyCursor)
		}
		lines = append(lines, fmt.Sprintf("%s %s", keyLabel, keyDisplay))

		// Value (textarea)
		valLabel := "Value :"
		if f.fieldIdx == valFieldIdx {
			valLabel = activeStyle.Render(valLabel)
		}
		lines = append(lines, valLabel)
		for _, l := range strings.Split(tc.valueArea.View(), "\n") {
			lines = append(lines, "  "+l)
		}
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
// cursor block when active.
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
// the raw input.
func prettyJSON(raw string) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return raw
	}
	return string(out)
}
