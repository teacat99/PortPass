# PortPass 开发计划

> 按需临时开放服务器端口的自助管理工具。通过 Web 页面手动下发"白名单 IP + 端口 + 有效期"规则，到期自动回收，避免长期暴露公网端口的安全风险。

---

## 项目定位

- **场景**：自用服务器上存在非 7×24 小时需要开放的服务（RDP、SSH、数据库端口、游戏服务器等），希望在需要时"临时开一会儿"，用完自动关闭。
- **替代方案对比**：
  - 传统 port knocking：需要客户端工具，不够直观
  - VPN：配置成本高，偶尔访问不值当
  - 手改 iptables：容易忘记关闭，留下安全隐患
- **PortPass 的定位**：**用网页当"临时钥匙"**，点一下开门，到点自动锁门。

---

## 核心需求

### 规则要素

每条端口开放规则包含以下参数：

| 参数 | 说明 | 备选 |
|---|---|---|
| 来源 IP | 白名单 IP，默认自动获取当前客户端 IP | 支持 `全部 IP (0.0.0.0/0)` 模式（IP 不稳定时使用） |
| 端口 | 要开放的端口号 | 支持手动输入，也提供预设快捷按钮（如 22/3389/3306/5432 等） |
| 协议 | TCP / UDP / TCP+UDP | 默认 TCP |
| 有效期 | 规则存续时长 | 支持"XX 分钟/小时后到期"快捷选项，也支持手动选择到期时刻 |
| 备注 | 可选文字描述 | 用于识别该规则用途 |

### 页面功能

1. **首页 / 创建规则页**
   - 自动检测并展示当前客户端公网 IP
   - 来源 IP 选择：`使用当前 IP` / `全部 IP` / `手动输入`
   - 端口：输入框 + 预设按钮组（SSH 22、RDP 3389、MySQL 3306、PostgreSQL 5432、Redis 6379、HTTP 80、HTTPS 443 等，支持后端配置扩展）
   - 时长：快捷按钮（15 分钟 / 1 小时 / 4 小时 / 12 小时 / 24 小时）+ 自定义到期时刻
   - 提交后立即展示规则状态和剩余时间

2. **规则列表页**
   - 展示所有活跃规则：ID、来源 IP、端口、协议、剩余时间、创建时间、备注
   - 支持操作：**提前终止**、**延长有效期**、**复制规则**
   - 已过期 / 已撤销的规则保留在历史记录中（可配置保留天数）

3. **历史记录页**
   - 审计日志：谁（会话 / IP）在何时开了什么规则，持续了多久
   - 可按时间 / 端口 / IP 过滤

4. **设置页**
   - 预设端口管理
   - 默认时长配置
   - 可信代理配置（用于从 `X-Forwarded-For` 正确解析客户端 IP）
   - 访问鉴权配置（见下文）

---

## 架构与技术选型

### 技术栈

```
后端:   Go 1.22+ + Gin
ORM:    GORM
存储:   SQLite (modernc.org/sqlite，纯 Go 免 CGO)
前端:   Vue 3 + Vite + TypeScript + vite-plugin-pwa
UI:     Element Plus（与 SubForge 风格一致） 或 Naive UI
打包:   Go embed 前端 dist/ 到单二进制
部署:   Docker 单容器（--cap-add=NET_ADMIN --network=host）
目标:   镜像 < 30MB，常驻内存 < 50MB
```

### 防火墙抽象

设计 `FirewallDriver` 接口，支持多后端以适配不同环境：

- `iptables` / `ip6tables`（默认，覆盖面最广）
- `nftables`（现代发行版）
- `ufw`（Ubuntu 场景）
- `firewalld`（RHEL / CentOS 场景）

所有下发规则统一打 comment 标签（如 `portpass:<ruleID>`），便于识别、清理，避免误伤用户已有规则。

### 规则生命周期管理（**核心，避免失效**）

这是本项目最关键的可靠性点，必须做到"无论什么情况规则都不会悬挂"。

1. **持久化优先**
   - 每条规则落库 SQLite：`id / source_ip / port / protocol / expire_at / status / driver_ref / comment_tag / created_by / created_at`
   - 状态机：`pending` → `active` → `expired` / `revoked` / `failed`

