package tools

import (
	"strings"
	"testing"

	"github.com/donknap/dpanel/app/mcp/internal/client"
)

// TestTextPayloadContract B（explorer_export / compose_log download）：
// 上游对导出/下载类请求返回 attachment 文件流（Content-Type 非 JSON），
// client 必须以 TextPayload 透传而不是报「响应解析失败」，dispatcher 直接返回文本。
// text/* 内容过 redact.Text() 脱敏（日志里的 KEY=VALUE/JWT 被 mask）；
// 二进制（zip/octet-stream）原样透传。
func TestTextPayloadContract(t *testing.T) {
	tp := client.TextPayload{Content: "services:\n  a: 1\n", ContentType: "text/plain"}
	out := renderPayload(tp)
	if !strings.Contains(out, "services") {
		t.Errorf("TextPayload 应透传文本内容，实际 %q", out)
	}
	// text/* 脱敏：日志中的敏感 KEY=VALUE 与 JWT 被 mask
	sensitive := client.TextPayload{
		Content:     "DB_PASSWORD=supersecret123\nJWT=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0SEs\n",
		ContentType: "text/plain; charset=utf-8",
	}
	out = renderPayload(sensitive)
	if strings.Contains(out, "supersecret123") {
		t.Errorf("text/* 内容应脱敏 KEY=VALUE，实际 %q", out)
	}
	if strings.Contains(out, "eyJhbGciOi") {
		t.Errorf("text/* 内容应脱敏 JWT，实际 %q", out)
	}
	// 二进制原样透传（不脱敏——正则对二进制无意义）
	bin := client.TextPayload{Content: "\x50\x4b\x03\x04\xff", ContentType: "application/zip"}
	out = renderPayload(bin)
	if out != "\x50\x4b\x03\x04\xff" {
		t.Errorf("二进制应原样透传，实际 %q", out)
	}
	// JSON 数据仍走脱敏 + marshal 路径
	out2 := renderPayload(map[string]any{"token": "secret"})
	if !strings.Contains(out2, `"token"`) {
		t.Errorf("map 数据应走 JSON 序列化，实际 %q", out2)
	}
}

// TestContainerStatRouteFix container_stat 改用 get-stat-list（follow=false）：
// 上游 /app/container/get-stat-info 是 ws 管道路由（无 AuthorizationFd 时 500 "fd not found"，
// 有 fd 时数据也只走 ws 广播），HTTP 响应永远不含 stats 数据。
// get-stat-list(follow=false) 一次性返回全部容器统计，MCP 层按 id 过滤。
func TestContainerStatRouteFix(t *testing.T) {
	var def *ToolDef
	for i := range Registry {
		if Registry[i].Name == "dpanel_container_stat" {
			def = &Registry[i]
			break
		}
	}
	if def == nil {
		t.Fatal("dpanel_container_stat 不在注册表")
	}
	if def.Route != "/common/home/get-stat-list" {
		t.Errorf("container_stat 路由应为 /common/home/get-stat-list，实际 %s", def.Route)
	}
	if def.PostProcess == nil {
		t.Error("container_stat 应有 PostProcess（按容器 id 过滤）")
	}
	// 过滤逻辑：完整 ID / 前缀 / 容器名匹配（每次用全新数据，filter 会原地改写 docker 数组）。
	// 上游 stats 的 name 带 "/" 前缀（daemon reserveName 存储 "/"+name，stat.go 直接透传 v.Name），
	// 过滤必须双侧裁剪前缀，否则按容器名查询静默返回空。
	newData := func() map[string]any {
		return map[string]any{
			"dockerEnvName": "local",
			"docker": []any{
				map[string]any{"container": "eb7e722760b7e89ccc0658d48201a638", "name": "/dpanel", "cpu": 1.2},
				map[string]any{"container": "bde20717e3bc1705", "name": "/sillytavern", "cpu": 0.1},
			},
		}
	}
	assertDocker := func(filtered any, want int, label string) {
		t.Helper()
		obj, ok := filtered.(map[string]any)
		if !ok {
			t.Fatalf("%s: 过滤结果应为 map，实际 %T", label, filtered)
		}
		if d, ok := obj["docker"].([]any); !ok || len(d) != want {
			t.Errorf("%s: 应命中 %d 个，实际 %v", label, want, obj["docker"])
		}
	}
	assertDocker(filterContainerStat(newData(), map[string]any{"id": "eb7e"}), 1, "ID 前缀过滤")
	assertDocker(filterContainerStat(newData(), map[string]any{"id": "sillytavern"}), 1, "容器名过滤（无斜杠）")
	assertDocker(filterContainerStat(newData(), map[string]any{"id": "/sillytavern"}), 1, "容器名过滤（带斜杠）")
	assertDocker(filterContainerStat(newData(), map[string]any{"id": "nonexist"}), 0, "无匹配")
	// 边界：id 缺失 / data 非 map / docker 缺失时原样返回不 panic
	if got := filterContainerStat(newData(), map[string]any{}); got == nil {
		t.Error("id 缺失应原样返回")
	}
	if got := filterContainerStat("not-a-map", map[string]any{"id": "x"}); got != "not-a-map" {
		t.Error("data 非 map 应原样返回")
	}
	noDocker := map[string]any{"dockerEnvName": "local"}
	if got := filterContainerStat(noDocker, map[string]any{"id": "x"}); !sameMap(got, noDocker) {
		t.Error("docker 字段缺失应原样返回")
	}
}

