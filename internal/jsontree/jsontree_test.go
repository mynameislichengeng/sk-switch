package jsontree

import (
	"strings"
	"testing"
)

func TestBuildBasics(t *testing.T) {
	data := []byte(`{
		"name": "claude-code",
		"version": 42,
		"settings": {
			"theme": "dark",
			"size": 16
		},
		"tags": ["a", "b"],
		"flag": true,
		"nothing": null
	}`)
	root, err := Build(data)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if root.Kind != KindObject {
		t.Fatalf("root kind = %v, want object", root.Kind)
	}
	if !root.Expanded {
		t.Fatalf("root expected expanded by default")
	}
	if len(root.Children) != 6 {
		t.Fatalf("root children = %d, want 6", len(root.Children))
	}
	wantKeys := []string{"flag", "name", "nothing", "settings", "tags", "version"}
	for i, want := range wantKeys {
		if got := root.Children[i].Key; got != want {
			t.Errorf("children[%d].Key = %q, want %q", i, got, want)
		}
	}
	settings := root.Children[3]
	if settings.Kind != KindObject {
		t.Errorf("settings kind = %v, want object", settings.Kind)
	}
	if settings.Expanded {
		t.Error("settings expected collapsed by default (only root expands)")
	}
	v := NewViewer(root, DefaultStyle())
	_, total, _ := v.Render()
	// 6 child lines + opening "{" + closing "}" = 8 lines.
	if total != 8 {
		t.Errorf("initial render total = %d, want 8", total)
	}
	// Expand "settings" → +3 lines (2 children + closing brace; the
	// settings line itself stays put, only swaps " { 2 项 } " for "{").
	v.cursor = settings
	v.Toggle()
	_, total2, _ := v.Render()
	if total2 != total+3 {
		t.Errorf("after expanding settings: total = %d, want %d", total2, total+3)
	}
}

func TestBuildBigInteger(t *testing.T) {
	const bigInt = "1735996800000000123" // 19 digits, > 2^53
	root, err := Build([]byte(`{"ts": ` + bigInt + `}`))
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if got := root.Children[0].ScalarRaw; got != bigInt {
		t.Errorf("big-int round-trip: got %q, want %q", got, bigInt)
	}
}

func TestBuildMalformed(t *testing.T) {
	cases := []string{
		`{foo: bar}`,       // unquoted keys
		`not json at all`,  // garbage
		``,                 // empty body
		`{"a": 1`,          // truncated
		`{"a": 1} extra`,   // trailing garbage AFTER a valid value
		`{"a": 1}{"b": 2}`, // two top-level values back-to-back
	}
	for _, in := range cases {
		if _, err := Build([]byte(in)); err == nil {
			t.Errorf("Build(%q) succeeded, want error", in)
		}
	}
}

func TestNavigationBounds(t *testing.T) {
	root, err := Build([]byte(`{"a": 1, "b": 2}`))
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	v := NewViewer(root, DefaultStyle())
	if v.Cursor() != root {
		t.Fatalf("initial cursor not on root")
	}
	v.MoveDown()
	if v.Cursor() != root.Children[0] {
		t.Errorf("MoveDown #1: cursor = %v, want children[0]", v.Cursor())
	}
	v.MoveDown()
	if v.Cursor() != root.Children[1] {
		t.Errorf("MoveDown #2: cursor = %v, want children[1]", v.Cursor())
	}
	// MoveDown past last navigable: cursor must NOT advance into a closer.
	v.MoveDown()
	if v.Cursor() != root.Children[1] {
		t.Errorf("MoveDown past end: cursor moved to %v", v.Cursor())
	}
	// MoveUp from top: stay on top.
	v.MoveTop()
	v.MoveUp()
	if v.Cursor() != root {
		t.Errorf("MoveUp from top: cursor = %v, want root", v.Cursor())
	}
	v.MoveBottom()
	if v.Cursor() != root.Children[1] {
		t.Errorf("MoveBottom: cursor = %v, want children[1]", v.Cursor())
	}
}

func TestToggleNoOps(t *testing.T) {
	root, err := Build([]byte(`{"x": 1, "empty": {}, "emptyArr": []}`))
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	v := NewViewer(root, DefaultStyle())
	scalar := root.Children[2] // "x" → 1 (sorted: empty, emptyArr, x)
	v.cursor = scalar
	prev := scalar.Expanded
	v.Toggle()
	if scalar.Expanded != prev {
		t.Error("Toggle on scalar must not flip Expanded")
	}
	emptyObj := root.Children[0] // "empty" → {}
	v.cursor = emptyObj
	prev = emptyObj.Expanded
	v.Toggle()
	if emptyObj.Expanded != prev {
		t.Error("Toggle on empty object must not flip Expanded")
	}
	emptyArr := root.Children[1] // "emptyArr" → []
	v.cursor = emptyArr
	prev = emptyArr.Expanded
	v.Toggle()
	if emptyArr.Expanded != prev {
		t.Error("Toggle on empty array must not flip Expanded")
	}
}

