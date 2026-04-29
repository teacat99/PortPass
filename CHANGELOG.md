# 更新日志

本文件汇总每次发布的关键变更。详细发版说明见 [GitHub Releases](https://github.com/teacat99/PortPass/releases)。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [SemVer](https://semver.org/lang/zh-CN/)。

## [1.1.3] - 2026-04-30

### 修复

- **iptables 后端不一致导致规则不生效**：在 CentOS 7 / RHEL 7 / Ubuntu 18.04 等仍以 `iptables-legacy (xtables)` 为活跃后端的宿主机上，Alpine 3.18+ 镜像默认 `iptables` 实际是 `iptables-nft`，PortPass 写入的 ACCEPT 规则虽然命令成功且 `-C` 自验通过，但写到的是与宿主机互不相通的另一张 nft 表，数据包根本不会经过 → UI 显示"规则已生效"，外网访问仍然被宿主机 firewalld 的 INPUT 末尾 `REJECT` 拦截

### 新增

- **iptables 后端自动适配**：运行时镜像同时打入 `iptables-legacy` 与 `iptables-nft` 两套二进制，新增 `docker-entrypoint.sh`：启动时探测宿主机活跃后端（识别 firewalld / ufw / docker 创建的标志性链 `INPUT_ZONES` / `ufw-input` / `DOCKER-USER` 等落在哪一套表里），自动把容器内的 `iptables` / `iptables-save` / `iptables-restore` / `ip6tables*` 软链到对应实现；日志输出 `[portpass-entrypoint] iptables backend = legacy/nft`
- **`PORTPASS_IPTABLES_BACKEND` 环境变量**：手动覆盖自动探测，取值 `legacy` / `nft`，留空走自动逻辑
- **README 系统兼容性矩阵**：中英文 README 新增主流发行版（CentOS 7+/RHEL 7+/Debian 10+/Ubuntu 18.04+/Fedora/OpenWrt 等）默认防火墙、iptables 后端与推荐 PortPass 驱动的对照表

### 部署

```bash
docker pull teacat99/portpass:1.1.3
docker pull ghcr.io/teacat99/portpass:1.1.3
```

无需任何配置变更：默认行为即"自动选择与宿主机一致的 iptables 后端"。已部署在 CentOS 7 / RHEL 7 / Ubuntu 18.04 等老主机的用户**强烈建议升级**，否则 UI 看上去成功的规则在外网视角实际未生效。

数据库无变更，自动迁移无需手动操作。

## [1.1.2] - 2026-04-26

### 新增

- **预设端口分类与图标管理**：预设端口编辑弹窗新增「分类」下拉，支持选择内置分类（远程登录 / Web 服务 / 数据库 / 消息队列 / 游戏服务 / 其他端口）、自定义新增分类、改名 / 改图标、删除自定义分类（二次确认；删除后引用该分类的预设回退为自动识别）
- **图标选择器**：双 Tab 设计，常用 emoji 网格（30+ 精选）+ 图片 URL（`favicon.im` / `t1.gstatic.com/faviconV2` 等返回站点图标的链接，自动圆角渲染）
- **PWA 移动端返回手势 = 关闭弹窗**：在弹窗打开时按系统返回手势 / 浏览器返回按钮优先关闭最顶层弹窗而非退出页面，符合主流 App 心理预期；支持多层嵌套弹窗按 LIFO 顺序逐层关闭

### 优化

- 预设保留按名称 / 端口的自动识别启发式作为 fallback：未手动选择分类时仍按现有规则归类
- 首页与设置页图标渲染统一支持 emoji 与图片 URL 混排

### 部署

```bash
docker pull teacat99/portpass:1.1.2
docker pull ghcr.io/teacat99/portpass:1.1.2
```

数据库会自动迁移：新增 `preset_categories` 表，`preset_ports` 增加可空字段 `category_id`。无需手动干预。

## [1.1.1] - 2026-04-26

### 优化

- **全面多语言适配**：设置页概览卡片、Tab 标签、用户/预设/受保护端口管理面板、历史页面重置/分页按钮、规则页面搜索/空状态文案，全部从硬编码中文改为 i18n 动态切换
- **API 错误消息本地化**：拦截器将后端英文错误消息（`duration exceeds`、`rate limit exceeded` 等）映射为前端 i18n 翻译，中英文均可正确展示
- **通知弹窗优化**：位置从左上角改为右上角；缩小 padding 使高度低于顶栏；关闭按钮移至消息窗口右侧
- **首页文案精简**：删除 IP 卡片的冗余描述文案；"只允许我（当前 IP）"简化为"只允许我"，中英文同步适配

### 部署

```bash
docker pull teacat99/portpass:1.1.1
docker pull ghcr.io/teacat99/portpass:1.1.1
```

数据库无变更，自动迁移无需手动操作。

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

[1.1.3]: https://github.com/teacat99/PortPass/releases/tag/v1.1.3
[1.1.2]: https://github.com/teacat99/PortPass/releases/tag/v1.1.2
[1.1.1]: https://github.com/teacat99/PortPass/releases/tag/v1.1.1
[1.1.0]: https://github.com/teacat99/PortPass/releases/tag/v1.1.0
[1.0.0]: https://github.com/teacat99/PortPass/releases/tag/v1.0.0
