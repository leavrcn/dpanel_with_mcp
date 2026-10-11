package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"

	"github.com/donknap/dpanel/app/mcp"
	"github.com/donknap/dpanel/common/function"
	"github.com/donknap/dpanel/common/types/define"
)

// Mcp 内置 MCP server 的管理面（走用户 JWT，面板前端调用）。
type Mcp struct {
	controller.Abstract
}

// GetStatus MCP 状态：是否启用、端点、profile、token 掩码、工具数。
func (self Mcp) GetStatus(http *gin.Context) {
	enable, token, profile := mcp.CurrentConfig()
	masked := ""
	if token != "" {
		masked = "******"
	}
	self.JsonResponseWithoutError(http, gin.H{
		"enable":   enable,
		"endpoint": mcp.McpEndpointPath,
		"profile":  profile,
		"token":    masked,
		"tools":    len(mcp.Tools()),
	})
}

type mcpUpdateParams struct {
	Enable  *bool   `json:"enable" binding:"required"`
	Token   *string `json:"token"`
	Profile *string `json:"profile" binding:"omitempty,oneof=read-only read-write admin"`
}

// Update 启用/禁用/改配置。启用时若无 token 自动生成；service 凭据由启用流程写入。
func (self Mcp) Update(http *gin.Context) {
	params := mcpUpdateParams{}
	if !self.Validate(http, &params) {
		return
	}
	_, token, profile := mcp.CurrentConfig()
	if params.Token != nil {
		token = *params.Token
	}
	if params.Profile != nil {
		profile = *params.Profile
	}
	if *params.Enable && token == "" {
		token = mcp.NewToken()
		if token == "" {
			self.JsonResponseWithError(http, function.ErrorMessage(define.ErrorMessageUnknow, "message", "failed to generate MCP token"), 500)
			return
		}
	}
	if err := mcp.SaveConfig(*params.Enable, token, profile); err != nil {
		self.JsonResponseWithError(http, err, 500)
		return
	}
	self.JsonSuccessResponse(http)
}

// RegenerateToken 重新生成 Bearer token（旧 token 立即失效——需重启生效）。
func (self Mcp) RegenerateToken(http *gin.Context) {
	enable, _, profile := mcp.CurrentConfig()
	token := mcp.NewToken()
	if token == "" {
		self.JsonResponseWithError(http, function.ErrorMessage(define.ErrorMessageUnknow, "message", "token generation failed"), 500)
		return
	}
	if err := mcp.SaveConfig(enable, token, profile); err != nil {
		self.JsonResponseWithError(http, err, 500)
		return
	}
	self.JsonResponseWithoutError(http, gin.H{"token": token})
}