func TestExpandCollapseIdempotent(t *testing.T) {
	root, _ := Build([]byte(`{"o": {"x": 1}}`))
	v := NewViewer(root, DefaultStyle())
	o := root.Children[0]
	v.cursor = o
	if o.Expanded {
		t.Fatalf("o expected collapsed by default")
	}
	v.Expand()
	if !o.Expanded {
		t.Errorf("Expand: o.Expanded = false, want true")
	}
	v.Expand() // idempotent
	if !o.Expanded {
		t.Errorf("Expand twice: o.Expanded = false")
	}
	v.Collapse()
	if o.Expanded {
		t.Errorf("Collapse: o.Expanded = true")
	}
	v.Collapse() // idempotent
	if o.Expanded {
		t.Errorf("Collapse twice: o.Expanded = true")
	}
}

func TestScalarRoot(t *testing.T) {
	cases := []struct {
		raw      string
		wantKind ScalarKind
	}{
		{`42`, ScalarNumber},
		{`"hello"`, ScalarString},
		{`true`, ScalarBool},
		{`null`, ScalarNull},
	}
	for _, tc := range cases {
		root, err := Build([]byte(tc.raw))
		if err != nil {
			t.Errorf("Build(%q) errored: %v", tc.raw, err)
			continue
		}
		if root.Kind != KindScalar {
			t.Errorf("Build(%q) root kind = %v, want scalar", tc.raw, root.Kind)
		}
		if root.ScalarKind != tc.wantKind {
			t.Errorf("Build(%q) ScalarKind = %v, want %v", tc.raw, root.ScalarKind, tc.wantKind)
		}
		v := NewViewer(root, DefaultStyle())
		_, total, cursorLine := v.Render()
		if total != 1 {
			t.Errorf("Build(%q) rendered %d lines, want 1", tc.raw, total)
		}
		if cursorLine != 0 {
			t.Errorf("Build(%q) cursor at line %d, want 0", tc.raw, cursorLine)
		}
	}
}

func TestArrayIndexing(t *testing.T) {
	root, err := Build([]byte(`[{"a": 1}, {"b": 2}, "tail"]`))
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if root.Kind != KindArray {
		t.Fatalf("root not array")
	}
	if len(root.Children) != 3 {
		t.Fatalf("root children = %d, want 3", len(root.Children))
	}
	for i, c := range root.Children {
		if !c.IsArrayElem {
			t.Errorf("children[%d].IsArrayElem = false", i)
		}
		if c.ArrayIndex != i {
			t.Errorf("children[%d].ArrayIndex = %d, want %d", i, c.ArrayIndex, i)
		}
	}
}

func TestRenderContainsExpectedSubstrings(t *testing.T) {
	// Plain unstyled (zero Style) makes substring assertions reliable
	// across terminal color settings.
	root, _ := Build([]byte(`{"name": "x", "items": [1, 2]}`))
	v := NewViewer(root, Style{})
	// Sorted alphabetically: items, name → items is at index 0.
	v.cursor = root.Children[0]
	v.Toggle()
	out, _, _ := v.Render()
	for _, want := range []string{`"items"`, `"name"`, `1`, `2`, `[`, `]`, `{`, `}`} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output missing %q\n--- output ---\n%s", want, out)
		}
	}
}

func TestCacheInvalidatedOnToggle(t *testing.T) {
	// Render once, then toggle, then render again; the new content must
	// reflect the toggled state. Catches a stale-cache regression.
	root, _ := Build([]byte(`{"o": {"x": 1, "y": 2}}`))
	v := NewViewer(root, Style{})
	o := root.Children[0]
	v.cursor = o
	out1, _, _ := v.Render()
	v.Toggle() // expand
	out2, _, _ := v.Render()
	if out1 == out2 {
		t.Error("cache was not invalidated after Toggle: output unchanged")
	}
	v.Toggle() // collapse
	out3, _, _ := v.Render()
	if out3 != out1 {
		t.Errorf("after Toggle/Toggle, output should match original\nbefore: %q\nafter:  %q", out1, out3)
	}
}

// TestNilRootNoPanic guards the defensive nil-root path in refreshCache.
// Callers theoretically shouldn't construct a Viewer with nil root, but
// returning sensible zero values (rather than panicking) is friendlier.
func TestNilRootNoPanic(t *testing.T) {
	v := NewViewer(nil, DefaultStyle())
	content, total, cursorLine := v.Render()
	if content != "" {
		t.Errorf("nil root: content = %q, want empty", content)
	}
	if total != 0 {
		t.Errorf("nil root: total = %d, want 0", total)
	}
	if cursorLine != -1 {
		t.Errorf("nil root: cursorLine = %d, want -1", cursorLine)
	}
	// Move/Toggle on nil root must also be no-ops, not panics.
	v.MoveDown()
	v.MoveUp()
	v.MoveTop()
	v.MoveBottom()
	v.Toggle()
	v.Expand()
	v.Collapse()
}

