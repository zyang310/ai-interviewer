# CI/CD + Auto-Update Plan (Wails v2, macOS, unsigned)

> Status: **planned, not yet implemented.** Design reference for adding a release pipeline and an in-app update flow.

## Context

Today the app has **no release pipeline, no version, and no update mechanism** (no `.github/`, no GoReleaser, no version constant, no update code). The app is built locally with `wails build`; there is no way for anyone else to get it.

Goals:
1. **CI that builds on every push to `main`** so the app is always known-good and buildable.
2. **Public, downloadable releases** so anyone on GitHub can grab the app (GitHub **Releases** — CI run artifacts are repo-access-only and expire).
3. **In-app "update available → download" flow** so existing users learn about and fetch new versions.

Decisions (from the user): **macOS only**, **notify + 1-click download** (not silent), **unsigned** (no Apple Developer account).

Repo: `github.com/zyang310/mogi` (confirmed via `git remote -v`). Note: `internal/ai/client.go` has a pre-existing typo'd referer URL using `zhihangyang`; the updater must use the correct owner **`zyang310`**.

### The unsigned reality (what "auto-update" can and can't be here)

Wails v2 has **no built-in self-updater**, and seamless in-place replacement on macOS effectively requires code signing + notarization (Sparkle-style). Staying unsigned, the realistic and robust design is:

- App checks GitHub Releases on launch → if a newer version exists, shows an **"Update available"** banner.
- Clicking **Download** opens the new release's `.zip` from GitHub.
- User unzips, drags `Mogi.app` to `/Applications`, and clears Gatekeeper **once** (`xattr -cr` / right-click → Open). The release notes spell this out.

This is the ceiling without signing — the same flow most unsigned indie macOS apps use. If an Apple Developer account is added later, this design upgrades cleanly to Sparkle silent updates (see end).

## How versioning & releases work (the model)

- **Version source of truth = a git tag** `vX.Y.Z`.
- **Push to `main`** → `build.yml` runs (compile + tests + a universal `.app` artifact). Proves every change builds. *No public release* — keeps the version line clean and avoids release spam.
- **Push a tag `vX.Y.Z`** → `release.yml` builds the versioned `.app`, zips it, and publishes a **GitHub Release**. This is the public download *and* what the in-app updater compares against.
- Cutting a release is one line: `git tag v0.2.0 && git push origin v0.2.0`.

> Alternative if *every* `main` push should be publicly downloadable: add a rolling `latest` pre-release to `build.yml`. Not recommended — it muddies semver comparison and the in-app check. Tag-driven is the standard.

---

## Phase A — Version + updater backend (Go)

**Goal:** the binary knows its own version and can ask GitHub if a newer release exists. Verify with `go test` before touching the UI.

1. **`main.go`** — add a package-level `var version = "dev"`. Injected at build time via `wails build -ldflags "-X main.version=$VERSION"`. Default `"dev"` for local builds.

2. **`internal/models/` (new `update.go`)** — add `UpdateInfo` boundary struct (json tags), matching the convention that all Wails-boundary structs live here:
   ```go
   type UpdateInfo struct {
       Available      bool   `json:"available"`
       CurrentVersion string `json:"currentVersion"`
       LatestVersion  string `json:"latestVersion"`
       ReleaseURL     string `json:"releaseUrl"`
       DownloadURL    string `json:"downloadUrl"` // the .zip asset
       Notes          string `json:"notes"`
   }
   ```

3. **`internal/updater/updater.go`** (new package, one concern):
   - `func Check(ctx context.Context, currentVersion string) (models.UpdateInfo, error)`.
   - GET `https://api.github.com/repos/zyang310/mogi/releases/latest` with headers `Accept: application/vnd.github+json`, `User-Agent: mogi`, `X-GitHub-Api-Version: 2022-11-28`.
   - **Reuse the existing HTTP pattern** from `internal/ai/client.go` (`http.Client{Timeout: 60s}`, `http.NewRequestWithContext`, `defer resp.Body.Close()`, `io.ReadAll`, status check, JSON unmarshal into an anonymous struct).
   - Parse `tag_name`, `html_url`, `body`, and `assets[].browser_download_url` (pick the asset whose name ends in `.zip`).
   - Compare with `golang.org/x/mod/semver` (`semver.IsValid` + `semver.Compare`). If `currentVersion == "dev"` or invalid → `Available:false` (never nag dev builds). `404` (no release yet) → `Available:false`, no error.
   - Add `golang.org/x/mod` via `go get` (one small, official module; alternatively hand-roll a ~30-line compare to add zero deps).

