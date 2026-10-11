# dpanel_with_mcp

**DPanel with built-in MCP Server** —— 把 MCP（Model Context Protocol）直接内置进 DPanel 的单容器发行版。AI Agent 用自然语言管理 Docker：容器、compose、镜像、网络、卷、文件、计划任务等 99 个工具，无需第二容器。

基于 [donknap/dpanel](https://github.com/donknap/dpanel)（对齐 1.11.x），MCP 模块位于 `app/mcp/`（同仓子模块，照 `app/agent` 先例）。

## 与外挂方案的区别

| | dpanel_with_mcp（本仓） | 外挂 MCP 容器 |
|---|---|---|
| 容器 | **单容器**（DPanel 本体） | DPanel + MCP 两个 |
| 更新 | `git merge upstream` 一条命令 | 两套版本号要对齐 |
| 配置 | 面板管理面 API / settings 表 | 环境变量 |
| MCP 端点 | `http://<dpanel>:8807/dpanel/mcp` | `http://<mcp>:8090/mcp` |

## 快速开始

```bash
# 构建（须传 APP_FAMILY=ce，产物名与 COPY 匹配）
docker build -f docker/Dockerfile --build-arg APP_FAMILY=ce -t dpanel_with_mcp:latest .

docker run -d --name dpanel \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v dpanel-data:/dpanel \
  -p 8807:8080 \
  -e APP_NAME=dpanel \
  -e DP_JWT_SECRET=<固定随机串> \
  dpanel_with_mcp:latest
```

### 启用 MCP

MCP 默认关闭。启用方式二选一：

**方式 A：环境变量（compose 部署）**

```yaml
environment:
  DP_MCP_ENABLE: "true"
  DP_MCP_TOKEN: "<openssl rand -hex 32>"
  DP_MCP_PROFILE: read-write        # read-only | read-write | admin
  DP_MCP_SERVICE_USERNAME: admin    # 回环调用 DPanel API 的面板账号
  DP_MCP_SERVICE_PASSWORD: <密码>
```

**方式 B：管理面 API（面板 JWT）**

```bash
curl -s http://127.0.0.1:8807/dpanel/api/common/mcp/update \
  -H "Authorization: Bearer <面板JWT>" -H 'Content-Type: application/json' \
  -d '{"enable":true,"profile":"read-write"}'
# 返回后 token 自动生成，用 get-status 查看（重启容器生效）
```

### MCP 客户端接入

```
URL: http://<dpanel-host>:<port>/dpanel/mcp
Header: Authorization: Bearer <token>
```

## 安全模型

- **Bearer 门禁 fail-closed**：未配 token = 全部 401；无/错 token 一律 401
- **三级能力分级**：`read-only` / `read-write` / `admin`（`DP_MCP_PROFILE`）
- **破坏性操作 confirm**：delete/prune/restore/destroy 类必须显式 `confirm=true`
- **递归脱敏**：password/token/JWT 键名与文本全部掩码
- **内部回环**：MCP 工具经 `127.0.0.1` 调 DPanel 自身 API（service 凭据存 settings 表，不暴露）

## 跟随上游

```bash
git fetch upstream
git merge upstream/master
go test ./app/mcp/...   # 路由契约测试：上游改路由立刻红
```

## 验证

- 契约单测全过（注册表完整性/路由契约/参数契约/归一化/门禁/脱敏）
- 真实容器 E2E 20/20（`scripts/e2e_native.py`）：握手、99 工具、9 只读工具回环真实数据、归一化、fail-closed、脱敏、负例

## License

继承上游 DPanel 许可证（见 LICENSE）。