func TestFindBasic(t *testing.T) {
	// Children are sorted alphabetically: items, meta, name. Pre-order
	// DFS visits root → items → 1 → 2 → meta → meta.name → name. The
	// first node whose key contains "name" is therefore meta.name (the
	// nested one), NOT the top-level "name" — even though both have the
	// same key, the nested one comes first in pre-order.
	root, _ := Build([]byte(`{"name": "x", "items": [1, 2], "meta": {"name": "y"}}`))
	v := NewViewer(root, Style{})
	n := v.Find("name")
	if n == nil {
		t.Fatal("Find returned nil for 'name'")
	}
	expected := root.Children[1].Children[0] // meta.name
	if n != expected {
		t.Errorf("Find('name'): matched node with key=%q value=%q, want meta.name (key=%q value=%q)",
			n.Key, n.ScalarRaw, expected.Key, expected.ScalarRaw)
	}
	if v.Cursor() != n {
		t.Errorf("cursor not at match")
	}
}

func TestFindNoMatchLeavesStateAlone(t *testing.T) {
	root, _ := Build([]byte(`{"a": 1}`))
	v := NewViewer(root, Style{})
	cursorBefore := v.Cursor()
	if n := v.Find("nonexistent"); n != nil {
		t.Errorf("Find returned %v, want nil", n)
	}
	if v.Cursor() != cursorBefore {
		t.Error("cursor moved despite no-match Find")
	}
}

func TestFindEmptyOrWhitespaceQuery(t *testing.T) {
	root, _ := Build([]byte(`{"a": 1}`))
	v := NewViewer(root, Style{})
	for _, q := range []string{"", "   ", "\t\n"} {
		if n := v.Find(q); n != nil {
			t.Errorf("Find(%q) = %v, want nil for empty/whitespace query", q, n)
		}
	}
}

func TestFindAutoExpandsAncestors(t *testing.T) {
	root, _ := Build([]byte(`{"outer": {"inner": {"target": 42}}}`))
	v := NewViewer(root, Style{})
	outer := root.Children[0]
	inner := outer.Children[0]
	if outer.Expanded || inner.Expanded {
		t.Fatal("expected outer/inner collapsed initially")
	}
	if n := v.Find("target"); n == nil {
		t.Fatal("Find('target') returned nil")
	}
	if !outer.Expanded {
		t.Error("outer must be expanded after find")
	}
	if !inner.Expanded {
		t.Error("inner must be expanded after find")
	}
}

func TestFindMatchesScalarValue(t *testing.T) {
	root, _ := Build([]byte(`{"theme": "dark", "name": "WezTerm"}`))
	v := NewViewer(root, Style{})
	n := v.Find("WezTerm")
	if n == nil {
		t.Fatal("Find('WezTerm') returned nil")
	}
	if n.Key != "name" {
		t.Errorf("Find by scalar value: got key %q, want name", n.Key)
	}
}

func TestFindCaseInsensitive(t *testing.T) {
	root, _ := Build([]byte(`{"FooBar": 1}`))
	v := NewViewer(root, Style{})
	if n := v.Find("foobar"); n == nil {
		t.Error("case-insensitive match failed")
	}
}

func TestFindNextWraps(t *testing.T) {
	// Sorted alphabetically: a_match, b_match → first match is a_match.
	root, _ := Build([]byte(`{"a_match": 1, "b_match": 2, "skip": 0}`))
	v := NewViewer(root, Style{})
	n1 := v.Find("match")
	if n1 == nil || n1.Key != "a_match" {
		t.Fatalf("Find first: got %v, want a_match", n1)
	}
	n2 := v.FindNext("match")
	if n2 == nil || n2.Key != "b_match" {
		t.Errorf("FindNext: got %v, want b_match", n2)
	}
	n3 := v.FindNext("match") // wraps from end back to top
	if n3 == nil || n3.Key != "a_match" {
		t.Errorf("FindNext wrap: got %v, want a_match", n3)
	}
}

func TestFindNextStartsFromCursor(t *testing.T) {
	// Cursor in the middle of the tree — FindNext should skip earlier
	// matches and land on a match strictly after the cursor.
	root, _ := Build([]byte(`{"a_match": 1, "b_match": 2, "c_match": 3}`))
	v := NewViewer(root, Style{})
	v.cursor = root.Children[1] // b_match
	n := v.FindNext("match")
	if n == nil || n.Key != "c_match" {
		t.Errorf("FindNext from b_match: got %v, want c_match", n)
	}
}

// TestSetStyleInvalidatesCache asserts that swapping the style after a
// previous Render flips the cache-dirty flag so the next Render rebuilds.
// We test the flag directly because lipgloss strips ANSI in non-TTY test
// runners, which would make rendered-text comparison unreliable.
func TestSetStyleInvalidatesCache(t *testing.T) {
	root, _ := Build([]byte(`{"a": 1}`))
	v := NewViewer(root, DefaultStyle())
	_, _, _ = v.Render()
	if v.cacheDirty {
		t.Fatal("cacheDirty should be false after a successful Render")
	}
	v.SetStyle(DefaultStyle())
	if !v.cacheDirty {
		t.Error("SetStyle did not mark cache dirty")
	}
}
