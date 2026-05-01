# PortPass

> Self-service, time-limited firewall port opener — the web page is a temporary key, click to open, auto-lock when time's up.

English | [中文](./README.md)

## Quick Start

### Docker Compose (recommended)

```bash
curl -LO https://raw.githubusercontent.com/teacat99/PortPass/main/docker-compose.yaml
# Optional: set PORTPASS_ADMIN_PASSWORD=<seed password>
# If unset, the first boot auto-creates admin / passwd (change it immediately).
docker compose up -d
```

Open `http://<server>:8080` and log in with `admin / passwd` (or your seed password).

> ⚠️ **Rotate the default password on first login**: `admin / passwd` is a bootstrap convenience only. Change it via the user menu → *Change password*, then create additional admins / regular users under **Users**.

### Single `docker run`

```bash
docker run -d \
  --name portpass \
  --restart unless-stopped \
  --network host \
  --cap-add NET_ADMIN \
  -v $PWD/data:/data \
  # Optional: set seed password; omit to fall back to admin/passwd on first boot.
  -e PORTPASS_ADMIN_PASSWORD="change-me" \
  ghcr.io/teacat99/portpass:latest
```

> `--network host` and `--cap-add NET_ADMIN` are **required**: without host networking, iptables rules only affect the container's own netns.

### From source

```bash
cd frontend && npm ci && npm run build && cd ..
go build -trimpath -ldflags="-s -w" -o portpass ./cmd/server/
PORTPASS_ADMIN_PASSWORD=dev ./portpass
```

## Features

- Temporary port opening with source IP + port + protocol + expiry
- Auto-populated client IP (with trusted-proxy `X-Forwarded-For` support)
- Preset one-click ports (SSH / RDP / MySQL / Redis / …)
- Rock-solid lifecycle: `AfterFunc` primary + 30s reconcile + boot reconciliation
- Multiple backends: iptables / nftables / ufw / firewalld, IPv4 & IPv6
- Installable PWA with offline shell caching
- Three auth modes: password + JWT / IP whitelist / none
- **Multi-user**: multiple admins + normal users, bcrypt-hashed passwords persisted to DB, self-service *Change password*
- **Port policy**: admins decide which preset ports & max duration normal users may open
- Full audit log (`user_id` + `created_by` captured on every rule)
- Chinese & English UI, fully mobile-responsive (tables ↔ cards, stacked forms, 44px touch targets)

## Environment Variables

| Name | Default | Description |
| --- | --- | --- |
| `PORTPASS_LISTEN` | `:8080` | HTTP listen address |
| `PORTPASS_AUTH_MODE` | `password` | `password` / `ipwhitelist` / `none` |
| `PORTPASS_ADMIN_USERNAME` | `admin` | Seed admin username (first boot only) |
| `PORTPASS_ADMIN_PASSWORD` | `passwd` | Seed admin password; falls back to `passwd` with a warning when unset on first boot; managed via UI thereafter |
| `PORTPASS_ADMIN_IP_WHITELIST` | — | Comma-separated CIDRs; required for `ipwhitelist` |
| `PORTPASS_TRUSTED_PROXIES` | — | Reverse-proxy CIDRs; enables `X-Forwarded-For` parsing |
| `PORTPASS_FIREWALL_DRIVER` | `iptables` | `iptables` / `nftables` / `ufw` / `firewalld` / `mock` |
| `PORTPASS_IPTABLES_BACKEND` | auto-detect | Only honoured by the `iptables` driver. `legacy` / `nft`; leave empty to follow the host's active backend |
| `PORTPASS_DATA_DIR` | `/data` | SQLite + audit log directory |
| `PORTPASS_JWT_SECRET` | random | Empty rotates on each restart |
| `PORTPASS_MAX_DURATION_HOURS` | `24` | Per-rule max lifetime |
| `PORTPASS_HISTORY_RETENTION_DAYS` | `30` | Audit-log retention |
| `PORTPASS_MAX_RULES_PER_IP` | `20` | Concurrent rule quota per creator IP |
| `PORTPASS_RATELIMIT_PER_MINUTE` | `10` | Create-rule rate per IP |

## Multi-User & Port Policy

