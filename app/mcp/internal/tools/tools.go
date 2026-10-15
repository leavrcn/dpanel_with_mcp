// Package tools 工具注册表。
//
// 架构：每个工具 = 一行注册表项 {name, description, route, schema, profile, destructive}。
// dispatcher 统一执行：门禁 → 分级检查 → confirm fail-closed → 内部转发 → 响应脱敏。
// 上游 DPanel 更新时 merge 后跑路由自检（tools_route_test.go 断言注册表路由全部命中
// live 实测验证集），新增 API 加一行表，删了 API 测试立刻红。
//
// 路由与参数契约对齐 DPanel v1.11.1（188 条路由源码级核对 + 真实实例实测）。
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/donknap/dpanel/app/mcp/internal/client"
	"github.com/donknap/dpanel/app/mcp/internal/profile"
	"github.com/donknap/dpanel/app/mcp/internal/redact"
)

// Version 与 DPanel 对齐的版本号。
const Version = "1.11.1-mcp7"

// ToolDef 工具注册表表项。
type ToolDef struct {
	Name        string
	Description string
	Route       string     // DPanel API 路由（/app/... 或 /common/...）
	Params      []ParamDef // 参数 schema（给 agent 的文档 + 取参）
	Minimum     string     // 最低能力分级
	Destructive bool       // 破坏性：必须 confirm=true
	// PostProcess 响应后处理：上游路由无法直接给出期望形态时（如 ws 管道路由
	// 换成一次性快照路由后按容器过滤），在 MCP 层加工响应数据。
	PostProcess func(data any, args map[string]any) any
	// PreCheck 本地前置校验：参数组合在上游必然失败（如走 WebSocket 管道的
	// 路由）时，在转发前报清晰错误，避免 agent 撞上游 500。
	PreCheck func(args map[string]any) error
	// Execute 本地执行（不走面板 API 转发）：Route 必须为空。
	// 用于面板 API 依赖 WebSocket 会话而 MCP 回环调用无法满足的场景
	// （如 image pull 的 ws progress fd）。参数已过 buildPayload 归一化。
	Execute func(ctx context.Context, payload map[string]any) (any, error)
}

// ParamDef 工具参数定义。
type ParamDef struct {
	Name        string
	Type        string   // "string" | "number" | "boolean" | "array"
	Description string
	Required    bool
	Enum        []string
	PayloadKey  string    // 为空时取 Name
	Normalize   string    // "int" | "int-list" | "str-list" | "json"（对齐 DPanel Go 类型契约）
	Merge       bool      // option 展开合并进 payload 顶层（对齐 Python payload.update(option)）
	Default     any       // 参数缺省时的默认值（对齐 Python 签名默认值）
	// MountPoint 校验：值必须以 volume:/container:/docker: 开头
	ValidateMountPoint bool
}

// Registry 全部工具注册表（各模块文件 init 时 append）。
var Registry []ToolDef

func register(defs ...ToolDef) {
	Registry = append(Registry, defs...)
}

func registerSlice(defs []ToolDef) {
	Registry = append(Registry, defs...)
}

// ---------------------------------------------------------------- dispatcher

type Dispatcher struct {
	api  *client.Client
	gate *profile.Gate
}

// RegisterAll 把注册表全部工具挂到 MCP server。
func RegisterAll(s *server.MCPServer, api *client.Client, gate *profile.Gate) {
	// 注入面板 API client 给本地执行工具（如 dpanel_image_pull 读镜像源配置）。
	// 漏掉这行会让本地工具的代理回退静默失效（loadRegistryRows 恒报未注入）。
	dispatcherAPIClient = api
	d := &Dispatcher{api: api, gate: gate}
	for _, def := range Registry {
		def := def
		tool := buildTool(def)
		s.AddTool(tool, d.handler(def))
	}
}

// buildTool 由注册表表项构造 mcp.Tool（schema）。
func buildTool(def ToolDef) mcp.Tool {
	opts := []mcp.ToolOption{mcp.WithDescription(def.Description)}
	for _, p := range def.Params {
		switch p.Type {
		case "number":
			opts = append(opts, mcp.WithNumber(p.Name, mcp.Description(p.Description)))
		case "boolean":
			opts = append(opts, mcp.WithBoolean(p.Name, mcp.Description(p.Description)))
		case "array":
			opts = append(opts, mcp.WithArray(p.Name, mcp.Description(p.Description), mcp.WithStringItems()))
		default:
			popts := []mcp.PropertyOption{mcp.Description(p.Description)}
			if len(p.Enum) > 0 {
				popts = append(popts, mcp.Enum(p.Enum...))
			}
			opts = append(opts, mcp.WithString(p.Name, popts...))
		}
		if p.Required {
			// required 标记挂在属性上（mcp.Required()）
			switch p.Type {
			case "number":
				opts = append(opts, mcp.WithNumber(p.Name, mcp.Description(p.Description), mcp.Required()))
			case "boolean":
				opts = append(opts, mcp.WithBoolean(p.Name, mcp.Description(p.Description), mcp.Required()))
			case "array":
				opts = append(opts, mcp.WithArray(p.Name, mcp.Description(p.Description), mcp.WithStringItems(), mcp.Required()))
			default:
				popts := []mcp.PropertyOption{mcp.Description(p.Description), mcp.Required()}
				if len(p.Enum) > 0 {
					popts = append(popts, mcp.Enum(p.Enum...))
				}
				opts = append(opts, mcp.WithString(p.Name, popts...))
			}
			continue
		}
	}
	if def.Destructive {
		opts = append(opts, mcp.WithBoolean("confirm", mcp.Description("破坏性操作必须显式传 true（fail-closed 人在回路确认）")))
	}
	return mcp.NewTool(def.Name, opts...)
}

