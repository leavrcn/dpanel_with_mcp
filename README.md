# DPanel with MCP

**DPanel with built-in MCP Server** —— 把 MCP（Model Context Protocol）直接内置进 DPanel 的单容器发行版。AI Agent 用自然语言管理 Docker：容器、compose、镜像、网络、卷、文件、计划任务等 100 个工具。

基于 [donknap/dpanel](https://github.com/donknap/dpanel)（对齐 1.11.x），MCP 模块位于 `app/mcp/`（同仓子模块）。

**版本**: v1.11.1-mcp7（基线 DPanel v1.11.1，MCP 第 7 版）

---

## 🚀 快速开始

### 方式 A：Docker Compose（推荐，镜像来自 GHCR）

```bash
# 1. 克隆仓库
git clone https://github.com/leavrcn/dpanel_with_mcp.git
cd dpanel_with_mcp

# 2. 配置环境变量（token 用 openssl rand -hex 32 生成）
cp .env.example .env
vi .env   # 填 DP_MCP_TOKEN / DP_MCP_SERVICE_USERNAME / DP_MCP_SERVICE_PASSWORD

# 3. 启动（镜像自动从 ghcr.io 拉取）
docker compose up -d

# 4. 验证
curl -H "Authorization: Bearer ***" http://localhost:8807/dpanel/mcp
```

**预期输出**（JSON）：
```json
{
  "jsonrpc": "2.0",
  "result": {
    "protocolVersion": "2025-03-26",
    "capabilities": {
      "tools": { "list_changed": true }
    },
    "serverInfo": {
      "name": "dpanel-mcp",
      "version": "1.11.1-mcp7"
    }
  }
}
```

### 方式 B：手动构建

```bash
# 构建镜像（CE lite）
docker build --target lite \
  -t dpanel-with-mcp:local \
  --build-arg APP_VERSION=1.11.1-mcp7 \
  --build-arg APP_FAMILY=ce \
  -f docker/Dockerfile .

# 运行容器（docker.sock 不加 :ro，面板需要管理容器）
docker run -d \
  --name dpanel-with-mcp \
  -p 8807:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v dpanel_data:/dpanel \
  -e DP_MCP_ENABLE=true \
  -e DP_MCP_TOKEN="YOUR_TOKEN_HERE" \
  -e DP_MCP_PROFILE=read-write \
  -e DP_MCP_SERVICE_USERNAME="YOUR_PANEL_USER" \
  -e DP_MCP_SERVICE_PASSWORD="YOUR_PANEL_PASSWORD" \
  dpanel-with-mcp:local
```

> 升级/回滚：改 compose 里的 `image:` tag（与仓库 Release 一一对应）后 `docker compose up -d`；数据卷独立，不受影响。

---

## 🎯 核心特性

- **100 个工具**：容器/镜像/网络/卷/compose/计划任务/文件管理
- **三级权限**：read-only(45) / read-write(79) / admin(100)
- **安全鉴权**：Bearer token + fail-closed + 25 个敏感键脱敏
- **单容器**：MCP server 内置于 DPanel 本体，`git merge upstream` 一条命令跟随上游

---

## ⚙️ 配置说明

### MCP 配置（环境变量）

| 变量 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `DP_MCP_ENABLE` | ✅ | - | 启用 MCP（`"true"`/`"false"`） |
| `DP_MCP_TOKEN` | ✅ | - | Bearer token（32 字节 hex） |
| `DP_MCP_PROFILE` | ❌ | `read-write` | 权限级别（见下） |
| `DP_MCP_SERVICE_USERNAME` | ⚠️ | - | DPanel 账号（回环调用登录用；env 或管理面 API 录入，二选一） |
| `DP_MCP_SERVICE_PASSWORD` | ⚠️ | - | DPanel 密码（同上；不配置则 MCP 工具调用全部失败） |

> **service 凭据两种配置方式**（v1.11.4-mcp 起）：
> 1. 环境变量（compose 部署，如上表）
> 2. 管理面 API（见下）
>
> 环境变量优先于 settings 表；改配置后需重启容器生效。

### 管理面 API 配置（免环境变量路径）

面板 Web UI 暂无 MCP 设置页；token / 凭据 / profile 可经管理面 API 配置
（走面板 JWT，与浏览器登录同一身份）。适合不想在 compose/.env 里放凭据的部署：

```bash
# 1. 登录拿 JWT（首次使用先在浏览器完成 founder 注册）
JWT=$(curl -s -X POST http://localhost:8807/dpanel/api/common/user/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"你的账号","password":"你的密码"}' \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["accessToken"])')

# 2. 启用 MCP 并录入 service 凭据（token 留空则自动生成 32 字节随机值）
curl -s -X POST http://localhost:8807/dpanel/api/common/mcp/update \
  -H "Authorization: Bearer $JWT" -H 'Content-Type: application/json' \
  -d '{"enable":true,"profile":"read-write","serviceUsername":"你的账号","servicePassword":"你的密码"}'

# 3.（可选）重新生成 token——明文只在这次响应里返回一次
curl -s -X POST http://localhost:8807/dpanel/api/common/mcp/regenerate-token \
  -H "Authorization: Bearer $JWT" -H 'Content-Type: application/json' -d '{}'

# 4. 查看状态（token 只显示掩码）
curl -s -X POST http://localhost:8807/dpanel/api/common/mcp/get-status \
  -H "Authorization: Bearer $JWT" -H 'Content-Type: application/json' -d '{}'

# 5. 重启容器生效
docker restart dpanel
```

> 走此路径时，compose 里的 `DP_MCP_SERVICE_USERNAME/PASSWORD` 两行应删除
> （环境变量优先于 API 写入的值，留着会一直覆盖）。

### MCP 权限分级

| Profile | 工具数 | 权限范围 | 适用场景 |
|---------|--------|----------|----------|
| `read-only` | 45 | 查询、列表、导出 | 纯监控/仪表盘 |
| `read-write` | 79 | + 启动/停止/创建/更新 | **日常运维（推荐）** |
| `admin` | 100 | + 删除/销毁/清理 | 完全自动化（高风险） |

---

## 🔌 接入 MCP 客户端

### Hermes Agent

编辑 `~/.hermes/config.yaml`（顶层 `mcp_servers`，`url` 直指 MCP 端点）：

```yaml
mcp_servers:
  dpanel:
    url: http://localhost:8807/dpanel/mcp
    headers:
      Authorization: "Bearer YOUR_TOKEN_HERE"
```

测试连接：

```bash
hermes mcp test dpanel        # 连通性与工具发现
hermes mcp list               # 查看已配置的 server
```

也可用命令行添加（自动写入配置）：

```bash
hermes mcp add dpanel --url http://localhost:8807/dpanel/mcp --auth header
```

### 其他 MCP 客户端（通用）

任何支持 Streamable HTTP 的客户端均可接入，端点 `http://<dpanel-host>:8807/dpanel/mcp`，请求头带 `Authorization: Bearer <token>`。

---

## 🛡️ 安全最佳实践

### Token 管理
```bash
# 生成强 token
openssl rand -hex 32 > .mcp_token
chmod 600 .mcp_token

# 使用 .env 文件
echo "DP_MCP_TOKEN=$(cat .mcp_token)" > .env
docker-compose --env-file .env up -d
```

### 网络隔离
```yaml
ports:
  - "127.0.0.1:8807:8080"  # 仅本机
```

### 权限最小化
```yaml
# 生产监控（只读）
DP_MCP_PROFILE: "read-only"

# 日常运维（推荐）
DP_MCP_PROFILE: "read-write"
```

---

## 🔧 开发与测试

```bash
# 单元测试（MCP 子模块）
cd app/mcp && go test -v ./...

# E2E 测试（需运行中的容器，MCP_TOKEN 环境变量）
python3 scripts/e2e_native.py

# 构建镜像
make build-lite  # lite 版
make build-full  # full 版（含前端）
```

---

## 📈 更新日志

> **版本方案**：`<上游基线>-mcp<N>`，如 `1.11.1-mcp6` = 基于 DPanel v1.11.1 的第 6 个 MCP 版本。
> 上游发新版后计数器重置（如 DPanel 1.11.2 → `v1.11.2-mcp1`）。历史版本曾用
> `1.11.2~1.11.6-mcp` 递增命名，自本版起废弃（那些版本号在上游并不存在，易误导）。

### v1.11.1-mcp7 (2026-10-16)

- 新增 `dpanel_image_pull` 工具（第 100 个）：同步拉取镜像到本地，含面板
  镜像源代理回退。修复 MCP 通道无法 pull 镜像的缺口——面板
  `/app/image/tag-sync` 的 pull 依赖 WebSocket progress fd，MCP 回环调用
  无 ws 会话必报 `fd not found`。新工具在 MCP 进程内直接调 docker SDK，
  镜像源配置经面板 API 读取（与面板 Web UI 同源），拉取后返回本地
  imageId 供验证

### v1.11.1-mcp6 (2026-10-14)

- 修复 `dpanel_explorer_export` / `dpanel_compose_log`（download=true）：
  上游导出/下载类接口返回 attachment 文件流（非 JSON），MCP 层按 JSON
  解析报「响应解析失败」。现以文本透传，text/* 内容过脱敏（日志中的
  KEY=VALUE/JWT 被 mask）
- 修复 `dpanel_container_stat` 恒 500 "fd not found"：上游路由是
  WebSocket 管道（HTTP 响应不含统计）。改用一次性快照路由 + 按容器
  ID/前缀/名称过滤（名称匹配裁剪 `/` 前缀，无匹配时附提示）
- 修复 `dpanel_compose_log` 默认走 WebSocket 管道：默认改为
  download=true 返回全量文本；显式传 false 时本地拒绝并给出清晰错误
- explorer_import/export 参数描述补齐实测行为（源路径固定在
  /dpanel/storage/temp/；导出为 octet-stream/zip，8MiB 截断）

### v1.11.5-mcp (2026-10-13)

- 修复 `dpanel_notice_unread`：面板 `/common/notice/unread` 的 binding 要求
  `action` 必填（new|init|clear），工具此前未注册该参数导致恒 500。
  现已注册并默认 `new`
- 修复 MCP client 错误提取：非 200 响应此前直接返回状态码文本
  （"Internal Server Error"），不解析错误体。现解析面板错误体的
  message/error 字段，agent 能看到真实错误（如 `.commonDataNotFoundOrDeleted`）
- explorer 工具描述与 mountPoint 校验错误信息补充前提：`docker:<环境名>`
  挂载点仅限开启 SSH 的远程环境，本地 sock 环境用 `volume:`/`container:`
- 新增 `TestNoticeUnreadActionParam` 契约测试防回归

### v1.11.4-mcp (2026-10-13)

- service 凭据可经管理面 API 录入：`/common/mcp/update` 新增可选参数
  `serviceUsername`/`servicePassword`，存 settings 表。部署可不设
  `DP_MCP_SERVICE_*` 环境变量，首登设 founder 后经 API 录入即可
  （环境变量仍优先于持久化值）
- `serverInfo.version` 硬编码 `1.11.2` 未随版本更新——改为与发布版本对齐
- README 修正：service 凭据标注、MCP 客户端接入命令、protocolVersion

### v1.11.3-mcp (2026-10-13)

- 合并上游 donknap/dpanel master（`ce4eb322` + `798db93d`，即上游 v1.11.1
  最终态）：sock pipe 代理监听复用优化、scoop 安装支持、环境变量统一
- 合并零冲突；MCP 模块与上游改动零重叠

### v1.11.2-mcp (2026-10-12)

- NewToken 失败检查（防止无门禁）
- MCP client HTTP ReadAll 错误处理（清晰诊断）
- 首个内置版发布

---

## 📝 License

继承上游 DPanel 许可证（见 [LICENSE](LICENSE)）

---

## 🔗 相关链接

- **上游 DPanel**: https://github.com/donknap/dpanel
- **MCP 协议**: https://modelcontextprotocol.io

---

**维护者**: leavrcn