2. **启动对账（reconcile on boot）**
   - 服务重启时扫描数据库：
     - 状态为 `active` 但已过期 → 立即清理防火墙并标记 `expired`
     - 状态为 `active` 且未过期 → 检查防火墙中是否存在对应规则，不存在则重新下发
     - 防火墙中存在 `portpass:` 标签但数据库无记录 → 清理（孤儿规则）

3. **定时 reconcile**
   - 每 30 秒扫描一次，处理：
     - 时钟漂移 / sleep 导致的过期漏处理
     - 手动改 iptables 被篡改的情况
     - 下发失败的重试

4. **双写校验**
   - 下发规则后立即回读防火墙确认生效，失败则标记 `failed` 并告警

5. **优雅退出**
   - 收到 SIGTERM 时**不清理规则**（容器重启不应导致规则丢失）
   - 只保存状态，依赖启动对账恢复

6. **精确到期**
   - 每条规则启动时用 `time.AfterFunc` 注册到期清理任务，作为主通道
   - 定时 reconcile 作为兜底

### 鉴权设计

管理界面本身必须受保护，否则等于把防火墙控制权暴露在公网。

- **模式 A（推荐）**：访问令牌 / 管理员密码登录，登录态走 JWT 或 Session
- **模式 B**：IP 白名单模式（仅允许预置 IP 访问管理页）
- **模式 C**：公益 / 本地模式（无鉴权，仅用于内网部署）
- 环境变量控制启用哪种模式，支持多模式共存（参考 SubForge 的登录模式设计）

### 数据模型草图

```
rules
  id              INTEGER PRIMARY KEY
  source_ip       TEXT          -- CIDR 或 "0.0.0.0/0"
  port            INTEGER
  protocol        TEXT          -- tcp / udp / both
  note            TEXT
  status          TEXT          -- pending/active/expired/revoked/failed
  expire_at       DATETIME
  created_by      TEXT          -- 会话标识或用户
  created_ip      TEXT          -- 创建者 IP（审计用）
  created_at      DATETIME
  terminated_at   DATETIME
  driver_name     TEXT          -- iptables/nftables/ufw
  driver_ref      TEXT          -- 驱动层返回的引用（便于清理）

preset_ports
  id / name / port / protocol / order

settings
  key / value
```

---

## 前端 PWA 需求

目标：让网页能安装到手机 / 桌面，打开即用，带本地缓存加速。

- **Manifest**：`name / short_name / icons(192/512/maskable) / theme_color / display: standalone`
- **Service Worker**（通过 `vite-plugin-pwa` + Workbox）：
  - 静态资源（JS / CSS / 字体 / 图标）→ CacheFirst
  - API 请求（`/api/rules` 等）→ NetworkFirst，离线时回退到最后一次响应
  - 新版本检测 → 提示用户刷新
- **移动端适配**：
  - 响应式布局，触控友好
  - 顶部状态条颜色适配 `theme_color`
  - 支持横竖屏
- **注意事项**：
  - PWA 安装要求 HTTPS（`localhost` 除外），部署文档需明确说明
  - iOS 支持"添加到主屏幕"但无推送通知（不影响核心功能）
  - 客户端 IP 检测：当走反代时，后端需正确从 `X-Forwarded-For` / `X-Real-IP` 解析

---

## 部署与运维

### Docker 镜像

- 基础镜像：`alpine` 或 `scratch`（需内置 iptables 则用 alpine + 安装 iptables）
- 运行需要 `NET_ADMIN` capability 或 `--network=host`
- 持久化目录：`/data`（SQLite 数据库）

```yaml
# 示意 docker-compose.yml
services:
  portpass:
    image: teacat/portpass:latest
    container_name: portpass
    restart: unless-stopped
    network_mode: host
    cap_add:
      - NET_ADMIN
    volumes:
      - ./data:/data
    environment:
      - PORTPASS_AUTH_MODE=password
      - PORTPASS_ADMIN_PASSWORD=xxx
      - PORTPASS_LISTEN=:8080
      - PORTPASS_TRUSTED_PROXIES=10.0.0.0/8
      - PORTPASS_FIREWALL_DRIVER=iptables
```

### 环境变量列表（初版）

