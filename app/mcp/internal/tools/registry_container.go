package tools

// container 模块工具（对齐 DPanel v1.11.1）。
func init() {
	registerSlice([]ToolDef{
		{
			Name:        "dpanel_container_list",
			Description: "容器列表（含状态、镜像、名称、网络）。可按 md5（容器ID）或站点名过滤。注意：API 不分页，返回全量",
			Route:       "/app/container/get-list",
			Params: []ParamDef{
			{Name: "md5", Type: "string", Description: "", Required: false},
			{Name: "site_title", Type: "string", Description: "", Required: false, PayloadKey: "siteTitle"},
		},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_container_detail",
			Description: "容器详情。md5 为容器 ID（完整或前缀）",
			Route:       "/app/container/get-detail",
			Params: []ParamDef{
			{Name: "md5", Type: "string", Description: "", Required: true},
		},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_container_stat",
			Description: "容器资源统计（CPU/内存/网络 IO）。id 为容器 ID（完整/前缀）或容器名",
			// 上游 /app/container/get-stat-info 是 ws 管道路由（HTTP 响应不含统计），
			// 改用 get-stat-list 一次性快照后按 id 过滤（见 filterContainerStat）。
			Route: "/common/home/get-stat-list",
			Params: []ParamDef{
				{Name: "id", Type: "string", Description: "容器 ID（完整/前缀）或容器名", Required: true},
			},
			Minimum:     "read-only",
			Destructive: false,
			PostProcess: filterContainerStat,
		},
		{
			Name:        "dpanel_container_process",
			Description: "容器内进程列表。id 为容器 ID",
			Route:       "/app/container/get-process-info",
			Params: []ParamDef{
			{Name: "id", Type: "string", Description: "", Required: true},
		},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_container_status",
			Description: "容器生命周期控制：operate ∈ start|stop|restart|pause|unpause",
			Route:       "/app/container/status",
			Params: []ParamDef{
			{Name: "md5", Type: "string", Description: "", Required: true},
			{Name: "operate", Type: "string", Description: "", Required: true, Enum: []string{"start", "stop", "restart", "pause", "unpause"}},
		},
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_container_update",
			Description: "更新容器配置（重启策略/环境变量/端口等，option 结构与面板一致）",
			Route:       "/app/container/update",
			Params: []ParamDef{
			{Name: "md5", Type: "string", Description: "", Required: true},
			{Name: "option", Type: "string", Description: "", Required: true, Normalize: "json"},
		},
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_container_copy",
			Description: "复制容器。copy_name 为新容器名",
			Route:       "/app/container/copy",
			Params: []ParamDef{
			{Name: "md5", Type: "string", Description: "", Required: true},
			{Name: "copy_name", Type: "string", Description: "", Required: true, PayloadKey: "copyName"},
		},
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_container_export",
			Description: "导出容器为镜像 tar",
			Route:       "/app/container/export",
			Params: []ParamDef{
			{Name: "md5", Type: "string", Description: "", Required: true},
		},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_container_commit",
			Description: "将容器提交为新镜像",
			Route:       "/app/container/commit",
			Params: []ParamDef{
			{Name: "md5", Type: "string", Description: "", Required: true},
			{Name: "option", Type: "string", Description: "", Required: true, Normalize: "json"},
		},
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_container_check_port",
			Description: "检查主机端口占用情况",
			Route:       "/app/container/check-port",
			Params: []ParamDef{
			{Name: "port", Type: "number", Description: "", Required: false, Normalize: "int"},
		},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_container_delete",
			Description: "删除容器（破坏性，需 confirm=true）。可选同时删除镜像/卷/链接",
			Route:       "/app/container/delete",
			Params: []ParamDef{
			{Name: "md5", Type: "string", Description: "", Required: true},
			{Name: "delete_image", Type: "boolean", Description: "", Required: false, PayloadKey: "deleteImage"},
			{Name: "delete_volume", Type: "boolean", Description: "", Required: false, PayloadKey: "deleteVolume"},
			{Name: "delete_link", Type: "boolean", Description: "", Required: false, PayloadKey: "deleteLink"},
			{Name: "confirm", Type: "boolean", Description: "", Required: false},
		},
			Minimum:     "admin",
			Destructive: true,
		},
		{
			Name:        "dpanel_container_prune",
			Description: "清理停止的容器（破坏性，需 confirm=true）",
			Route:       "/app/container/prune",
			Params: []ParamDef{
			{Name: "confirm", Type: "boolean", Description: "", Required: false},
		},
			Minimum:     "admin",
			Destructive: true,
		},
		{
			Name:        "dpanel_container_upgrade_list",
			Description: "容器升级检查列表（含可升级镜像 digest 对比）",
			Route:       "/app/container-upgrade/get-list",
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_container_upgrade_check",
			Description: "检查单个容器镜像更新。container_id 为容器 ID",
			Route:       "/app/container-upgrade/check",
			Params: []ParamDef{
			{Name: "container_id", Type: "string", Description: "", Required: true, PayloadKey: "containerId"},
		},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_container_upgrade",
			Description: "用本地已有镜像重建容器（保留配置）。不 pull 新镜像——需更新镜像时先调 dpanel_image_pull。image_tag 留空用原镜像",
			Route:       "/app/container-upgrade/upgrade",
			Params: []ParamDef{
			{Name: "md5", Type: "string", Description: "", Required: true},
			{Name: "image_tag", Type: "string", Description: "", Required: false, PayloadKey: "imageTag", Default: ""},
			{Name: "enable_bak", Type: "boolean", Description: "", Required: false, PayloadKey: "enableBak", Default: true},
		},
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_container_upgrade_ignore",
			Description: "忽略容器升级（加入忽略清单）",
			Route:       "/app/container-upgrade/ignore",
			Params: []ParamDef{
			{Name: "md5", Type: "string", Description: "", Required: true},
			{Name: "image_id", Type: "string", Description: "", Required: false, PayloadKey: "imageId"},
		},
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_container_backup_list",
			Description: "容器备份列表",
			Route:       "/app/container-backup/get-list",
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_container_backup_detail",
			Description: "备份详情。id 为备份记录 id",
			Route:       "/app/container-backup/get-detail",
			Params: []ParamDef{
			{Name: "id", Type: "number", Description: "", Required: true, Normalize: "int"},
		},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_container_backup_create",
			Description: "创建容器备份快照。id 为容器 ID",
			Route:       "/app/container-backup/create",
			Params: []ParamDef{
			{Name: "id", Type: "string", Description: "", Required: true},
		},
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_container_backup_delete",
			Description: "删除备份（破坏性，需 confirm=true）",
			Route:       "/app/container-backup/delete",
			Params: []ParamDef{
			{Name: "id", Type: "string", Description: "", Required: true, Normalize: "int-list"},
			{Name: "confirm", Type: "boolean", Description: "", Required: false},
		},
			Minimum:     "admin",
			Destructive: true,
		},
		{
			Name:        "dpanel_container_backup_restore",
			Description: "从备份恢复容器（破坏性，需 confirm=true）",
			Route:       "/app/container-backup/restore",
			Params: []ParamDef{
			{Name: "id", Type: "number", Description: "", Required: true, Normalize: "int"},
			{Name: "confirm", Type: "boolean", Description: "", Required: false},
		},
			Minimum:     "admin",
			Destructive: true,
		},
	})
}
