---
name: portpass-release
description: 发布新的 PortPass 版本（后端 + 前端 + Docker + tag）。当用户要求"发布 PortPass""打版本""构建 Docker 镜像"或"发新 tag"时使用。
---

# 技能：PortPass 发版

本技能指导你产出一个可复现的 PortPass 版本。

## 适用场景

- 用户要求"发布 PortPass""打一个版本""推送新 tag"
- 用户要求"重新构建 Docker 镜像"或"在本地验证发版构建"

## 你需要询问/获取

1. 目标语义版本号（例如 `v0.2.0`）。若未提供请先问。
2. 是否推送到 GHCR（需要推送权限），还是仅本地构建。

## 发版流程

### 1. 预检

在仓库根目录执行：

```bash
git status                                     # 工作区必须干净
go test ./...                                  # 所有后端测试通过
(cd frontend && npm ci && npm run build)       # 前端可构建
go vet ./...                                   # vet 无问题
```

### 2. 更新版本元数据

- `README.md` / `README.en.md`：检查安装示例是否引用 `:latest` 或新 tag。
- `frontend/package.json`：`version` 与 tag 保持一致（不带 `v`）。
- `CHANGELOG.md`：追加本次变更摘要（不存在则创建）。

### 3. 提交并打 tag

```bash
git add -A
git commit -m "chore(release): v0.2.0"
git tag -a v0.2.0 -m "v0.2.0"
```

### 4. 推送（仅当用户确认远端存在时）

```bash
git push origin main
git push origin v0.2.0
```

GitHub Actions 的 `release.yml` 会在 `v*` tag 上触发，构建多架构镜像至
`ghcr.io/<owner>/portpass`。

### 5. 验证镜像

```bash
docker pull ghcr.io/<owner>/portpass:v0.2.0
docker run --rm ghcr.io/<owner>/portpass:v0.2.0 --help || true
```

仅本地构建时跳过 4-5 步，改为：

```bash
docker build -t portpass:dev .
docker run -d --rm --name portpass-dev --network host --cap-add NET_ADMIN \
  -e PORTPASS_ADMIN_PASSWORD=dev -v $PWD/data:/data portpass:dev
```

## 安全栏

- 禁止强制推送 `main` 或发布 tag。
- 工作区脏时禁止发版；先让用户 stash。
- 若有测试失败，必须先与用户确认再打 tag。
- 远端未配置时，必须与用户确认后再推送。

## 配套文档

本技能与 [`SKILL.md`](./SKILL.md) 英文版并存，修改任一份时必须同步另一份。
