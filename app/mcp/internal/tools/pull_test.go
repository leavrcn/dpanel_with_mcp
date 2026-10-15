package tools

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/registry"
)

// TestImagePullToolContract dpanel_image_pull 工具契约：
// 该工具是 MCP 侧本地执行（不走面板 API 转发），修复 MCP 通道无法 pull 镜像的缺口
// （面板 /app/image/tag-sync 的 pull 依赖 ws progress fd，MCP 回环调用无 ws 会话必报 fd not found）。
func TestImagePullToolContract(t *testing.T) {
	var def *ToolDef
	for i := range Registry {
		if Registry[i].Name == "dpanel_image_pull" {
			def = &Registry[i]
			break
		}
	}
	if def == nil {
		t.Fatal("注册表缺少 dpanel_image_pull 工具")
	}
	// 本地执行工具：无面板路由，必须有 Execute 实现
	if def.Execute == nil {
		t.Errorf("dpanel_image_pull: Execute 未实现（本地执行工具必须有）")
	}
	if def.Route != "" {
		t.Errorf("dpanel_image_pull: 本地执行工具不应有面板路由，实际 %q", def.Route)
	}
	// 必填参数：tag
	var tagParam *ParamDef
	for i := range def.Params {
		if def.Params[i].Name == "tag" {
			tagParam = &def.Params[i]
			break
		}
	}
	if tagParam == nil {
		t.Fatal("dpanel_image_pull: 缺少 tag 参数")
	}
	if !tagParam.Required {
		t.Errorf("dpanel_image_pull: tag 参数必须必填")
	}
	// pull 是写操作但非破坏性（不删除数据），read-write 分级
	if def.Minimum != "read-write" {
		t.Errorf("dpanel_image_pull: 分级应为 read-write，实际 %q", def.Minimum)
	}
	if def.Destructive {
		t.Errorf("dpanel_image_pull: 不应标记 Destructive（pull 不删除任何数据）")
	}
}

// TestLocalExecuteToolsRouteExempt 本地执行工具（Route 为空）豁免路由契约测试。
func TestLocalExecuteToolsRouteExempt(t *testing.T) {
	localTools := map[string]bool{}
	for _, def := range Registry {
		if def.Execute != nil {
			if def.Route != "" {
				t.Errorf("%s: 本地执行工具不应有 Route", def.Name)
			}
			localTools[def.Name] = true
		}
	}
	if len(localTools) == 0 {
		t.Fatal("应至少有一个本地执行工具（dpanel_image_pull）")
	}
}

// TestImagePullTagValidation tag 格式校验：reference.ParseNormalizedNamed 真实校验。
func TestImagePullTagValidation(t *testing.T) {
	cases := []struct {
		tag  string
		want bool
	}{
		{"ghcr.io/yetone/magpie:latest", true},
		{"busybox:latest", true},
		{"nginx", true}, // 无 tag 默认 latest，docker 语义允许
		{"registry.example.com:5000/app:v1", true},
		{"", false},
		{"   ", false},
		{":::", false},
		{"busybox:latest:extra", false}, // 多余段，旧校验放行、daemon 报无关错误
		{"foo bar", false},              // 空格，同上
	}
	for _, c := range cases {
		err := validatePullTag(c.tag)
		if (err == nil) != c.want {
			t.Errorf("validatePullTag(%q) 错误=%v，期望通过=%v", c.tag, err, c.want)
		}
	}
}

// makeRow 构造测试用 registryRow。
func makeRow(id int32, address string, proxy []string, username string) registryRow {
	row := registryRow{
		ID:            id,
		ServerAddress: address,
	}
	row.Setting = &struct {
		EnableHttp bool     `json:"enableHttp"`
		Proxy      []string `json:"proxy"`
		Username   string   `json:"username"`
		Password   string   `json:"password"`
	}{Proxy: proxy, Username: username}
	return row
}

