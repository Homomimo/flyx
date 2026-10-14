# 070: 节点搭建（多内核可插拔，单用户自用）

## 背景与目标

FLVX 目前只做端口转发 / 隧道。目标是在**同一个面板**里增加"搭建代理节点"能力：

- 在已接入的节点（agent）上一键部署代理服务端，**内核可选**：Xray、sing-box、Snell，后续可扩展 Hysteria2 官方版、shadowsocks-rust 等
- **不使用 GOST 自带协议**做节点，GOST 继续只负责转发 / 隧道
- **单用户自用**：不做多用户、配额、到期、按用户计费
- 生成分享链接、二维码和订阅，客户端一键导入
- 后期支持"节点经由中转入口"，订阅地址自动替换为转发入口

### 非目标

- 多用户 / 用户同步 / 按用户流量统计
- 计费、套餐、在线 IP 限制
- 修改或删除现有多用户相关功能（只在新功能中不使用它们）
- 远程节点（`is_remote=1`）和 nftables 模式节点（`forward_mode=nftables`）上部署内核

## 总体架构

```
面板 go-backend
  ├─ node_kernel   节点上的内核实例（类型、版本、状态）
  ├─ node_inbound  入站（协议、端口、传输、TLS/Reality、凭据）
  ├─ 配置渲染：通用模型 → 每个内核的配置文本（在面板侧完成）
  ├─ 分享链接 / 订阅生成（只依赖协议，不依赖内核）
  └─ 通过现有 WebSocket 控制通道下发 Kernel* 命令
          │
          ▼
Agent go-gost/x/kernel（新包）
  ├─ Manager：安装、守护子进程、崩溃重启、日志环形缓冲
  ├─ Adapter：xray / singbox / snell / ...（描述二进制、启动参数、配置文件名）
  └─ nft 计数：独立表 inet flvx_kernel，按入站端口计数并上报
```

**关键决定：配置渲染放在面板侧。**
Agent 只负责"拿到配置文本，写文件，启动或重启进程"，不理解协议细节。好处是：

- 新增或调整协议时只改面板，不需要升级所有 agent
- 渲染逻辑可以在 `go test` 里用 golden 文件做单元测试
- Agent 的适配器很薄（每个约几十行）

## 数据模型

遵守项目约定：定义 `TableName()`，不用 `jsonb` / `serial`，JSON 存 `text`。新表加入 `repository.go` 的 AutoMigrate 列表。

### `node_kernel`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | int64 PK | |
| node_id | int64, uniqueIndex(node_id, kernel_type) | 每个节点每种内核最多一个实例 |
| kernel_type | varchar(20) | `xray` / `singbox` / `snell` |
| version | varchar(50) | 期望版本，如 `25.9.11`、`1.12.8`、`5.0.0` |
| installed_version | varchar(50) | agent 上报的实际版本 |
| status | varchar(20) | `pending` / `installing` / `running` / `stopped` / `error` |
| last_error | text | |
| config_hash | varchar(64) | 最近一次下发配置的 sha256，用于判断是否需要重新下发 |
| enabled | int | 1=启用，0=停用 |
| created_time / updated_time | int64 | |

### `node_inbound`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | int64 PK | |
| kernel_id | int64, index | 所属 node_kernel |
| node_id | int64, index | 冗余字段，便于做端口冲突检查和查询 |
| name | varchar(100) | 显示名，也用作订阅里的节点名 |
| protocol | varchar(30) | `vless` / `vmess` / `trojan` / `shadowsocks` / `hysteria2` / `tuic` / `snell` … |
| listen | varchar(100) | 默认 `::` |
| port | int | |
| network | varchar(20) | `tcp` / `udp` / `tcp,udp`，用于 nft 计数和防火墙 |
| transport | text(JSON) | `{type: tcp|ws|grpc|httpupgrade|xhttp, path, host, serviceName}` |
| security | text(JSON) | `{type: none|tls|reality, sni, alpn, certMode, realityPrivateKey, realityPublicKey, shortIds, dest, fingerprint}` |
| settings | text(JSON) | 协议凭据和参数：`uuid`、`flow`、`password`、`method`、`psk`、`obfs`、`version` 等 |
| extra | text(JSON) | 内核特有参数透传（高级用户使用，原样合并进渲染结果） |
| share_host | varchar(255) | 分享 / 订阅中使用的地址，为空时使用节点 `server_ip` |
| via_forward_id | int64, nullable | 第 4 阶段：经由某条转发，订阅时使用转发入口的地址和端口 |
| enabled | int | |
| upload_bytes / download_bytes | int64 | 累计流量（由 nft 计数累加） |
| created_time / updated_time | int64 | |

