# 更新日志

本文件汇总每次发布的关键变更。详细发版说明见 [GitHub Releases](https://github.com/teacat99/PortPass/releases)。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [SemVer](https://semver.org/lang/zh-CN/)。

## [1.1.0] - 2026-04-25

### 新增

- **主题三态切换**：从亮/暗二态改为「跟随系统 → 浅色 → 深色」三态循环，图标分别用 SunMoon / Sun / Moon
- **密码显隐切换**：登录页与修改密码弹窗的密码输入框增加 Eye / EyeOff 显隐按钮
- **theme-color 暗色适配**：`<meta name="theme-color">` 改为双标签（light/dark media query），JS 切换时同步更新
- **i18n 补齐**：新增 `theme.light` / `theme.dark` / `theme.auto` / `theme.switchTo.*` 中英文案

### 优化

- PWA manifest `theme_color` / `background_color` 统一为浅色底色 `#f6f8fb`
- Toast 位置从 `top-right` 改为 `top-left`，减少与顶栏操作按钮的冲突

### 修复

- 移动端「立即开放」按钮与底部导航栏之间有空隙（`padding-bottom` / `bottom` 从 `4.5rem` 调整为 `3.5rem`）

### 基础设施

- CI 修复：合并前端与后端为单 job，解决 `//go:embed all:dist` 在 CI 中找不到文件的问题
- Dependabot 安全升级：`actions/setup-go` v6、`actions/setup-node` v6、`docker/metadata-action` v6、`golang` 1.26-alpine、`alpine` 3.23

### 部署

```bash
docker pull teacat99/portpass:1.1.0
docker pull ghcr.io/teacat99/portpass:1.1.0
```

数据库无变更，自动迁移无需手动操作。

## [1.0.0] - 2026-04-25

首个公开发布版本。

### 新增

- **核心规则引擎**：来源 IP / 端口 / 协议 / 有效期 / 备注，到期自动撤销
- **多防火墙驱动**：`iptables` / `nftables` / `ufw` / `firewalld` / `mock`，IPv4 + IPv6
- **生命周期可靠性**：`time.AfterFunc` 主通道 + 30s 周期对账 + 启动对账，SIGTERM 不清理规则
- **多用户体系**：管理员（可多位）+ 普通用户，bcrypt 落库；`/users` 页面管理账号；保留至少一位活跃管理员
- **端口策略**：管理员决定普通用户可见的预设端口与单条最大时长
- **三种鉴权模式**：`password`（JWT）/ `ipwhitelist` / `none`
- **登录加固**：IP + 账号双重限流、暴力破解防护、登录历史、数学验证码
- **审计日志**：所有规则变更落库 `user_id` + `created_by`，按天清理（可配置保留天数）
- **运行时参数热改**：管理员可在设置页直接调整 `MAX_DURATION_HOURS` 等参数
- **网段聚合**：多条同源 IP 规则在 UI 智能合并展示
- **ntfy 推送**：规则创建 / 即将到期 / 被清理可推送通知
- **PWA**：可安装到桌面 / 手机，Workbox 离线缓存
- **国际化**：中 / 英双语，登录页、错误码均覆盖；DateTimePicker 自适应语言
- **响应式 UI**：基于 shadcn-vue + Tailwind CSS 4，表格 ↔ 卡片自动切换，44px 触控区
- **客户端 IP 自动识别**：支持可信代理 `X-Forwarded-For` 解析

### 部署

```bash
docker pull teacat99/portpass:1.0.0
docker pull ghcr.io/teacat99/portpass:1.0.0
```

镜像约 40 MB（含 iptables/ip6tables、nftables、前端资源）。常驻内存约 40 MB。

数据库由 GORM 自动迁移，无需手动操作。

[1.1.0]: https://github.com/teacat99/PortPass/releases/tag/v1.1.0
[1.0.0]: https://github.com/teacat99/PortPass/releases/tag/v1.0.0