// handler 生成单个工具的调用处理器：门禁 → 分级 → confirm → 转发 → 脱敏。
func (d *Dispatcher) handler(def ToolDef) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// 1. 分级门禁
		if !d.gate.Allowed(def.Name, def.Minimum) {
			return mcp.NewToolResultError(d.gate.ExplainDenial(def.Name, def.Minimum)), nil
		}
		// 2. 破坏性 fail-closed
		if def.Destructive {
			confirmed := request.GetBool("confirm", false)
			if !confirmed {
				return mcp.NewToolResultError(profile.ConfirmRequired(def.Name).Error()), nil
			}
		}
		// 3. 本地前置校验（参数组合在上游必然失败时提前报清晰错误）
		if def.PreCheck != nil {
			if err := def.PreCheck(request.GetArguments()); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
		}
		// 4. 组装 payload（参数归一化对齐 DPanel Go 类型）
		payload, err := buildPayload(def, request.GetArguments())
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		// 5. 执行：本地工具走 Execute，其余转发 DPanel API
		var data any
		if def.Execute != nil {
			data, err = def.Execute(ctx, payload)
			if err != nil {
				return mcp.NewToolResultErrorFromErr("本地执行失败", err), nil
			}
		} else {
			data, err = d.api.Post(ctx, def.Route, payload)
			if err != nil {
				return mcp.NewToolResultErrorFromErr("DPanel API 调用失败", err), nil
			}
		}
		// 6. 响应后处理（如 container_stat 按容器 id 过滤）
		if def.PostProcess != nil {
			data = def.PostProcess(data, request.GetArguments())
		}
		// 7. 响应脱敏 + 渲染
		return mcp.NewToolResultText(renderPayload(data)), nil
	}
}

// renderPayload 渲染响应数据：
//   - client.TextPayload（导出/下载类接口的文件流）按内容类型分流：
//     text/* 过 redact.Text() 脱敏后透传（日志里的 KEY=VALUE/JWT 会被 mask）；
//     二进制（zip/octet-stream）原样透传——正则脱敏对二进制无意义且可能破坏内容。
//   - 其余走 redact.Value + JSON 序列化。
func renderPayload(data any) string {
	if tp, ok := data.(client.TextPayload); ok {
		if strings.HasPrefix(tp.ContentType, "text/") {
			return redact.Text(tp.Content)
		}
		return tp.Content
	}
	data = redact.Value(data)
	out, err := json.Marshal(data)
	if err != nil {
		return fmt.Sprintf(`{"error":"响应序列化失败: %v"}`, err)
	}
	return string(out)
}

// buildPayload 组装 DPanel API payload：默认值填充 → 必填校验 → mountPoint 校验 →
// 类型归一化 → Merge 展开。抽成独立函数便于单测。
func buildPayload(def ToolDef, args map[string]any) (map[string]any, error) {
	payload := map[string]any{}
	for _, p := range def.Params {
		key := p.PayloadKey
		if key == "" {
			key = p.Name
		}
		raw, exists := args[p.Name]
		if !exists || raw == nil {
			// 缺省时用默认值；必填参数缺失报本地错误（不静默跳过）
			if p.Default != nil {
				if val, err := normalize(p, p.Default); err == nil {
					payload[key] = val
				} else {
					payload[key] = p.Default
				}
				continue
			}
			if p.Required {
				return nil, fmt.Errorf("缺少必填参数 %s", p.Name)
			}
			continue
		}
		// mountPoint 前缀校验（对齐 Python _mp()）
		if p.ValidateMountPoint {
			s, _ := raw.(string)
			if _, err := validateMountPoint(s); err != nil {
				return nil, fmt.Errorf("参数 %s %v", p.Name, err)
			}
		}
		val, err := normalize(p, raw)
		if err != nil {
			return nil, fmt.Errorf("参数 %s 类型错误: %v", p.Name, err)
		}
		// Merge：option 展开合并进 payload 顶层（对齐 Python payload.update(option)）
		if p.Merge {
			if m, ok := val.(map[string]any); ok {
				for k, v := range m {
					payload[k] = v
				}
				continue
			}
			return nil, fmt.Errorf("参数 %s 应为 JSON 对象以合并进请求", p.Name)
		}
		payload[key] = val
	}
	return payload, nil
}