后端为每种协议定义一个 Go 结构体，对 `settings`、`transport`、`security` 做强类型解析和校验，避免把 JSON 原样透传给前端编辑。

### `node_inbound_counter`（nft 计数快照）

结构参考 `NftCounterState`：`inbound_id + protocol + direction` 唯一，记录上次的计数值，用于计算增量，处理 agent 重启后计数器归零的情况。

### 订阅 token

存在现有的 `vite_config` 键值表里：`node_sub_token`。首次访问时自动生成，支持在页面上重置。

## 内核 × 协议能力矩阵（第一版）

| 协议 / 组合 | Xray | sing-box | Snell |
|---|---|---|---|
| VLESS + Reality + Vision | ✅ | ✅ | |
| VLESS + WS/gRPC + TLS | ✅ | ✅ | |
| VLESS + XHTTP + TLS/Reality | ✅ | ❌ | |
| VMess + WS + TLS | ✅ | ✅ | |
| Trojan + TLS | ✅ | ✅ | |
| Shadowsocks（AEAD / 2022） | ✅ | ✅ | |
| Hysteria2 | ⚠️ 需要确认版本支持 | ✅ | |
| TUIC v5 | | ✅ | |
| AnyTLS / ShadowTLS | | ✅ | |
| Snell v4 / v5 | | | ✅ |

该矩阵在面板后端的 `kernel/capabilities.go` 中以静态表维护，并通过 API 下发给前端。前端选择内核后，只显示该内核支持的协议和传输。

> 实施前需要对照各内核当前版本的官方文档，确认字段名和协议支持。sing-box 1.11 之后入站配置格式有调整，Xray 也在持续新增传输。

## 凭据与证书

全部在面板侧用 Go 标准库生成，不依赖各内核的命令行：

| 用途 | 生成方式 |
|---|---|
| UUID | `crypto/rand`，v4 |
| 密码 / Snell PSK | 32 字节随机，base64url |
| SS2022 密钥 | 按 method 生成 16 或 32 字节，base64 |
| Reality 密钥对 | `crypto/ecdh` X25519，私钥和公钥都用 base64url（无 padding） |
| Reality shortId | 8 字节随机，hex |

TLS 证书提供三种模式：

1. **Reality**（推荐默认）：不需要证书和域名
2. **自签证书**：面板生成 ECDSA 证书，随配置一起下发，分享链接带 `allowInsecure` / `insecure=1`。适合 Hysteria2、TUIC
3. **ACME**：只有 sing-box 内置 ACME，第一版只对 sing-box 开放；Xray 用户需要自己提供证书路径

### 预设模板

创建入站时先选模板，再自动填充参数，只需要用户填 SNI 或域名：

- VLESS-Reality-Vision（Xray / sing-box）
- Hysteria2 自签（sing-box）
- SS2022（Xray / sing-box）
- Trojan-TLS（sing-box ACME）
- Snell v5（Snell）

## 控制通道命令（面板 → agent）

复用 `ws.Server.SendCommand`，新增以下命令。agent 端加入 `routeCommand` 分支，并在 `isMutationCommand` 中登记，以获得变更锁：

| 命令 | 数据 | 说明 |
|---|---|---|
| `KernelInstall` | `{type, version, url, checksumUrl, archive: zip/tar.gz/raw, binPath}` | 下载、校验 sha256、解压到 `/etc/flux_agent/kernels/<type>/<version>/`。下载地址由面板生成（`{ARCH}` 占位符由 agent 替换），与 `UpgradeAgent` 的方式一致 |
| `KernelApply` | `{type, version, files: [{name, content, mode}], args, nftPorts: [{inboundId, port, network}]}` | 写入配置文件，原子替换（先写临时文件再 rename），然后重启进程，并同步 nft 计数规则 |
| `KernelControl` | `{type, action: start/stop/restart/remove}` | `remove` 同时删除二进制、配置和对应的 nft 规则 |
| `KernelStatus` | `{}` | 返回每个内核的运行状态、pid、版本、最近 N 行日志 |

agent 主动上报（新的消息类型）：

- `kernel_status`：进程状态变化时（启动、退出、崩溃重启）立即上报
- `kernel_traffic`：随 metric 周期（或每 60 秒）上报 nft 计数快照 `[{inboundId, proto, dir, bytes, packets}]`

### Agent 重连后的状态恢复

