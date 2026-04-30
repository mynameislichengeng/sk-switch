# Tab：数据源配置

## 功能

管理 skill 数据源（增 / 删 / 改 / 查看）。数据源定义 skill 的存放根目录，是「技能列表」tab 的扫描来源。

## 文件

- `tui/config_list.go` — `ConfigListModel`（`configTypeDataSource`）
- `tui/styles.go` — `popupBorderColor`
- 与 agent 配置 tab 共用一份 `ConfigListModel`，由 `configType` 字段区分

## 数据结构

`~/.config/sk-switch/source.json`：

```json
[
  {
    "name": "default",
    "path": "~/.agents/skills",
    "count": 12,
    "visible": true
  }
]
```

```go
type DataSource struct {
    Name    string `json:"name"`
    Path    string `json:"path"`
    Count   int    `json:"count"`   // last-scan snapshot
    Visible bool   `json:"visible"`
}
```

`count` 是上次 `Store.Refresh()` 扫到的 skill 数量；UI 渲染读这个值（不再实时扫盘）。

## 列表展示

```
 #  │   名称   │       位置        │ 数量 │ 是否开启
────┼──────────┼───────────────────┼──────┼──────────
 1  │ default  │ ~/.agents/skills  │  12  │   开启

 2  │ skill1   │ ~/.agents/skills1 │  36  │   开启


  a=新增 | e=编辑 | d=删除 | r=刷新 | ↑↓=移动
```

- 表头绿色加粗
- 当前光标行整行 reverse
- 列宽随窗口宽度自适应（名称/位置按权重分配剩余空间）
- 行间留 1 空行透气
- agent 配置 tab 没有「数量」列，自动只显示 4 列

## 三个弹窗

新增 / 编辑 / 删除全部用居中弹窗，边框 `popupBorderColor`（橘红）。详见 [ui设计原则.md](ui设计原则.md)。

### 新增（`a`）

```
┌──────────────────────────────┐
│ 新增数据源                    │
│                              │
│ 名称: my-source█              │ ← 当前编辑字段绿色加粗
│ 路径: ~/.agents/skills        │
│ 开启: 开启                    │
│                              │
│ Tab 切换字段 | Enter 确认 | Esc 取消
└──────────────────────────────┘
```

字段：
- 名称（不能空，不能与现有重复）
- 路径（不能空，必须是已存在的目录）
- 开启（true/false）

校验失败 → 弹窗内红色 `❌` 提示，弹窗保持打开。Esc 关闭弹窗时清空错误。

### 编辑（`e`）

跟新增完全对称，字段预填当前行的值。校验：
- 名称：不能与**其它**项重复（自己除外）
- 路径：不能与**其它**项的 `ExpandPath` 后路径相同；且必须是合法目录

通过 `Store.UpdateSource(idx, ds)` 写回。

### 删除（`d`）

```
┌──────────────────────────────┐
│ 删除数据源？                   │
│                              │
│ 名称:  skill1                 │
│ 路径:  ~/.agents/skills1      │
│ 数量:  36                     │
│ 开启:  开启                   │
│                              │
│ Enter 确认删除 | Esc 取消     │
└──────────────────────────────┘
```

直接删，不再阻拦"已分配"。物理 symlink 不动 —— agent 目录里的 symlink 会变成"孤儿引用"，但程序不再扫描该数据源，所以不会再出现在技能列表中。

## 后端写入路径

```
用户操作
  ─▶ Store.AddSource / UpdateSource / RemoveSource
       ├─ 校验（路径合法、名称/路径不重复）
       ├─ 修改 Store.sources
       └─ Refresh()
            ├─ 过滤无效路径（IsDirPath 失败的全部丢弃）
            ├─ 重扫所有数据源 + 更新 Skills
            ├─ 重算 Assignments
            └─ saveSources(s.sources)
```

之后 `refreshCmd` 发出 `storeRefreshedMsg`，UI 各 tab 同步刷新。

## 路径处理

| 工具 | 作用 |
|------|------|
| `config.NormalizePath(p)` | 去前后空格、去末尾 `/` 或 `\`，让 `~/.agents/skills` 与 `~/.agents/skills/` 持久化为同一份 |
| `config.ExpandPath(p)` | 展开 `~/` 为家目录绝对路径 |
| `config.IsDirPath(p)` | `ExpandPath` + `EvalSymlinks` + `os.Stat` 是否目录 |

数据源路径可以是 symlink（如 `~/.agents/skills1` → `~/.cc-switch/skills`），扫描时会 `EvalSymlinks` 后再 `ReadDir`。

## 默认配置

第一次运行（`runtime-config.yaml` 不存在或 `first_run: true`）会写入：

```json
[{"name": "default", "path": "~/.agents/skills", "count": 0, "visible": true}]
```

如果 `~/.agents/skills` 不存在，下次 `Refresh()` 会把这条直接从 `s.sources` 里过滤掉，source.json 也跟着清理 —— 此后 UI 显示空列表，需要 `a` 加新数据源才会有内容。