// normalize 参数归一化：对齐 DPanel Go 类型契约（int32 / []int32 / []string / object）。
func normalize(p ParamDef, raw any) (any, error) {
	switch p.Normalize {
	case "int":
		return asInt(raw)
	case "int-list":
		return asIntList(raw)
	case "str-list":
		return asStrList(raw)
	case "json":
		return asJSON(raw)
	case "cron-expression":
		return asCronExpression(raw)
	default:
		return raw, nil
	}
}

// asJSON 把字符串参数解析为 JSON 对象/数组透传（agent 传 JSON 字符串给 dict/list 参数）。
// 已是 map/slice 的值直接透传（客户端可能原生传对象）。
func asJSON(v any) (any, error) {
	switch t := v.(type) {
	case map[string]any, []any:
		return t, nil
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil, fmt.Errorf("应为 JSON 对象或数组字符串，实际为空")
		}
		var out any
		if err := json.Unmarshal([]byte(s), &out); err != nil {
			return nil, fmt.Errorf("应为合法 JSON（对象或数组），解析失败: %v", err)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("应为 JSON 字符串或对象，实际 %T", v)
	}
}

// asCronExpression 把 cron 表达式包装成 DPanel 的 Expression 数组契约：
// [{"unit":"code","code":"<表达式>"}]。@manual 与 6 字段表达式均按 code 透传，
// 表达式合法性由 DPanel 侧 CheckExpression 校验。
func asCronExpression(v any) (any, error) {
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("应为 cron 表达式字符串，实际 %T", v)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("cron 表达式不能为空")
	}
	return []any{map[string]any{"unit": "code", "code": s}}, nil
}

// validateMountPoint 校验 explorer 挂载点格式（对齐 Python _mp()）。
func validateMountPoint(s string) (string, error) {
	for _, prefix := range []string{"volume:", "container:", "docker:"} {
		if strings.HasPrefix(s, prefix) {
			return s, nil
		}
	}
	return "", fmt.Errorf(
		"必须为 'volume:<name>'、'container:<md5>' 或 'docker:<env>'（如 'volume:dpanel-data', 'container:abc123'；docker: 仅限开启 SSH 的远程环境）")
}

// filterContainerStat container_stat 的响应后处理：
// 上游 /app/container/get-stat-info 是 ws 管道路由（数据只走 WebSocket 广播，
// HTTP 响应不含统计），改用 /common/home/get-stat-list（follow=false）一次性
// 拉全量后，按容器 id（完整 ID / 前缀）或容器名过滤出目标容器。
// 注意：上游 stats 的 name 带 "/" 前缀（daemon reserveName 存储 "/"+name，
// stat.go 直接透传 v.Name），比较前必须双侧裁剪，否则按容器名查询静默返回空。
func filterContainerStat(data any, args map[string]any) any {
	id, _ := args["id"].(string)
	if id == "" {
		return data
	}
	id = strings.TrimPrefix(id, "/")
	obj, ok := data.(map[string]any)
	if !ok {
		return data
	}
	docker, ok := obj["docker"].([]any)
	if !ok {
		return data
	}
	out := make([]any, 0, len(docker))
	for _, item := range docker {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		container, _ := m["container"].(string)
		name, _ := m["name"].(string)
		name = strings.TrimPrefix(name, "/")
		if strings.HasPrefix(container, id) || name == id {
			out = append(out, m)
		}
	}
	// 无匹配时给出可区分的提示（容器不存在 vs 无统计），避免静默空数组
	if len(out) == 0 {
		obj["docker"] = out
		obj["matched"] = 0
		obj["hint"] = "no container matched id " + id + "（可用 dpanel_container_list 查容器 ID/名称）"
		return obj
	}
	obj["docker"] = out
	return obj
}

func asInt(v any) (int, error) {
	switch t := v.(type) {
	case float64: // JSON number
		return int(t), nil
	case int:
		return t, nil
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return 0, fmt.Errorf("应为整数或数字字符串，实际 %q", t)
		}
		return n, nil
	case bool:
		return 0, fmt.Errorf("应为整数，实际 bool")
	default:
		return 0, fmt.Errorf("应为整数，实际 %T", v)
	}
}

func asIntList(v any) ([]int, error) {
	switch t := v.(type) {
	case []any:
		out := make([]int, 0, len(t))
		for _, item := range t {
			n, err := asInt(item)
			if err != nil {
				return nil, err
			}
			out = append(out, n)
		}
		return out, nil
	default:
		n, err := asInt(v)
		if err != nil {
			return nil, err
		}
		return []int{n}, nil
	}
}

func asStrList(v any) ([]string, error) {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, fmt.Sprintf("%v", item))
		}
		return out, nil
	case string:
		return []string{t}, nil
	default:
		return []string{fmt.Sprintf("%v", v)}, nil
	}
}
