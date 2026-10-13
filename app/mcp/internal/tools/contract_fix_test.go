package tools

import (
	"fmt"
	"testing"
)

// TestJSONNormalizeImplemented B1（阻断级）：Normalize:"json" 必须被实现——
// 字符串参数解析为 JSON 对象/数组透传，非法 JSON 报本地错误。
func TestJSONNormalizeImplemented(t *testing.T) {
	p := ParamDef{Name: "option", Type: "string", Normalize: "json"}
	// 合法 JSON 对象
	v, err := normalize(p, `{"name":"x","value":"y"}`)
	if err != nil {
		t.Fatalf("合法 JSON 对象应解析成功: %v", err)
	}
	m, ok := v.(map[string]any)
	if !ok || m["name"] != "x" {
		t.Errorf("json normalize 应返回 map，实际 %T: %v", v, v)
	}
	// 合法 JSON 数组
	v, err = normalize(p, `[{"name":"a"},{"name":"b"}]`)
	if err != nil {
		t.Fatalf("合法 JSON 数组应解析成功: %v", err)
	}
	if _, ok := v.([]any); !ok {
		t.Errorf("json normalize 应返回 slice，实际 %T", v)
	}
	// 非法 JSON
	if _, err := normalize(p, `not-json`); err == nil {
		t.Error("非法 JSON 应报错")
	}
}

// TestMergeSemantics C1：registry_create / image_build_create 的 option 应展开
// 合并进 payload 顶层（对齐 Python payload.update(option)）。
func TestMergeSemantics(t *testing.T) {
	for _, tool := range []string{"dpanel_registry_create", "dpanel_image_build_create"} {
		var def *ToolDef
		for i := range Registry {
			if Registry[i].Name == tool {
				def = &Registry[i]
				break
			}
		}
		if def == nil {
			t.Fatalf("%s 不在注册表", tool)
		}
		found := false
		for _, p := range def.Params {
			if p.Name == "option" {
				found = true
				if !p.Merge {
					t.Errorf("%s.option 应有 Merge=true（展开合并进 payload 顶层）", tool)
				}
			}
		}
		if !found {
			t.Errorf("%s 缺少 option 参数", tool)
		}
	}
}

// TestPayloadKeyContract C1/C2：手工组装 payload 的工具，字段名映射必须对齐 Python 版。
func TestPayloadKeyContract(t *testing.T) {
	expect := map[string]string{ // "tool.param" -> PayloadKey
		"dpanel_compose_create.compose_type":       "type",
		"dpanel_container_list.site_title":         "siteTitle",
		"dpanel_container_upgrade_ignore.image_id": "imageId",
		"dpanel_explorer_permission.file_list":     "fileList",
		"dpanel_explorer_import.dst_path":          "dstPath",
		"dpanel_compose_log.line_total":            "lineTotal",
		"dpanel_store_create.store_type":           "type",
		"dpanel_store_deploy.store_id":             "storeId",
		"dpanel_cron_create.trigger_type":          "triggerType",
		"dpanel_compose_deploy.deploy_service_name": "deployServiceName",
	}
	byName := map[string]ToolDef{}
	for _, def := range Registry {
		byName[def.Name] = def
	}
	for key, want := range expect {
		tool, param := splitKey(key)
		def, ok := byName[tool]
		if !ok {
			t.Errorf("工具缺失: %s", tool)
			continue
		}
		found := false
		for _, p := range def.Params {
			if p.Name == param {
				found = true
				got := p.PayloadKey
				if got == "" {
					got = p.Name
				}
				if got != want {
					t.Errorf("%s: PayloadKey=%q，期望 %q", key, got, want)
				}
			}
		}
		if !found {
			t.Errorf("%s: 缺少参数 %s", tool, param)
		}
	}
}

