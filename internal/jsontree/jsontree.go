// Package jsontree provides a foldable, keyboard-navigable JSON tree
// renderer for terminal UIs.
//
// The package is self-contained: it returns a styled string that the caller
// pushes into any viewport (bubbles/viewport, raw stdout, etc.). It owns
// only cursor + fold state, never reads from stdin or from any framework
// model — so it composes with bubbletea, tview, or a plain CLI dump.
//
// Typical usage:
//
//	root, err := jsontree.Build(data)
//	if err != nil {
//	    // not valid JSON — fall back to plain-text display
//	}
//	v := jsontree.NewViewer(root, jsontree.DefaultStyle())
//	content, totalLines, cursorLine := v.Render()
//	// On key press:
//	v.MoveDown()       // ↓ / j
//	v.Toggle()         // Space / Enter
//	v.Collapse()       // ← / h
//	// then re-Render() and refresh the surrounding viewport.
//
// Numbers are decoded with json.Number so large integers (Unix-nanosecond
// timestamps, snowflake IDs) survive the round-trip without float64
// precision loss. Object keys are sorted alphabetically so navigation is
// deterministic across runs — the original on-disk order is sacrificed in
// exchange for findability ("settings" is always near "s").
package jsontree

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Kind classifies what a Node holds. Containers (Object/Array) can be
// expanded or collapsed; Scalars are always leaves.
type Kind int

const (
	KindObject Kind = iota
	KindArray
	KindScalar
)

// ScalarKind picks the color for scalar values and (for strings) preserves
// the original JSON quoting via strconv.Quote.
type ScalarKind int

const (
	ScalarString ScalarKind = iota
	ScalarNumber
	ScalarBool
	ScalarNull
)

// Node is one node in a JSON tree. Fields are exported so callers can walk
// the tree directly (e.g., to find a node by path) — but state-changing
// operations should go through Viewer methods so its cache stays consistent.
type Node struct {
	Kind        Kind
	Key         string // object child's key; "" for root and array elements
	IsArrayElem bool   // true → render as "[i]" prefix instead of a key
	ArrayIndex  int
	Expanded    bool    // only meaningful for KindObject / KindArray
	Children    []*Node // empty for scalars
	ScalarKind  ScalarKind
	ScalarRaw   string // already JSON-quoted for strings; raw text otherwise
}

// Style centralizes every color and decoration the renderer uses. A zero
// Style produces plain unstyled output; use DefaultStyle for a sensible
// 256-color palette that works on light and dark backgrounds.
//
// Cursor is the prefix applied to the focused row (typically a colored
// "▸ "). Marker is applied to the fold indicators ("▼ " / "▶ "). The Value
// fields color scalar payloads.
type Style struct {
	Marker      lipgloss.Style
	Cursor      lipgloss.Style
	Key         lipgloss.Style
	StringValue lipgloss.Style
	NumberValue lipgloss.Style
	BoolValue   lipgloss.Style
	NullValue   lipgloss.Style
	Summary     lipgloss.Style // "{ N 项 }" / "[ N 项 ]"
}

// DefaultStyle returns a sensible color palette that works on most
// terminals without depending on the host application's theme.
func DefaultStyle() Style {
	return Style{
		Marker:      lipgloss.NewStyle().Foreground(lipgloss.Color("14")),
		Cursor:      lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true),
		Key:         lipgloss.NewStyle().Foreground(lipgloss.Color("12")),
		StringValue: lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		NumberValue: lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
		BoolValue:   lipgloss.NewStyle().Foreground(lipgloss.Color("13")),
		NullValue:   lipgloss.NewStyle().Faint(true),
		Summary:     lipgloss.NewStyle().Faint(true),
	}
}

// Build parses raw JSON bytes into a tree. The root container is expanded
// by default so the user sees one level of structure on first open;
// descendants start collapsed.
//
// Build returns an error for any input that doesn't decode as exactly one
// JSON value. Empty input, malformed JSON, AND trailing junk after a
// valid value all produce errors so the caller can fall back to a
// plain-text view rather than silently hiding broken bytes inside the
// tree (Decoder by itself accepts trailing content; we explicitly reject
// it by attempting a second decode and requiring io.EOF).
func Build(data []byte) (*Node, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var rest any
	if err := dec.Decode(&rest); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("unexpected content after top-level JSON value")
		}
		return nil, err
	}
	root := convert("", false, 0, v)
	if root.Kind == KindObject || root.Kind == KindArray {
		root.Expanded = true
	}
	return root, nil
}

