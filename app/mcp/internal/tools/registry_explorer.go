package tools

// explorer 模块工具（对齐 DPanel v1.11.1）。
func init() {
	registerSlice([]ToolDef{
		{
			Name:        "dpanel_explorer_list",
			Description: "文件列表。mount_point ∈ volume:<卷名> | container:<容器ID> | docker:<环境名>（docker: 仅限开启 SSH 的远程环境；本地 sock 环境用 volume:/container:）",
			Route:       "/common/explorer/get-path-list",
			Params: []ParamDef{
			{Name: "mount_point", Type: "string", Description: "", Required: true, PayloadKey: "mountPoint", ValidateMountPoint: true},
			{Name: "path", Type: "string", Description: "", Required: false, Default: "/"},
		},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_explorer_content",
			Description: "读取文件内容",
			Route:       "/common/explorer/get-content",
			Params: []ParamDef{
			{Name: "mount_point", Type: "string", Description: "", Required: true, PayloadKey: "mountPoint", ValidateMountPoint: true},
			{Name: "file", Type: "string", Description: "", Required: true},
		},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_explorer_stat",
			Description: "文件 stat 信息",
			Route:       "/common/explorer/get-file-stat",
			Params: []ParamDef{
			{Name: "mount_point", Type: "string", Description: "", Required: true, PayloadKey: "mountPoint", ValidateMountPoint: true},
			{Name: "path", Type: "string", Description: "", Required: true},
		},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_explorer_path_size",
			Description: "目录大小统计",
			Route:       "/common/explorer/get-path-size",
			Params: []ParamDef{
			{Name: "mount_point", Type: "string", Description: "", Required: true, PayloadKey: "mountPoint", ValidateMountPoint: true},
			{Name: "path", Type: "string", Description: "", Required: true},
		},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_explorer_export",
			Description: "导出文件：单文件返回原始字节流，多文件打包为 zip，均以文本透传（二进制内容经 UTF-8 文本化，无法还原为文件；超过 8MiB 截断）。适合查看文本类文件内容",
			Route:       "/common/explorer/export",
			Params: []ParamDef{
				{Name: "mount_point", Type: "string", Description: "", Required: true, PayloadKey: "mountPoint", ValidateMountPoint: true},
				{Name: "file_list", Type: "string", Description: "文件绝对路径数组（如 [\"/path/file\"]）", Required: true, PayloadKey: "fileList", Normalize: "str-list"},
			},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_explorer_import",
			Description: "导入文件。file_list: [{name, path}]。注意：path 是 DPanel 容器内 /dpanel/storage/temp/ 下的源文件（上传暂存目录），MCP 无上传通道，需该目录已有文件",
			Route:       "/common/explorer/import",
			Params: []ParamDef{
				{Name: "mount_point", Type: "string", Description: "", Required: true, PayloadKey: "mountPoint", ValidateMountPoint: true},
				{Name: "file_list", Type: "string", Description: "源文件列表 [{name, path}]；path 为 /dpanel/storage/temp/ 下的相对路径（上传暂存目录）", Required: true, PayloadKey: "fileList", Normalize: "json"},
				{Name: "dst_path", Type: "string", Description: "目标目录（绝对路径）", Required: true, PayloadKey: "dstPath"},
			},
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_explorer_unzip",
			Description: "解压压缩文件",
			Route:       "/common/explorer/unzip",
			Params: []ParamDef{
			{Name: "mount_point", Type: "string", Description: "", Required: true, PayloadKey: "mountPoint", ValidateMountPoint: true},
			{Name: "file", Type: "string", Description: "", Required: true, Normalize: "str-list"},
			{Name: "path", Type: "string", Description: "", Required: true},
		},
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_explorer_mkdir",
			Description: "新建目录",
			Route:       "/common/explorer/mkdir",
			Params: []ParamDef{
			{Name: "mount_point", Type: "string", Description: "", Required: true, PayloadKey: "mountPoint", ValidateMountPoint: true},
			{Name: "dst_path", Type: "string", Description: "", Required: true, PayloadKey: "dstPath"},
		},
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_explorer_permission",
			Description: "修改文件权限/所有者。mod 如 644；owner 为用户名",
			Route:       "/common/explorer/permission",
			Params: []ParamDef{
			{Name: "mount_point", Type: "string", Description: "", Required: true, ValidateMountPoint: true},
			{Name: "file_list", Type: "string", Description: "", Required: true, Normalize: "str-list", PayloadKey: "fileList"},
			{Name: "mod", Type: "string", Description: "", Required: false, Default: ""},
			{Name: "owner", Type: "string", Description: "", Required: false, Default: ""},
		},
			Minimum:     "read-write",
			Destructive: false,
		},
		{
			Name:        "dpanel_explorer_delete",
			Description: "删除文件（破坏性，需 confirm=true）",
			Route:       "/common/explorer/delete",
			Params: []ParamDef{
			{Name: "mount_point", Type: "string", Description: "", Required: true, PayloadKey: "mountPoint", ValidateMountPoint: true},
			{Name: "file_list", Type: "string", Description: "", Required: true, PayloadKey: "fileList", Normalize: "str-list"},
			{Name: "confirm", Type: "boolean", Description: "", Required: false},
		},
			Minimum:     "admin",
			Destructive: true,
		},
	})
}
