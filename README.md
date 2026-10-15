# FLVX

自用的流量转发 + 节点搭建面板。在一个面板里管理中转机的端口转发 / 隧道，同时在落地机上一键部署 Xray、sing-box、Snell 节点，并把节点直接对接到中转，生成订阅即可使用。

基于 [Sagit-chu/flvx](https://github.com/Sagit-chu/flvx)（源自 [bqlpfy/flux-panel](https://github.com/bqlpfy/flux-panel)）独立维护，不与上游同步。

## 功能

### 节点搭建（落地机）

- **多内核**：Xray、sing-box、Snell v4 / v5，按节点安装，面板生成全部配置；节点端只负责下载、运行和守护（崩溃自动拉起，重启后自动恢复）
- **协议**
  - Xray：VLESS（Reality / Vision / XHTTP / WS / gRPC）、VMess、Trojan、Shadowsocks（含 2022）
  - sing-box：Hysteria2（可选端口跳跃）、TUIC v5、AnyTLS、NaïveProxy、SS2022 + ShadowTLS v3，以及 VLESS / VMess / Trojan / SS
  - Snell：v4 / v5，可套 ShadowTLS v3
- **快速模板**：选内核 → 点模板 → 保存；UUID、密码、密钥、端口留空自动生成
- **证书**：自动生成自签证书、使用节点上的证书文件，或由 sing-box 自动申请（ACME，支持 HTTP 验证和 Cloudflare / 阿里云 DNS 验证）
- **分享与订阅**：分享链接 + 二维码；订阅支持通用 base64、Clash / Mihomo、sing-box、Surge，按客户端自动识别格式
- **流量统计**：按入站端口用 nftables 计数（TCP / UDP）
- **安全**：内核和附加程序下载后强制校验 SHA256；Snell 只从官方地址下载，并使用内置的校验值

### 对接中转

- **一键对接**：点击入站 → “中转”，选择隧道即可
  - 端口转发：中转机直接转发到落地机公网地址
  - 隧道转发：经隧道到落地机后再转发到 `127.0.0.1`，入站可以只监听本机
  - 多跳：隧道出口选其他节点（如 IX），由出口节点连接落地机公网地址，实现“前置 → IX → 落地”
- **导入节点**：粘贴分享链接、Clash / sing-box / Surge 配置或订阅地址（转换规则参考 Sub-Store），把别处搭好的节点导入后同样可以对接中转、加入订阅
- 中转线路自动出现在订阅里
- **获取真实客户端 IP**（PROXY protocol，仅 Xray）
- **入站白名单**：只放行中转机 / 隧道出口节点，直连落地机端口的流量会被丢弃

### 流量转发

- TCP / UDP 端口转发与隧道转发
- 用户 / 隧道配额、指定用户指定隧道限速、单向或双向计费
- 分组权限、批量下发与启停、面板共享（面板对接面板）

## 安装

### 面板

```bash
curl -L https://github.com/kure29/flvx/releases/latest/download/panel_install.sh -o panel_install.sh && chmod +x panel_install.sh && ./panel_install.sh
```

菜单选 `1. 安装面板`。默认使用 SQLite，也可以选 PostgreSQL。安装完成后访问 `http://服务器IP:6366`。

默认管理员：`admin_user` / `admin_user`，**首次登录后请立即修改**。

### 节点

先在面板 **网站配置** 里填好“面板后端地址”，然后在 **节点** 页面新建节点，复制面板生成的安装命令，到节点服务器上执行。

也可以手动安装：

```bash
curl -L https://github.com/kure29/flvx/releases/latest/download/install.sh -o install.sh && chmod +x install.sh && ./install.sh
```

> 节点需要 Linux（amd64 / arm64）和 root 权限。落地机建议安装 nftables，流量统计、白名单和端口跳跃都依赖它。

### 指定版本

到 [Releases](https://github.com/kure29/flvx/releases) 复制对应版本的命令，例如：

```bash
curl -L https://github.com/kure29/flvx/releases/download/3.0.1/panel_install.sh -o panel_install.sh && chmod +x panel_install.sh && ./panel_install.sh
```

## 快速上手：搭一个节点并对接中转

1. **节点**：分别添加落地机和中转机，执行安装命令，等状态变为在线
2. **入站 → 内核 → 安装内核**：在落地机上安装 Xray（或 sing-box / Snell）
3. **入站 → 新建入站**：选择落地机和内核，点一个模板（推荐 VLESS + Reality + Vision），保存后自动下发
4. **中转**（可选）：点击入站 → “中转”，选择隧道
5. **订阅**：在入站页的“订阅”标签复制订阅链接导入客户端，直连和中转线路都在里面

详细说明见 [节点搭建指南](doc/nodes.md)。

## 升级

- **面板内升级**：网站配置 → 面板自升级 → 检查更新 → 立即升级
- **脚本升级**：重新运行上面的面板安装命令，选 `2. 更新面板`（数据会保留，更新前自动备份）

升级后如果页面没有变化，通常是浏览器或 CDN 缓存，刷新或清理缓存即可。

## 文档

- [节点搭建指南](doc/nodes.md)
- [安装部署](doc/install.md)
- [使用指南](doc/usage.md)
- [PostgreSQL（部署与从 SQLite 迁移）](doc/postgresql.md)
- [通行证密钥登录](doc/passkey.md)
- [常见问题](doc/faq.md)

## 开发

```bash
# 后端（默认监听 :6365）
(cd go-backend && go run ./cmd/paneld)
(cd go-backend && go test ./...)

# 前端（0.0.0.0:3000）
(cd vite-frontend && pnpm install && pnpm run dev)

# 节点 agent
(cd go-gost && go run .)
```

发版：在 GitHub Actions 里运行 **Build and Push Images**，填写版本号（如 `3.0.2`），会自动构建镜像和二进制并创建 Release。

## 来源与许可

- 原项目：[bqlpfy/flux-panel](https://github.com/bqlpfy/flux-panel)，Apache License 2.0
- 上游：[Sagit-chu/flvx](https://github.com/Sagit-chu/flvx)，在原项目基础上用 Go 重写后端、重构前端，修改部分以 GPLv3 发布
- 本仓库：在上游基础上增加节点内核 / 入站管理与中转对接、移除商业授权等，继续以 GPLv3 发布

完整说明见 [NOTICE](NOTICE) 和 [LICENSE](LICENSE)。

## 免责声明

本项目仅供个人学习与研究使用，仅限合法、合规用途。使用者须确保其使用行为符合所在国家或地区的法律法规，并自行承担使用本项目带来的一切风险，包括但不限于配置不当导致的服务异常、服务器被入侵或滥用造成的损失，以及违反当地法律法规产生的法律责任。

作者不对因使用本项目导致的任何法律责任、经济损失或其他后果承担责任，亦不提供任何形式的担保。禁止将本项目用于网络攻击、数据窃取、非法访问等任何违法或未经授权的行为。如不同意上述条款，请勿使用本项目。