func convert(key string, isArrElem bool, idx int, v any) *Node {
	n := &Node{Key: key, IsArrayElem: isArrElem, ArrayIndex: idx}
	switch x := v.(type) {
	case map[string]any:
		n.Kind = KindObject
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			n.Children = append(n.Children, convert(k, false, 0, x[k]))
		}
	case []any:
		n.Kind = KindArray
		for i, e := range x {
			n.Children = append(n.Children, convert("", true, i, e))
		}
	case json.Number:
		n.Kind = KindScalar
		n.ScalarKind = ScalarNumber
		n.ScalarRaw = x.String()
	case string:
		n.Kind = KindScalar
		n.ScalarKind = ScalarString
		n.ScalarRaw = strconv.Quote(x)
	case bool:
		n.Kind = KindScalar
		n.ScalarKind = ScalarBool
		n.ScalarRaw = strconv.FormatBool(x)
	case nil:
		n.Kind = KindScalar
		n.ScalarKind = ScalarNull
		n.ScalarRaw = "null"
	default:
		// json.Decoder produces only the types above when UseNumber is
		// active. This branch exists only for defensive completeness:
		// future Go versions could in theory introduce a new mapping.
		n.Kind = KindScalar
		n.ScalarRaw = fmt.Sprintf("%v", v)
	}
	return n
}

// line is one rendered display row plus a back-pointer to the node it
// describes. Closer pseudo-rows ("}" / "]") have node==nil so cursor
// navigation skips them.
type line struct {
	node *Node
	text string
}

// Viewer carries cursor + fold state on top of a Node tree, and caches the
// most recent walk result so cursor moves don't re-render the entire tree
// each keystroke. Toggle / Expand / Collapse mark the cache dirty; the
// next read rebuilds it on demand.
type Viewer struct {
	root   *Node
	cursor *Node
	style  Style

	cache      []line
	cacheNav   []int // indices into cache that are navigable (node != nil)
	cacheDirty bool
}

// NewViewer constructs a Viewer focused on root. Use DefaultStyle() for a
// drop-in palette, or pass a Style with fields populated from the host
// application's theme.
func NewViewer(root *Node, style Style) *Viewer {
	return &Viewer{root: root, cursor: root, style: style, cacheDirty: true}
}

// Cursor returns the focused node, or nil if the tree is empty.
func (v *Viewer) Cursor() *Node { return v.cursor }

// SetStyle replaces the active style and forces a re-render on the next
// Render call. Useful when the host application's theme changes at runtime.
func (v *Viewer) SetStyle(s Style) {
	v.style = s
	v.cacheDirty = true
}

// Render returns the full rendered content (with embedded ANSI styles),
// the total number of rendered lines, and the line index where the
// cursor's marker has been placed. The latter two are exposed so a
// surrounding viewport can scroll the cursor into view without re-walking
// the tree itself.
//
// Render does NOT re-tokenize on every call — the walk result is cached
// and only rebuilt when fold state has changed since the last walk.
func (v *Viewer) Render() (content string, totalLines, cursorLine int) {
	v.refreshCache()
	cursorLine = -1
	if v.cursor != nil {
		for _, idx := range v.cacheNav {
			if v.cache[idx].node == v.cursor {
				cursorLine = idx
				break
			}
		}
	}
	if cursorLine < 0 && len(v.cacheNav) > 0 {
		// Defensive: cursor went out of sync with the tree (a parent was
		// collapsed externally). Snap back to the first navigable node.
		cursorLine = v.cacheNav[0]
		v.cursor = v.cache[cursorLine].node
	}
	var b strings.Builder
	for i, l := range v.cache {
		if i > 0 {
			b.WriteByte('\n')
		}
		if i == cursorLine {
			b.WriteString(v.style.Cursor.Render("▸ "))
		} else {
			b.WriteString("  ")
		}
		b.WriteString(l.text)
	}
	return b.String(), len(v.cache), cursorLine
}

// MoveDown / MoveUp step the cursor to the adjacent navigable node. Closer
// rows are skipped because they have no node pointer.
func (v *Viewer) MoveDown() {
	v.refreshCache()
	if cur := v.cursorNavIndex(); cur >= 0 && cur < len(v.cacheNav)-1 {
		v.cursor = v.cache[v.cacheNav[cur+1]].node
	}
}

func (v *Viewer) MoveUp() {
	v.refreshCache()
	if cur := v.cursorNavIndex(); cur > 0 {
		v.cursor = v.cache[v.cacheNav[cur-1]].node
	}
}

// MoveTop / MoveBottom jump to the first / last navigable node.
func (v *Viewer) MoveTop() {
	v.refreshCache()
	if len(v.cacheNav) > 0 {
		v.cursor = v.cache[v.cacheNav[0]].node
	}
}

