# FLVX 文档

**FLVX** 是自用的流量转发 + 节点搭建面板：管理中转机的端口转发 / 隧道，同时在落地机上部署 Xray、sing-box、Snell 节点，并一键对接中转。转发部分基于 [go-gost/gost](https://github.com/go-gost/gost) 和 [go-gost/x](https://github.com/go-gost/x)。

## 核心特性

- **节点搭建**：Xray、sing-box、Snell v4 / v5，覆盖 VLESS Reality、Hysteria2、TUIC、AnyTLS、NaïveProxy、ShadowTLS 等协议，模板一键创建
- **证书**：自签、证书文件，或由 sing-box 自动申请（ACME）
- **分享与订阅**：分享链接 + 二维码；订阅支持 base64、Clash / Mihomo、sing-box、Surge
- **对接中转**：端口转发或隧道转发，中转线路自动进入订阅；可获取真实客户端 IP、只放行中转机
- **流量转发**：TCP / UDP 端口转发与隧道转发，配额、限速、单向或双向计费

## 快速开始

- [安装部署](./install.md)
- [节点搭建指南](./nodes.md)
- [使用指南](./usage.md)
- [PostgreSQL 数据库指南](./postgresql.md)
- [通行证密钥登录](./passkey.md)
- [AI Skill 接入](./ai-skill.md) - 让大模型直接操作面板
- [常见问题](./faq.md)

## 免责声明

本项目仅供个人学习与研究使用，基于开源项目进行二次开发。
使用本项目所带来的任何风险均由使用者自行承担。本项目仅限合法、合规用途。
**禁止将本项目用于任何违法或未经授权的行为，包括但不限于网络攻击、数据窃取、非法访问等。**
