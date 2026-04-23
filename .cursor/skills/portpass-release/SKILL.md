---
name: portpass-release
description: Cut a new PortPass release (backend + frontend + Docker + tag). Use when the user asks to "release PortPass", "cut a version", "build the docker image", or tag a new version.
---

# Skill: PortPass Release

This skill guides you through producing a reproducible PortPass release.

## When to use

- User asks to "release PortPass", "cut a version", "publish a new tag"
- User wants to "rebuild the Docker image" or "verify the release build locally"

## Inputs you need

1. Target semantic version (e.g. `v0.2.0`). Ask if not provided.
2. Whether to publish to GHCR (requires push permissions) or build locally only.

## Release procedure

### 1. Pre-flight checks

Run from the repo root:

```bash
# Clean working tree
git status
# All tests must pass
go test ./...
# Frontend type-checks and builds
(cd frontend && npm ci && npm run build)
# Backend vet clean
go vet ./...
```

### 2. Update version metadata

- `README.md` / `README.en.md` — ensure install snippet references `:latest` or the new tag.
- `frontend/package.json` — bump `version` to match the tag (no leading `v`).
- Append an entry to `CHANGELOG.md` (create if missing) summarising changes
  since the previous tag.

### 3. Commit and tag

```bash
git add -A
git commit -m "chore(release): v0.2.0"
git tag -a v0.2.0 -m "v0.2.0"
```

### 4. Push (only when user confirmed remote exists)

```bash
git push origin main
git push origin v0.2.0
```

GitHub Actions `release.yml` triggers on `v*` tags and builds multi-arch
images to `ghcr.io/<owner>/portpass`.

### 5. Verify the image

```bash
docker pull ghcr.io/<owner>/portpass:v0.2.0
docker run --rm ghcr.io/<owner>/portpass:v0.2.0 --help || true
```

For local-only builds, skip steps 4-5 and run:

```bash
docker build -t portpass:dev .
docker run -d --rm --name portpass-dev --network host --cap-add NET_ADMIN \
  -e PORTPASS_ADMIN_PASSWORD=dev -v $PWD/data:/data portpass:dev
```

## Safety rails

- Never force-push `main` or release tags.
- Never release from a dirty worktree; abort and ask the user to stash first.
- Confirm with the user BEFORE creating the tag if any test failed.
- Confirm with the user BEFORE pushing if the remote is not yet configured.

## Companion document

This skill ships alongside [`SKILL.zh-CN.md`](./SKILL.zh-CN.md) containing
the Simplified Chinese version. Keep both in sync whenever you edit either.
