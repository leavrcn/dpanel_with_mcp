// Package mcp 配置与凭据管理：settings 存取通过注入的 Store 接口解耦，
// 避免子模块反向依赖主仓（主仓的 docker 依赖链含 cgo，replace 会拖进全部依赖）。
//
// 优先级：环境变量 DP_MCP_* > Store 持久化值 > 默认值。
package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"strconv"

	"github.com/donknap/dpanel/app/mcp/internal/tools"
)

const (
	DefaultProfile = "read-write"
)

// Store settings 持久化接口（由主仓侧注入实现，存 settings 表）。
type Store interface {
	// LoadMcpConfig 读 MCP 配置（不存在返回零值）
	LoadMcpConfig() (enable bool, token, profile string)
	// SaveMcpConfig 写 MCP 配置
	SaveMcpConfig(enable bool, token, profile string) error
	// LoadServiceCredential 读回环 service 身份
	LoadServiceCredential() (username, password string)
	// SaveServiceCredential 写回环 service 身份
	SaveServiceCredential(username, password string) error
}

// store 全局注入点（主仓 provider 注册时注入）。
var store Store

// SetStore 注入持久化实现（DPanel 启动时调用一次）。
func SetStore(s Store) {
	store = s
}

// ---------------------------------------------------------------- 环境变量覆盖

func envOverride(key, def string) string {
	if v := os.Getenv("DP_MCP_" + key); v != "" {
		return v
	}
	return def
}

func loadConfig() (enable bool, token, profile string) {
	if store != nil {
		return store.LoadMcpConfig()
	}
	return false, "", ""
}

// enabled MCP 是否启用：DP_MCP_ENABLE > 持久化 enable（默认关）。
func (provider Provider) enabled() bool {
	if v := os.Getenv("DP_MCP_ENABLE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	enable, _, _ := loadConfig()
	return enable
}

// token Bearer 门禁 token：DP_MCP_TOKEN > 持久化 token。
// 为空时端点直接 401（fail-closed：没配 token = 不放行）。
func (provider Provider) token() string {
	_, token, _ := loadConfig()
	return envOverride("TOKEN", token)
}

// profile 能力分级：DP_MCP_PROFILE > 持久化 profile > read-write。
func (provider Provider) profile() string {
	_, _, p := loadConfig()
	p = envOverride("PROFILE", p)
	switch p {
	case "read-only", "read-write", "admin":
		return p
	default:
		return DefaultProfile
	}
}

// serviceUsername / servicePassword 回环 service 身份：
// DP_MCP_SERVICE_USERNAME/PASSWORD > 持久化凭据。
func (provider Provider) serviceUsername() string {
	u, _ := provider.loadService()
	return envOverride("SERVICE_USERNAME", u)
}

func (provider Provider) servicePassword() string {
	_, p := provider.loadService()
	return envOverride("SERVICE_PASSWORD", p)
}

func (provider Provider) loadService() (username, password string) {
	if store != nil {
		return store.LoadServiceCredential()
	}
	return "", ""
}

// NewToken 生成 32 字节随机 hex token（管理面「重新生成」用）。
func NewToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		slog.Error("mcp generate token", "error", err.Error())
		return ""
	}
	return hex.EncodeToString(b)
}

// selfURL 回环地址：DPanel 自己的监听端口。
// DP_MCP_SELF_URL 可覆盖（默认 http://127.0.0.1:8080）。
func (provider Provider) selfURL() string {
	if v := os.Getenv("DP_MCP_SELF_URL"); v != "" {
		return v
	}
	port := os.Getenv("APP_SERVER_PORT")
	if port == "" {
		port = "8080"
	}
	return "http://127.0.0.1:" + port
}

// ---------------------------------------------------------------- 导出 API（管理面用）

// CurrentConfig 当前配置（供管理面 controller 读取）。
func CurrentConfig() (enable bool, token, profile string) {
	return loadConfig()
}

// SaveConfig 保存配置（供管理面 controller 写入）。
func SaveConfig(enable bool, token, profile string) error {
	if store == nil {
		return nil
	}
	return store.SaveMcpConfig(enable, token, profile)
}

// SaveServiceCredential 保存回环 service 凭据（供管理面 controller 写入）。
// 环境变量 DP_MCP_SERVICE_USERNAME/PASSWORD 仍优先于此持久化值。
func SaveServiceCredential(username, password string) error {
	if store == nil {
		return nil
	}
	return store.SaveServiceCredential(username, password)
}

// CurrentServiceCredential 当前回环 service 凭据（供管理面展示是否已配置）。
func CurrentServiceCredential() (username, password string) {
	provider := Provider{}
	return provider.loadService()
}

// Tools 工具注册表（供管理面展示工具数）。
func Tools() []string {
	names := make([]string, 0, len(tools.Registry))
	for _, def := range tools.Registry {
		names = append(names, def.Name)
	}
	return names
}
