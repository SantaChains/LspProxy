// LspProxy 程序入口
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SantaChains/LspProxy/cmd"
)

// 以下三个变量由 -ldflags 在构建时注入：
//
//	-X main.version=v1.2.3   → release tag
//	-X main.commit=abc1234   → git commit SHA
//	-X main.date=2026-04-03  → 构建日期
//
// 若未注入（本地 go build / go install latest），
// cmd 包会通过 debug.ReadBuildInfo() 从 VCS 元数据中自动填充 commit 和 date。
var (
	version = "dev"
	commit  = ""
	date    = ""
)

// proxyNames 是 LspProxy 自身的可执行名集合（小写、无扩展名）。
// 当 argv[0] 的基础名不在此集合中时，视为被以某个 LSP 名称调用，
// 自动以代理模式启动，代理该名称对应的真实 LSP。
var proxyNames = map[string]struct{}{
	"lspproxy":  {},
	"lsp-proxy": {},
}

func main() {
	cmd.SetVersionInfo(version, commit, date)

	// 若以 LSP 名称被调用（如复制 LspProxy.exe 为 rust-analyzer.exe），
	// 自动拼装为 "LspProxy -- <lspName> <args...>"，免去 wrapper 脚本。
	if lspName := detectLSPName(os.Args[0]); lspName != "" {
		os.Args = append([]string{os.Args[0], "--", lspName}, os.Args[1:]...)
	}

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// detectLSPName 检查 argv[0] 的基础名是否为 LSP 名称。
// 若是 LspProxy 自身名称则返回空串（按正常 CLI 流程处理）。
func detectLSPName(argv0 string) string {
	base := filepath.Base(argv0)
	ext := filepath.Ext(base)
	name := strings.ToLower(strings.TrimSuffix(base, ext))
	if _, ok := proxyNames[name]; ok {
		return ""
	}
	return name
}
