<div align="center">

# LspProxy

**让每一位开发者都能无障碍阅读英文 LSP 文档**

透明代理插入编辑器与 LSP 之间，实时将悬停文档、补全说明、签名提示、诊断信息翻译为中文。

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat-square&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey?style=flat-square)](https://github.com/SantaChains/LspProxy)

</div>

---

## 它是什么

`LspProxy` 是一个 **LSP（Language Server Protocol）中文翻译代理**。

它以完全透明的方式插入在编辑器（VSCode / Neovim / Zed / Emacs）和真实 LSP 进程之间，拦截并翻译 LSP 响应中的英文文档，其余所有消息原样透传——**编辑器无需任何修改，LSP 无需感知代理存在**。

```
编辑器 stdin  ──► [forwardClientToLsp] ──► rust-analyzer
编辑器 stdout ◄── [forwardLspToClient] ◄── rust-analyzer
                         ↑
                   翻译处理发生在此处
                   （只修改文档字段，协议帧完整保留）
```

### 翻译覆盖范围

| LSP 消息类型 | 字段 | 说明 |
|---|---|---|
| `textDocument/hover` | `contents` | 悬停文档 |
| `textDocument/completion` | `documentation` | 补全项说明 |
| `textDocument/signatureHelp` | `documentation` | 参数/函数签名文档 |
| `textDocument/publishDiagnostics` | `message` | 诊断错误消息 |
| `completionItem/resolve` | `documentation` | 补全项解析文档 |
| `textDocument/diagnostic` | `message` | 拉取式诊断（LSP 3.17+） |

---

## 核心特性

### 四级缓存，极速响应

```
词汇本  →  内存 LRU  →  磁盘 JSON 词典  →  在线翻译 API
（精确匹配） （热数据）    （跨进程持久化）     （首次请求）
```

- 内存缓存按**字节大小**限制（默认 30 MB），重复文档毫秒响应
- 磁盘词典跨进程持久化，重启后无需重新联网翻译
- 原子写入（`.tmp` → `rename`），避免词典文件损坏

### 两阶段超时，兼顾速度与质量

| 阶段 | 超时 | 行为 |
|---|---|---|
| 快速路径 | 50 ms | 缓存命中时极速返回 |
| 等待窗口 | 可配置（默认 600 ms） | 给在线翻译足够时间 |
| 超时后 | — | 立即返回英文原文，后台继续预热缓存 |

### 占位符保护，代码块不被翻译

翻译时自动将代码块替换为 `$CODE_N$` 占位符，整体发送给翻译引擎（保留上下文），翻译完成后还原——代码块永远不会被误译。

### 诊断零延迟

`publishDiagnostics` 通知会立即以英文显示，翻译完成后**异步推送**中文版本，不阻塞编辑器渲染。

---

## 安装

### 从源码构建

```bash
git clone https://github.com/SantaChains/LspProxy
cd LspProxy
go build -o LspProxy .
```

### 安装到 `$GOPATH/bin`

```bash
go install github.com/SantaChains/LspProxy@latest
```

---

## 快速开始

```bash
# 代理 rust-analyzer（使用默认 Google 翻译，无需密钥）
LspProxy -- rust-analyzer

# 代理 clangd
LspProxy -- clangd

# 代理 gopls
LspProxy -- gopls

# 使用 OpenAI（或 DeepSeek / Qwen / Ollama）翻译
LspProxy -e openai -- rust-analyzer

# 指定配置文件
LspProxy --config ~/my-config.yaml -- typescript-language-server --stdio

# 启动 TUI 管理界面（可视化配置 + 日志查看）
LspProxy --tui
```

### 命令行标志

| 标志 | 简写 | 默认值 | 说明 |
|---|---|---|---|
| `--config` | — | `~/.config/lsp-proxy/config.yaml` | 配置文件路径 |
| `--engine` | `-e` | 继承配置文件 | 覆盖翻译引擎（`google` \| `openai`） |
| `--tui` | — | `false` | 启动 TUI 管理界面 |

---

## 编辑器集成

### Neovim（nvim-lspconfig）

```lua
require('lspconfig').rust_analyzer.setup({
  cmd = { 'lsp-proxy', '--', 'rust-analyzer' },
})
```

### VSCode

VSCode 的 `server.path` 只接受单个可执行文件路径，不支持传参。需要创建一个 wrapper 脚本：

**Unix**（`rust-analyzer-proxy.sh`）：

```sh
#!/bin/sh
exec LspProxy -- rust-analyzer "$@"
```

```bash
chmod +x /usr/local/bin/rust-analyzer-proxy.sh
```

**Windows**（`rust-analyzer-proxy.bat`）：

```bat
@echo off
LspProxy -- rust-analyzer %*
```

**settings.json**：

```json
{
  "rust-analyzer.server.path": "/usr/local/bin/rust-analyzer-proxy.sh"
}
```

> 其他 LSP（clangd、gopls 等）同理，将脚本中的 `rust-analyzer` 替换为对应的 LSP 命令即可。

### Zed（settings.json）

```json
{
  "lsp": {
    "rust-analyzer": {
      "binary": {
        "path": "lsp-proxy",
        "arguments": ["--", "rust-analyzer"]
      }
    }
  }
}
```

---

## 配置

配置文件位于 `~/.config/lsp-proxy/config.yaml`，首次运行自动创建。

```yaml
translate:
  engine: google          # 翻译引擎：google | openai
  openai:
    base_url: https://api.openai.com/v1   # 也可指向 DeepSeek / Qwen / Ollama
    api_key: sk-xxxx
    model: gpt-4o-mini

proxy:
  target_lang: zh-CN      # 目标语言，BCP 47 标签
  cache_size: 30          # 内存缓存上限，单位 MB
  dict_file: ~/.local/share/lsp-proxy/dict.json   # 磁盘词典路径
  translation_timeout: 600  # 翻译等待超时，单位毫秒，0 = 无限等待

log:
  level: info             # debug | info | warn | error
  file: ~/.local/share/lsp-proxy/proxy.log
```

### 翻译引擎对比

| 引擎 | API 密钥 | 速度 | 质量 | 适合场景 |
|---|---|---|---|---|
| **Google**（默认） | 不需要 | 快 | 好 | 日常使用，零配置 |
| **OpenAI** | 需要 | 中 | 优秀 | 复杂文档，高质量需求 |
| **DeepSeek** | 需要 | 快 | 优秀 | 高性价比替代 |
| **Ollama**（本地） | 不需要 | 依赖硬件 | 取决于模型 | 离线/隐私场景 |

#### 使用 DeepSeek

```yaml
translate:
  engine: openai
  openai:
    base_url: https://api.deepseek.com/v1
    api_key: sk-xxxx
    model: deepseek-chat
```

#### 使用 Ollama（本地）

```yaml
translate:
  engine: openai
  openai:
    base_url: http://localhost:11434/v1
    api_key: dummy           # Ollama 不需要 API Key
    model: qwen2.5:7b
```

---

## TUI 管理界面

运行 `LspProxy --tui` 启动可视化管理界面：

```
┌──────────────────────────────────────────────────────────┐
│  [1] 状态  [2] 配置  [3] 日志  [4] 提示词  [5] 词典  [6] 词汇本 │
├──────────────────────────────────────────────────────────┤
│  翻译引擎：  Google                                        │
│  目标语言：  zh-CN                                         │
│  内存缓存：  30 MB                                         │
│  翻译超时：  600 ms                                        │
│  日志级别：  info                                          │
└──────────────────────────────────────────────────────────┘
```

| 快捷键 | 功能 |
|---|---|
| `1`-`6` | 切换标签页（状态页有效，配置页输入框中不拦截） |
| `Tab` / `↑↓` | 配置表单字段导航 |
| `Ctrl+S` | 保存配置 |
| `j/k/PgUp/PgDn/g/G` | 日志滚动 |
| `q` / `Ctrl+C` | 退出 |

---

## 项目结构

```
LspProxy/
├── cmd/
│   ├── root.go          # 根命令，--config 全局标志
│   ├── run.go           # 代理模式 & TUI 模式、日志初始化
│   └── version.go       # version 子命令
├── internal/
│   ├── config/          # 配置加载/保存（YAML + viper）
│   ├── glossary/        # 术语词汇本（含内嵌 builtin 词库 + 热重载）
│   ├── lsp/
│   │   ├── jsonrpc.go   # LSP Content-Length 帧读写
│   │   ├── message.go   # JSON-RPC 消息类型
│   │   └── handler.go   # 翻译调度、两阶段超时
│   ├── markdown/
│   │   └── splitter.go  # 代码块占位符保护
│   ├── proxy/
│   │   └── proxy.go     # 子进程管理、双向消息转发
│   └── translate/
│       ├── engine.go             # Engine 接口 + LRU 内存缓存
│       ├── new.go                # 翻译引擎工厂函数
│       ├── google.go             # Google 免费翻译
│       ├── openai.go             # OpenAI 兼容 API
│       ├── dict.go               # 磁盘 JSON 词典 + 三级缓存
│       ├── singleflight_engine.go # 并发请求合并装饰器
│       ├── prompt.go             # 提示词模板加载与热重载
│       └── template.go           # 诊断消息模板化翻译
├── tui/
│   ├── app.go           # Bubble Tea TUI（6 个标签页）
│   └── styles/          # lipgloss 样式常量
└── website/             # Astro 文档网站
```

**分层职责（关注点严格分离）：**

```
cmd/          → CLI 解析、参数校验、组件装配
config/       → 配置加载/保存，零外部依赖
lsp/          → 协议实现，不知道翻译细节
translate/    → 翻译逻辑，不知道 LSP 协议
markdown/     → 文本分割，零外部依赖
proxy/        → 流程编排，组合所有组件
tui/          → 界面展示，只调用 config 包
```

---

## 开发

```bash
# 格式化代码（提交前必须执行）
go fmt ./...

# 静态分析
go vet ./...

# 运行测试
go test ./...

# 竞态检测
go test ./... -race

# 构建
go build -o LspProxy .
```

### 文档网站开发

```bash
cd website
bun install
bun run dev      # 启动开发服务器
bun run build    # 生产构建
```

---

## 设计原则

- **零污染**：日志严格写文件，绝不写 stdout，LSP 协议帧不被污染
- **降级优先**：磁盘词典失败 → 纯内存缓存；翻译失败 → 透传原文；任何环节故障都不中断代理主流程
- **并发安全**：翻译 goroutine 与写出 goroutine 通过 channel 解耦，读取循环永不阻塞，并发翻译上限 32
- **协议透明**：仅修改文档字段内容，方法名、ID、其他所有字段原样保留

---

## 致谢

本项目基于 [zerx-lab/LspProxy](https://github.com/zerx-lab/LspProxy) 开发，感谢原作者 [@zerx-lab](https://github.com/zerx-lab) 的贡献。

## 许可证

[MIT License](LICENSE) — Copyright (c) 2026 zerx-lab, SantaChains

---

<div align="center">

Made with love for Chinese developers who deserve better tooling.

</div>
