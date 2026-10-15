package tools

// image 模块工具（对齐 DPanel v1.11.1）。
import (
	"errors"
)

func init() {
	registerSlice([]ToolDef{
		{
			Name:        "dpanel_image_list",
			Description: "镜像列表",
			Route:       "/app/image/get-list",
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_image_detail",
			Description: "镜像详情。md5 为镜像 sha256 digest（如 sha256:abc123...）",
			Route:       "/app/image/get-detail",
			Params: []ParamDef{
			{Name: "md5", Type: "string", Description: "", Required: true},
		},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_image_tag_search",
			Description: "在 Docker Hub 搜索镜像",
			Route:       "/app/image/tag-search",
			Params: []ParamDef{
			{Name: "keyword", Type: "string", Description: "", Required: true},
		},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_image_tag_add",
			Description: "为镜像添加 tag。md5 为镜像 digest，tag 形如 repo:tag",
			Route:       "/app/image/tag-add",
			Params: []ParamDef{
			{Name: "md5", Type: "string", Description: "", Required: true},
			{Name: "tag", Type: "string", Description: "", Required: true},
		},
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_image_tag_sync",
			Description: "推送镜像 tag 到远程仓库（push）。注意：pull 已不可用（面板 pull 依赖 WebSocket 会话，MCP 调用必报 fd not found）——拉取镜像请用 dpanel_image_pull",
			Route:       "/app/image/tag-sync",
			Params: []ParamDef{
				{Name: "tag", Type: "string", Description: "", Required: true},
				{Name: "sync_type", Type: "string", Description: "", Required: false, PayloadKey: "type", Enum: []string{"pull", "push"}, Default: "pull"},
			},
			PreCheck: func(args map[string]any) error {
				// pull 分支走 ws progress fd（MCP 无 ws 会话必 500），本地拒绝并指向替代工具
				t, _ := args["sync_type"].(string)
				if t == "" {
					t = "pull" // Default 为 pull
				}
				if t == "pull" {
					return errors.New("pull 已不可用：面板 pull 依赖 WebSocket 会话，MCP 调用必报 fd not found。请改用 dpanel_image_pull")
				}
				return nil
			},
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_image_tag_push_batch",
			Description: "批量推送镜像 tag 到远程仓库",
			Route:       "/app/image/tag-push-batch",
			Params: []ParamDef{
			{Name: "md5", Type: "string", Description: "", Required: true, Normalize: "str-list"},
			{Name: "registry_server_address", Type: "string", Description: "", Required: true, PayloadKey: "registryServerAddress", Normalize: "str-list"},
		},
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_image_import_by_image_tar",
			Description: "导入镜像 tar（从 DPanel 存储目录）",
			Route:       "/app/image/import-by-image-tar",
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_image_tag_delete",
			Description: "删除镜像 tag（破坏性，需 confirm=true）",
			Route:       "/app/image/tag-delete",
			Params: []ParamDef{
			{Name: "tag", Type: "string", Description: "", Required: true},
			{Name: "confirm", Type: "boolean", Description: "", Required: false},
		},
			Minimum:     "admin",
			Destructive: true,
		},
		{
			Name:        "dpanel_image_delete",
			Description: "删除镜像（破坏性，需 confirm=true）。md5 为镜像 sha256 digest 数组",
			Route:       "/app/image/delete",
			Params: []ParamDef{
			{Name: "md5", Type: "string", Description: "", Required: true, Normalize: "str-list"},
			{Name: "confirm", Type: "boolean", Description: "", Required: false},
		},
			Minimum:     "admin",
			Destructive: true,
		},
		{
			Name:        "dpanel_image_prune",
			Description: "清理悬空镜像（破坏性，需 confirm=true）",
			Route:       "/app/image/prune",
			Params: []ParamDef{
			{Name: "confirm", Type: "boolean", Description: "", Required: false},
		},
			Minimum:     "admin",
			Destructive: true,
		},
		{
			Name:        "dpanel_image_build_list",
			Description: "镜像构建任务列表",
			Route:       "/app/image-build/get-list",
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_image_build_detail",
			Description: "镜像构建任务详情",
			Route:       "/app/image-build/get-detail",
			Params: []ParamDef{
			{Name: "id", Type: "number", Description: "", Required: true, Normalize: "int"},
		},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_image_build_create",
			Description: "创建镜像构建任务（从 Dockerfile）。tags 为目标镜像 tag",
			Route:       "/app/image-build/create",
			Params: []ParamDef{
			{Name: "tags", Type: "string", Description: "", Required: true},
			{Name: "option", Type: "string", Description: "", Required: false, Normalize: "json", Merge: true},
		},
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_image_build_run",
			Description: "执行镜像构建任务",
			Route:       "/app/image-build/build",
			Params: []ParamDef{
			{Name: "id", Type: "number", Description: "", Required: true, Normalize: "int"},
		},
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_image_build_delete",
			Description: "删除镜像构建任务（破坏性，需 confirm=true）",
			Route:       "/app/image-build/delete",
			Params: []ParamDef{
			{Name: "id", Type: "string", Description: "", Required: true, Normalize: "int-list"},
			{Name: "confirm", Type: "boolean", Description: "", Required: false},
		},
			Minimum:     "admin",
			Destructive: true,
		},
		{
			Name:        "dpanel_image_build_prune",
			Description: "清理构建缓存（破坏性，需 confirm=true）",
			Route:       "/app/image-build/prune",
			Params: []ParamDef{
			{Name: "confirm", Type: "boolean", Description: "", Required: false},
		},
			Minimum:     "admin",
			Destructive: true,
		},
	})
}
