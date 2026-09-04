<div align="center">

# LspProxy

**透明代理 LSP 消息，将英文文档实时翻译为中文**

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go\&style=flat-square)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey?style=flat-square)](https://github.com/SantaChains/LspProxy)

</div>

***

## 简介

`LspProxy` 是一个 LSP 中文翻译代理。它以透明方式插入编辑器（VSCode / Neovim / Zed / Emacs）与真实 LSP 进程之间，拦截并翻译 LSP 响应中的英文文档，其余消息原样透传。编辑器和 LSP 均无需感知代理存在。

```
编辑器 stdin  ──► [forwardClientToLsp] ──► LSP
编辑器 stdout ◄── [forwardLspToClient] ◄── LSP
                       ↑
                  仅翻译文档字段，协议帧完整保留
```

### 翻译覆盖范围

| LSP 消息                            | 字段              | 说明               |
| --------------------------------- | --------------- | ---------------- |
| `textDocument/hover`              | `contents`      | 悬停文档             |
| `textDocument/completion`         | `documentation` | 补全项说明            |
| `textDocument/signatureHelp`      | `documentation` | 签名提示             |
| `textDocument/publishDiagnostics` | `message`       | 诊断消息             |
| `completionItem/resolve`          | `documentation` | 补全项解析            |
| `textDocument/diagnostic`         | `message`       | 拉取式诊断（LSP 3.17+） |

***

## 特性

- **四级缓存**：词汇本 → 内存 LRU → 磁盘 JSON 词典 → 在线翻译 API，重复文档毫秒响应

- **两阶段超时**：缓存命中走 50 ms 快速路径；未命中在可配置超时内等待翻译，超时后返回原文并后台预热缓存

- **占位符保护**：代码块与技术术语替换为 `$CODE_N$` 占位符后整体翻译，还原后代码不被误译

- **诊断零延迟**：`publishDiagnostics` 立即以英文显示，翻译完成后异步推送中文版本

- **并发合并**：相同文本的并发翻译请求只发起一次 API 调用

- **降级优先**：任何环节失败均透传原文，绝不中断代理主流程

***

## 安装

```bash
# 从源码构建
git clone https://github.com/SantaChains/LspProxy
cd LspProxy
go build -o LspProxy .

# 或安装到 $GOPATH/bin
go install github.com/SantaChains/LspProxy@latest
```

要求 Go 1.25+。

构建后运行 `LspProxy --version`，版本号会自动识别：

- `go install github.com/SantaChains/LspProxy@latest` → 显示模块版本（含 commit 与日期）

- 本地 `go build` → 从 git 元数据读取 commit 与日期

- release 构建（`-ldflags` 注入）→ 显示语义化版本号

***

## 快速开始

```bash
# 代理 rust-analyzer（默认 Google 翻译，无需密钥）
LspProxy -- rust-analyzer

# 代理 clangd，并传额外参数
LspProxy -- clangd --background-index

# 使用 OpenAI 兼容翻译引擎
LspProxy -e openai -- rust-analyzer

# 指定配置文件
LspProxy --config ~/my-config.yaml -- gopls

# 启动 TUI 管理界面
LspProxy --tui
```

### 命令行标志

| 标志                | 简写     | 说明                                           |
| ----------------- | ------ | -------------------------------------------- |
| `--config <path>` | <br /> | 配置文件路径（默认 `~/.config/lsp-proxy/config.yaml`） |
| `--engine <name>` | `-e`   | 覆盖翻译引擎：`google` \| `openai`                  |
| `--tui`           | <br /> | 启动 TUI 管理界面                                  |
| `--version`       | `-v`   | 显示版本信息                                       |
| `--help`          | `-h`   | 显示帮助                                         |

***

## 编辑器集成

LspProxy 对所有支持自定义 LSP 命令的编辑器通用。Neovim 和 Zed 原生支持传参，直接指定 `LspProxy -- <lsp>` 即可；VSCode 的 `server.path` 不支持传参，需用自调用副本。

### Neovim（nvim-lspconfig）

```lua
require('lspconfig').rust_analyzer.setup({
  cmd = { 'LspProxy', '--', 'rust-analyzer' },
})
```

hover 触发：默认绑定大写 `K`（不是小写 `k`，`k` 是上移光标），或 `:lua vim.lsp.buf.hover()`。LspProxy 只拦截翻译内容，不影响 hover 触发机制。

clangd 示例：

```lua
require('lspconfig').clangd.setup({
  cmd = { 'LspProxy', '--', 'clangd' },
})
```

### Zed（settings.json）

```json
{
  "lsp": {
    "rust-analyzer": {
      "binary": {
        "path": "LspProxy",
        "arguments": ["--", "rust-analyzer"]
      }
    }
  }
}
```

### VSCode

`rust-analyzer.server.path` 仅接受单个可执行文件路径，不支持传参。用**自调用副本**方案最稳定：

1. 把 `LspProxy.exe` 复制一份命名为 `rust-analyzer.exe`，放在独立目录（不要与真实 rust-analyzer 同目录）：

```powershell
copy "$(go env GOPATH)\bin\LspProxy.exe" "D:\tools\lsp-proxy\rust-analyzer.exe"
```

1. VSCode `settings.json`：

```json
{ "rust-analyzer.server.path": "D:\\tools\\lsp-proxy\\rust-analyzer.exe" }
```

1. 重启 VSCode。

LspProxy 启动时检测到自身名为 `rust-analyzer`，自动在 PATH 中查找真实 rust-analyzer 并代理（跳过自身副本所在目录，不会递归）。真实 rust-analyzer 须在 PATH 中且不与副本同目录，或在副本同目录放置 `rust-analyzer.real.exe`。

> 其他 LSP（clangd、gopls、pyright 等）同理：复制为对应 LSP 名，配置对应扩展的 `server.path`。
>
> clangd 示例（VSCode `clangd` 扩展）：
>
> ```powershell
> copy "$(go env GOPATH)\bin\LspProxy.exe" "D:\tools\lsp-proxy\clangd.exe"
> ```
>
> ```json
> { "clangd.path": "D:\\tools\\lsp-proxy\\clangd.exe" }
> ```

> **clangd 注意事项：**
> - `clangd.arguments` 只能放 clangd 自身参数（`--background-index`、`--clang-tidy` 等），**不要放编译器参数**（如 `-Wno-unused-value`、`-std=c++17`），否则 clangd 会因未知参数崩溃。
> - 编译参数应放在 `compile_commands.json` 或 `compile_flags.txt` 中，由 clangd 自动读取。
> - CMake 项目生成编译数据库：`cmake -B build -DCMAKE_EXPORT_COMPILE_COMMANDS=ON`，再将 `build/compile_commands.json` 链接到项目根目录。

***

## 配置

配置文件位于 `~/.config/lsp-proxy/config.yaml`，首次运行自动创建。

```yaml
translate:
  engine: google          # google | openai
  openai:
    base_url: https://api.openai.com/v1
    api_key: sk-xxxx
    model: gpt-4o-mini

proxy:
  target_lang: zh-CN      # 目标语言，BCP 47 标签
  cache_size: 30          # 内存缓存上限，MB
  dict_file: ~/.local/share/lsp-proxy/dict.json
  translation_timeout: 600  # 翻译等待超时，毫秒，0 = 无限等待

log:
  level: info             # debug | info | warn | error
  file: ~/.local/share/lsp-proxy/proxy.log
```

### 翻译引擎

| 引擎         | 密钥 | 适用场景     |
| ---------- | -- | -------- |
| Google（默认） | 无需 | 日常使用，零配置 |
| OpenAI     | 需要 | 复杂文档，高质量 |
| DeepSeek   | 需要 | 高性价比     |
| Ollama（本地） | 无需 | 离线 / 隐私  |

使用 DeepSeek：

```yaml
translate:
  engine: openai
  openai:
    base_url: https://api.deepseek.com/v1
    api_key: sk-xxxx
    model: deepseek-chat
```

使用 Ollama：

```yaml
translate:
  engine: openai
  openai:
    base_url: http://localhost:11434/v1
    api_key: dummy
    model: qwen2.5:7b
```

***

## TUI 管理界面

`LspProxy --tui` 启动可视化管理界面，提供状态、配置、日志、提示词、词典、词汇本六个标签页。

| 快捷键                 | 功能       |
| ------------------- | -------- |
| `1`–`6`             | 切换标签页    |
| `Tab` / `↑↓`        | 配置表单字段导航 |
| `Ctrl+S`            | 保存配置     |
| `j/k/PgUp/PgDn/g/G` | 日志滚动     |
| `q` / `Ctrl+C`      | 退出       |

***

## 项目结构

```
LspProxy/
├── cmd/                    # CLI 命令（root、run、version）
├── internal/
│   ├── config/             # 配置加载 / 保存（YAML + viper）
│   ├── glossary/           # 术语词汇本（内置词库 + 热重载）
│   ├── lsp/                # JSON-RPC 帧读写、消息类型、翻译调度
│   ├── markdown/           # 代码块占位符保护
│   ├── proxy/              # 子进程管理、双向消息转发
│   └── translate/          # 翻译引擎与缓存（内存 LRU / 磁盘词典 / 在线 API）
├── tui/                    # Bubble Tea 管理界面
└── website/                # Astro 文档网站
```

分层职责：`cmd` 装配组件，`config` 不依赖其他内部包，`lsp` 不知翻译细节，`translate` 不知 LSP 协议，`proxy` 编排全部，`tui` 只调用 `config`。

***

## 开发

```bash
go fmt ./...            # 格式化
go vet ./...            # 静态分析
go test ./...           # 测试
go test ./... -race     # 竞态检测
go build -o LspProxy .  # 构建
```

文档站开发：

```bash
cd website
bun install
bun run dev
bun run build
```

***

## 设计原则

- **零污染**：日志严格写文件，不写 stdout，LSP 协议帧不被破坏

- **降级优先**：词典失败退化为纯内存缓存；翻译失败透传原文

- **并发安全**：翻译与写出通过 channel 解耦，读取循环永不阻塞

- **协议透明**：仅修改文档字段，方法名、ID 等其余字段原样保留

***

## 路线图

### P0（已完成）

- **FallbackEngine 并发竞速**：免费引擎（Google、MyMemory）并发请求，首个成功立即返回，全部失败后串行降级到 AI 接口，避免串行等待超时

- **新增 MyMemoryEngine**：MyMemory 免费翻译 API，国内可达，无需 API key，匿名每日 5000 字限额

### P1（已完成）

- **引擎熔断**：连续失败 3 次的引擎进入 60s 冷却期，期间跳过，避免每次先试必败的引擎

- **提示词精简**：明确保留范围（标识符、关键字、API 名、错误类型名），翻译描述性文字，减少 token 消耗

### P2

- **Bing 翻译接入**：研究 Bing 免费端点的稳定调用方式

- **TUI 实时翻译日志**：新增日志标签页，便于调试

### P3

- **DeepSeek 结构化输出**：返回原文-译文-术语对照 JSON，支持自动术语提取

- **批量翻译预热**：项目启动时批量翻译 completion 文档，减少首次 hover 等待

***

## 致谢

基于 [zerx-lab/LspProxy](https://github.com/zerx-lab/LspProxy) 开发。

## 许可证

[MIT](LICENSE) — Copyright (c) 2026 zerx-lab, SantaChains
