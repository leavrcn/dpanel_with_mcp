package tools

import (
	"strings"
	"testing"
)

// TestRegistryCompleteness 注册表完整性：100 个工具、无重名、路由非空（本地执行工具除外）、分级合法。
func TestRegistryCompleteness(t *testing.T) {
	if len(Registry) != 100 {
		t.Fatalf("注册表应有 100 个工具，实际 %d", len(Registry))
	}
	seen := map[string]bool{}
	for _, def := range Registry {
		if seen[def.Name] {
			t.Errorf("工具重名: %s", def.Name)
		}
		seen[def.Name] = true
		// 分级校验对所有工具生效（含本地执行工具）
		switch def.Minimum {
		case "read-only", "read-write", "admin":
		default:
			t.Errorf("%s: 分级无效 %q", def.Name, def.Minimum)
		}
		// 本地执行工具（Execute != nil）无面板路由，Route 必须为空
		if def.Execute != nil {
			if def.Route != "" {
				t.Errorf("%s: 本地执行工具不应有路由 %q", def.Name, def.Route)
			}
			continue
		}
		if def.Route == "" || !strings.HasPrefix(def.Route, "/") {
			t.Errorf("%s: 路由无效 %q", def.Name, def.Route)
		}
	}
}

// TestDestructiveToolsRequireConfirm 破坏性工具清单（fail-closed 保护对象）。
func TestDestructiveToolsRequireConfirm(t *testing.T) {
	expected := map[string]bool{
		"dpanel_env_delete": true, "dpanel_container_delete": true, "dpanel_container_prune": true,
		"dpanel_container_backup_delete": true, "dpanel_container_backup_restore": true,
		"dpanel_compose_destroy": true, "dpanel_image_delete": true, "dpanel_image_prune": true,
		"dpanel_image_tag_delete": true, "dpanel_image_build_delete": true, "dpanel_image_build_prune": true,
		"dpanel_network_delete": true, "dpanel_network_prune": true,
		"dpanel_volume_delete": true, "dpanel_volume_prune": true,
		"dpanel_explorer_delete": true, "dpanel_cron_delete": true,
		"dpanel_cron_prune_log": true,
		"dpanel_store_delete": true, "dpanel_registry_delete": true, "dpanel_notice_delete": true,
	}
	for _, def := range Registry {
		if expected[def.Name] != def.Destructive {
			t.Errorf("%s: Destructive=%v，期望 %v", def.Name, def.Destructive, expected[def.Name])
		}
	}
}

// TestRouteContract 路由契约：注册表全部转发类路由必须命中 DPanel v1.11.1 live 实测验证集。
// 上游重命名/删除路由时此测试立刻红，防止「注册表悄悄失效」。
// 本地执行工具（Execute != nil，无 Route）豁免。
func TestRouteContract(t *testing.T) {
	// DPanel v1.11.1 全量 188 条路由（源码 provider.go 提取 + 真实实例探测验证）
	verified := map[string]bool{}
	for _, r := range verifiedRoutes {
		verified[r] = true
	}
	for _, def := range Registry {
		if def.Execute != nil {
			continue // 本地执行工具不走面板路由
		}
		if !verified[def.Route] {
			t.Errorf("%s: 路由 %s 不在 v1.11.1 验证集（上游已改名/删除？）", def.Name, def.Route)
		}
	}
}

// TestParamContract 参数契约：id 类参数必须带归一化钩子（对齐 DPanel Go int32/[]int32/[]string）。
func TestParamContract(t *testing.T) {
	intTools := map[string]string{ // tool -> param
		"dpanel_container_backup_detail": "id", "dpanel_container_backup_restore": "id",
		"dpanel_image_build_detail": "id", "dpanel_image_build_run": "id",
		"dpanel_cron_detail": "id", "dpanel_cron_log_list": "id",
		"dpanel_cron_run_once": "id", "dpanel_cron_prune_log": "id",
		"dpanel_store_deploy": "store_id", "dpanel_registry_detail": "id",
	}
	intListTools := map[string]string{
		"dpanel_container_backup_delete": "id", "dpanel_image_build_delete": "id",
		"dpanel_cron_delete": "id", "dpanel_store_delete": "id",
		"dpanel_registry_delete": "id", "dpanel_notice_delete": "id",
	}
	strListTools := map[string]string{
		"dpanel_env_delete": "env_name", "dpanel_network_container_list": "name",
		"dpanel_network_delete": "name", "dpanel_explorer_unzip": "file",
		"dpanel_image_tag_push_batch": "md5",
	}
	byName := map[string]ToolDef{}
	for _, def := range Registry {
		byName[def.Name] = def
	}
	check := func(set map[string]string, want string) {
		for tool, param := range set {
			def, ok := byName[tool]
			if !ok {
				t.Errorf("工具缺失: %s", tool)
				continue
			}
			found := false
			for _, p := range def.Params {
				if p.Name == param {
					found = true
					if p.Normalize != want {
						t.Errorf("%s.%s: Normalize=%q，期望 %q", tool, param, p.Normalize, want)
					}
				}
			}
			if !found {
				t.Errorf("%s: 缺少参数 %s", tool, param)
			}
		}
	}
	check(intTools, "int")
	check(intListTools, "int-list")
	check(strListTools, "str-list")
}
