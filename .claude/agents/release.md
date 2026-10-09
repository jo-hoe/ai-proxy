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
  - Bash(timeout *)
  - Bash(sleep *)
  - Bash(for *)
  - Bash(seq *)
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
- **No release needed** — only non-app, non-chart files changed (e.g. `.claude/`, `scripts/`, `docs/`, `Makefile`, CI config). Skip Steps 2–3 chart-bump and tag steps, but still commit + push any uncommitted working-tree changes (Step 3 commit logic applies). Report: `[Step 1/5] No version bump needed — committing non-release changes and pushing.`
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

> **Ordering invariant (full releases):** the chart's `appVersion` makes the
> deployment pull `ghcr.io/jo-hoe/ai-proxy:<appVersion>` (see
> `charts/ai-proxy/templates/deployment.yaml`:
> `image.tag | default .Chart.AppVersion`). The image for that version is built
> **only** by the `v<new-version>` tag push. If the chart is published before the
> image exists, a fresh install hits `ImagePullBackOff`. Therefore for full
> releases the **app image must be built and pushed BEFORE the chart is published**:
> push the tag first, wait for the image build to complete (Step 3b), and only
> then push the Chart.yaml bump that triggers `Release Chart`.
>
> Chart-only releases reuse an existing `appVersion` whose image already exists,
> so there is no ordering constraint — push the chart bump directly.

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

Commit the chart bump (do NOT push it yet for full releases):
```bash
git add charts/ai-proxy/Chart.yaml
git commit -m "chore: bump chart and appVersion to <new-version>"
```

**Chart-only release** — push main now; this is the only trigger:
```bash
git push origin main
```

**Full release** — push the app commits and the tag FIRST so the image builds,
but keep the chart bump commit local until the image exists:
```bash
# push every commit EXCEPT the chart bump (which must wait for the image):
git push origin HEAD~1:main          # pushes app/test/config commits only
git tag v<new-version>
git push origin v<new-version>       # triggers the Docker image build
```
If there are no app commits ahead of origin/main (all code already on main, only
the chart bump is local), just push the tag — the tag ref builds the image from
HEAD~1 (the already-pushed app code):
```bash
git tag v<new-version>
git push origin v<new-version>
```

If a push fails due to remote changes, rebase first, then re-push (and re-tag if needed):
```bash
git fetch origin && git rebase origin/main
```

Report: `[Step 3/5] ✓ Pushed app code + tag v<new-version>` (full) or `✓ Pushed main` (chart-only)

### Step 3b — Full release only: wait for the image, THEN push the chart

Report: `[Step 3b/5] Waiting for image build before publishing chart...`

Skip this step entirely for chart-only releases.

Find the image-build run (the `CI` run on the `v<new-version>` tag ref) and wait
for it to complete **success** before publishing the chart. Use bounded polling
(see Step 4 rules — fixed cap, per-command timeout, explicit exit):
```bash
# resolve the tag run id:
RUN_ID=$(gh run list --repo jo-hoe/ai-proxy --branch v<new-version> \
  --workflow CI --limit 1 --json databaseId -q '.[0].databaseId')
# poll up to 20 times, 30s apart (hard ceiling ~10 min):
for i in $(seq 1 20); do
  S=$(timeout 30 gh run view "$RUN_ID" --repo jo-hoe/ai-proxy \
      --json status,conclusion -q '.status+"/"+(.conclusion // "pending")')
  echo "[Step 3b/5] Poll $i/20 — image: $S"
  case "$S" in
    completed/success) break ;;
    completed/*) echo "image build failed: $S"; gh run view "$RUN_ID" --repo jo-hoe/ai-proxy --log-failed; exit 1 ;;
  esac
  [ "$i" -eq 20 ] && { echo "[Step 3b/5] ✗ Timeout waiting for image"; exit 1; }
  sleep 30
done
```
Only after the image build reports `completed/success`, push the chart bump that
triggers `Release Chart`:
```bash
git push origin main
```

Report: `[Step 3b/5] ✓ Image v<new-version> built; chart bump pushed`

### Step 4 — Babysit CI

Report: `[Step 4/5] Waiting for CI (timeout: 10 minutes)...`

**Bounded-polling rules (never let this step hang):**
- Hard cap of 20 iterations, 30s apart (~10 min ceiling). Never loop unbounded.
- Wrap every `gh` call in `timeout 30 gh ...` so a single hung network call cannot
  stall the whole step.
- Treat `null` conclusion as `pending` (`.conclusion // "pending"`), never an error.
- Exit the loop the instant all tracked runs are `completed`, or at iteration 20.

Poll with a bounded loop. On each poll:
```bash
timeout 30 gh run list --repo jo-hoe/ai-proxy --limit 8
```

**Full release** — the image build was already confirmed success in Step 3b, so
here you only track the main-push CI and the chart publish:
- `CI` (main) — Lint + Test on the chart-bump push
- `Release Chart` — triggered by the `charts/ai-proxy/Chart.yaml` change on main
- image build (`CI` on the `v<new-version>` tag ref) — already `completed/success`
  from Step 3b; report it as such, do not re-wait.

**Chart-only release** — track:
- `CI` — triggered by the main push (Lint + Test)
- `Release Chart` — triggered by the `charts/ai-proxy/Chart.yaml` change on main

Report each poll as: `[Step 4/5] Poll <n>/20 — CI(main): <status>, chart: <status>`

Stop as soon as all tracked workflows show `completed`. If any shows `failure`, fetch logs immediately:
```bash
timeout 60 gh run view <id> --repo jo-hoe/ai-proxy --log-failed
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