节点上线时（`SetNodeOnlineHook`），面板比较各 `node_kernel.config_hash` 与 agent 上报的 hash，不一致就重新执行 `KernelApply`。同时 agent 在本地持久化最后一次的 Apply 内容，所以 agent 重启时即使面板不在线，也能直接拉起内核。

## Agent 内核管理器（`go-gost/x/kernel/`）

```
kernel/
  manager.go      // 内核实例表、Apply/Control/Status，持久化 state.json
  process.go      // 子进程守护：启动、退出检测、指数退避重启、stdout/stderr 环形缓冲
  install.go      // 下载、sha256、解压（zip / tar.gz / 单文件）
  nft.go          // inet flvx_kernel 表的同步与计数读取
  adapters.go     // 适配器注册表
  adapter_xray.go
  adapter_singbox.go
  adapter_snell.go
```

适配器接口（agent 端很薄，配置内容由面板渲染）：

```go
type Adapter interface {
    Type() string
    BinaryName() string                    // xray / sing-box / snell-server
    ConfigFileName() string                // config.json / config.json / snell.conf
    Args(binPath, configPath string) []string
    Version(binPath string) (string, error) // 执行 -version 等命令并解析
}
```

| 内核 | 启动参数 | 版本命令 |
|---|---|---|
| xray | `run -c config.json` | `version` |
| sing-box | `run -c config.json` | `version` |
| snell | `-c snell.conf` | `-v` |

目录布局：

```
/etc/flux_agent/kernels/
  state.json                  // 最后一次 Apply 的内容（用于离线自启）
  xray/25.9.11/xray
  xray/run/config.json
  singbox/1.12.8/sing-box
  singbox/run/config.json
  snell/5.0.0/snell-server
  snell/run/snell.conf
```

进程以 root 运行（与 agent 相同）。实现时注意：

- 关闭 agent 时停止所有子进程（`Setpgid`，按进程组 kill）
- 进程退出后按 1s→2s→4s… 退避重启，最长 60 秒；连续失败时上报 `error` 和最后的日志

## 流量统计（nft，按入站端口）

**不复用**后端现有的 SSH 方式 nftables runtime。那套依赖 `NodeSSHConfig`，而且以 `forward_id` 为键。这里改由 agent 在本机直接管理一张独立的表：

```
table inet flvx_kernel {
  counter in_<id>_tcp  {}
  counter out_<id>_tcp {}
  counter in_<id>_udp  {}
  counter out_<id>_udp {}
  chain input  { type filter hook input  priority -150; policy accept;
                 tcp dport <port> counter name in_<id>_tcp ... }
  chain output { type filter hook output priority -150; policy accept;
                 tcp sport <port> counter name out_<id>_tcp ... }
}
```

- `in` = 客户端上传，`out` = 下发给客户端。只统计"客户端 ↔ 节点"这一段，不重复统计"节点 ↔ 目标网站"的流量
- 只用计数，**不做 drop / accept 以外的动作**，不影响现有防火墙规则
- 每次 Apply 都整表重建（`nft -f` 原子替换）
- 机器上没有 `nft` 时降级：内核照常运行，面板显示"流量统计不可用"
- 面板侧沿用 `nftables_traffic.go` 的做法：根据快照和上次的值算增量，计数值变小时视为计数器被重置，然后累加到 `node_inbound.upload_bytes/download_bytes`

## 面板 API

