package tools

// compose 模块工具（对齐 DPanel v1.11.1）。
func init() {
	registerSlice([]ToolDef{
		{
			Name:        "dpanel_compose_list",
			Description: "Compose 项目列表（含运行容器概览）。注意：dangling 项目 id=0，仅可查看",
			Route:       "/app/compose/get-list",
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_compose_task",
			Description: "Compose 项目详情与任务状态。id 为项目 id（数字字符串，列表返回）",
			Route:       "/app/compose/get-task",
			Params: []ParamDef{
			{Name: "id", Type: "string", Description: "", Required: true},
		},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_compose_log",
			Description: "Compose 项目日志（全量文本）。line_total ∈ 50|100|200|500|1000|5000|-1。默认 download=true 返回 text/plain 全量日志；download=false 走面板 WebSocket 推送（MCP 场景不可用）",
			Route:       "/app/compose/container-log",
			Params: []ParamDef{
				{Name: "id", Type: "string", Description: "", Required: true},
				{Name: "line_total", Type: "number", Description: "", Required: false, PayloadKey: "lineTotal", Normalize: "int", Default: 50},
				// 上游 download=false 走 ws 管道（无 WebSocket 连接时 500 "fd not found"），
				// download=true 直接返回 text/plain 全量日志 —— MCP 默认走后者。
				{Name: "download", Type: "boolean", Description: "默认 true：返回全量文本", Required: false, Default: true},
			},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_compose_get_from_uri",
			Description: "从 URL 拉取 compose 文件内容",
			Route:       "/app/compose/get-from-uri",
			Params: []ParamDef{
			{Name: "uri", Type: "string", Description: "", Required: true},
		},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_compose_get_from_git",
			Description: "从 Git 仓库拉取 compose 文件",
			Route:       "/app/compose/get-from-git",
			Params: []ParamDef{
			{Name: "uri", Type: "string", Description: "", Required: true},
			{Name: "name", Type: "string", Description: "", Required: true},
		},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_compose_create",
			Description: "创建/更新 compose 项目。compose_type ∈ text|remoteUrl|outPath（storagePath/store 不可手动建）",
			Route:       "/app/compose/create",
			Params: []ParamDef{
			{Name: "name", Type: "string", Description: "", Required: true},
			{Name: "yaml", Type: "string", Description: "", Required: true},
			{Name: "compose_type", Type: "string", Description: "", Required: false, Enum: []string{"text", "remote"}, PayloadKey: "type", Default: "text"},
			{Name: "title", Type: "string", Description: "", Required: false, Default: ""},
			{Name: "id", Type: "string", Description: "", Required: false, Default: ""},
			{Name: "environment", Type: "string", Description: "", Required: false, Normalize: "json"},
		},
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_compose_deploy",
			Description: "部署/更新 compose 项目。id 为项目 id。deploy_service_name 可指定只部署部分服务",
			Route:       "/app/compose/container-deploy",
			Params: []ParamDef{
			{Name: "id", Type: "string", Description: "", Required: true},
			{Name: "environment", Type: "string", Description: "", Required: false, Normalize: "json"},
			{Name: "deploy_service_name", Type: "string", Description: "", Required: false, PayloadKey: "deployServiceName", Normalize: "str-list"},
			{Name: "remove_orphans", Type: "boolean", Description: "", Required: false, PayloadKey: "removeOrphans", Default: false},
		},
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_compose_container_ctrl",
			Description: "Compose 项目控制：op ∈ start|stop|restart|pause|unpause|ls",
			Route:       "/app/compose/container-ctrl",
			Params: []ParamDef{
			{Name: "id", Type: "string", Description: "", Required: true},
			{Name: "op", Type: "string", Description: "", Required: true, Enum: []string{"start", "stop", "restart", "pause", "unpause", "ls"}},
		},
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_compose_destroy",
			Description: "销毁 compose 项目（破坏性，需 confirm=true）。delete_data=true 同时删除项目记录；",
			Route:       "/app/compose/container-destroy",
			Params: []ParamDef{
			{Name: "id", Type: "string", Description: "", Required: true},
			{Name: "delete_image", Type: "boolean", Description: "", Required: false, PayloadKey: "deleteImage", Default: false},
			{Name: "delete_volume", Type: "boolean", Description: "", Required: false, PayloadKey: "deleteVolume", Default: false},
			{Name: "delete_data", Type: "boolean", Description: "", Required: false, PayloadKey: "deleteData", Default: false},
			{Name: "delete_path", Type: "boolean", Description: "", Required: false, PayloadKey: "deletePath", Default: false},
			{Name: "confirm", Type: "boolean", Description: "", Required: false},
		},
			Minimum:     "admin",
			Destructive: true,
		},
	})
}
