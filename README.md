# PortPass

> 按需临时开放服务器端口的自助管理工具 — 网页当"临时钥匙"，点一下开门，到点自动锁门。

[English](./README.en.md) | 中文

## 快速开始

### Docker Compose（推荐）

```bash
curl -LO https://raw.githubusercontent.com/teacat99/PortPass/main/docker-compose.yaml
# 可选：在 docker-compose.yaml 里设置 PORTPASS_ADMIN_PASSWORD=<种子密码>
# 若未设置，首次启动会自动创建默认管理员：admin / passwd（请登录后立即修改）
docker compose up -d
```

访问 `http://<server>:8080`，使用默认的 `admin / passwd`（或你设置的种子密码）登录。

> ⚠️ **首次登录即改密**：默认 `admin / passwd` 仅用于引导，登录后请在右上角的"修改密码"菜单里立刻更换，后续可在 **用户管理** 页面创建更多管理员/普通用户。

### 单命令 Docker

```bash
docker run -d \
  --name portpass \
  --restart unless-stopped \
  --network host \
  --cap-add NET_ADMIN \
  -v $PWD/data:/data \
  # 可选：设置种子密码；省略则首次启动回落到 admin/passwd
  -e PORTPASS_ADMIN_PASSWORD="change-me" \
  ghcr.io/teacat99/portpass:latest
```

> **注意**：`--network host` 与 `--cap-add NET_ADMIN` 是必需的。容器中的 iptables 只能影响容器自己的网络命名空间，除非使用 host 网络。

### 从源码构建

```bash
# 前端
cd frontend && npm ci && npm run build && cd ..
# 后端（含 embed 前端）
go build -trimpath -ldflags="-s -w" -o portpass ./cmd/server/
PORTPASS_ADMIN_PASSWORD=dev ./portpass
```

## 镜像与代码

