# Tab：技能列表

## 功能

展示当前所有数据源里扫到的 skill，按 `.skill-lock.json` 中的 `source` 仓库分组。每行显示 skill 名 / 来源仓库 / 数据源，以及对每个 visible agent 是否已分配（`Y / -`）。

选中一行后按空格 → 弹出**「技能详情 / 分配」弹窗**：在弹窗里选 agent + 空格 toggle 分配。

## 文件

- `tui/list.go` — `ListModel`
- `tui/skill_view.go` — `skillView`，全屏 SKILL.md 查看器（glamour 渲染 + viewport 滚动）

## 数据来源

ListModel 持有 `*config.Store` 引用，所有数据从 Store 读取，不直接扫盘。`storeRefreshedMsg` 到达时同步 `m.skills` / `m.visible` / `m.linkState` 缓存。

## ListModel 字段

```go
type ListModel struct {
    store            *config.Store
    skills           []config.Skill // group-ordered, matches render order
    visible          []config.Agent
    linkState        map[[2]int]bool // [skillRow][agentCol] → assigned
    cursor           int             // selected skill row
    popupAgentCursor int             // selected agent inside the popup
    err              string
    conflict         bool            // showing global conflict prompt
    assignPopup      bool            // showing assign popup
    viewHeight       int
    viewWidth        int
    scrollY          int
}
```

去掉了之前的 `colCursor` / `showDetail`：详情和分配合并到一个弹窗，左右切列已不需要。

## 状态机

```
列表态 ─空格─▶ assignPopup
                 │
                 ├─空格 toggle 成功 ─▶ refreshCmd ─▶ 仍在 popup（更新 linkState）
                 │
                 ├─空格 toggle，CanLink 返回 exists
                 │     ─▶ assignPopup=false, conflict=true
                 │           │
                 │           ├─ o 覆盖：forceOverwriteLink → 回到 popup
                 │           └─ s/n/Esc：跳过 → 回到 popup
                 │
                 └─Esc：关闭 popup → 列表态
```

## 按键

### 列表态

| 按键 | 功能 | 处理位置 |
|------|------|---------|
| `↑` / `k` | 上移一行（自动滚动） | ListModel |
| `↓` / `j` | 下移一行 | ListModel |
| `空格` | 打开当前行的「技能详情 / 分配」弹窗 | ListModel |
| `v` | 全屏查看当前行的 SKILL.md（glamour 渲染） | ListModel |
| `r` | 刷新 Store | tui.go（全局） |
| `Tab` / `Shift+Tab` | 切 tab | tui.go（全局） |
| `q` | 退出 | tui.go（全局） |

底部帮助行：`r 刷新 | s 筛选 | g 分组 | 空格 分配 | v 查看 | Tab 切换 | q 退出`。

### SKILL.md 查看器内（`m.view.active` 时）

| 按键 | 功能 |
|------|------|
| `↑/↓` `j/k` | 单行滚动 |
| `PgUp` / `PgDn` | 翻页 |
| `g` / `G` | 跳到顶 / 底 |
| `Esc` / `q` | 关闭查看器，回到列表 |

bubbles/viewport 还顺带绑定了 vim 风的 `f/b/space` 翻页和 `u/d/Ctrl+U/Ctrl+D` 半页，未在帮助行显示但仍可用。

打开是异步的：按 `v` 立刻显示「加载中…」，glamour 渲染完成后通过 `skillRenderedMsg` 回填内容。`main.go` 在启动时 `tui.WarmupGlamour()` 预热 chroma 词法库，避免第一次打开卡顿。

查看器属于 `inSpecialState()`，全局 `Tab` / `r` / `q` 在打开期间被屏蔽（`Ctrl+C` 仍生效）。

### 分配弹窗内

| 按键 | 功能 |
|------|------|
| `↑/↓` 或 `j/k` | 选 agent |
| `空格` 或 `Enter` | 切换分配 / 取消分配当前 agent |
| `Esc` 或 `q` | 关闭弹窗 |

### 冲突态（agent 目录下已存在非 symlink 的同名条目）

| 按键 | 功能 |
|------|------|
| `o` | 覆盖（`os.RemoveAll` → `agent.Link`），回到弹窗 |
| `s` / `n` / `Esc` | 跳过，回到弹窗 |

## 分配/取消逻辑（toggleInPopup）

```
linkState[(row, col)] == true（已分配）
  ─▶ agent.Unlink()
       ├─ ok → refreshCmd
       └─ err（非 symlink 等）→ m.err = "..."

linkState[(row, col)] == false（未分配）
  ─▶ agent.CanLink()
       ├─ ok / already_linked → agent.Link() → refreshCmd
       └─ exists → 进入 conflict 态
```

## 渲染

### 主体结构

```
总数：36                                ← 顶部 bar（renderTopBar）

vercel-labs/agent-browser（1）          ← 分组标题（绿色）
 #  │ 技能名称 │ 来源仓库 │ 数据源 │ claude-code │ opencode-code
────┼──────────┼──────────┼────────┼─────────────┼──────────────
  1 │ agent... │ vercel.. │ skill1 │      Y      │      -

obra/superpowers（14）
...

r 刷新 | Tab 切换 | q 退出              ← 底部 helpLine
```

### 视口滚动

主模型在 `WindowSizeMsg` 时调 `list.SetSize(w, h-tabBarHeight)`：

1. `renderScrollContent()` 生成完整内容（所有分组 + 行）
2. 取 `lines[scrollY : scrollY+tableHeight]` 渲染
3. `adjustScroll()` 在光标移动时确保光标行落在可视区域内

### 高亮

- 当前选中行：`lipgloss.Reverse(true)` 整行反色
- 弹窗内当前选中的 agent 行：同上
- 已分配 `Y`：绿色加粗；未分配 `-`：faint 灰色

### 分组

`groupBySource()` 按 `Skill.Source`（`.skill-lock.json` 中读到的仓库 owner/repo）分组，保持首次出现顺序，无 source 的归入 `(no source)`。

`Skills` 数组在 `refreshFromStore` 内**按分组顺序扁平化**后存入 `m.skills`，确保 `m.cursor` 索引和渲染顺序一致（避免"光标 vs 详情错位"）。

## 分配弹窗内容

```
┌─────────────────────────────────┐
│ 技能详情 / 分配                  │
│                                 │
│ 技能名称:   agent-browser        │
│                                 │
│ 来源仓库:   vercel-labs/agent... │
│                                 │
│ 数据源:    skill1                │
│                                 │
│                                 │
│ agent 分配                       │
│                                 │
│   claude-code        [ Y ]      │ ← 当前光标行 reverse
│                                 │
│   opencode-code      [ - ]      │
│                                 │
│                                 │
│ ↑/↓ 选 agent | 空格 切换 | Esc 关闭
└─────────────────────────────────┘
```

边框使用 `popupBorderColor`（橘红，详见 ui设计原则.md），定位水平居中 + 垂直靠上 1/4。