4. **`app.go`** — three thin bound methods (delegating, per the "keep app.go thin" rule):
   - `GetAppVersion() string` → returns `version`.
   - `CheckForUpdate() (models.UpdateInfo, error)` → `updater.Check(a.ctx, version)`.
   - `OpenReleasePage(url string) error` → `runtime.BrowserOpenURL(a.ctx, url)` (opens the download in the user's browser). Note `runtime` is already imported and used in `app.go`.

5. **`internal/updater/updater_test.go`** — table tests for the version-compare logic (equal / newer / older / `dev` / malformed). Pure function, fast.

**Verify:** `go build ./...`, `go test ./...`, `gofmt`.

---

## Phase B — Update UI (frontend)

**Goal:** surface the version and the update banner without breaking the transparent overlay.

1. **Regenerate bindings:** `wails generate module`, then export `GetAppVersion`, `CheckForUpdate`, `OpenReleasePage`, and the `UpdateInfo` model from `frontend/src/lib/wailsBridge.ts` (single import-point rule).

2. **`frontend/src/lib/useUpdateCheck.ts`** (new hook) — on mount, fire-and-forget `CheckForUpdate()`; store `{info, loading, error}`. **Fail silent** (no banner on error/dev), handle loading/error per the React rule. Runs once per launch.

3. **`frontend/src/components/UpdateBanner.tsx` + `.css`** (new) — small banner: "Update available — vX.Y.Z" + **Download** button (calls `OpenReleasePage(downloadUrl || releaseUrl)`) + dismiss. **Reuse** the shared `.btn*` classes, glass-panel, and status-dot rather than new styles (reusable-UI rule). Mirror the existing `app-warning` / `app-error` banner pattern in `App.tsx`.

4. **`frontend/src/App.tsx`** — mount `useUpdateCheck`; render `<UpdateBanner>` inside `app-content` **only in the idle hub / non-overlay views**, never in overlay mode (preserves the frameless/transparent overlay invariant — the overlay early-returns before the main `return`).

5. **`frontend/src/components/Settings.tsx`** — add a small **About** section: current version (`GetAppVersion()`) + a "Check for updates" button reusing the existing Settings-section pattern.

**Verify:** `cd frontend && npx tsc --noEmit`; browser preview with Wails calls stubbed to confirm banner render/dismiss.

---

## Phase C — CI build workflow (every push to `main`)

**`.github/workflows/build.yml`** — triggers on `push` to `main` and `pull_request`. Single `macos-latest` runner:
- `actions/setup-go@v5` (`go-version: '1.25'`, module cache) + `actions/setup-node@v4` (`node-version: 20`).
- `go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0` (pinned to the project's Wails version); add `$(go env GOPATH)/bin` to `PATH`.
- **Checks:** `go build ./...`, `go test ./...`, `gofmt -l .`, and `cd frontend && npm ci && npx tsc --noEmit`.
- **Build:** `wails build -platform darwin/universal -ldflags "-X main.version=dev-${{ github.sha }}"` (universal = Intel + Apple Silicon from one binary).
- **Artifact:** `ditto -c -k --keepParent "build/bin/Mogi.app" build.zip` → `actions/upload-artifact@v4` (downloadable from the run for testing; satisfies "builds every time I change main").

**Verify:** push a branch / open a PR and confirm the run goes green and produces the artifact.

---

## Phase D — Release workflow (on tag) + install docs

1. **`.github/workflows/release.yml`** — triggers on `push` tags `v*`. `permissions: contents: write`. Same macOS build setup as Phase C, then:
   - `VERSION=${GITHUB_REF_NAME}` (e.g. `v0.2.0`); patch `wails.json` `info.productVersion` to the tag (strip leading `v`) so the macOS plist version matches.
   - `wails build -platform darwin/universal -ldflags "-X main.version=${VERSION}"`.
   - `ditto -c -k --keepParent "build/bin/Mogi.app" "Mogi-${VERSION}-macos-universal.zip"`.
   - `softprops/action-gh-release@v2` with `files:` the zip, `generate_release_notes: true`, and a `body` that includes the **first-launch Gatekeeper steps** (`xattr -cr "/Applications/Mogi.app"` or right-click → Open).

2. **`wails.json`** — add an `info` block (`companyName`, `productName: "Mogi"`, `productVersion: "0.1.0"`, `copyright`) so local/plist versions are sane; CI overrides `productVersion` per tag.

3. **Docs:**
   - `README.md` — add a **Download & Install** section (link to Releases, the de-quarantine one-liner, universal-binary note).
   - `CLAUDE.md` — add `internal/updater/` to the codebase map and a "See also" link to this doc.
   - `docs/roadmap.md` — note the new distribution/auto-update capability (there's an uncommitted edit there already; append, don't clobber).

**Verify:** `git tag v0.1.0 && git push origin v0.1.0` → confirm a public Release appears with the `.zip`, downloadable while logged out. Then locally run a build with `-ldflags "-X main.version=v0.0.1"` and confirm `CheckForUpdate()` reports `v0.1.0` available and Download opens the release.

---

## Files created / modified

**New:** `.github/workflows/build.yml`, `.github/workflows/release.yml`, `internal/updater/updater.go`, `internal/updater/updater_test.go`, `internal/models/update.go`, `frontend/src/lib/useUpdateCheck.ts`, `frontend/src/components/UpdateBanner.tsx`, `frontend/src/components/UpdateBanner.css`.

**Modified:** `main.go` (version var), `app.go` (3 bound methods), `go.mod`/`go.sum` (x/mod), `wails.json` (info block), `frontend/src/lib/wailsBridge.ts`, `frontend/src/App.tsx`, `frontend/src/components/Settings.tsx`, `frontend/wailsjs/**` (regenerated, not hand-edited), `README.md`, `CLAUDE.md`, `docs/roadmap.md`.

---

## End-to-end verification

1. **Backend:** `go build ./...` && `go test ./...` (updater compare tests pass) && `gofmt -l .` clean.
2. **Frontend:** `cd frontend && npx tsc --noEmit` clean; browser-stub preview shows the banner.
3. **CI build:** push to a branch → `build.yml` green + artifact present.
4. **Release:** push `v0.1.0` → public Release with `.zip`; download while signed out to confirm it's public.
5. **Update flow:** run a `v0.0.1`-stamped local build → banner appears → Download opens the `v0.1.0` release. Fresh-Mac install check: unzip → move to `/Applications` → `xattr -cr` → launches.

## Later upgrade path (only if signing is added)

With an Apple Developer account: add signing + notarization to `release.yml` (removes the Gatekeeper step) and swap the notify-banner for **Sparkle** (`.app` embeds the framework, CI publishes an `appcast.xml` to GitHub Pages/Releases) for true silent background updates. The version/release plumbing above stays as-is.