// sameMap 比较两个 map（any 包装后无法直接 ==）。
func sameMap(a, b any) bool {
	ma, ok1 := a.(map[string]any)
	mb, ok2 := b.(map[string]any)
	if !ok1 || !ok2 {
		return false
	}
	if len(ma) != len(mb) {
		return false
	}
	for k, v := range ma {
		if mb[k] != v {
			return false
		}
	}
	return true
}

// TestComposeLogDownloadDefault compose_log 默认 download=true：
// 上游 download=false 走 ws 管道（fd not found）；download=true 返回 text/plain 全量日志。
// 显式传 false 会被 PreCheck 本地拒绝（fail-fast 而非撞上游 500）。
func TestComposeLogDownloadDefault(t *testing.T) {
	var def *ToolDef
	for i := range Registry {
		if Registry[i].Name == "dpanel_compose_log" {
			def = &Registry[i]
			break
		}
	}
	if def == nil {
		t.Fatal("dpanel_compose_log 不在注册表")
	}
	found := false
	for _, p := range def.Params {
		if p.Name == "download" {
			found = true
			if p.Default != true {
				t.Errorf("compose_log.download 默认应为 true（走 text/plain 全量返回），实际 %v", p.Default)
			}
		}
	}
	if !found {
		t.Error("compose_log 缺少 download 参数")
	}
	if def.PreCheck == nil {
		t.Error("compose_log 应有 PreCheck（download=false 本地拒绝）")
	} else {
		if err := def.PreCheck(map[string]any{"download": false}); err == nil {
			t.Error("PreCheck 应拒绝 download=false")
		}
		if err := def.PreCheck(map[string]any{"download": true}); err != nil {
			t.Errorf("PreCheck 不应拒绝 download=true: %v", err)
		}
		if err := def.PreCheck(map[string]any{}); err != nil {
			t.Errorf("PreCheck 不应拒绝缺省（默认 true）: %v", err)
		}
	}
}

// TestExplorerDescriptions explorer 导入/导出的参数说明补齐实测行为：
// import 源路径固定在 /dpanel/storage/temp/ 下；export 返回打包文本流。
func TestExplorerDescriptions(t *testing.T) {
	byName := map[string]ToolDef{}
	for _, def := range Registry {
		byName[def.Name] = def
	}
	imp := byName["dpanel_explorer_import"]
	if imp.Params == nil {
		t.Fatal("explorer_import 缺参数")
	}
	for _, p := range imp.Params {
		if p.Name == "file_list" && !strings.Contains(p.Description, "storage/temp") {
			t.Errorf("explorer_import.file_list 说明应提示源路径在 /dpanel/storage/temp/ 下，实际 %q", p.Description)
		}
	}
	exp := byName["dpanel_explorer_export"]
	if !strings.Contains(exp.Description, "文本") && !strings.Contains(exp.Description, "打包") {
		t.Errorf("explorer_export 描述应说明返回打包文本流，实际 %q", exp.Description)
	}
}
