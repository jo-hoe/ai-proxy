---
name: release
description: Release a new version of ai-proxy. Supports two release types — full (new image + chart) and chart-only (chart bump, existing image). Run in foreground (run_in_background: false) so step progress is visible in real time.
allowedTools:
  - Read
  - Edit
  - Bash(git *)
  - Bash(gh *)
  - Bash(helm *)
  - Bash(grep *)
---

## Release process for ai-proxy

Follow these steps in order. Do not skip steps. After each step, report completion with a one-line status so the user can track progress.

### Step 1 — Determine version and release type

Report: `[Step 1/5] Determining version and release type...`

Check the current versions and what has changed since the last tag:
```bash
grep -E '^version:|^appVersion:' charts/ai-proxy/Chart.yaml
git tag --sort=-v:refname | head -3
git log $(git tag --sort=-v:refname | head -1)..HEAD --oneline
git diff $(git tag --sort=-v:refname | head -1)..HEAD -- charts/ai-proxy/ *.go cmd/ internal/ go.mod go.sum Dockerfile
```

**Determine release type from the diff:**
- **No release needed** — only non-app, non-chart files changed (e.g. `.claude/`, docs, CI config). Stop here and report: `[Step 1/5] No release needed — no app or chart changes since last tag.`
- **Full release** — app code changed (root `*.go`, `cmd/`, `internal/`, `go.mod`, `go.sum`, `Dockerfile`). Bumps both `version` and `appVersion`. Pushes a new semver tag to trigger the Docker image build.
- **Chart-only release** — only chart templates/values changed (`charts/`), no app code changes. Bumps only `version`, keeps `appVersion`. No new tag pushed.

Report: `[Step 1/5] ✓ Release type: <full|chart-only>, chart version: <new-version>, appVersion: <app-version>`

### Step 2 — Bump Chart.yaml

Report: `[Step 2/5] Bumping Chart.yaml...`

**Full release:** update both fields in `charts/ai-proxy/Chart.yaml`:
```yaml
version: <new-version>
appVersion: "<new-version>"
```

**Chart-only release:** update only `version`, leave `appVersion` unchanged:
```yaml
version: <new-version>
appVersion: "<current-app-version>"   # unchanged
```

Report: `[Step 2/5] ✓ Chart.yaml updated`

### Step 3 — Commit, push, and (for full releases) tag

Report: `[Step 3/5] Committing and pushing...`

**First, check for uncommitted code changes and commit them if present:**
```bash
git status --short
```

If there are any modified or untracked files outside of `charts/` (i.e. app code, tests, config), stage and commit them before the chart bump commit:
```bash
git add <each modified or untracked file>
git commit -m "feat: <short summary of the changes>"
```

Use `git diff --cached` and `git log --oneline -5` to write an accurate commit message reflecting what actually changed.

Then commit the chart bump:
```bash
git add charts/ai-proxy/Chart.yaml
git commit -m "chore: bump chart and appVersion to <new-version>"
git push origin main
```

**Full release only** — also push the semver tag to trigger the image build:
```bash
git tag v<new-version>
git push origin v<new-version>
```

If push fails due to remote changes, rebase first:
```bash
git fetch origin && git rebase origin/main
```
Then re-push (and re-tag if needed).

Report: `[Step 3/5] ✓ Pushed main` (and `+ tag v<new-version>` for full releases)

### Step 4 — Babysit CI

Report: `[Step 4/5] Waiting for CI (timeout: 10 minutes)...`

Poll every 30 seconds, up to 20 times. On each poll:
```bash
gh run list --repo jo-hoe/ai-proxy --limit 8
```

**Full release** — track these:
- `CI` — triggered by the main push (Lint + Test; on the tag it also runs the Docker image build)
- `CI` on the `v<new-version>` tag ref — builds and pushes the Docker image
- `Release Chart` — triggered by the `charts/ai-proxy/Chart.yaml` change on main

**Chart-only release** — track only:
- `CI` — triggered by the main push (Lint + Test)
- `Release Chart` — triggered by the `charts/ai-proxy/Chart.yaml` change on main

Report each poll as: `[Step 4/5] Poll <n>/20 — CI(main): <status>, image: <status|n/a>, chart: <status>`

Stop as soon as all tracked workflows show `completed`. If any shows `failure`, fetch logs immediately:
```bash
gh run view <id> --log-failed
```
Then report the failure and stop.

Note: `Release Chart` publishes to the `gh-pages` branch, which then triggers `pages build and deployment` (and, on failure, `Retry Pages Deployment`). The chart is not live on the Pages site until that Pages deployment completes — allow for it in Step 5.

If 20 polls pass without completion: `[Step 4/5] ✗ Timeout after 10 minutes` and stop.

Report: `[Step 4/5] ✓ All workflows completed successfully`

### Step 5 — Verify and confirm

Report: `[Step 5/5] Verifying published artifacts...`

Verify the chart was published as a GitHub release by `chart-releaser` (this is created before the Pages deployment, so it's the most reliable signal):
```bash
gh release view ai-proxy-<new-version> --repo jo-hoe/ai-proxy
```

Then confirm it is served from the Pages Helm repo (may lag until the Pages deployment from Step 4 finishes — retry a few times if needed):
```bash
helm repo add ai-proxy https://jo-hoe.github.io/ai-proxy 2>/dev/null; helm repo update ai-proxy
helm search repo ai-proxy/ai-proxy --version <new-version>
```

For full releases, the image tag is confirmed by the Docker job succeeding in Step 4 (the packages API requires `read:packages` scope which may not be available).

If chart verification fails, report the error and stop.

Report: `[Step 5/5] ✓ Release complete`

Confirm:
- Chart release: `ai-proxy-<new-version>` on jo-hoe/ai-proxy
- Helm repo: `https://jo-hoe.github.io/ai-proxy` → `ai-proxy/ai-proxy --version <new-version>`
- Image: `ghcr.io/jo-hoe/ai-proxy:v<app-version>` (full release) or `unchanged at v<current-app-version>` (chart-only)
