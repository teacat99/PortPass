# 安全策略

## 受支持的版本

| 版本 | 安全更新 |
|------|---------|
| `1.x` | ✅ 持续维护 |
| `< 1.0` | ❌ 已停止维护，请尽快升级 |

## 报告漏洞

PortPass 直接控制服务器防火墙规则，安全问题影响面较大。**请不要在公开 Issue 里披露漏洞**。

请通过以下任一渠道私下联系：

- **GitHub Security Advisories**（推荐）：在仓库的 [Security → Advisories → New draft](https://github.com/teacat99/PortPass/security/advisories/new) 提交私密报告
- 或在 GitHub 上 @teacat99 私信告知一个可联系的邮箱

报告时请尽量包含：

- 受影响的版本 / 部署方式（Docker / 源码 / 鉴权模式）
- 复现步骤或 PoC
- 漏洞影响（信息泄露 / 越权 / RCE / DoS / 防火墙规则被绕过 等）
- 你期望的披露时间表（默认 90 天协调披露）

## 响应时间

- 收到报告后 **3 个工作日** 内确认
- 评估并给出修复计划，**14 天** 内提供修复版本（高危漏洞优先）
- 修复发布后通过 GitHub Releases 公告，致谢报告者（除非你选择匿名）

## 安全配置建议

参见 [README.md → 安全建议](./README.md#%E5%AE%89%E5%85%A8%E5%BB%BA%E8%AE%AE) 章节，重点：

1. 管理界面**必须**走 HTTPS
2. 反向代理时务必精确配置 `PORTPASS_TRUSTED_PROXIES`，避免 `X-Forwarded-For` 伪造
3. 首次登录立刻修改默认 `admin / passwd`
4. 公网部署优先使用 `ipwhitelist` 模式
5. `PORTPASS_MAX_DURATION_HOURS` 控制单条规则上限，避免"临时变永久"
