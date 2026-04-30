# Tab：agent 配置

## 功能

管理 agent（增 / 删 / 改 / 查看）。每个 agent 对应一个 skill 加载路径，`visible: true` 的 agent 会出现在「技能列表」表格列里 + 分配弹窗里。

## 文件

- `tui/config_list.go` — `ConfigListModel`（`configTypeAgent`）
- 与「数据源配置」tab 共用一份 Model，由 `configType` 字段区分

## 数据结构

`~/.config/sk-switch/agents.json`：

```json
[
  {"name": "claude-code",   "path": "~/.claude/skills",          "visible": true},
  {"name": "opencode-code", "path": "~/.config/opencode/skills", "visible": true}
]
```

```go
type Agent struct {
    Name    string `json:"name"`
    Path    string `json:"path"`
    Visible bool   `json:"visible"`
}
```

没有 `count` 字段（数据源才有）。

## 列表展示

```
 #  │     名称       │              位置                 │ 是否开启
────┼────────────────┼───────────────────────────────────┼──────────
 1  │ claude-code    │ ~/.claude/skills                  │   开启

 2  │ opencode-code  │ ~/.config/opencode/skills         │   开启


  a=新增 | e=编辑 | d=删除 | r=刷新 | ↑↓=移动
```

跟数据源 tab 共用渲染逻辑，少一列「数量」。

## 三个弹窗

跟数据源 tab 的弹窗完全对称：

| 按键 | 弹窗 | 行为 |
|------|------|------|
| `a` | 新增 | 三字段表单 |
| `e` | 编辑 | 三字段表单，预填当前行 |
| `d` | 删除 | 详情确认 |

弹窗内统一约定：
- `Tab` 切换字段焦点
- `Enter` 提交
- `Esc` 取消并清空错误

字段：
- 名称（不能空，不能与现有重复）
- 路径（不能空，去末尾 `/` 后保存）
- 开启（true / false，按任意键 toggle）

### 与数据源弹窗的关键差异

| 校验 | 数据源 | agent |
|------|--------|------|
| 名称重复 | ✅ | ✅ |
| 路径重复（ExpandPath 后） | ✅ | ✅ |
| 路径必须存在且是目录 | ✅ | ❌ |

agent 路径**不**校验是否存在 —— 例如新装的 Claude Code 还没分配过任何 skill 时，`~/.claude/skills/` 可能尚未创建。第一次 `agent.Link()` 调用 `os.MkdirAll` 会自动建出来。

## visible 字段的作用

`Visible` 控制该 agent 是否出现在「技能列表」中：

- `Visible: true` → 进入 `Store.VisibleAgents()`，技能列表表格多一列、分配弹窗多一行
- `Visible: false` → 仅保留在 agents.json，UI 不再展示，也不参与 assignment 计算

## 删除（`d`）

直接删除 —— 只把这条从 `agents.json` 移除。**不动文件系统**，agent 目录里已经存在的 symlink/真目录全部保持原样。

```
┌──────────────────────────────┐
│ 删除 agent？                   │
│                              │
│ 名称:  opencode-code          │
│ 路径:  ~/.config/opencode/skills │
│ 开启:  开启                   │
│                              │
│ Enter 确认删除 | Esc 取消     │
└──────────────────────────────┘
```

## 后端写入路径

```
用户操作
  ─▶ Store.AddAgent / UpdateAgent / RemoveAgent
       ├─ 校验（名称/路径不重复，无 IsDirPath 检查）
       ├─ 修改 Store.agents
       ├─ saveAgents(snap)        ← agents.json 立即落盘
       └─ Refresh()                ← 重算 assignments、写 source.json
```

之后 `refreshCmd` 发出 `storeRefreshedMsg`，「技能列表」tab 的 `m.visible` 重建，新加 agent 立刻出现在表格列里。

## 默认配置

第一次运行写入：

| name | path | visible |
|------|------|---------|
| `claude-code` | `~/.claude/skills` | true |
| `opencode-code` | `~/.config/opencode/skills` | true |

其它 agent（Droid、Codex、自研工具等）由用户自行 `a` 添加。
