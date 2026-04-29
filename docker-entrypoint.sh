#!/bin/sh
# PortPass container entrypoint.
#
# 在使用 `--network=host` 部署时，容器内的 iptables 必须与宿主机使用的
# netfilter 后端保持一致（legacy/xtables 或 nft）。两套后端在内核里是
# 互不相通的两张表，如果不一致，PortPass 的 ACCEPT 规则即便写入"成功"
# 也不会出现在真实数据包路径上（例如 CentOS 7 + Alpine 3.18+ 镜像组合）。
#
# 本脚本在启动 PortPass 之前自动探测：哪个后端能"看到"宿主机活跃的链
# （firewalld / ufw / docker 创建的标志性链），就把容器内 iptables /
# iptables-save / iptables-restore / ip6tables 等命令软链到对应实现。
#
# 也允许通过环境变量 PORTPASS_IPTABLES_BACKEND={legacy,nft} 手动覆盖。

set -u

LOG_PREFIX='[portpass-entrypoint]'
# Logs go to stderr so callers of resolve_backend_auto can capture the
# backend name from stdout without log lines mixing in.
log() { echo "$LOG_PREFIX $*" >&2; }

resolve_backend_auto() {
    legacy_out=$(/usr/sbin/iptables-legacy -S 2>/dev/null || true)
    nft_out=$(/usr/sbin/iptables-nft -S 2>/dev/null || true)

    pattern='INPUT_ZONES|INPUT_direct|FORWARD_direct|ufw-before-input|ufw-input|DOCKER-USER|DOCKER-FORWARD'
    legacy_hits=$(printf '%s\n' "$legacy_out" | grep -cE "$pattern" 2>/dev/null || true)
    nft_hits=$(printf '%s\n' "$nft_out" | grep -cE "$pattern" 2>/dev/null || true)
    legacy_hits=${legacy_hits:-0}
    nft_hits=${nft_hits:-0}

    log "host-marker hits: legacy=$legacy_hits nft=$nft_hits"

    if [ "$legacy_hits" -gt "$nft_hits" ]; then
        echo legacy
        return
    fi
    if [ "$nft_hits" -gt "$legacy_hits" ]; then
        echo nft
        return
    fi

    legacy_lines=$(printf '%s\n' "$legacy_out" | wc -l 2>/dev/null || echo 0)
    nft_lines=$(printf '%s\n' "$nft_out" | wc -l 2>/dev/null || echo 0)
    log "tie-break by total rule count: legacy=$legacy_lines nft=$nft_lines"
    if [ "$legacy_lines" -gt "$nft_lines" ]; then
        echo legacy
    else
        echo nft
    fi
}

backend="${PORTPASS_IPTABLES_BACKEND:-}"
if [ -n "$backend" ]; then
    log "iptables backend = $backend (manual override via PORTPASS_IPTABLES_BACKEND)"
else
    backend=$(resolve_backend_auto)
    log "iptables backend = $backend (auto-detected)"
fi

case "$backend" in
    legacy|nft)
        ;;
    *)
        log "WARN: invalid PORTPASS_IPTABLES_BACKEND=$backend, falling back to nft"
        backend=nft
        ;;
esac

# Alpine ships the variants as `<family>-<backend>[-<op>]`, e.g.
#   iptables-legacy / iptables-legacy-save / iptables-legacy-restore
#   iptables-nft    / iptables-nft-save    / iptables-nft-restore
# (and the same for ip6tables). We re-link the canonical names to the
# selected backend.
for tool in iptables iptables-save iptables-restore ip6tables ip6tables-save ip6tables-restore; do
    case "$tool" in
        *-save) base=${tool%-save}; suffix=-save ;;
        *-restore) base=${tool%-restore}; suffix=-restore ;;
        *) base=$tool; suffix= ;;
    esac
    variant="${base}-${backend}${suffix}"
    target="/usr/sbin/${variant}"
    if [ -L "$target" ] || [ -x "$target" ]; then
        ln -sf "$variant" "/usr/sbin/$tool"
    else
        log "WARN: $target missing, leaving /usr/sbin/$tool unchanged"
    fi
done

exec /usr/local/bin/portpass "$@"