// TestResolveRegistryConfig 表驱动覆盖：精确匹配、代理命中置首、去重、
// EnableHttp、nil Setting、无匹配、行序 first-match-wins。
func TestResolveRegistryConfig(t *testing.T) {
	t.Run("精确匹配取该行配置", func(t *testing.T) {
		rows := []registryRow{
			makeRow(1, "ghcr.1ms.run", []string{"ghcr.example.com"}, ""),
			makeRow(2, "ghcr.io", []string{"ghcr.nju.edu.cn"}, "u"),
		}
		got := resolveRegistryConfig("ghcr.io", rows)
		if got.Address != "ghcr.io" {
			t.Errorf("Address=%q，期望 ghcr.io", got.Address)
		}
		// 代理列表：源地址自身剔除，仅剩 nju
		if len(got.ProxyAddress) != 1 || got.ProxyAddress[0] != "ghcr.nju.edu.cn" {
			t.Errorf("ProxyAddress=%v，期望 [ghcr.nju.edu.cn]", got.ProxyAddress)
		}
	})
	t.Run("代理命中置首且原地址优先", func(t *testing.T) {
		rows := []registryRow{
			makeRow(1, "ghcr.io", []string{"ghcr.1ms.run", "ghcr.nju.edu.cn"}, ""),
		}
		got := resolveRegistryConfig("ghcr.nju.edu.cn", rows)
		if got.Address != "ghcr.io" {
			t.Errorf("Address=%q，期望回源 ghcr.io", got.Address)
		}
		if len(got.ProxyAddress) != 2 || got.ProxyAddress[0] != "ghcr.nju.edu.cn" {
			t.Errorf("ProxyAddress=%v，期望首项为镜像原地址（firstProxyAddress 置首）", got.ProxyAddress)
		}
	})
	t.Run("无匹配保留原地址", func(t *testing.T) {
		rows := []registryRow{makeRow(1, "docker.io", nil, "")}
		got := resolveRegistryConfig("ghcr.io", rows)
		if got.Address != "ghcr.io" {
			t.Errorf("Address=%q，期望原地址透传", got.Address)
		}
		if len(got.ProxyAddress) != 0 {
			t.Errorf("ProxyAddress 应为空，实际 %v", got.ProxyAddress)
		}
	})
	t.Run("nil Setting 跳过", func(t *testing.T) {
		rows := []registryRow{{ID: 1, ServerAddress: "ghcr.io"}}
		got := resolveRegistryConfig("ghcr.io", rows)
		if got.Address != "ghcr.io" || len(got.ProxyAddress) != 0 {
			t.Errorf("nil Setting 行应被跳过，got %+v", got)
		}
	})
	t.Run("同地址多行按 ID 升序 first-match-wins", func(t *testing.T) {
		rows := []registryRow{
			makeRow(5, "ghcr.io", []string{"proxy-new"}, "u2"),
			makeRow(2, "ghcr.io", []string{"proxy-old"}, "u1"),
		}
		// loadRegistryRows 已按 ID 升序排序，此处模拟升序输入
		sorted := []registryRow{rows[1], rows[0]}
		got := resolveRegistryConfig("ghcr.io", sorted)
		if len(got.ProxyAddress) != 1 || got.ProxyAddress[0] != "proxy-old" {
			t.Errorf("ID=2 行应先命中，ProxyAddress=%v", got.ProxyAddress)
		}
	})
	t.Run("EnableHttp 透传", func(t *testing.T) {
		row := makeRow(1, "registry.local", nil, "")
		row.Setting.EnableHttp = true
		got := resolveRegistryConfig("registry.local", []registryRow{row})
		if !got.EnableHttp {
			t.Error("EnableHttp 应透传 true")
		}
	})
}

// TestConsumePullStream 消费进度流的错误判定。
func TestConsumePullStream(t *testing.T) {
	ctx := context.Background()
	t.Run("errorDetail 返回错误", func(t *testing.T) {
		stream := io.NopCloser(strings.NewReader("{\"errorDetail\":{\"message\":\"manifest unknown\"}}\n"))
		err := consumePullStream(ctx, stream)
		if err == nil || err.Error() != "manifest unknown" {
			t.Errorf("期望 manifest unknown 错误，实际 %v", err)
		}
	})
	t.Run("正常流 EOF 返回 nil", func(t *testing.T) {
		stream := io.NopCloser(strings.NewReader("{\"status\":\"Pull complete\"}\n{\"status\":\"Pull complete\"}\n"))
		if err := consumePullStream(ctx, stream); err != nil {
			t.Errorf("正常流应返回 nil，实际 %v", err)
		}
	})
	t.Run("nil reader 报空流", func(t *testing.T) {
		err := consumePullStream(ctx, nil)
		if err == nil || !strings.Contains(err.Error(), "empty") {
			t.Errorf("nil reader 应报空流，实际 %v", err)
		}
	})
	t.Run("ctx 取消传播", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()
		stream := io.NopCloser(strings.NewReader("{\"status\":\"Pulling\"}\n"))
		err := consumePullStream(cancelCtx, stream)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("已取消 ctx 应返回 Canceled，实际 %v", err)
		}
	})
}

// fakePoster 注入用 apiPoster 桩。
type fakePoster struct {
	responses map[string]any
	calls     []string
}

func (f *fakePoster) Post(ctx context.Context, path string, payload any) (any, error) {
	f.calls = append(f.calls, path)
	if data, ok := f.responses[path]; ok {
		return data, nil
	}
	return nil, errors.New("not mocked")
}

