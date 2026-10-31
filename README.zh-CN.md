# newapi-operator

[![CI](https://github.com/Ruanyuxi1337/newapi-operator/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/Ruanyuxi1337/newapi-operator/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/Ruanyuxi1337/newapi-operator.svg)](https://pkg.go.dev/github.com/Ruanyuxi1337/newapi-operator)
[![Latest release](https://img.shields.io/github/v/release/Ruanyuxi1337/newapi-operator)](https://github.com/Ruanyuxi1337/newapi-operator/releases/latest)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[English](README.md) | 中文

一个 MCP 服务器，让 AI Agent 完整接管自建的
[New API](https://github.com/QuantumNous/new-api) 网关：渠道、下游 Key、用户、
额度、定价、调用日志，以及运行版本暴露的其余全部管理接口。

```
AI Agent  ◄── MCP (stdio / HTTP) ──►  newapi-operator  ◄── /api/* ──►  New API
```

47 个专用工具 · 312 条管理接口透传 · 路由表由上游源码生成，每周做一次漂移检测。

## 快速开始

需要 Go 1.26+。

```bash
git clone https://github.com/Ruanyuxi1337/newapi-operator.git
cd newapi-operator
make build
```

在 New API 面板生成访问令牌：**头像 → 个人设置 → 生成访问令牌**。

> 用访问令牌，不要用中转 Key（`sk-…`），也不要用登录密码。New API 单用户只允许
> 一个会话，浏览器开着面板时脚本用密码登录会返回 409。

把二进制注册到任意支持 stdio 的 MCP 客户端：

```json
{
  "mcpServers": {
    "newapi": {
      "command": "/绝对路径/bin/newapi-operator",
      "env": {
        "NEWAPI_BASE_URL": "http://127.0.0.1:3001",
        "NEWAPI_ACCESS_TOKEN": "nap_…",
        "NEWAPI_USER_ID": "1"
      }
    }
  }
}
```

重启客户端，先让 Agent 调一次 `user_session_info`。它会返回运行中的 New API
版本和功能开关，是最快的连通性验证。

## 封装解决了什么

New API 的管理 API 不是标准 REST。下面这些约定，照着常规写法写必然踩坑，也是
这批专用工具存在的理由：

| 约定 | 想当然的写法 |
|---|---|
| 调用失败也返回 **HTTP 200** + `{"success": false}` | 把所有报错当成成功 |
| 建渠道要嵌套 `{"mode": …, "channel": {…}}` | 平铺 body，直接被拒 |
| 更新走 `PUT /api/channel`，**id 放在 body 里** | 调 `PUT /api/channel/:id`，404 |
| `models`、`model_mapping` 是**字符串** | 传数组，被拒 |
| OpenAI 渠道的 `base_url` **不能以 `/v1` 结尾** | 请求打到 `/v1/v1/chat/completions`，404 |
| 额度是内部单位（**500,000 = $1**） | 预算差 50 万倍 |

服务器会把 `success: false` 转成工具错误，所以**只要返回了数据，调用就一定成功**。

## 配置

| 变量 | 必填 | 默认 | 说明 |
|---|---|---|---|
| `NEWAPI_BASE_URL` | 是 | — | 实例地址，如 `http://127.0.0.1:3001`。结尾的 `/` 和 `/api` 会被去掉 |
| `NEWAPI_ACCESS_TOKEN` | 是 | — | 管理员访问令牌 |
| `NEWAPI_USER_ID` | 否 | `1` | 作为 `New-Api-User` 发送，须为令牌属主的数字 id |
| `NEWAPI_TIMEOUT` | 否 | `60s` | 单请求超时 |
| `NEWAPI_READ_ONLY` | 否 | `false` | 拒绝所有写操作 |
| `NEWAPI_QUOTA_PER_UNIT` | 否 | `500000` | 每 1 货币单位的额度，供 `*_usd` 参数换算 |
| `MCP_TRANSPORT` | 否 | `stdio` | `stdio` 或 `http` |
| `MCP_HTTP_ADDR` | 否 | `127.0.0.1:8080` | HTTP 传输的监听地址 |
| `MCP_HTTP_TOKEN` | 仅 http | — | `/mcp` 的 Bearer 令牌 |
| `LOG_LEVEL` | 否 | `info` | `debug`、`info`、`warn`、`error` |
| `LOG_FORMAT` | 否 | `text` | `text` 或 `json` |

必填项缺失，或地址、超时、传输方式非法时，启动直接失败。

**直连网关端口，不要走公网域名。** 会规范化结尾斜杠的反代会回 307 并丢掉请求
body，所有创建/更新调用会静默失效。

## 工具

| 分类 | 数量 | 工具 |
|---|---|---|
| 渠道 | 22 | `channel_list`、`channel_create`、`channel_test`、`channel_balance`、`channel_priority_reorder`、`channel_upstream_apply`、`channel_multi_key` …… |
| 下游 Key | 9 | `token_list`、`token_create`、`token_update`、`token_reveal_key`、`token_batch_keys` …… |
| 用户 | 6 | `user_list`、`user_manage`、`user_session_info`、`user_access_tokens` …… |
| 设置 / 定价 / 日志 | 7 | `option_list`、`option_set`、`pricing_get`、`pricing_patch`、`log_list`、`log_stats` …… |
| 通用 | 3 | `api_request`、`api_endpoints`、`api_coverage` |

完整参数说明见 [docs/TOOLS.md](docs/TOOLS.md)。

### 通用透传

`api_request` 能打任意 `/api/*` 接口，包括没有专用工具的部分：订阅、部署、任务
插件、性能诊断、Ollama 拉模型。先用 `api_endpoints` 查路径：

```
api_endpoints {"group": "subscription"}
→ GET   /api/subscription/plans
  POST  /api/subscription/admin/plans
  PATCH /api/subscription/admin/plans/:id
```

调用前把路径参数替换成实际值，例如 `/api/channel/:id` → `/api/channel/7`。

## 跟上游版本保持同步

路由表在 `internal/spec/routes_gen.go`，是生成的，不要手改。

```bash
make sync                              # 按最新 release 重新生成
./bin/routesync --tag latest --report  # 按分组统计接口数
./bin/routesync --tag latest --check   # 有漂移则退出码 1
```

`routesync` 读取目标 tag 的 `router/*.go`，覆盖 New API 混用的三种注册写法：

```go
channelRoute.GET("/test/:id", handler)                       // 直接注册
handlePermissionRoute(channelRoute, "GET", "/ops", perm, h)   // 辅助函数，常折行
var channelPermissionRoutes = []permissionRoute{              // 表驱动 —— 主 CRUD
    {method: http.MethodGet, path: "/priority/order", ...},
}
```

只认第一种写法的解析器会漏掉大半管理接口；`internal/spec` 三种都覆盖，各有测试。

采纳新版本：

```bash
make sync && make sync-baseline
git diff internal/spec     # 看清上游加了什么、删了什么
go test ./...
```

每周的[工作流](.github/workflows/upstream-drift.yml)会在路由表落后于最新 release
时自动开 issue。

## 部署

### stdio

默认方式。由 MCP 客户端拉起进程；所有日志走 stderr，不会污染协议流。

### HTTP

```bash
NEWAPI_BASE_URL=http://127.0.0.1:3001 \
NEWAPI_ACCESS_TOKEN=… \
MCP_TRANSPORT=http MCP_HTTP_ADDR=127.0.0.1:8080 \
MCP_HTTP_TOKEN=$(openssl rand -hex 16) \
  ./bin/newapi-operator
```

`POST /mcp` 需要 `Authorization: Bearer $MCP_HTTP_TOKEN`。`GET /healthz`
不需要鉴权，返回二进制版本和路由表版本。

### Docker

```bash
docker build -t newapi-operator .

docker run --rm --network host \
  -e NEWAPI_BASE_URL=http://127.0.0.1:3001 \
  -e NEWAPI_ACCESS_TOKEN=… \
  -e MCP_TRANSPORT=http \
  -e MCP_HTTP_ADDR=0.0.0.0:8080 \
  -e MCP_HTTP_TOKEN=$(openssl rand -hex 16) \
  newapi-operator
```

`--network host` 仅限 Linux。其他平台把 `NEWAPI_BASE_URL` 指向容器可达的地址，
并映射 8080 端口。

## 安全模型

- **只读模式**（`NEWAPI_READ_ONLY=true`）在注册表层拒绝写工具，请求根本不会构造。
- **路径白名单**：客户端拒绝 `/api/*` 之外的路径，不会被误用成中转流量的开放代理。
- **凭据脱敏**：`internal/redact` 会把 Bearer、`sk-*`、JWT 和 JSON 里的 secret
  字段从日志和返回给 Agent 的错误信息里抹掉。
- **无额外出网**：只连 `NEWAPI_BASE_URL`。没有遥测；不设 `MCP_TRANSPORT=http`
  就不会开监听端口。
- **删除 ≠ 禁用**：删渠道、删令牌不可恢复。工具描述写明了，Agent 指令里也重复了。

漏洞报告方式见 [SECURITY.md](SECURITY.md)。

## 开发

```bash
make build      # 编译两个二进制
make test       # go test ./... -race
make vet        # go vet ./...
make fmt        # gofmt
make sync       # 按最新上游 release 重建路由表
make smoke      # 对活实例跑 stdio 端到端
```

```
cmd/newapi-operator/   MCP 服务器
cmd/routesync/         路由表生成器 + 漂移检测
internal/client/       管理 API 客户端（信封、错误分类、脱敏）
internal/config/       环境配置
internal/redact/       凭据脱敏
internal/spec/         路由表、上游解析器、生成产物
internal/tools/        专用工具 + 通用透传
docs/                  工具参考与设计说明
```

提 PR 前先看 [CONTRIBUTING.md](CONTRIBUTING.md)；内部实现见
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)。

## 许可

MIT，见 [LICENSE](LICENSE)。与 New API 项目无隶属关系，是其管理 API 的独立客户端。