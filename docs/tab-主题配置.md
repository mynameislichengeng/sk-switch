# Tab：主题配置

## 功能

集中管理 TUI 全局色板。每个颜色 token 是 `(light, dark)` 一对 `#RRGGBB`，分别在浅色 / 深色终端下生效。配置写入磁盘，编辑后立即热更新到所有界面。

## 文件

- `tui/theme_config.go` — `ThemeConfigModel`（列表 + 编辑弹窗）
- `tui/styles.go` — `Theme` / `ApplyTheme` / `reloadStyles`，运行时全局色板
- `config/theme.go` — `ThemeConfig` 持久化（YAML）
- 启动顺序：`main.go` 中 `config.LoadTheme()` → `tui.ApplyTheme()` → `tui.NewModel()`

## 数据结构

`~/.config/sk-switch/theme.yaml`：

```yaml
tab_active_bg:
  light: "#006400"
  dark:  "#228B22"
tab_active_fg:
  light: "#FFFFFF"
  dark:  "#FFFFFF"
popup_border:
  light: "#B22222"
  dark:  "#FFA07A"
active_highlight:
  light: "#FFD700"
  dark:  "#FFD700"
row_highlight_bg:
  light: "#E5E5E5"
  dark:  "#555555"
hint_fg:
  light: "#909090"
  dark:  "#909090"
modified_fg:
  light: "#FF6B6B"
  dark:  "#FF6B6B"
success_fg:
  light: "#006400"
  dark:  "#90EE90"
```

```go
type ThemeConfig struct {
    TabActiveBg     ThemePair `yaml:"tab_active_bg"`
    TabActiveFg     ThemePair `yaml:"tab_active_fg"`
    PopupBorder     ThemePair `yaml:"popup_border"`
    ActiveHighlight ThemePair `yaml:"active_highlight"`
    RowHighlightBg  ThemePair `yaml:"row_highlight_bg"`
    HintFg          ThemePair `yaml:"hint_fg"`
    ModifiedFg      ThemePair `yaml:"modified_fg"`
    SuccessFg       ThemePair `yaml:"success_fg"`
}

type ThemePair struct {
    Light string `yaml:"light"`
    Dark  string `yaml:"dark"`
}
```

## Token 语义

| Token | 用途 |
|------|------|
| `tab_active_bg` / `tab_active_fg` | 当前选中 tab 的背景与文字（`activeTabStyle`） |
| `popup_border` | 弹窗边框 + 弹窗主标题字体 + 列表分类标题（`popupBorderColor` / `popupTitleStyle`） |
| `active_highlight` | 弹窗中聚焦字段、列表选中行字体、二次确认 sign（`popupActiveStyle`） |
| `row_highlight_bg` | 列表选中行背景，技能列表与配置列表共用（`rowHighlightStyle`） |
| `hint_fg` | 主屏 / 弹窗底部帮助行字色（`helpLineStyle`） |
| `modified_fg` | 已修改 / 错误 `❌` 提示 |
| `success_fg` | 顶部 `✓` 刷新提示等成功语义 |

## 列表展示

```
  Token             Light             Dark

▸ TAB 选中背景      ██ #006400        ██ #228B22
  TAB 选中文字      ██ #FFFFFF        ██ #FFFFFF
  弹窗边框/标题     ██ #B22222        ██ #FFA07A
  激活高亮          ██ #FFD700        ██ #FFD700
  列表选中背景      ██ #E5E5E5        ██ #555555
  帮助行字色        ██ #909090        ██ #909090
  已修改/错误       ██ #FF6B6B        ██ #FF6B6B
  成功提示          ██ #006400        ██ #90EE90

                                  ↑↓ 选择 | Enter 编辑 | Esc 返回
```

每行：`▸ 标签 │ Light 色块 + hex │ Dark 色块 + hex`。光标行的 `▸` 与标签用金黄色加粗（`active_highlight`），右侧色块直接用 `Background(用户hex)` 绘制 `"  "`，所见即所得。

## 编辑弹窗

