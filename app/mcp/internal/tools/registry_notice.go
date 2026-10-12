package tools

// notice 模块工具（对齐 DPanel v1.11.1）。
func init() {
	registerSlice([]ToolDef{
		{
			Name:        "dpanel_notice_list",
			Description: "通知列表（操作事件流水）",
			Route:       "/common/notice/get-list",
			Params: []ParamDef{
			{Name: "page", Type: "number", Description: "", Required: false, Normalize: "int", Default: 1},
			{Name: "page_size", Type: "number", Description: "", Required: false, PayloadKey: "pageSize", Normalize: "int", Default: 20},
		},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_notice_unread",
			Description: "未读通知。action=new 查询未读；init 取最近 5 条；clear 清空全部已读状态",
			Route:       "/common/notice/unread",
			Params: []ParamDef{
				{Name: "action", Type: "string", Description: "new | init | clear", Required: false, Default: "new"},
			},
			Minimum:     "read-only",
			Destructive: false,
		},
		{
			Name:        "dpanel_notice_delete",
			Description: "删除通知（破坏性，需 confirm=true）",
			Route:       "/common/notice/delete",
			Params: []ParamDef{
			{Name: "id", Type: "string", Description: "", Required: true, Normalize: "int-list"},
			{Name: "confirm", Type: "boolean", Description: "", Required: false},
		},
			Minimum:     "admin",
			Destructive: true,
		},
	})
}
