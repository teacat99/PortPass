# PortPass

> 按需临时开放服务器端口的自助管理工具 — 网页当"临时钥匙"，点一下开门，到点自动锁门。

[English](./README.en.md) | 中文

## 远程仓库

- **当前状态**：仅本地仓库，暂未关联远程
- **建议远端**：`github.com/teacat99/PortPass`
- 开通远程仓库后，可通过：
  ```bash
  git remote add origin git@github.com:teacat99/PortPass.git
  git push -u origin main
  ```

## 项目简介

PortPass 是一个轻量级的临时端口开放管理工具，解决"非 7×24 小时需要暴露的服务如何安全开放"这一痛点：

- 通过 Web 页面（支持 PWA 安装到手机桌面）下发"白名单 IP + 端口 + 有效期"规则
- 到期**自动回收**，避免手改 iptables 后忘记关闭造成的长期暴露风险
- 支持 iptables / nftables / ufw / firewalld 多种防火墙后端
- 规则持久化 + 启动对账 + 定时 reconcile，确保"无论什么情况规则都不会悬挂"
- 单 Docker 容器部署，镜像目标 < 30MB，常驻内存 < 50MB

## 核心特性

- ✅ **临时端口开放**：指定来源 IP / 端口 / 协议 / 有效期，到期自动清理
- ✅ **自动 IP 识别**：页面自动填入当前客户端公网 IP（支持反代 `X-Forwarded-For`）
- ✅ **预设快捷操作**：SSH / RDP / MySQL 等常用端口一键选择
- ✅ **规则生命周期可靠性**：AfterFunc 到期 + 30s reconcile + 启动对账
- ✅ **PWA 支持**：可安装到手机/桌面，离线缓存
- ✅ **多鉴权模式**：密码 + JWT / IP 白名单 / 无鉴权（内网）
- ✅ **审计日志**：所有规则变更可追溯

## 开发进度

项目按 M1-M5 里程碑推进，当前位于 **M0：项目骨架**。详见 [plan.md](./plan.md)。

## 开发与构建（预留）

完整 README 将在 M5 里程碑完善，包括：

- 快速开始（Docker / docker-compose）
- 完整环境变量表
- 防火墙驱动选择建议
- 安全最佳实践
- 开发者指南

## License

[MIT](./LICENSE)（M5 里程碑添加）
