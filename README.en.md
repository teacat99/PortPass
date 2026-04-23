# PortPass

> Self-service, time-limited firewall port opener — the web page is a temporary key, click to open, auto-lock when time's up.

English | [中文](./README.md)

## Quick Start

### Docker Compose (recommended)

```bash
curl -LO https://raw.githubusercontent.com/teacat99/PortPass/main/docker-compose.yaml
# Edit docker-compose.yaml; at minimum change PORTPASS_ADMIN_PASSWORD
docker compose up -d
```

Open `http://<server>:8080` and log in with your password.

### Single `docker run`

```bash
docker run -d \
  --name portpass \
  --restart unless-stopped \
  --network host \
  --cap-add NET_ADMIN \
  -v $PWD/data:/data \
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
- Full audit log
- Chinese & English UI, mobile-first responsive

## Environment Variables

| Name | Default | Description |
| --- | --- | --- |
| `PORTPASS_LISTEN` | `:8080` | HTTP listen address |
| `PORTPASS_AUTH_MODE` | `password` | `password` / `ipwhitelist` / `none` |
| `PORTPASS_ADMIN_PASSWORD` | — | Required when `AUTH_MODE=password` |
| `PORTPASS_ADMIN_IP_WHITELIST` | — | Comma-separated CIDRs; required for `ipwhitelist` |
| `PORTPASS_TRUSTED_PROXIES` | — | Reverse-proxy CIDRs; enables `X-Forwarded-For` parsing |
| `PORTPASS_FIREWALL_DRIVER` | `iptables` | `iptables` / `nftables` / `ufw` / `firewalld` / `mock` |
| `PORTPASS_DATA_DIR` | `/data` | SQLite + audit log directory |
| `PORTPASS_JWT_SECRET` | random | Empty rotates on each restart |
| `PORTPASS_MAX_DURATION_HOURS` | `24` | Per-rule max lifetime |
| `PORTPASS_HISTORY_RETENTION_DAYS` | `30` | Audit-log retention |
| `PORTPASS_MAX_RULES_PER_IP` | `20` | Concurrent rule quota per creator IP |
| `PORTPASS_RATELIMIT_PER_MINUTE` | `10` | Create-rule rate per IP |

## Choosing a Firewall Driver

| Driver | When to use | Notes |
| --- | --- | --- |
| `iptables` | Default — any Linux | Needs `iptables` + `NET_ADMIN`; IPv6 via `ip6tables` |
| `nftables` | Modern distros | Owns a dedicated `inet portpass` table |
| `ufw` | Ubuntu with ufw enabled | Rules appear with `# portpass:<id>` comment |
| `firewalld` | RHEL/CentOS/Fedora | Uses runtime rich-rules only (not permanent) |
| `mock` | Development | In-memory only; does not touch real firewall |

## Reliability

1. **Primary**: `time.AfterFunc` fires exactly at `expire_at`
2. **Periodic reconcile** every 30s fixes drift:
   - Overdue rules not cleaned by the timer
   - DB-present but firewall-missing (operator ran `iptables -F`)
   - Firewall-present but DB-missing (orphan cleanup)
3. **Boot reconcile** runs synchronously before the HTTP server starts
4. **SIGTERM does NOT flush rules** — a restart is not a revocation

## Security Recommendations

1. Always front the admin UI with HTTPS (Caddy / Nginx)
2. Configure `PORTPASS_TRUSTED_PROXIES` to your proxy CIDR only
3. Prefer `ipwhitelist` mode in production
4. Review the audit log regularly
5. Keep `MAX_DURATION_HOURS` low (≤24) to avoid "temporary" rules becoming permanent

## Benchmarks

| Metric | Value |
| --- | --- |
| Docker image | ~40 MB (iptables/ip6tables + frontend assets) |
| Resident memory | ~40 MB |
| iptables rule apply latency | < 50 ms |
| 1000-rule reconcile | < 500 ms |

## License

[MIT](./LICENSE)