| 变量 | 说明 | 默认 |
|---|---|---|
| `PORTPASS_LISTEN` | 监听地址 | `:8080` |
| `PORTPASS_AUTH_MODE` | `password` / `ipwhitelist` / `none` | `password` |
| `PORTPASS_ADMIN_PASSWORD` | 管理员密码（`password` 模式必填） | - |
| `PORTPASS_ADMIN_IP_WHITELIST` | 管理端 IP 白名单（`ipwhitelist` 模式） | - |
| `PORTPASS_TRUSTED_PROXIES` | 可信反代 CIDR（逗号分隔） | - |
| `PORTPASS_FIREWALL_DRIVER` | `iptables` / `nftables` / `ufw` / `firewalld` | `iptables` |
| `PORTPASS_DATA_DIR` | 数据目录 | `/data` |
| `PORTPASS_HISTORY_RETENTION_DAYS` | 历史记录保留天数 | `30` |
| `PORTPASS_MAX_DURATION_HOURS` | 单条规则最大有效期上限 | `24` |

---

## 开发任务拆分

### 里程碑 M1：后端核心（可用 CLI / API 验证）

- [ ] 项目骨架：Go module、Gin 路由、SQLite 初始化、配置加载
- [ ] 数据模型与 GORM 迁移
- [ ] `FirewallDriver` 接口定义 + `iptables` 驱动实现（含 comment 标签）
- [ ] 规则 CRUD API：创建、列表、查询、提前终止、延长
- [ ] 生命周期管理：`time.AfterFunc` 到期清理 + 30s reconcile 兜底
- [ ] 启动对账逻辑
- [ ] 客户端 IP 解析（支持 `X-Forwarded-For`、可信代理配置）
- [ ] 审计日志

### 里程碑 M2：前端 Web UI

- [ ] Vue3 + Vite 项目初始化 + Element Plus 接入
- [ ] 路由与布局：首页 / 列表 / 历史 / 设置
- [ ] 创建规则表单（IP 自动检测 / 端口预设 / 时长快捷）
- [ ] 规则列表（实时剩余时间、操作按钮）
- [ ] 历史记录与过滤
- [ ] 设置页（预设端口管理、参数配置）
- [ ] 多语言支持（中 / 英，参考 SubForge）
- [ ] 移动端适配

### 里程碑 M3：PWA 与打包

- [ ] `vite-plugin-pwa` 接入 + manifest + 图标全套
- [ ] Service Worker 缓存策略调优
- [ ] 前端 dist 通过 Go `embed` 打包进单二进制
- [ ] 安装/更新提示 UI

### 里程碑 M4：鉴权与多驱动

- [ ] 密码鉴权 + JWT 会话
- [ ] IP 白名单鉴权
- [ ] `nftables` 驱动
- [ ] `ufw` 驱动
- [ ] `firewalld` 驱动（可选）
- [ ] IPv6 支持（`ip6tables`）

### 里程碑 M5：发布与文档

- [ ] Dockerfile（多阶段构建，目标镜像 < 30MB）
- [ ] GitHub Actions：tag 自动构建并推送 Docker 镜像（参考 SubForge 发版流程）
- [ ] README（中 / 英双语），含部署示例与变量列表
- [ ] 资源占用基准测试与文档
- [ ] `.cursor/skills/portpass-release/` 发版 Skill

---

## 安全注意事项

1. **防横向越权**：规则操作必须验证会话，防止未授权用户删除/修改他人规则（如果启用多用户）
2. **防重放**：创建规则的接口应有幂等或频率限制，避免恶意刷
3. **规则上限**：单客户端 IP 并发规则数限制、单日创建次数限制（防刷）
4. **最大时长限制**：通过 `PORTPASS_MAX_DURATION_HOURS` 强制约束，避免"永久开放"
5. **日志完整**：所有规则变更写入审计日志，至少保留 N 天
6. **管理端必须 HTTPS**：文档明确提示不要用 HTTP 暴露到公网

---

## 待决策 / 待讨论

- [ ] 是否支持"规则模板"（一键应用常用组合）
- [ ] 是否支持 Webhook / 通知（规则创建、即将到期、被清理时）
- [ ] 是否支持多用户（不同用户各自管理自己的规则）
- [ ] 是否需要提供 CLI 工具（用于脚本化创建规则）
- [ ] 前端是否需要图形化展示"规则时间轴"