所有接口都只允许管理员访问（复用现有的 admin 鉴权中间件）。响应格式沿用 `{code, msg, data, ts}`。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/kernel/capabilities` | 内核 × 协议 × 传输能力矩阵、各内核推荐版本 |
| GET | `/api/v1/kernel/list?nodeId=` | 节点上的内核实例及状态 |
| POST | `/api/v1/kernel/install` | `{nodeId, type, version}` |
| POST | `/api/v1/kernel/control` | `{kernelId, action}` |
| POST | `/api/v1/kernel/logs` | `{kernelId}`，实时拉取最近日志 |
| GET | `/api/v1/inbound/list` | 全部入站（含所属节点、内核、流量） |
| POST | `/api/v1/inbound/create` | 创建入站，后端补全凭据，然后渲染并下发 |
| POST | `/api/v1/inbound/update` | |
| POST | `/api/v1/inbound/delete` | |
| POST | `/api/v1/inbound/reset-traffic` | |
| POST | `/api/v1/inbound/regenerate` | 重新生成凭据（UUID、密码、Reality 密钥） |
| GET | `/api/v1/inbound/share?id=` | 分享链接和二维码数据 |
| POST | `/api/v1/inbound/sub/reset-token` | 重置订阅 token |
| GET | `/api/v1/sub/{token}?format=` | **公开接口**，靠 token 鉴权；默认根据 UA 自动识别客户端 |

### 端口冲突校验

创建或修改入站时，检查同一节点上：

- 其他入站的端口
- 现有转发和隧道正在使用的端口（复用 plan 014 中的端口占用校验逻辑）
- 节点 `port` 字段配置的转发端口范围：入站端口可以在范围外，但不能和已占用的端口重叠

## 配置渲染（`go-backend/internal/kernel/`）

```
kernel/
  capabilities.go     // 能力矩阵、推荐版本、下载地址模板
  model.go            // 协议设置的强类型结构体与校验
  credentials.go      // UUID / 密码 / X25519 / shortId / 自签证书
  render_xray.go      // []Inbound → config.json
  render_singbox.go
  render_snell.go     // 单个入站 → snell.conf（Snell 每个实例只有一个入站，见下文）
  share.go            // 单个入站 → URI（vless:// vmess:// ss:// trojan:// hysteria2:// tuic://）
  subscription.go     // []Inbound → clash/mihomo YAML、sing-box JSON、Surge conf、base64
  testdata/*.golden
```

不放在 `http/handler` 里，因为这部分是纯函数，便于单元测试。handler 只负责调用。

### Snell 的特殊处理

Snell 每个进程只能有一个监听端口和一个 PSK。处理方式：

- 每个 Snell 入站对应一个进程。agent 的实例键从 `type` 扩展为 `type + "-" + inboundId`，配置文件写到各自目录
- 对 `node_kernel` 来说仍然只有一条 snell 记录（管理版本和安装），入站各自对应一个子进程

### 渲染规则

- Xray / sing-box：一个节点、一种内核、一份配置，包含该内核下所有启用的入站。出站只配 `direct`（第一版不做分流）
- 关闭内核自带的统计 / API（不需要），日志级别默认 `warning`
- 合并 `extra` 时，内核特有字段以深合并方式覆盖默认值

## 分享链接与订阅

| 客户端格式 | 参数 | 包含的协议 |
|---|---|---|
| Base64 URI 列表 | `format=base64` | 除 Snell 外的全部 |
| Clash Meta / Mihomo | `format=clash` | 全部（Mihomo 支持 snell） |
| sing-box | `format=singbox` | 除 Snell 外的全部 |
| Surge | `format=surge` | Surge 支持的子集，包含 snell |

- 响应头带 `subscription-userinfo: upload=..; download=..; total=0; expire=0`，以及 `profile-update-interval`
- Clash / sing-box 格式附带一份最小可用的规则模板（全局代理 + 局域网直连），第一版不做规则定制
- 未传 `format` 时根据 UA 推断：`clash`/`mihomo`/`stash` → clash，`sing-box` → singbox，`surge` → surge，其余返回 base64

## 前端

新增页面 `/inbound`（侧边栏名称："节点服务"），组件全部从 `src/shadcn-bridge/heroui/*` 引入。

- **列表**：名称、节点、内核、协议/传输、端口、状态灯、上行/下行流量、操作（分享、编辑、启用/停用、删除）
- **新建 / 编辑弹窗**：
  1. 选节点（只列出非远程、非 nftables 模式的节点）
  2. 选内核（未安装时提示安装，并选择版本）
  3. 选模板或自定义协议，表单按能力矩阵动态生成
  4. 高级：自定义端口、`share_host`、`extra` JSON
- **分享弹窗**：URI、二维码、复制按钮
- **订阅卡片**（页面顶部）：订阅地址和各格式快捷链接、重置 token
- **节点详情页**增加"内核"标签页：版本、状态、启动/停止/重启/卸载、查看日志

API 封装放在 `src/api/` 中，与现有文件风格保持一致。

## 与上游合并的策略

- 新代码集中放在：`go-backend/internal/kernel/`、`go-backend/internal/http/handler/kernel_*.go`、`go-gost/x/kernel/`、`vite-frontend/src/pages/inbound*`
- 对已有文件的改动控制在"注册一行"的程度：AutoMigrate 列表、路由注册、`routeCommand` 分支、`App.tsx` 路由、侧边栏菜单
- 不改现有表结构，不删除多用户功能

## 风险与待确认

| 项 | 说明 | 处理 |
|---|---|---|
| 内核配置格式变化 | sing-box 和 Xray 迭代快，字段经常调整 | 能力矩阵中记录每个版本区间；渲染和测试锁定在推荐版本 |
| Snell 闭源 | 下载地址和许可以官方为准，不允许把二进制打包进仓库 | agent 安装时从官方地址下载，下载地址做成配置项 |
| 下载被墙 | 国内节点访问 GitHub 困难 | 下载地址支持配置镜像前缀（与 agent 升级共用设置，如果存在的话） |
| 端口冲突 | 与 GOST 转发端口冲突 | 创建时校验；agent Apply 前再用 `net.Listen` 试探一次 |
| nft 与现有防火墙 | 可能与用户自定义规则相互影响 | 独立表，只计数不过滤，优先级 -150 |
| 证书 | Xray 没有内置 ACME | 第一版 Xray 只提供 Reality 和自签证书 |
| CI Go 版本 | CI 用 1.23 构建 | 只使用 1.23 已有的标准库（`crypto/ecdh` 从 1.20 开始提供） |

## 阶段与任务清单

### 第 1 阶段：框架 + Xray（目标：可用）

- [ ] B1 新增模型 `NodeKernel`、`NodeInbound`、`NodeInboundCounter` 并加入 AutoMigrate
- [ ] B2 `internal/kernel`：能力矩阵、协议模型与校验、凭据生成
- [ ] B3 `render_xray.go` + golden 测试（VLESS-Reality-Vision、SS2022、VMess-WS-TLS、Trojan）
- [ ] B4 kernel / inbound API（含端口冲突校验）+ contract 测试
- [ ] B5 `node_online` 钩子：根据 config_hash 补发配置
- [ ] A1 `x/kernel`：Manager、进程守护、安装、state.json 自启
- [ ] A2 xray 适配器 + `KernelInstall/Apply/Control/Status` 命令接入
- [ ] A3 nft 计数表同步与 `kernel_traffic` 上报；面板侧增量累加
- [ ] F1 `/inbound` 页面：列表、新建/编辑（模板化）、启停删除
- [ ] F2 节点详情页"内核"标签页（安装、状态、日志）

### 第 2 阶段：分享与订阅

- [ ] B6 `share.go`：各协议 URI + 测试
- [ ] B7 `subscription.go`：base64 / clash / singbox / surge + UA 识别 + `subscription-userinfo`
- [ ] B8 订阅 token 生成与重置
- [ ] F3 分享弹窗（二维码）、订阅卡片

### 第 3 阶段：更多内核

- [ ] B9 `render_singbox.go` + golden（含 Hysteria2、TUIC、AnyTLS、ACME）
- [ ] A4 sing-box 适配器
- [ ] B10 `render_snell.go`、Snell 每个入站一个进程的实例键
- [ ] A5 snell 适配器
- [ ] （可选）Hysteria2 官方版、shadowsocks-rust 适配器

### 第 4 阶段：与转发打通

- [ ] B11 `via_forward_id`：订阅和分享中使用转发入口的地址和端口；创建入站时可选择"同时创建一条指向它的转发"
- [ ] F4 入站编辑中选择中转转发

### 验证（每个阶段）

- [ ] `(cd go-backend && go test ./...)`
- [ ] `(cd go-gost && go build ./...)`，以及 `x` 模块的单元测试
- [ ] `(cd vite-frontend && pnpm run build && pnpm run lint)`
- [ ] 在真实节点上实测：安装内核 → 创建入站 → 客户端导入订阅 → 能正常连接 → 流量计数增长 → agent 重启后内核自动恢复

## 文件变更清单（预计）

| 文件 | 变更类型 |
|---|---|
| `go-backend/internal/store/model/node_kernel.go` | 新增 |
| `go-backend/internal/store/repo/repository.go` | 修改（AutoMigrate 列表） |
| `go-backend/internal/store/repo/node_kernel.go` | 新增（Repository 方法） |
| `go-backend/internal/kernel/*` | 新增 |
| `go-backend/internal/http/handler/kernel_*.go` | 新增 |
| `go-backend/internal/http/handler/handler.go` | 修改（路由注册） |
| `go-backend/internal/ws/server.go` | 修改（处理 `kernel_status` / `kernel_traffic` 消息） |
| `go-backend/tests/contract/kernel_*_test.go` | 新增 |
| `go-gost/x/kernel/*` | 新增 |
| `go-gost/x/socket/websocket_reporter.go` | 修改（命令路由、上报） |
| `vite-frontend/src/api/kernel.ts` | 新增 |
| `vite-frontend/src/pages/inbound.tsx`、`src/pages/inbound/*` | 新增 |
| `vite-frontend/src/pages/node/*` | 修改（内核标签页） |
| `vite-frontend/src/App.tsx`、侧边栏菜单 | 修改 |
