# DPanel with MCP

**DPanel with built-in MCP Server** —— 把 MCP（Model Context Protocol）直接内置进 DPanel 的单容器发行版。AI Agent 用自然语言管理 Docker：容器、compose、镜像、网络、卷、文件、计划任务等 99 个工具。

基于 [donknap/dpanel](https://github.com/donknap/dpanel)（对齐 1.11.x），MCP 模块位于 `app/mcp/`（同仓子模块）。

**版本**: v1.11.4-mcp

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
      "version": "1.11.4-mcp"
    }
  }
}
```

### 方式 B：手动构建

```bash
# 构建镜像（CE lite）
docker build --target lite \
  -t dpanel-with-mcp:local \
  --build-arg APP_VERSION=1.11.4-mcp \
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

- **99 个工具**：容器/镜像/网络/卷/compose/计划任务/文件管理
- **三级权限**：read-only(45) / read-write(78) / admin(99)
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
> 2. 管理面 API：首登设 founder 账密后，调 `POST /dpanel/api/common/mcp/update`
>    （带面板 JWT）传 `serviceUsername`/`servicePassword`，存 settings 表
>
> 环境变量优先于 settings 表；改配置后需重启容器生效。

### MCP 权限分级

| Profile | 工具数 | 权限范围 | 适用场景 |
|---------|--------|----------|----------|
| `read-only` | 45 | 查询、列表、导出 | 纯监控/仪表盘 |
| `read-write` | 78 | + 启动/停止/创建/更新 | **日常运维（推荐）** |
| `admin` | 99 | + 删除/销毁/清理 | 完全自动化（高风险） |

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