- **代码**：[github.com/teacat99/PortPass](https://github.com/teacat99/PortPass)
- **镜像（推荐）**：
  - Docker Hub：`teacat99/portpass:latest`
  - GHCR：`ghcr.io/teacat99/portpass:latest`
- **发版规则**：以 `vX.Y.Z` tag 触发 GitHub Actions，自动构建 `linux/amd64` + `linux/arm64` 镜像并推送到上述两个仓库

## 核心特性

- ✅ **临时端口开放**：指定来源 IP / 端口 / 协议 / 有效期，到期自动清理
- ✅ **客户端 IP 自动识别**：首页自动填入当前公网 IP（支持 `X-Forwarded-For`）
- ✅ **预设快捷端口**：SSH / RDP / MySQL / Redis 等一键选择
- ✅ **规则生命周期可靠性**：`time.AfterFunc` 主通道 + 30s 周期对账 + 启动对账
- ✅ **多防火墙驱动**：iptables / nftables / ufw / firewalld，IPv4+IPv6
- ✅ **PWA**：可安装到手机桌面，Workbox 离线缓存
- ✅ **多鉴权模式**：密码 + JWT / IP 白名单 / 无鉴权（内网）
- ✅ **多用户体系**：管理员（可多位）+ 普通用户；管理员在 UI 管理账号，密码落库 bcrypt
- ✅ **端口策略**：管理员决定普通用户可开放的预设端口与单条最大时长
- ✅ **审计日志**：所有规则变更可追溯（记录 `user_id` + `created_by`）
- ✅ **中英双语 + 移动端自适应**（表格 ↔ 卡片，表单堆叠，44px 触控区）

## 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `PORTPASS_LISTEN` | `:8080` | HTTP 监听地址 |
| `PORTPASS_AUTH_MODE` | `password` | `password` / `ipwhitelist` / `none` |
| `PORTPASS_ADMIN_USERNAME` | `admin` | 种子管理员的用户名（仅首次启动生效） |
| `PORTPASS_ADMIN_PASSWORD` | `passwd` | 种子管理员的密码；首次启动未设置时落到 `passwd` 并打印告警，后续在 UI 管理 |
| `PORTPASS_ADMIN_IP_WHITELIST` | —— | 逗号分隔的 CIDR 列表，`ipwhitelist` 模式下必填 |
| `PORTPASS_TRUSTED_PROXIES` | —— | 反代 CIDR；配置后才解析 `X-Forwarded-For` |
| `PORTPASS_FIREWALL_DRIVER` | `iptables` | `iptables` / `nftables` / `ufw` / `firewalld` / `mock` |
| `PORTPASS_IPTABLES_BACKEND` | 自动探测 | 仅在 `iptables` 驱动下生效；`legacy` / `nft`，留空则按宿主机活跃后端自动选 |
| `PORTPASS_DATA_DIR` | `/data` | SQLite 与日志目录 |
| `PORTPASS_JWT_SECRET` | 随机 | 为空则每次启动重新生成（旧 token 失效） |
| `PORTPASS_MAX_DURATION_HOURS` | `24` | 单条规则最大有效期 |
| `PORTPASS_HISTORY_RETENTION_DAYS` | `30` | 审计日志保留天数 |
| `PORTPASS_MAX_RULES_PER_IP` | `20` | 同一创建者并发规则上限 |
| `PORTPASS_RATELIMIT_PER_MINUTE` | `10` | 每 IP 每分钟创建速率 |

## 多用户与端口策略

PortPass 把账号体系全部落库（bcrypt hash），鉴权模式只是决定"用哪条身份访问 API"：

| 鉴权模式 | 身份来源 | 谁能管理账号 |
| --- | --- | --- |
| `password` | 登录表单校验 DB 中的用户 | 任一管理员 |
| `ipwhitelist` | 命中白名单即以**内置系统管理员**身份登入 | 同上（可在 UI 继续建/改账号） |
| `none` | 无鉴权，全部请求视为系统管理员 | 同上（仅建议内网使用） |

**管理员规则**：

1. 首次启动若未设置 `PORTPASS_ADMIN_PASSWORD`，自动创建 `admin / passwd`，日志打印告警，请立即修改。
2. 支持**多位管理员**并行存在，任一管理员可在 `/users` 页面创建/重置/禁用账号。
3. 管理员**不能删除或降级/禁用自身**（API 返回 `400 cannot modify ... on self`）。
4. 系统**始终保留至少一位活跃管理员**，删除 / 降级 / 禁用"最后一位 admin"的请求被拒绝。
5. 删除用户时，该用户名下所有活跃规则会被一并撤销（防火墙条目同步清理）。

**端口策略**（管理员 → 普通用户）：

- 在 **设置 → 预设端口** 里为每条预设勾选 `user_allowed` 和 `max_duration_sec`。
- 普通用户只能看到并选择被标记 `user_allowed=true` 的预设。
- 普通用户创建/续期规则时，`duration_sec` 若超过预设的 `max_duration_sec`，返回 `400 duration exceeds allowed ...`。
- 管理员不受端口策略限制，且可在 UI 按用户过滤规则列表（`GET /api/rules?user_id=<id>`）。

## 防火墙驱动选择

| 驱动 | 适用场景 | 注意事项 |
| --- | --- | --- |
| `iptables` | 通用 Linux，默认选择 | 需要 `iptables` 命令 + `NET_ADMIN`；IPv6 通过 `ip6tables` 自动处理；容器启动时自动选用与宿主机一致的 legacy / nft 后端 |
| `nftables` | 新发行版（Debian 11+、RHEL 9+、Ubuntu 22+） | PortPass 独占 `inet portpass` table，与操作员规则互不干扰 |
| `ufw` | 已启用 ufw 的 Ubuntu | PortPass 规则以 `# portpass:<id>` 注释可见 |
| `firewalld` | RHEL / CentOS / Fedora | 使用 `firewall-cmd --add-rich-rule`（运行时，不写入 permanent） |
| `mock` | 开发/测试 | 仅内存状态，不操作真实防火墙 |

### iptables 后端自动适配

容器底镜像 Alpine 3.23 同时打入了 **`iptables-legacy`** 与 **`iptables-nft`** 两套二进制。容器启动时（`docker-entrypoint.sh`）会自动探测宿主机活跃的 netfilter 后端，识别 firewalld / ufw / docker 创建的标志性链（如 `INPUT_ZONES`、`ufw-input`、`DOCKER-USER`）落在哪一套表里，再把 `iptables` / `iptables-save` / `iptables-restore` / `ip6tables*` 全部软链到对应实现。日志会打印一行：

```
[portpass-entrypoint] iptables backend = legacy (auto-detected)
```

无需任何配置即可在 CentOS 7（legacy）和 Debian 12（nft）上同时正常工作。如需手动覆盖（例如调试），传入环境变量：

```bash
docker run ... -e PORTPASS_IPTABLES_BACKEND=legacy ghcr.io/teacat99/portpass:latest
# 取值：legacy / nft；其他值会回退为 nft
```

> **为什么需要这个**：宿主机如果是 CentOS 7 + firewalld，规则全在 iptables-legacy（xtables）那张表上，`INPUT` 链最后一条是 `REJECT`；而 Alpine 3.18+ 镜像默认的 `iptables` 实际是 `iptables-nft`，写到 nft 那张独立表里 —— 自查能成功，但数据包根本不会经过这张表，结果就是"UI 显示规则已生效，外网仍然连不上"。

## 主机系统兼容性

PortPass 仅支持 Linux 主机（Windows / macOS / *BSD 不在支持范围）。下表列出常见发行版的默认情况，使用 `iptables` 驱动 + 自动后端探测时**无需任何额外配置**即可正常工作。

| 发行版 | 内核 | 默认防火墙 | iptables 后端 | 推荐 PortPass 驱动 |
| --- | --- | --- | --- | --- |
| CentOS 7 / RHEL 7 | 3.10 | firewalld | **legacy** | `iptables` |
| CentOS 8 / RHEL 8 / Rocky 8 / AlmaLinux 8 | 4.18 | firewalld（默认 nft） | nft | `iptables` 或 `nftables` |
| RHEL 9 / Rocky 9 / AlmaLinux 9 / Stream 9 | 5.14 | firewalld（默认 nft） | nft | `iptables` 或 `nftables` |
| Fedora 36+ | 5.17+ | firewalld（nft） | nft | `iptables` 或 `nftables` |
| Debian 10 (Buster) | 4.19 | nftables（首个默认 nft 的 Debian） | nft | `iptables` 或 `nftables` |
| Debian 11 / 12 | 5.10 / 6.1 | nftables | nft | `iptables` 或 `nftables` |
| Ubuntu 18.04 LTS | 4.15 | ufw（legacy） | **legacy** | `iptables` 或 `ufw` |
| Ubuntu 20.04 LTS | 5.4 | ufw（legacy 默认，可切 nft） | 取决于 `update-alternatives` | `iptables` 或 `ufw` |
| Ubuntu 22.04 / 24.04 LTS | 5.15 / 6.8 | ufw（nft） | nft | `iptables` 或 `ufw` 或 `nftables` |
| OpenWrt 21.02 及之前 | 5.4 及之前 | iptables（fw3） | **legacy** | `iptables` |
| OpenWrt 22.03+ | 5.10+ | nftables（fw4） | nft | `nftables` |

> 表中"iptables 后端"特指容器需要选择的后端。**legacy** 行均依赖容器的 entrypoint 自动探测，请保持镜像版本 ≥ `v1.1.3`。

## 可靠性设计

1. **主通道**：每条规则通过 `time.AfterFunc` 在 `expire_at` 精确触发移除
2. **周期对账**：每 30s 扫描 DB × 实时防火墙状态，修复以下漂移
   - 已过期但仍存活的规则（例如 `AfterFunc` 因进程睡眠错过触发）
   - DB 中存在、防火墙中丢失（例如操作员手动执行过 `iptables -F`）
   - 防火墙中存在、DB 中无（孤儿清理）
3. **启动对账**：HTTP 服务启动前先做一次完整对账，避免刚启动时状态不一致
4. **SIGTERM 不清理**：容器重启不会被视为"撤销"，下次启动对账会恢复计时器

## 启用「到期断开旧连接」的部署前提

PortPass 的"到期断开旧连接"（`cleanup_on_expire`）功能调用宿主机的 `conntrack -D` 精确删除目标 `(源 IP, 目的端口, 协议)` 三元组的连接跟踪条目，**不会**误删其他防火墙规则放行的连接。但 Linux 连接跟踪的语义决定了：**只有清掉条目还不够，必须配合"无放行规则时数据包会被丢弃"才能真正切断已建立的 TCP/UDP 连接**。请按以下两点检查生产环境：

1. **容器层面**：使用 PortPass 官方镜像 ≥ `v1.x.y`（已内置 `conntrack-tools`），自建镜像则需要在 alpine 上 `apk add conntrack-tools`。
2. **宿主防火墙层面**：宿主机的 `INPUT` 链应该具备**默认丢弃 / 末尾兜底 DROP** 的语义，例如：
   - CentOS / RHEL + firewalld：默认 zone 末尾 `REJECT --reject-with icmp-host-prohibited` 即满足
   - Ubuntu + ufw：开启 `ufw default deny incoming` 即满足
   - 自管 iptables：`iptables -P INPUT DROP` 或末尾 `iptables -A INPUT -j DROP`
   - 云 ECS 单纯依赖安全组：在 PortPass 触发 cleanup 后，已有连接的下一条数据包还会被云安全组的状态检测放行，这种环境下 conntrack 清理会被快速重建抵消，请额外在主机层加兜底 DROP

> 如果不满足第 2 点，触发清理后客户端在内核 conntrack 表里短暂消失，但下一个数据包仍然能进 INPUT，立刻又会建出新的跟踪条目 —— UI 上 `已断开 N 条旧连接` 的提示是真实的（kernel 表项确实被删了），但你看到的是"连接立刻自愈"。这是 Linux 连接跟踪的固有语义，不是 PortPass 缺陷。

## 架构速览

```
┌──────────────┐  HTTPS/HTTP  ┌────────────────────────────────┐
│ 浏览器/PWA   │ ───────────▶ │ PortPass (单二进制)            │
└──────────────┘              │  ├─ Gin API                     │
                              │  ├─ Auth (JWT / IP / none)      │
                              │  ├─ Lifecycle Manager           │
                              │  │   ├─ time.AfterFunc          │
                              │  │   └─ 30s reconcile           │
                              │  ├─ Store (SQLite via GORM)     │
                              │  └─ FirewallDriver              │
                              │      ├─ iptables / ip6tables    │
                              │      ├─ nftables (inet portpass)│
                              │      ├─ ufw                     │
                              │      └─ firewalld (rich-rule)   │
                              └────────────────────────────────┘
```

## 开发指南

```bash
# 后端（热重载可选使用 air）
PORTPASS_ADMIN_PASSWORD=dev PORTPASS_FIREWALL_DRIVER=mock \
  go run ./cmd/server

# 前端（代理到 :8080）
cd frontend && npm run dev

# 单元测试
go test ./...
```

## 安全建议

1. **管理界面一定要走 HTTPS**：建议在 Caddy/Nginx 后面，把真实客户端 IP 通过 `X-Forwarded-For` 转发
2. 将 `PORTPASS_TRUSTED_PROXIES` 精确配置为反代 CIDR，避免 XFF 伪造
3. 生产环境优先使用 `ipwhitelist` 模式（天然免于密码爆破）
4. 定期检查 `审计日志`，关注异常创建者 IP
5. `PORTPASS_MAX_DURATION_HOURS` 建议≤24，避免"临时"变"永久"
6. **首次登录即改掉默认密码**：默认种子 `admin / passwd` 日志会打印红色告警，上线前务必在 UI 里改掉；如需多人管理请创建独立的管理员账号，保留至少一位作为最后防线

## 基准测试

| 项目 | 指标 |
| --- | --- |
| Docker 镜像大小 | ~40 MB（含 iptables/ip6tables、前端资源） |
| 常驻内存 | ~40 MB |
| 创建 1 条 iptables 规则延迟 | < 50 ms |
| 1000 条规则 reconcile | < 500 ms |

## License

[MIT](./LICENSE)