// TestJSONTypeParams C2：dict/list 结构参数应为 json 类型（agent 传 JSON 字符串）。
func TestJSONTypeParams(t *testing.T) {
	expect := map[string]string{ // "tool.param" -> 期望 Normalize
		"dpanel_compose_create.environment":   "json",
		"dpanel_compose_deploy.environment":   "json",
		"dpanel_explorer_import.file_list":    "json",
		"dpanel_store_create.apps":            "json",
		"dpanel_container_update.option":      "json",
		"dpanel_container_commit.option":      "json",
		"dpanel_cron_create.environment":      "json",
		"dpanel_store_deploy.option":          "json",
		"dpanel_explorer_permission.file_list": "str-list",
		"dpanel_cron_create.spec":              "cron-expression",
	}
	byName := map[string]ToolDef{}
	for _, def := range Registry {
		byName[def.Name] = def
	}
	for key, want := range expect {
		tool, param := splitKey(key)
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
					t.Errorf("%s: Normalize=%q，期望 %q", key, p.Normalize, want)
				}
			}
		}
		if !found {
			t.Errorf("%s: 缺少参数 %s", tool, param)
		}
	}
}

// TestDefaults C5：Python 版有默认值的参数，Go 版必须声明 Default。
func TestDefaults(t *testing.T) {
	expect := map[string]string{ // "tool.param" -> 默认值字符串
		"dpanel_compose_log.line_total":       "50",
		"dpanel_compose_log.download":         "true",
		"dpanel_explorer_list.path":           "/",
		"dpanel_log_list.page":                "1",
		"dpanel_log_list.page_size":           "50",
		"dpanel_notice_list.page":             "1",
		"dpanel_notice_list.page_size":        "20",
		"dpanel_network_create.driver":        "bridge",
		"dpanel_volume_create.driver":         "local",
		"dpanel_image_tag_sync.sync_type":     "pull",
		"dpanel_container_upgrade.enable_bak": "true",
		"dpanel_compose_create.compose_type":  "text",
	}
	byName := map[string]ToolDef{}
	for _, def := range Registry {
		byName[def.Name] = def
	}
	for key, want := range expect {
		tool, param := splitKey(key)
		def, ok := byName[tool]
		if !ok {
			t.Errorf("工具缺失: %s", tool)
			continue
		}
		found := false
		for _, p := range def.Params {
			if p.Name == param {
				found = true
				if p.Default == nil {
					t.Errorf("%s: 缺少默认值（期望 %s）", key, want)
				} else if fmt.Sprintf("%v", p.Default) != want {
					t.Errorf("%s: 默认值 %v，期望 %s", key, p.Default, want)
				}
			}
		}
		if !found {
			t.Errorf("%s: 缺少参数 %s", tool, param)
		}
	}
}

// TestMountPointValidation C4：mount_point 必须校验前缀。
func TestMountPointValidation(t *testing.T) {
	cases := []struct {
		in    string
		isErr bool
	}{
		{"volume:dpanel-data", false},
		{"container:abc123", false},
		{"docker:local", false},
		{"garbage", true},
		{"", true},
	}
	for _, c := range cases {
		_, err := validateMountPoint(c.in)
		if c.isErr && err == nil {
			t.Errorf("validateMountPoint(%q) 应报错", c.in)
		}
		if !c.isErr && err != nil {
			t.Errorf("validateMountPoint(%q) 不应报错: %v", c.in, err)
		}
	}
}

// TestRequiredParamMissing C3：必填参数缺失应报本地错误（不是静默跳过）。
func TestRequiredParamMissing(t *testing.T) {
	def := ToolDef{
		Name:  "t_req",
		Route: "/x",
		Params: []ParamDef{
			{Name: "md5", Type: "string", Required: true},
			{Name: "opt", Type: "string"},
		},
	}
	// 缺必填
	_, err := buildPayload(def, map[string]any{})
	if err == nil {
		t.Error("Required 参数缺失应报错")
	}
	// 全给
	payload, err := buildPayload(def, map[string]any{"md5": "abc", "opt": "x"})
	if err != nil {
		t.Fatalf("参数齐全不应报错: %v", err)
	}
	if payload["md5"] != "abc" {
		t.Errorf("payload 不符: %v", payload)
	}
}

