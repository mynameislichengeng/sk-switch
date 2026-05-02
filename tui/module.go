package tui

// module identifies one of the top-level navigation areas selected from the
// entry modal. Each module owns its own tab list and sub-models; switching
// modules restores the user to that module's first tab.
//
// Adding a module:
//  1. Append a const here (before moduleCount).
//  2. Add an entry to moduleNames / moduleDescs / moduleKeys.
//  3. Add the module's tab list to moduleTabs.
//  4. Wire the module's sub-models into Model and the routing in tui.go.
type module int

const (
	moduleSkills module = iota
	moduleMcp
	moduleSettings
	moduleCount
)

// moduleKeys are the stable identifiers persisted in runtime-config.yaml.
// Translating to/from this string keeps the on-disk format independent of the
// internal const ordering.
var moduleKeys = [moduleCount]string{
	moduleSkills:   "skills",
	moduleMcp:      "mcp",
	moduleSettings: "settings",
}

// moduleNames are shown in the entry modal and the module banner.
var moduleNames = [moduleCount]string{
	moduleSkills:   "SKILLS",
	moduleMcp:      "MCP",
	moduleSettings: "SETTINGS",
}

// moduleDescs are the one-line subtitles shown next to each entry option.
var moduleDescs = [moduleCount]string{
	moduleSkills:   "管理 skill / 数据源 / agent 分配",
	moduleMcp:      "管理 MCP server / agent 分配",
	moduleSettings: "主题色、运行配置",
}

// moduleTabs is the per-module tab name list. Tab indices are zero-based and
// scoped to the module — i.e. tab 0 means a different page in each module.
var moduleTabs = [moduleCount][]string{
	moduleSkills:   {"技能列表", "技能安装", "数据源配置", "agent配置"},
	moduleMcp:      {"MCP 列表", "AGENTS 配置"},
	moduleSettings: {"主题配置"},
}

func moduleByKey(key string) (module, bool) {
	for i, k := range moduleKeys {
		if k == key {
			return module(i), true
		}
	}
	return moduleSkills, false
}

func (m module) Key() string  { return moduleKeys[m] }
func (m module) Name() string { return moduleNames[m] }
func (m module) Tabs() []string {
	return moduleTabs[m]
}