All accounts live in SQLite (bcrypt hashed). The auth mode only decides which identity an API request assumes:

| Mode | Identity source | Who manages accounts |
| --- | --- | --- |
| `password` | Login form → DB users | Any admin |
| `ipwhitelist` | Matching CIDR → built-in **system admin** | Same (you can still create more accounts in UI) |
| `none` | Anyone → system admin | Same (internal networks only) |

**Admin rules**:

1. On first boot, if `PORTPASS_ADMIN_PASSWORD` is unset, PortPass creates `admin / passwd` and prints a red warning — change it right away.
2. **Multiple admins** may exist simultaneously; any admin can create / reset / disable accounts from `/users`.
3. An admin **cannot delete / demote / disable themselves** (API replies `400 cannot modify ... on self`).
4. There must always be **at least one active admin**; the last-admin deletion / demotion / disable is rejected.
5. Deleting a user also revokes all their active firewall rules (driver entries are cleaned up too).

**Port policy** (admin → user):

- Edit each preset in **Settings → Preset ports** and toggle `user_allowed` + `max_duration_sec`.
- Normal users only see and select `user_allowed=true` presets.
- When a normal user creates/extends a rule, `duration_sec` must not exceed the preset's `max_duration_sec` (otherwise `400 duration exceeds allowed ...`).
- Admins bypass the policy and can filter rules by owner with `GET /api/rules?user_id=<id>`.

## Choosing a Firewall Driver

| Driver | When to use | Notes |
| --- | --- | --- |
| `iptables` | Default — any Linux | Needs `iptables` + `NET_ADMIN`; IPv6 via `ip6tables`; entrypoint auto-selects the host's legacy / nft backend on container start |
| `nftables` | Modern distros (Debian 11+, RHEL 9+, Ubuntu 22+) | Owns a dedicated `inet portpass` table |
| `ufw` | Ubuntu with ufw enabled | Rules appear with `# portpass:<id>` comment |
| `firewalld` | RHEL/CentOS/Fedora | Uses runtime rich-rules only (not permanent) |
| `mock` | Development | In-memory only; does not touch real firewall |

### Automatic iptables backend selection

The runtime image ships **both** `iptables-legacy` and `iptables-nft` binaries. On every container start, `docker-entrypoint.sh` probes which backend the host is actively using by looking for tell-tale chains created by firewalld / ufw / docker (e.g. `INPUT_ZONES`, `ufw-input`, `DOCKER-USER`) and re-symlinks `iptables` / `iptables-save` / `iptables-restore` / `ip6tables*` to the matching implementation. You will see one log line:

```
[portpass-entrypoint] iptables backend = legacy (auto-detected)
```

This makes a single image work transparently on CentOS 7 (legacy) and Debian 12 (nft) alike. To override (typically for debugging) set:

```bash
docker run ... -e PORTPASS_IPTABLES_BACKEND=legacy ghcr.io/teacat99/portpass:latest
# Allowed values: legacy / nft; anything else falls back to nft.
```

> **Why this matters**: On CentOS 7 + firewalld, *every* host rule (including the trailing `REJECT` in `INPUT`) lives in iptables-legacy / xtables. Alpine 3.18+ ships `iptables` as `iptables-nft`, which writes to a separate nft table — the rule applies and self-verifies, but packets never traverse it. The visible symptom is "UI says rule is active, but the port is still unreachable from the outside".

## Host OS Compatibility

PortPass is **Linux-only** (Windows / macOS / *BSD are not supported because the firewall drivers rely on Linux netfilter). With the `iptables` driver and the auto-backend entrypoint, the following common distributions work out of the box without any extra configuration:

