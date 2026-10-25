# newapi-operator

[English](README.md) | 中文

一个用 Go 写的 MCP 服务器，让 AI Agent **完整运营**自建的
[New API](https://github.com/QuantumNous/new-api) 网关 —— 渠道、下游 Key、用户、
额度、模型定价、调用日志，以及运行版本暴露的**全部** 管理接口。

内置的接口表直接从 New API 的 `router/*.go` 源码按 tag 生成，并配有漂移检测，
所以工具集不会悄悄落后于上游版本。

---

## 为什么需要它

New API 的管理 API 有一批"反直觉"行为，照着常规 REST 写法几乎必然踩坑：

| 行为 | 想当然的后果 |
|---|---|
| 失败也返回 **HTTP 200** + `{"success": false}` | 所有报错都被当成成功 |
| 建渠道必须嵌套 `{"mode", "channel"}` | 平铺 body 直接被拒 |
| 改渠道/令牌是 `PUT /api/channel`，**id 在 body 里** | 用 `/api/channel/:id` 返回 404 |
| `models`、`model_mapping` 是**字符串** | 传数组报错 |
| OpenAI 渠道 `base_url` **不能带 `/v1`** | 请求打到 `…/v1/v1/chat/completions` → 404 |
| 额度是内部单位（500 000 = $1） | 预算算错 50 万倍 |

这些都在对应工具里做好了。剩下没封装的接口走通用透传，不会有"够不着"的地方。

市面上给 New API 写的第三方 MCP 都带一份**冻结的** OpenAPI 快照 —— 其中一个流行项目
的快照落后两个小版本，缺少 `v1.0.0-rc.47` 里 **55%** 的接口。本项目交付的是**生成器**：
`cmd/routesync` 从上游源码重建接口表，表与锁定版本不一致时 CI 直接失败。

---

## 安装

需要 Go 1.24+。

```bash
git clone https://github.com/Ruanyuxi1337/newapi-operator.git
cd newapi-operator
make build          # 产出 bin/newapi-operator、bin/routesync
```

### 拿访问令牌

用的是 New API 的 **access token**，不是中转 Key：

> 登录面板 → 右上角头像 → **个人设置** → **生成访问令牌** → 复制

**不要用密码登录**：New API 单用户会话上限为 1，浏览器挂着后台时脚本登录会返回 409。

---

## 配置

| 环境变量 | 必填 | 默认 | 说明 |
|---|---|---|---|
| `NEWAPI_BASE_URL` | ✅ | — | 实例地址，如 `http://127.0.0.1:3000`。结尾的 `/` 和 `/api` 会自动去掉 |
| `NEWAPI_ACCESS_TOKEN` | ✅ | — | 管理员访问令牌 |
| `NEWAPI_USER_ID` | | `1` | 作为 `New-Api-User` 头发送，须是令牌属主的数字 id |
| `NEWAPI_TIMEOUT` | | `60s` | 单请求超时 |
| `NEWAPI_READ_ONLY` | | `false` | 拒绝所有写操作 |
| `NEWAPI_QUOTA_PER_UNIT` | | `500000` | 每 1 货币单位对应的额度，供 `*_usd` 参数换算 |
| `MCP_TRANSPORT` | | `stdio` | `stdio` 或 `http` |
| `MCP_HTTP_ADDR` | | `127.0.0.1:8080` | http 传输的监听地址 |
| `MCP_HTTP_TOKEN` | http 时必填 | — | `/mcp` 的 Bearer 令牌 |
| `LOG_LEVEL` | | `info` | `debug`/`info`/`warn`/`error` |

> **务必直连容器/网关端口，不要走公网域名。** 会规范化结尾斜杠的反代会回 307 并
> 丢掉请求 body，所有创建/更新调用会静默失效。

---

## 接入 Hermes Agent

```bash
hermes config set mcp_servers.newapi.command /绝对路径/bin/newapi-operator
hermes config set mcp_servers.newapi.env.NEWAPI_BASE_URL http://127.0.0.1:3000
hermes config set mcp_servers.newapi.env.NEWAPI_ACCESS_TOKEN '<令牌>'
```

重启 Hermes，工具以 `mcp_newapi_*` 出现。Hermes 会把 `mcp-*` 工具集放到
`tool_search` 后面按需加载，所以工具再多也不占常驻 prompt。

任何 MCP 客户端（JSON 配置）：

```json
{
  "mcpServers": {
    "newapi": {
      "command": "/绝对路径/bin/newapi-operator",
      "env": {
        "NEWAPI_BASE_URL": "http://127.0.0.1:3000",
        "NEWAPI_ACCESS_TOKEN": "你的令牌",
        "NEWAPI_USER_ID": "1"
      }
    }
  }
}
```

---

## 工具一览（47 个）

**渠道** `channel_list` / `channel_get` / `channel_create` / `channel_update` /
`channel_delete` / `channel_set_status` / `channel_batch_status` /
`channel_test` / `channel_test_all` / `channel_fetch_models` / `channel_balance` /
`channel_ops` / `channel_priority_order` / `channel_priority_reorder` /
`channel_codex_usage` / `channel_codex_refresh` /
`channel_upstream_detect` / `channel_upstream_apply` /
`channel_get_key` / `channel_models` / `channel_copy` / `channel_multi_key`

**下游 Key** `token_list` / `token_get` / `token_search` / `token_create` /
`token_update` / `token_delete` / `token_reveal_key` / `token_batch_keys` /
`token_auto_groups`

**用户 / 设置 / 日志** `user_list` / `user_get` / `user_search` / `user_manage` /
`user_access_tokens` / `user_session_info` / `option_list` / `option_set` /
`pricing_get` / `pricing_patch` / `pricing_preview` / `log_list` / `log_stats`

**通用** `api_endpoints` / `api_coverage` / `api_request`

### 通用透传

`api_request` 能打**任意** `/api/*` 路由 —— 订阅、部署、任务插件、性能诊断、
Ollama 拉模型，全部 312 条，不只是有封装的那部分。先用 `api_endpoints` 查路径：

```
api_endpoints {"group": "subscription"}
→ GET  /api/subscription/plans
  POST /api/subscription/admin/plans
  PATCH /api/subscription/admin/plans/:id
  …
```

---

## 按 Key 限制模型

这是大多数人"在网关前面再套一层网关"的真正原因。

```json
{
  "name": "alice-flash-only",
  "models": ["gemini-2.5-flash", "qwen3.8-flash"],
  "remain_quota_usd": 5
}
```

`token_create` 会生成一个**只能调这两个模型**的 Key。请求别的模型时 New API
在进上游渠道之前就拒掉，账号池完全看不到这次请求。

不传 `models` 就是不限模型；`remain_quota_usd` 会按 `NEWAPI_QUOTA_PER_UNIT`
换算成 New API 的内部额度单位。

---

## 跟上游版本保持同步

接口表在 `internal/spec/routes_gen.go`，是生成的，不要手改。

```bash
make sync                                      # 按最新上游 release 重新生成
./bin/routesync --tag v1.0.0-rc.47 --report    # 按分组统计接口数
./bin/routesync --tag latest --check           # 有漂移则退出码 1
```

`routesync` 会读指定 tag 的 `router/*.go`，并覆盖 New API 混用的**三种**注册写法：

```go
channelRoute.GET("/test/:id", handler)                        // 直接注册
handlePermissionRoute(channelRoute, "GET", "/ops", perm, h)    // 辅助函数，常折行
var channelPermissionRoutes = []permissionRoute{               // 表驱动 —— 主 CRUD
    {method: http.MethodGet, path: "/priority/order", ...},    //   都是这么注册的
}
```

只认第一种写法的解析器会漏掉大半个管理接口面，`internal/spec` 三种都覆盖并且各有测试。

**自动化**：`.github/workflows/upstream-drift.yml` 每周解析最新 release，落后时自动
开/更新一个 issue，附带精确的"新增/删除"接口清单。

采纳新版本：

```bash
make sync           # 或 ./bin/routesync --tag vX.Y.Z
make sync-baseline  # 刷新基线
git diff internal/spec
go test ./...
```

---

## 安全模型

- **只读模式**（`NEWAPI_READ_ONLY=true`）在注册表层拦截所有写工具，请求根本不会构造。
- **路径白名单**：客户端拒绝 `/api/*` 之外的任何路径，不会被误用成中转流量的开放代理。
- **凭据脱敏**：`internal/redact` 会把 Bearer、`sk-*`、JWT、JSON 里的 secret 字段
  从错误串里抹掉，然后才进日志或工具返回。
- **无额外出网**：本进程只连 `NEWAPI_BASE_URL`。没有遥测、不主动开监听端口、
  没有任何 `init()` 副作用。
- **删除 ≠ 禁用**：删渠道/删令牌不可恢复，工具描述和 Agent 指令里都写明了。

---

## 开发

```bash
make build    # 编译两个二进制
make test     # go test ./... -race
make vet      # go vet
make fmt      # gofmt
make sync     # 按最新上游重建接口表
make smoke    # 对活实例跑 stdio 端到端
```

```
cmd/newapi-operator/   MCP 服务器入口
cmd/routesync/         接口表生成器 + 漂移检测
internal/client/       管理 API 客户端（信封解析、错误映射、脱敏）
internal/config/       环境配置
internal/redact/       凭据脱敏
internal/spec/         接口表、上游解析器、生成产物
internal/tools/        封装的工具集 + 通用透传
scripts/smoke_test.py  stdio 端到端自检
```

## 许可

MIT，见 [LICENSE](LICENSE)。

与 New API 项目无隶属关系，是其管理 API 的独立客户端。