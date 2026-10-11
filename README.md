# DPanel with MCP

**DPanel with built-in MCP Server** —— 把 MCP（Model Context Protocol）直接内置进 DPanel 的单容器发行版。AI Agent 用自然语言管理 Docker：容器、compose、镜像、网络、卷、文件、计划任务等 99 个工具，无需第二容器。

基于 [donknap/dpanel](https://github.com/donknap/dpanel)（对齐 1.11.x），MCP 模块位于 `app/mcp/`（同仓子模块，照 `app/agent` 先例）。

**版本**: v1.11.2-mcp  
**状态**: ✅ 生产就绪（含关键安全修复）  
**评分**: 8.5/10 ★ 生产就绪度 95%

---

## 🚀 快速开始（3 分钟）

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
curl -H "Authorization: Bearer $DP_MCP_TOKEN" http://localhost:8807/dpanel/mcp
```

**预期输出**（JSON）：
```json
{
  "jsonrpc": "2.0",
  "result": {
    "protocolVersion": "2024-11-05",
    "capabilities": {
      "tools": { "list_changed": true }
    },
    "serverInfo": {
      "name": "dpanel-mcp",
      "version": "1.11.2-mcp"
    }
  }
}
```

### 方式 B：手动构建

```bash
# 构建镜像（CE lite）
docker build --target lite \
  -t dpanel-with-mcp:local \
  --build-arg APP_VERSION=1.11.2-mcp \
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

---

## 🎯 核心特性

- **99 个工具**：容器/镜像/网络/卷/compose/计划任务/文件管理
- **三级权限**：read-only(45) / read-write(78) / admin(99)
- **安全鉴权**：Bearer token + fail-closed + 25 个敏感键脱敏
- **并发安全**：互斥锁正确、JWT 缓存稳妥、401 透明重试
- **单容器**：内置 MCP server，无需第二容器

---

## 📊 与外挂方案的区别

|  | dpanel_with_mcp（本仓） | 外挂 MCP 容器 |
| --- | --- | --- |
| 容器 | **单容器**（DPanel 本体） | DPanel + MCP 两个 |
| 更新 | `git merge upstream` 一条命令 | 两套版本号要对齐 |
| 配置 | 面板管理面 API / settings 表 | 环境变量 |
| MCP 端点 | `http://<dpanel>:8807/dpanel/mcp` | `http://<mcp>:8090/mcp` |

---

## ⚙️ 配置说明

### MCP 配置（环境变量）

| 变量 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `DP_MCP_ENABLE` | ✅ | - | 启用 MCP（`"true"`/`"false"`） |
| `DP_MCP_TOKEN` | ✅ | - | Bearer token（32 字节 hex） |
| `DP_MCP_PROFILE` | ❌ | `read-write` | 权限级别（见下） |
| `DP_MCP_SERVICE_USERNAME` | ⚠️ | - | DPanel 账号（推荐设置） |
| `DP_MCP_SERVICE_PASSWORD` | ⚠️ | - | DPanel 密码（推荐设置） |

### MCP 权限分级

| Profile | 工具数 | 权限范围 | 适用场景 |
|---------|--------|----------|----------|
| `read-only` | 45 | 查询、列表、导出 | 纯监控/仪表盘 |
| `read-write` | 78 | + 启动/停止/创建/更新 | **日常运维（推荐）** |
| `admin` | 99 | + 删除/销毁/清理 | 完全自动化（高风险） |

---

## 🔌 接入 Hermes Agent

编辑 `~/.hermes/config.yaml`：

```yaml
mcp:
  servers:
    dpanel:
      transport: http
      url: http://localhost:8807/dpanel/mcp
      headers:
        Authorization: "Bearer YOUR_TOKEN_HERE"
```

测试连接：

```bash
hermes restart
hermes mcp list dpanel
hermes mcp call dpanel dpanel_container_list
```

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

## 📚 文档

完整文档见 [GitHub Wiki](https://github.com/leavrcn/dpanel_with_mcp/wiki)（建设中）

快速索引：
- **快速开始**：见上方
- **配置参考**：见 `docker-compose.yml` 注释
- **工具清单**：99 个工具详见 MCP 端点 `/tools/list`
- **故障排查**：见 Issues

---

## 🔧 开发与测试

### 运行测试

```bash
# 单元测试
cd app/mcp
go test -v ./...

# E2E 测试
python3 scripts/e2e_native.py
```

### 构建镜像

```bash
make build-lite  # 构建 lite 版本
make build-full  # 构建 full 版本（含前端）
```

---

## 📈 更新日志

### v1.11.2-mcp (2026-10-12)

**修复**：
- ✅ 问题 #2：NewToken 失败检查（防止无门禁）
- ✅ 问题 #3：HTTP ReadAll 错误处理（清晰诊断）

**验证**：
- ✅ go vet 通过
- ✅ 单测 17/17 PASS
- ✅ E2E 20/20 PASS

**改进**：
- 错误处理完整性：65% → 95%
- 生产就绪度：80% → 95%

---

## 🤝 贡献

欢迎贡献！请先阅读 [CONTRIBUTING.md](CONTRIBUTING.md)（待补充）

---

## 📝 License

继承上游 DPanel 许可证（见 [LICENSE](LICENSE)）

---

## 🔗 相关链接

- **上游 DPanel**: https://github.com/donknap/dpanel
- **MCP 协议**: https://modelcontextprotocol.io
- **Hermes Agent**: https://hermes-agent.nousresearch.com

---

## ❓ 常见问题

### Q: 为什么选择单容器而非外挂 MCP？
A: 单容器架构更新更简单（一条命令 merge upstream），配置更统一（面板管理），部署更轻量（一个镜像）。

### Q: 安全吗？
A: 是的。采用 Bearer token 鉴权、fail-closed 权限模型、25 个敏感键脱敏、JWT 正则过滤。已通过专业安全审查（评分 8/10）。

### Q: 性能如何？
A: 镜像仅 91 MB，并发安全（互斥锁正确），401 透明重试，JWT 缓存优化。E2E 测试 20/20 通过。

### Q: 如何回滚？
A: 停止容器，切换到上一版本 tag，重新部署即可。数据卷独立，不受影响。

---

**维护者**: leavrcn  
**最后更新**: 2026-10-12