// TestLoadRegistryRowsInjection api client 注入 + ID 升序排序。
func TestLoadRegistryRowsInjection(t *testing.T) {
	orig := dispatcherAPIClient
	defer func() { dispatcherAPIClient = orig }()

	t.Run("未注入时报错", func(t *testing.T) {
		dispatcherAPIClient = nil
		_, err := loadRegistryRows(context.Background())
		if err == nil || !strings.Contains(err.Error(), "未注入") {
			t.Errorf("未注入应报错，实际 %v", err)
		}
	})
	t.Run("注入后按 ID 升序返回", func(t *testing.T) {
		poster := &fakePoster{responses: map[string]any{
			"/common/registry/get-list": map[string]any{
				"list": []any{
					map[string]any{"id": float64(9), "serverAddress": "b.io"},
					map[string]any{"id": float64(2), "serverAddress": "a.io"},
				},
			},
		}}
		dispatcherAPIClient = poster
		rows, err := loadRegistryRows(context.Background())
		if err != nil {
			t.Fatalf("注入后不应报错: %v", err)
		}
		if len(rows) != 2 || rows[0].ID != 2 || rows[1].ID != 9 {
			t.Errorf("应按 ID 升序，实际 [%d, %d]", rows[0].ID, rows[1].ID)
		}
	})
}

// TestFillRegistryAuth 凭据填充：get-detail 明文 → EncodeAuthConfig。
func TestFillRegistryAuth(t *testing.T) {
	orig := dispatcherAPIClient
	defer func() { dispatcherAPIClient = orig }()

	row := makeRow(7, "registry.example.com", nil, "user1")
	rows := []registryRow{row}

	t.Run("nil config 短路", func(t *testing.T) {
		dispatcherAPIClient = nil
		if err := fillRegistryAuth(context.Background(), nil, rows); err != nil {
			t.Errorf("nil config 应短路返回 nil，实际 %v", err)
		}
	})
	t.Run("无凭据行静默跳过", func(t *testing.T) {
		poster := &fakePoster{responses: map[string]any{}}
		dispatcherAPIClient = poster
		cfg := resolveRegistryConfig("registry.example.com", rows)
		// username 为空 → 不应调 get-detail
		emptyRow := makeRow(8, "registry.example.com", nil, "")
		if err := fillRegistryAuth(context.Background(), cfg, []registryRow{emptyRow}); err != nil {
			t.Errorf("无凭据应静默跳过，实际 %v", err)
		}
		if len(poster.calls) != 0 {
			t.Errorf("无凭据不应调 get-detail，实际调了 %v", poster.calls)
		}
	})
	t.Run("有凭据行经 get-detail 填充 auth", func(t *testing.T) {
		poster := &fakePoster{responses: map[string]any{
			"/common/registry/get-detail": map[string]any{
				"info": map[string]any{
					"id":            float64(7),
					"serverAddress": "registry.example.com",
					"setting": map[string]any{
						"username": "user1",
						"password": "plain-pass",
					},
				},
			},
		}}
		dispatcherAPIClient = poster
		cfg := resolveRegistryConfig("registry.example.com", rows)
		if err := fillRegistryAuth(context.Background(), cfg, rows); err != nil {
			t.Fatalf("fillRegistryAuth 失败: %v", err)
		}
		if cfg.Auth == "" {
			t.Fatal("Auth 应被填充为 base64 串")
		}
		// 解码验证内容（EncodeAuthConfig 是 base64url(JSON)）
		decoded, err := registry.DecodeAuthConfig(cfg.Auth)
		if err != nil {
			t.Fatalf("auth 解码失败: %v", err)
		}
		if decoded.Username != "user1" || decoded.Password != "plain-pass" {
			t.Errorf("auth 内容应为 user1/plain-pass，实际 %s/%s", decoded.Username, decoded.Password)
		}
	})
}

// TestTagSyncPreCheck pull 分支本地拒绝并指向替代工具。
func TestTagSyncPreCheck(t *testing.T) {
	var def *ToolDef
	for i := range Registry {
		if Registry[i].Name == "dpanel_image_tag_sync" {
			def = &Registry[i]
			break
		}
	}
	if def == nil {
		t.Fatal("注册表缺少 dpanel_image_tag_sync")
	}
	if def.PreCheck == nil {
		t.Fatal("dpanel_image_tag_sync 应有 PreCheck（pull 分支本地拒绝）")
	}
	// pull（显式与默认）都应被拒
	for _, args := range []map[string]any{
		{"sync_type": "pull"},
		{}, // 缺省走 Default=pull
	} {
		if err := def.PreCheck(args); err == nil {
			t.Errorf("pull 分支应被本地拒绝，args=%v", args)
		} else if !strings.Contains(err.Error(), "dpanel_image_pull") {
			t.Errorf("拒绝信息应指向 dpanel_image_pull，实际 %v", err)
		}
	}
	// push 不受影响
	if err := def.PreCheck(map[string]any{"sync_type": "push"}); err != nil {
		t.Errorf("push 分支不应被拒，实际 %v", err)
	}
}