func (v *Viewer) MoveBottom() {
	v.refreshCache()
	if len(v.cacheNav) > 0 {
		v.cursor = v.cache[v.cacheNav[len(v.cacheNav)-1]].node
	}
}

// Toggle flips the cursor's expanded state. No-op for scalars and empty
// containers (those have nothing meaningful to fold).
func (v *Viewer) Toggle() {
	if v.foldable(v.cursor) {
		v.cursor.Expanded = !v.cursor.Expanded
		v.cacheDirty = true
	}
}

// Expand opens the cursor if it's a non-empty container and currently
// collapsed. Idempotent on already-expanded containers.
func (v *Viewer) Expand() {
	if v.foldable(v.cursor) && !v.cursor.Expanded {
		v.cursor.Expanded = true
		v.cacheDirty = true
	}
}

// Collapse closes the cursor if it's a non-empty container and currently
// expanded. Idempotent on already-collapsed containers.
func (v *Viewer) Collapse() {
	if v.foldable(v.cursor) && v.cursor.Expanded {
		v.cursor.Expanded = false
		v.cacheDirty = true
	}
}

// foldable returns true when the node is a non-empty container. Hiding
// the marker on empty containers (and rejecting fold ops on them) keeps
// "expanded but no children" from ever being a possible state.
func (v *Viewer) foldable(n *Node) bool {
	if n == nil {
		return false
	}
	if n.Kind != KindObject && n.Kind != KindArray {
		return false
	}
	return len(n.Children) > 0
}

func (v *Viewer) refreshCache() {
	if !v.cacheDirty {
		return
	}
	v.cache = v.cache[:0]
	v.cacheNav = v.cacheNav[:0]
	if v.root != nil {
		v.walk(v.root, 0)
	}
	v.cacheDirty = false
}

func (v *Viewer) walk(n *Node, depth int) {
	v.cacheNav = append(v.cacheNav, len(v.cache))
	v.cache = append(v.cache, line{
		node: n,
		text: v.renderLine(n, depth),
	})
	if (n.Kind == KindObject || n.Kind == KindArray) && n.Expanded && len(n.Children) > 0 {
		for _, c := range n.Children {
			v.walk(c, depth+1)
		}
		v.cache = append(v.cache, line{
			node: nil,
			text: v.renderCloser(n, depth),
		})
	}
}

func (v *Viewer) cursorNavIndex() int {
	for i, lineIdx := range v.cacheNav {
		if v.cache[lineIdx].node == v.cursor {
			return i
		}
	}
	return -1
}

func (v *Viewer) renderLine(n *Node, depth int) string {
	indent := strings.Repeat("  ", depth)
	marker := "  "
	if (n.Kind == KindObject || n.Kind == KindArray) && len(n.Children) > 0 {
		if n.Expanded {
			marker = v.style.Marker.Render("▼ ")
		} else {
			marker = v.style.Marker.Render("▶ ")
		}
	}
	keyPart := ""
	switch {
	case n.IsArrayElem:
		keyPart = v.style.Key.Render(fmt.Sprintf("[%d]", n.ArrayIndex)) + ": "
	case n.Key != "":
		keyPart = v.style.Key.Render(strconv.Quote(n.Key)) + ": "
	}
	return indent + marker + keyPart + v.renderValue(n)
}

func (v *Viewer) renderValue(n *Node) string {
	switch n.Kind {
	case KindObject:
		switch {
		case len(n.Children) == 0:
			return "{}"
		case n.Expanded:
			return "{"
		default:
			return v.style.Summary.Render(fmt.Sprintf("{ %d 项 }", len(n.Children)))
		}
	case KindArray:
		switch {
		case len(n.Children) == 0:
			return "[]"
		case n.Expanded:
			return "["
		default:
			return v.style.Summary.Render(fmt.Sprintf("[ %d 项 ]", len(n.Children)))
		}
	case KindScalar:
		switch n.ScalarKind {
		case ScalarString:
			return v.style.StringValue.Render(n.ScalarRaw)
		case ScalarNumber:
			return v.style.NumberValue.Render(n.ScalarRaw)
		case ScalarBool:
			return v.style.BoolValue.Render(n.ScalarRaw)
		case ScalarNull:
			return v.style.NullValue.Render(n.ScalarRaw)
		}
	}
	return ""
}

func (v *Viewer) renderCloser(n *Node, depth int) string {
	indent := strings.Repeat("  ", depth)
	switch n.Kind {
	case KindObject:
		return indent + "}"
	case KindArray:
		return indent + "]"
	}
	return ""
}
