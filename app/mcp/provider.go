// Package mcp DPanel 内置 MCP server：把面板能力暴露给 AI Agent。
//
// 架构（单容器，无独立进程）：
//
//	gin 引擎
//	 ├─ /dpanel/api/*            原有面板 API（JWT，不动）
//	 ├─ /dpanel/mcp              MCP streamable-http 端点（Bearer 门禁）
//	 │     └─ mcp-go StreamableHTTPServer
//	 │           └─ Dispatcher（100 工具注册表）
//	 │                 └─ HTTP 回环 127.0.0.1:<port> 调 DPanel 自己的 API
//	 └─ /dpanel/api/common/mcp/* 管理面（用户 JWT）：token/profile/开关
//
// 鉴权设计：
//   - MCP 端点挂非 /api 前缀路径，AuthMiddleware 天然放行，用自己的 Bearer 门禁
//   - 回环调用用 service 身份登录（founder 凭据或专用 token），JWT 自续期 + 401 自愈
//
// 配置存 settings 表（setting/mcp 组），环境变量 DP_MCP_* 可覆盖（compose 兼容）。
package mcp

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/server"

	"github.com/donknap/dpanel/app/mcp/internal/client"
	"github.com/donknap/dpanel/app/mcp/internal/profile"
	"github.com/donknap/dpanel/app/mcp/internal/tools"
)

const (
	// McpEndpointPath MCP 端点路径（非 /api 前缀：AuthMiddleware 放行，走 Bearer 门禁）
	McpEndpointPath = "/dpanel/mcp"
	// SettingGroup settings 表的组名
	SettingGroup = "mcp"
	// SettingNameToken Bearer token 的 settings 键
	SettingNameToken = "token"
	// SettingNameProfile 能力分级的 settings 键
	SettingNameProfile = "profile"
	// SettingNameEnable 开关的 settings 键
	SettingNameEnable = "enable"
)

// Provider DPanel 启动时注册 MCP 路由。
type Provider struct{}

// Register 把 MCP 端点挂到 gin 引擎。
func (provider Provider) Register(engine *gin.Engine) {
	if !provider.enabled() {
		slog.Info("mcp server disabled")
		// 未启用也注册路由：明确 404 JSON（不落 SPA 兜底，客户端可区分）
		engine.POST(McpEndpointPath, func(c *gin.Context) {
			c.JSON(http.StatusNotFound, gin.H{"error": "mcp server disabled"})
		})
		engine.GET(McpEndpointPath, func(c *gin.Context) {
			c.JSON(http.StatusNotFound, gin.H{"error": "mcp server disabled"})
		})
		engine.DELETE(McpEndpointPath, func(c *gin.Context) {
			c.JSON(http.StatusNotFound, gin.H{"error": "mcp server disabled"})
		})
		return
	}

	api := client.New(
		provider.selfURL(),
		provider.serviceUsername(),
		provider.servicePassword(),
		true,  // autoLogin: 30 天长效 token
		false, // 回环不走 TLS
		60,
	)
	gate := profile.New(provider.profile())

	mcpServer := server.NewMCPServer(
		"dpanel-mcp",
		tools.Version,
		server.WithToolCapabilities(false),
	)
	tools.RegisterAll(mcpServer, api, gate)

	httpServer := server.NewStreamableHTTPServer(mcpServer)
	token := provider.token()

	// Bearer 门禁：无 token / 错 token 一律 401（fail-closed）
	engine.POST(McpEndpointPath, func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if token == "" || auth != "Bearer "+token {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		httpServer.ServeHTTP(c.Writer, c.Request)
	})
	// GET/DELETE（MCP 协议的会话管理）同样过门禁
	engine.GET(McpEndpointPath, func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if token == "" || auth != "Bearer "+token {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		httpServer.ServeHTTP(c.Writer, c.Request)
	})
	engine.DELETE(McpEndpointPath, func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if token == "" || auth != "Bearer "+token {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		httpServer.ServeHTTP(c.Writer, c.Request)
	})

	slog.Info("mcp server enabled", "endpoint", McpEndpointPath, "profile", provider.profile(), "tools", len(tools.Registry))
}
