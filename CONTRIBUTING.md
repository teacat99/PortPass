# 参与贡献

感谢你关注 PortPass！欢迎以 Issue / PR / 讨论的形式参与改进。

## Issue 规范

### Bug Report

请提供：

- **环境信息**：操作系统、内核版本、Docker 版本（或源码编译时的 Go 版本）
- **防火墙驱动**：`PORTPASS_FIREWALL_DRIVER` 的值（`iptables` / `nftables` / `ufw` / `firewalld` / `mock`）
- **复现步骤**：从初始状态到问题出现的完整操作流程
- **期望行为 vs 实际行为**：附截图或日志（注意脱敏 IP / token）

### Feature Request

- **使用场景**：实际遇到的问题或想达到的目标
- **建议方案**（可选）：你期望的实现方式
- **替代方案**（可选）：是否考虑过其他方案

## Pull Request 规范

### 流程

1. Fork 本仓库
2. 创建功能分支：`git checkout -b feature/your-feature`
3. 提交变更（见下方 Commit 规范）
4. 推送到 Fork 后向 `main` 分支提 PR

### Commit 消息格式

```
<type>(optional-scope): <description>

[optional body]
```

| Type | 说明 |
|------|------|
| `feat` | 新功能 |
| `fix` | Bug 修复 |
| `docs` | 文档变更 |
| `style` | 代码格式（不影响逻辑） |
| `refactor` | 重构（非新功能 / 非修复） |
| `test` | 测试相关 |
| `chore` | 构建 / 工具 / 依赖变更 |

示例：

```
feat(firewall): add nftables driver
fix(auth): jwt secret reset on restart
docs: clarify trusted_proxies parsing
```

### PR 描述建议

参考 `.github/PULL_REQUEST_TEMPLATE.md`。请尽量给出测试方式（自测命令、覆盖到的鉴权模式 / 防火墙驱动）。

## 开发环境

### 要求

- Go 1.25+
- Node.js 20+
- Linux（涉及防火墙驱动调试时）；其它平台可使用 `PORTPASS_FIREWALL_DRIVER=mock`

### 本地开发

```bash
git clone git@github.com:teacat99/PortPass.git
cd PortPass

# 后端（mock 驱动，免 NET_ADMIN）
PORTPASS_ADMIN_PASSWORD=dev PORTPASS_FIREWALL_DRIVER=mock \
  go run ./cmd/server

# 前端（自动代理到 :8080）
cd frontend && npm ci && npm run dev
```

### 校验

提交前请确保：

```bash
go vet ./...
go test ./...
(cd frontend && npm run build)
```

CI（`.github/workflows/ci.yml`）会在 push / PR 时自动跑这三件事。

## 代码风格

- **Go**：`gofmt` / `goimports` 标准格式；公开 API 需简要注释；错误用 `errors.Is` / `errors.As` 判断而非字符串比较。
- **TypeScript / Vue**：跟随仓库现有 ESLint / Prettier 习惯；组件名 PascalCase；变量 camelCase。
- **i18n**：新增任何用户可见文案务必同时补齐 zh-CN 与 en-US。

## 安全相关贡献

如果你发现的是安全漏洞，请**不要**直接提 Issue，按 [SECURITY.md](./SECURITY.md) 走私下报告渠道。