| Distribution | Kernel | Default firewall | iptables backend | Recommended driver |
| --- | --- | --- | --- | --- |
| CentOS 7 / RHEL 7 | 3.10 | firewalld | **legacy** | `iptables` |
| CentOS 8 / RHEL 8 / Rocky 8 / AlmaLinux 8 | 4.18 | firewalld (nft) | nft | `iptables` or `nftables` |
| RHEL 9 / Rocky 9 / AlmaLinux 9 / Stream 9 | 5.14 | firewalld (nft) | nft | `iptables` or `nftables` |
| Fedora 36+ | 5.17+ | firewalld (nft) | nft | `iptables` or `nftables` |
| Debian 10 (Buster) | 4.19 | nftables (first nft-default Debian) | nft | `iptables` or `nftables` |
| Debian 11 / 12 | 5.10 / 6.1 | nftables | nft | `iptables` or `nftables` |
| Ubuntu 18.04 LTS | 4.15 | ufw (legacy) | **legacy** | `iptables` or `ufw` |
| Ubuntu 20.04 LTS | 5.4 | ufw (legacy default, may switch to nft) | depends on `update-alternatives` | `iptables` or `ufw` |
| Ubuntu 22.04 / 24.04 LTS | 5.15 / 6.8 | ufw (nft) | nft | `iptables` / `ufw` / `nftables` |
| OpenWrt ≤ 21.02 | ≤ 5.4 | iptables (fw3) | **legacy** | `iptables` |
| OpenWrt 22.03+ | 5.10+ | nftables (fw4) | nft | `nftables` |

> Entries marked **legacy** rely on the entrypoint's auto-detection. Keep your image at `v1.1.3` or newer.

## Reliability

1. **Primary**: `time.AfterFunc` fires exactly at `expire_at`
2. **Periodic reconcile** every 30s fixes drift:
   - Overdue rules not cleaned by the timer
   - DB-present but firewall-missing (operator ran `iptables -F`)
   - Firewall-present but DB-missing (orphan cleanup)
3. **Boot reconcile** runs synchronously before the HTTP server starts
4. **SIGTERM does NOT flush rules** — a restart is not a revocation

## Deployment requirements for "drop existing connections on expiry"

The "drop existing connections on expiry" feature (`cleanup_on_expire`) calls `conntrack -D` on the host to surgically remove tracking entries that match the rule's `(source IP, destination port, protocol)` tuple. It will **never** affect connections allowed by other firewall rules. However, Linux's connection-tracking semantics mean that **deleting a conntrack entry alone is not enough**: to really tear down an established TCP/UDP flow you also need a fallback path that drops packets when no ACCEPT rule is present. Verify both points in production:

1. **Container side**: use the official PortPass image (≥ `v1.x.y`, ships with `conntrack-tools`), or, if you build your own, `apk add conntrack-tools` on Alpine.
2. **Host firewall side**: the `INPUT` chain must have a **default-drop or trailing DROP** semantic, e.g.:
   - CentOS / RHEL + firewalld: the default zone ends with `REJECT --reject-with icmp-host-prohibited`, which satisfies this.
   - Ubuntu + ufw: enable `ufw default deny incoming`.
   - Self-managed iptables: either `iptables -P INPUT DROP` or a trailing `iptables -A INPUT -j DROP`.
   - Cloud VMs that rely on security groups only: after PortPass triggers cleanup, the next packet of an established flow is still allowed back through the cloud security group's stateful inspection, so the conntrack flush is immediately undone. Add a host-side fallback DROP in such environments.

> Without point 2, the conntrack entries are genuinely deleted (the toast `dropped N existing connections` is honest), but the very next packet to arrive will be re-accepted by the default-permissive `INPUT` chain, and the kernel will rebuild a tracking entry — so the connection appears to "self-heal" instantly. This is intrinsic to Linux conntrack, not a PortPass bug.

## Security Recommendations

1. Always front the admin UI with HTTPS (Caddy / Nginx)
2. Configure `PORTPASS_TRUSTED_PROXIES` to your proxy CIDR only
3. Prefer `ipwhitelist` mode in production
4. Review the audit log regularly
5. Keep `MAX_DURATION_HOURS` low (≤24) to avoid "temporary" rules becoming permanent
6. **Rotate `admin / passwd` on day one**: the default seed is logged with a loud warning. Before exposing PortPass, change it via the UI and create individual admins per operator — always keep at least one admin account as a recovery anchor

## Benchmarks

| Metric | Value |
| --- | --- |
| Docker image | ~40 MB (iptables/ip6tables + frontend assets) |
| Resident memory | ~40 MB |
| iptables rule apply latency | < 50 ms |
| 1000-rule reconcile | < 500 ms |

## License

[MIT](./LICENSE)