// TestBuildPayloadDefaults C5：默认值填充。
func TestBuildPayloadDefaults(t *testing.T) {
	def := ToolDef{
		Name:  "t_def",
		Route: "/x",
		Params: []ParamDef{
			{Name: "line_total", Type: "number", PayloadKey: "lineTotal", Normalize: "int", Default: float64(50)},
			{Name: "download", Type: "boolean", Default: false},
		},
	}
	payload, err := buildPayload(def, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := payload["lineTotal"].(int); v != 50 {
		t.Errorf("lineTotal 默认值应为 50，实际 %v", payload["lineTotal"])
	}
	if payload["download"] != false {
		t.Errorf("download 默认值应为 false，实际 %v", payload["download"])
	}
}

// TestBuildPayloadMerge C1：Merge 语义——option 展开进顶层。
func TestBuildPayloadMerge(t *testing.T) {
	def := ToolDef{
		Name:  "t_merge",
		Route: "/x",
		Params: []ParamDef{
			{Name: "title", Type: "string", Required: true},
			{Name: "server_address", Type: "string", PayloadKey: "serverAddress", Required: true},
			{Name: "option", Type: "string", Normalize: "json", Merge: true},
		},
	}
	payload, err := buildPayload(def, map[string]any{
		"title": "t", "server_address": "docker.io",
		"option": `{"username":"u","password":"p"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload["username"] != "u" || payload["password"] != "p" {
		t.Errorf("Merge 未展开: %v", payload)
	}
	if payload["title"] != "t" || payload["serverAddress"] != "docker.io" {
		t.Errorf("常规字段被破坏: %v", payload)
	}
	if _, exists := payload["option"]; exists {
		t.Errorf("option 不应作为独立字段存在: %v", payload)
	}
}

// TestBuildPayloadMountPoint C4：mountPoint 校验在组装层生效。
func TestBuildPayloadMountPoint(t *testing.T) {
	def := ToolDef{
		Name:  "t_mp",
		Route: "/x",
		Params: []ParamDef{
			{Name: "mount_point", Type: "string", PayloadKey: "mountPoint", Required: true, ValidateMountPoint: true},
		},
	}
	if _, err := buildPayload(def, map[string]any{"mount_point": "garbage"}); err == nil {
		t.Error("非法 mountPoint 应报错")
	}
	payload, err := buildPayload(def, map[string]any{"mount_point": "volume:data"})
	if err != nil {
		t.Fatal(err)
	}
	if payload["mountPoint"] != "volume:data" {
		t.Errorf("payload 不符: %v", payload)
	}
}

func splitKey(key string) (string, string) {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '.' {
			return key[:i], key[i+1:]
		}
	}
	return key, ""
}


// TestNoticeUnreadActionParam notice_unread 必须带 action 参数（面板 binding 要求
// action ∈ new|clear|init，缺参数会被 500 拒绝——v1.11.5 修复的回归保护）。
func TestNoticeUnreadActionParam(t *testing.T) {
	var def *ToolDef
	for i := range Registry {
		if Registry[i].Name == "dpanel_notice_unread" {
			def = &Registry[i]
			break
		}
	}
	if def == nil {
		t.Fatalf("dpanel_notice_unread 不在注册表")
	}
	found := false
	for _, p := range def.Params {
		if p.Name == "action" {
			found = true
			if p.Default != "new" {
				t.Errorf("action 默认值应为 new，实际 %v", p.Default)
			}
		}
	}
	if !found {
		t.Errorf("dpanel_notice_unread 应有 action 参数（面板 /common/notice/unread 的 binding 要求 action 必填）")
	}
	// 空参数调用应能组装出 action=new 的 payload（不再触发面板 500）
	payload, err := buildPayload(*def, map[string]any{})
	if err != nil {
		t.Fatalf("空参数应走默认值: %v", err)
	}
	if payload["action"] != "new" {
		t.Errorf("payload.action 应为 new，实际 %v", payload["action"])
	}
}