```
┌────────────────────────────────────┐
│ 编辑主题                            │  ← 橘红色加粗标题
│                                    │
│ Token: TAB 选中背景                 │  ← Faint 提示
│                                    │
│ Light: #006400█                    │  ← 当前字段：金黄加粗，行末光标块
│ Dark:  #228B22                     │  ← 非当前字段：Faint
│                                    │
│           ↑↓ 切换字段 | Enter 确认 | Esc 取消
└────────────────────────────────────┘
```

按键：

| 按键 | 行为 |
|------|------|
| `↑/↓` | 在 Light 与 Dark 之间切换 |
| 字符 | 输入到当前字段（rune-aware） |
| `Backspace` | 删字符 |
| `Enter` | 校验 + 保存 + 立即应用 |
| `Esc` | 取消，磁盘不动 |

## 校验规则

`config.IsValidHexColor` 仅接受标准 6 位 `#RRGGBB`：

```go
^#[0-9A-Fa-f]{6}$
```

短形式（`#FFF`）不接受 —— 让用户始终写完整 6 位避免歧义。校验失败时弹窗内红色 `❌` 提示，不写盘。

## 持久化与热更新

```
用户在编辑弹窗按 Enter
  ─▶ ThemeConfigModel.commit()
       ├─ 校验 light / dark
       ├─ 写入 m.cfg.<Token>.{Light,Dark}
       ├─ config.SaveTheme(m.cfg)        ← 写 theme.yaml
       └─ tui.ApplyTheme(m.cfg)
            ├─ theme = ThemeFromConfig(t)
            └─ reloadStyles()           ← 重建所有派生 lipgloss.Style
```

下一帧 render 时所有 tab、所有弹窗都自动用上新色板。

## 异常处理（启动鲁棒性）

| 情况 | 行为 |
|------|------|
| `theme.yaml` 不存在 | `LoadTheme` 写入 `DefaultTheme()` 后返回，nil error。首次运行用户能看到该文件 |
| YAML 解析失败 | 返回 `DefaultTheme()` + error；UI 顶部红色 `⚠ ...（已使用默认主题）`，启动不阻塞 |
| 字段缺失（旧版本配置） | `mergeWithDefaults` 用默认值补齐缺失字段 |
| 字段值非法（不匹配 hex 正则） | `mergeWithDefaults` 替换为默认值，静默 |
| 写盘失败（磁盘只读等） | 编辑弹窗显示 `保存失败: ...`，弹窗保持，theme 也不替换 |

启动期 `main.go` 加载失败仅打印 stderr 警告，TUI 仍用默认色板进入。

## 主题热更新涉及的派生 Style

`reloadStyles()` 一次性重建：

| 变量 | 来源 token |
|------|-----------|
| `popupBorderColor` | `PopupBorder` |
| `popupTitleStyle` | `PopupBorder` + Bold |
| `popupActiveStyle` | `ActiveHighlight` + Bold |
| `helpLineStyle` | `HintFg` |
| `rowHighlightStyle` | `RowHighlightBg` 背景 + `ActiveHighlight` 前景 + Bold |
| `activeTabStyle` | `TabActiveBg` 背景 + `TabActiveFg` 前景 + Bold + Padding |
| `flashStyle` | `SuccessFg` + Bold |

`inactiveTabStyle` 不依赖主题，永不重建。

## 添加新 token 的步骤

1. `config/theme.go` — 给 `ThemeConfig` 加字段（带 yaml tag）；`DefaultTheme()` 给默认值；`mergeWithDefaults` 加 `fixPair` 调用
2. `tui/styles.go` — 给 `Theme` 加字段；`ThemeFromConfig` 加映射；`reloadStyles()` 重建对应派生 Style（如有）
3. `tui/theme_config.go` — 给 `themeRows` 加一项 `{标签, 取址函数}`
4. 调用点用新 Style；如果是临时局部，直接 `lipgloss.NewStyle().Foreground(theme.<NewField>)`

新加字段时旧 `theme.yaml` 仍能加载（缺失字段走默认）。
