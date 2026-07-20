# CI/CD & Auto-Update — A Guide

> Read this top-to-bottom to understand how Mogi is built, released, and
> updated. It explains the concepts first, then how this repo wires them together,
> then the trade-offs behind each decision. For the bound-method reference and data
> flow, see [architecture.md](architecture.md); for the original design notes, see
> [ci-cd-and-auto-update-plan.md](ci-cd-and-auto-update-plan.md).

## Why this exists

The app is a [Wails](https://wails.io) desktop binary. Before this system existed, the
only way to get it was to clone the repo and run `wails build` yourself — there was no
version number, no download link, and no way for an existing copy to learn that a newer
one shipped. Three gaps, three goals:

1. **Continuous builds** — every push to `main` should prove the app still compiles and
   passes its checks, and produce a runnable macOS build.
2. **Public downloads** — anyone should be able to grab the app from GitHub.
3. **In-app update awareness** — a running copy should notice when a newer version is
   out and point the user to it.

The constraints that shaped the design: **macOS only**, and updates that are
**user-initiated, not silent** — the app checks and shows a banner rather than quietly
patching itself in the background. The app also shipped *unsigned* at first; it is now
signed with a Developer ID and notarized by Apple, which is what unlocked the app installing
its own updates rather than requiring a manual drag-and-replace. This guide keeps that
history where it explains a decision. The rest of it explains what those constraints mean
and why they lead to the design we have.

## Concepts, from first principles

### CI vs CD

**Continuous Integration (CI)** is the habit of automatically building and testing every
change, so a broken commit is caught in minutes instead of at release time. **Continuous
Delivery (CD)** is the next step: automatically packaging a build into something a user
can install. This repo uses both, split across two workflows:

- CI runs on **every push to `main`** ([build.yml](../.github/workflows/build.yml)).
- CD runs when you **push a version tag** ([release.yml](../.github/workflows/release.yml)).

A third workflow ([refresh-problems.yml](../.github/workflows/refresh-problems.yml)) is
neither CI nor CD but **scheduled automation**: on the 1st and 15th of each month it
regenerates the committed Company Practice dataset from its upstream sources and opens a
PR when anything changed — automating a *data* chore rather than a build. Its design (and
the GITHUB_TOKEN quirks it works around) is documented in the workflow file itself; the
data pipeline it drives is in [company-practice-plan.md](company-practice-plan.md).

### GitHub Actions in one paragraph

GitHub Actions runs your automation on GitHub's servers. A **workflow** is a YAML file in
`.github/workflows/`. Each workflow has **triggers** (`on: push`, `on: pull_request`,
`on: push: tags`, or `on: schedule` — a cron timer, used by the dataset refresh) and one or
more **jobs**. A job runs on a fresh virtual machine called a **runner** (the build
workflows use `macos-latest`, because building a macOS `.app` needs macOS; the dataset
refresh runs pure Go and gets by on `ubuntu-latest`) and is a list of **steps** — either a shell command (`run:`) or a reusable **action**
(`uses: actions/checkout@v4`). When a job finishes it can keep files in one of two very
different places:

| Output | Who can download it | Lifetime | Used here for |
|---|---|---|---|
| **Artifact** (`actions/upload-artifact`) | People with **repo access** | Expires (default 90 days) | Test builds from `main` |
| **Release** (`softprops/action-gh-release`) | **Anyone**, even logged-out | Permanent | The public, downloadable app |

That distinction is the whole reason build and release are separate workflows: a CI
artifact is great for "did this commit build?", but only a **Release** satisfies "anyone
can download it."

### Versioning: tags are the source of truth

We use [Semantic Versioning](https://semver.org): `vMAJOR.MINOR.PATCH` (e.g. `v0.2.0`).
The version that gets **built** lives in exactly one place — a **git tag** — and flows
everywhere else from there. There is deliberately no version number compiled in from
source: a release is "whatever tag was pushed," so the version can never drift out of sync
with what was actually built.

What the tag *doesn't* tell you is what the **next** version should be. That used to be a
judgement call made at `git tag` time, which is exactly the kind of manual step that
quietly goes wrong (ship a breaking change as a patch, or forget a release entirely).
So the next number is now **derived from the commit messages** by
[release-please](https://github.com/googleapis/release-please), using the
[Conventional Commits](https://www.conventionalcommits.org) prefixes this repo already
writes:

| Commit prefix | Bump | Example |
|---|---|---|
| `fix:` | patch | 0.8.0 → 0.8.1 |
| `feat:` | minor | 0.8.1 → 0.9.0 |
| `feat!:` / `BREAKING CHANGE:` in body | major | 0.9.0 → 1.0.0 |
| `chore:` `docs:` `refactor:` `test:` `ci:` | *none* | no release; listed in the changelog only |

The consequence worth internalizing: **the commit prefix is now load-bearing.** A
user-facing bug fix committed as `chore:` will not ship. (This repo has historical
`chore: fix bug` commits from before the change — that habit has to go.)

Deriving the version means release-please needs a little committed bookkeeping to know
where it is: `.release-please-manifest.json` (the last released version) and `version.txt`.
That's a real trade — two files that restate the tag — bought in exchange for never doing
semver arithmetic by hand, and for a `CHANGELOG.md` that writes itself. Note the *build*
still reads the tag, so those files can't corrupt a build; at worst they'd propose a wrong
next number, which is visible in the Release PR before anything ships.

Declaring a version that the commits *don't* imply — most obviously 1.0.0, which for an
app is a product decision, not a semantic one — is done with a `Release-As: 1.0.0` footer
in a commit message.

### macOS distribution: signing, notarization, Gatekeeper

This is the concept that shapes the install experience most, so it's worth understanding.

- **Code signing** stamps an app with a certificate proving who built it. It requires an
  **Apple Developer account** ($99/year).
- **Notarization** is Apple scanning a signed app and issuing an "OK" ticket for it.
  **Stapling** writes that ticket into the bundle so it validates offline.
- **Gatekeeper** is the macOS gate that, on first launch of a *downloaded* app, checks for
  that signature/notarization. Anything downloaded from the internet also gets a
  **quarantine** attribute (`com.apple.quarantine`) set by the browser.

We ship **signed and notarized**. `release.yml` signs the bundle with a Developer ID
certificate under the **hardened runtime** (`codesign --options runtime --timestamp`),
submits it to Apple's notary service, staples the returned ticket, and then verifies the
result the way a user's Mac will: `spctl --assess` must report
`source=Notarized Developer ID` before the release zip is built. For the user, install is
download → unzip → drag to `/Applications` → open. That's the whole flow.

**What this replaced.** The app originally shipped unsigned, so Gatekeeper refused to open
it — *"app is damaged / from an unidentified developer"* — and every install needed a
one-time workaround (right-click → **Open**, or `xattr -cr`). That is normal for indie
macOS apps and it worked, but it asked every user to explicitly override a security
warning, which is a bad habit to teach. The $99/year bought that away.

Two details worth knowing, because either can silently spoil a release:

- **The hardened runtime needs explicit entitlements.** `--options runtime` denies things
  like microphone access unless the entitlement is declared, so a perfectly-signed build
  can still ship with voice input broken. `release.yml` asserts
  `com.apple.security.device.audio-input` survived signing rather than trusting it.
- **Stapling must happen before the zip is built**, or users get an app that has to phone
  Apple on first launch — and fails when they're offline.

### The auto-update spectrum (and why Wails v2 has none built in)

Auto-update isn't one thing; it's a spectrum from "tell the user" to "replace yourself
while running, with no click at all":

```
  least automatic                                                    most automatic
  ───────────────────────────────────────────────────────────────────────────────────►
  Notify + download          Self-replace binary            Framework (Sparkle/WinSparkle)
  (show a banner,            (app downloads the build,      (periodic background check,
   user drags it in)         verifies it, swaps itself,      silent apply, no click —
                              relaunches — one click)         relaunches on its own)
                                     ▲
                                     │ where we are
```

Frameworks like **Sparkle** (the macOS standard) do the fully-silent version: a periodic
background check with no launch required, and an apply step the user never has to trigger.
They also **verify a cryptographic signature** before applying an update and rely on the app
being signed/notarized so the swapped-in copy isn't quarantined — the same two things
`internal/updater/install.go` already does by hand for the middle position. Electron and
Tauri ship updaters in this family; **Wails v2 ships no auto-updater at all**, so whatever we
do, we build it ourselves.

We used to sit at the **left end**: the app checked GitHub and opened a download, and the
user dragged the new build into `/Applications` by hand. That was not a choice — shipping
unsigned ruled out a safe silent swap, since there was no way to prove a downloaded build
was genuinely ours. Signing removed that block, and `InstallUpdate` (`app.go`,
`internal/updater/install.go`) now occupies the **self-replace** position: one click
downloads, verifies, swaps, and relaunches. What's left between here and Sparkle is entirely
about *when* the check happens and *whether a click is required* — the verify-and-swap
mechanics are already the same. That gap stays deferred because reaching it costs a nested
framework, a hosted signed appcast feed, and reworking `release.yml`'s current assumption
that the bundle carries no nested code (see the signing section above) — real costs against
a click that already takes a few seconds.

## How the pipeline works, end-to-end

```
   ┌────────────────────────────────────────────────────────────────────────┐
   │                        git push  →  main                               │
   └──────────────┬──────────────────────────────────┬──────────────────────┘
                  ▼                                   ▼
       .github/workflows/build.yml      .github/workflows/release-please.yml
       runner: macos-latest             runner: ubuntu-latest  (API calls only)
       ├─ npm ci && npm run build       ├─ read conventional commits since
       ├─ go build ./...                │    the last release
       ├─ go test ./...                 └─ open/update the standing Release PR
       ├─ gofmt -l .                         · bumps version.txt + wails.json
       ├─ wails build …dev-<sha> -s          · writes CHANGELOG.md
       └─ upload-artifact (repo-only)             │
                  │                                │   ◄── you merge it when ready
                  ▼                                ▼
       "did this commit build?"        tag vX.Y.Z  +  GitHub Release (changelog,
                                                       no app attached yet)
                                                   │
                                                   │  calls  (workflow_call)
                                                   ▼
                                        .github/workflows/release.yml
                                        runner: macos-latest
                                        ├─ stamp wails.json productVersion
                                        ├─ wails build darwin/universal
                                        │     -ldflags main.version=vX.Y.Z
                                        ├─ ditto  → Mogi-vX.Y.Z.zip
                                        └─ attach .zip + append install notes
                                                   │
                                                   ▼
                                     ┌─────────────────────────────┐
                                     │  Public GitHub Release      │
                                     │  • anyone can download .zip │
                                     │  • the updater checks this  │
                                     └─────────────────────────────┘
```

`build.yml` and `release.yml` run the same macOS build; they differ in **trigger**,
**version**, and **where the output goes**. Every `main` push produces a throwaway
`dev-<sha>` build as a repo-only artifact (continuous proof it compiles). A merged Release
PR produces a real `vX.Y.Z` build published as a public Release.

> **Why release-please *calls* release.yml instead of letting the tag trigger it.**
> `release.yml` still declares `on: push: tags`, but that event will never fire here:
> GitHub deliberately does **not** start workflows for pushes made with the automatic
> `GITHUB_TOKEN`, as a guard against workflows infinitely triggering each other. The tag
> lands; the tag event never arrives. Two ways out — hand release-please a Personal Access
> Token so the push looks human, or chain the workflows explicitly. We chain: `release.yml`
> also declares `on: workflow_call`, so release-please invokes it directly as a reusable
> workflow. No PAT to create, store, or rotate, and the build logic still exists once. The
> tag trigger stays as a hand-operated escape hatch.

> **Why the frontend builds first.** `main.go` embeds the compiled UI with
> `//go:embed all:frontend/dist`, so that directory must exist before *any*
> `go build`/`go test` of the `main` package — otherwise the compile fails with
> `pattern all:frontend/dist: no matching files found`. `frontend/dist` is a gitignored
> build output, so CI builds the frontend up front; `wails build -s` then packages
> without rebuilding it. (`release.yml` sidesteps this by running only `wails build`,
> which builds the frontend before the Go compile anyway.)

### The version flow

A single tag fans out to two destinations:

```
   git tag v0.2.0
        │
        ├──►  -ldflags "-X main.version=v0.2.0"  ──►  the Go variable `version` (main.go)
        │                                              │
        │                                              ├──► App.GetAppVersion()  → Settings "About"
        │                                              └──► updater.Check(version) → compare vs GitHub
        │
        └──►  wails.json info.productVersion = "0.2.0" ──►  macOS Info.plist
                                                            (CFBundleShortVersionString → Finder "Get Info")
```

`-ldflags "-X main.version=..."` is a Go linker feature: it sets the value of a package
variable at **build time** without changing source. Our `main.go` declares
`var version = "dev"`; the linker overwrites `"dev"` with the tag. Local builds keep
`"dev"`, which (as we'll see) is exactly what suppresses update nags during development.

## How the update check works

The check lives in the Go backend ([internal/updater/updater.go](../internal/updater/updater.go)),
consistent with the project rule that *all* external HTTP calls happen in Go — the same
pattern as [internal/ai/client.go](../internal/ai/client.go). The frontend only renders
the result.

```
  app launch
     │
     ▼
  useUpdateCheck()  ── React hook, runs once on mount (frontend/src/lib/useUpdateCheck.ts)
     │   calls →  App.CheckForUpdate()         (Wails-bound, app.go)
     │                 │
     │                 ▼
     │          updater.Check(ctx, version)    (internal/updater/updater.go)
     │                 │
     │                 ├─ version is "dev"/invalid semver?  ──► return {available:false}   (no network call)
     │                 │
     │                 ├─ GET api.github.com/repos/zyang310/mogi/releases/latest
     │                 │      (404 = no releases yet ──► {available:false}, not an error)
     │                 │
     │                 └─ semver.Compare(latestTag, version) > 0  ──► {available:true, …urls}
     │
     ▼
  available?  ──► <UpdateBanner> on the hub  ──►  [Update & Restart] ──► App.InstallUpdate(url)
                  (idle screens only; never                                (see the next section)
                   over the overlay or mid-interview)
```

Three details worth calling out, because they're the kind of thing an interviewer probes:

- **It fails silent.** Offline, GitHub down, a dev build, or running in a plain browser
  with no Wails runtime — every failure path leaves the banner hidden. A broken update
  check must never disrupt the actual app. (`useUpdateCheck` swallows errors;
  `CheckForUpdate` returns them but the caller ignores them.)
- **Dev builds never nag.** `version` is `"dev"` locally, which isn't valid semver, so
  `updater.Check` returns "no update" *before* making any network request. That also keeps
  development off GitHub's unauthenticated rate limit.
- **The comparison is real semver, not string compare.** We use
  `golang.org/x/mod/semver` (`IsValid` + `Compare`), so `v0.10.0 > v0.9.0` (a naive string
  compare would get that wrong). The logic is a pure function (`isNewer`) with table tests
  in [updater_test.go](../internal/updater/updater_test.go).

## How self-install works

Clicking **Update & Restart** calls `App.InstallUpdate(downloadURL)` (`app.go`), which
delegates to `updater.Install` (`internal/updater/install.go`, macOS-only — see
`install_other.go` for the stub everywhere else) and then quits the app:

```
  [Update & Restart] ──► App.InstallUpdate(url)
                              │
                              ├─ version == "dev"?             ──► refuse (nothing to swap)
                              ├─ a session is active?          ──► refuse ("end it first")
                              │
                              ▼
                    updater.Install(ctx, url)
                              │
                              │  1. download the .zip to a temp dir
                              │  2. ditto -x -k   (extract — NOT archive/zip; ditto keeps
                              │       symlinks/resource forks/xattrs intact, any of which
                              │       archive/zip would drop and invalidate the signature)
                              │  3. codesign --verify --deep --strict
                              │  4. spctl --assess --type execute
                              │       (either failing aborts here — nothing is touched or
                              │        installed; the same two checks release.yml itself
                              │        runs before publishing, see above)
                              │  5. spawn a detached helper (setsid), pass it our pid
                              │
                              ▼
                    App.InstallUpdate quits the app   (window.go: runtime.Quit)
                              │
                              ▼
        detached helper, running independently of the app that spawned it:
          while kill -0 $PID; do sleep 0.2; done   ← waits for us to actually exit
          mv  Mogi.app      Mogi.app.old
          mv  <downloaded>  Mogi.app
          rm -rf            Mogi.app.old
          open              Mogi.app                ──► new version launches
          rm -rf            <temp dir>
```

Worth calling out:

- **Verification is a hard gate, not a warning.** A downloaded build that fails either
  `codesign` or `spctl` is never installed — the function returns an error before anything
  quits. It's the same trust boundary Gatekeeper enforces on a human double-clicking the
  app; self-install just checks it programmatically first, so a corrupted download or a
  bad redirect can't silently become the running app.
- **The swap happens outside the process being replaced.** A running program can't safely
  overwrite the bundle it's executing from, so `Install` hands off to a `/bin/sh` helper,
  detached with `Setsid` so it survives this process exiting, that waits for our pid to
  disappear before touching anything.
- **The helper deliberately does not get the request's `context.Context`.** Every other
  shelled-out command in this file (`ditto`, `codesign`, `spctl`) uses
  `exec.CommandContext(ctx, …)`, so it's cancelled if the app shuts down mid-check. The swap
  helper is the one exception: `ctx` dies with this process, and `exec.CommandContext`
  SIGKILLs its child the instant that happens — which would kill the helper before it ever
  got to run. It's started with plain `exec.Command` instead, specifically so it outlives us.
- **Two refusals happen before any of this starts.** A `dev` build has no published release
  to compare against and nothing sensible to swap into, and a live interview session is
  refused the same way `ClearAllLocalData` refuses mid-session (`a.interview.ActiveID()`) —
  installing pulls the rug out from under whatever's on screen.
- **First install still can't use any of this.** There's no already-running app to click
  "Update & Restart" from — the very first copy on a machine is still download → unzip →
  drag → open, same as always.

## The install & update experience, concretely

What this actually means for a user:

1. **First install:** download `.zip` → unzip → drag `Mogi.app` to `/Applications` → open
   it. No warning, no workaround. (Nothing can shortcut this one — there's no running app
   yet for anything to self-install from.)
2. **An update ships:** on next launch the app shows *"A new version is available."* → click
   **Update & Restart** → the app downloads it, verifies it's a genuine signed Mogi build,
   and relaunches on the new version. A few seconds, one click, no Finder involved.

It is **not** a silent background swap: the app only checks at launch (not on a timer while
running), and applying it still takes an explicit click. That remaining gap is app design,
not a technical ceiling — Wails ships no updater framework, so closing it means embedding one
(Sparkle) rather than flipping a setting. The honest framing: signing bought away both the
security-warning friction *and* the manual drag; what's left is a click and a few seconds,
which isn't yet painful enough to justify Sparkle's complexity.

## Design decisions & trade-offs

**Tag-driven releases, not "publish every push to `main`."** Publishing a release on
every commit would spam the releases page and, worse, break the updater: semver comparison
needs clean, monotonic version numbers, and GitHub's "latest release" endpoint ignores
pre-releases anyway. So `main` pushes only *verify* (artifact), and a release is a separate,
deliberate act.

**Computed versions, but a human-gated release.** These are two decisions, and it's worth
separating them. *Computing* the version from commit messages removes hand arithmetic and a
hand-written changelog — pure upside, no judgement lost, since the prefixes were already
being written. *Publishing* is still gated on a human merging the Release PR, and that gate
is deliberate: every release a user installs costs them a restart, even now that installing
one is a single click. Auto-tagging each `fix:` would multiply restart-prompts across users
for no benefit. Batching several commits into one release is therefore not laziness — it's
the correct response to the distribution constraint. (Signing removed the *Gatekeeper* half
of the old friction, and self-install removed the *manual replace* half; what's left is the
interruption itself. If a Sparkle-style background updater ever lands — checking
periodically instead of only at launch, applying without a click — the argument for batching
weakens further and auto-release-on-every-fix becomes reasonable.)

The standing Release PR also doubles as a **preview**: the proposed version number and the
generated changelog are visible, and `build.yml` runs against them, before anything is
public.

**Notify-and-install, not silent.** Covered above — the app checks once at launch and waits
for a click rather than polling in the background and applying on its own. Originally that
shape was forced by shipping unsigned (no safe silent swap was even possible); now it's a
scope choice, since closing the remaining gap means embedding a framework like Sparkle
rather than adjusting what's already here.

**The update check lives in Go, not the frontend.** Every other external call in this app
is centralized in the Go backend (`internal/ai`, `internal/voice`, `internal/googletts`),
so the updater follows suit. It reuses the established `http.Client` pattern and keeps the
frontend as pure UI. It would *work* as a `fetch()` in React, but it would break the
codebase's "all network in Go" invariant for no benefit.

**A universal binary, not per-arch downloads.** `wails build -platform darwin/universal`
produces one `.app` that runs natively on both Apple Silicon and Intel (via `lipo`). One
download, no "which one do I need?" — at the cost of a slightly larger file. For a
two-architecture target that's clearly worth it.

**`golang.org/x/mod/semver`, not a hand-rolled compare.** It's a small, official,
well-tested module that correctly handles the `v` prefix and pre-release ordering. The
alternative — ~30 lines of split-and-compare — is exactly the kind of code that looks
trivial and then mishandles `v0.10.0` vs `v0.9.0`. One tiny dependency buys correctness.

**macOS-only, for now.** The app's machine target is macOS and parts of it are macOS-tuned
(the voice path re-encodes audio specifically for WKWebView). Windows would *compile*
(there's NSIS config in `build/windows/`), but the voice path would need work, so the
pipeline doesn't pretend to support it yet. Adding a Windows job later is a localized
change to the workflows.

## Operational runbook

**Cut a release** — there is nothing to type. Commit with conventional prefixes and push to
`main`; release-please keeps a PR titled **`chore(main): release X.Y.Z`** open with the
computed version and changelog. Review it, **merge it**, and the tag, the GitHub Release,
and the attached `.zip` follow automatically. Merging that PR *is* cutting the release.

To override the computed version (declaring 1.0.0, or forcing a patch), add a footer to any
commit that will be in the release:

```
Release-As: 1.0.0
```

**Release something that isn't shipping** — if the Release PR isn't appearing, the commits
since the last release are probably all non-releasing types (`chore:`, `docs:`, `refactor:`).
That's working as intended; a user-facing fix needs to be committed as `fix:`.

**Escape hatch: release by hand** — still supported, for a hotfix or to re-run a release
whose build failed. A hand-pushed tag triggers `release.yml` directly, which creates the
Release itself:

```bash
git tag v0.8.1
git push origin v0.8.1
```

If you do this, bump `.release-please-manifest.json` and `version.txt` to match, or
release-please will propose a stale next version.

**Check a `main` build compiled** — open the repo's **Actions** tab, find the latest
*Build* run; its artifact (`Mogi-macos-universal`) is the test build.

**Test the update flow locally** — build the app pretending to be an old version, then
launch it; with a real release published, the banner should appear:

```bash
wails build -ldflags "-X main.version=v0.0.1"
open build/bin/Mogi.app
```

This proves the *check* and the banner. It can't prove the *install*, on purpose: a local
`wails build` has no signing secrets, so `verifySignedAndNotarized` correctly refuses it —
clicking **Update & Restart** against a local build fails verification and surfaces that
error instead of installing something unverified. Exercising the real swap-and-relaunch path
needs an actual published release; there is deliberately no test bypass for the signature
check that would let a local build stand in for one.

**Where the version shows** — Settings → **About** (calls `GetAppVersion`), and macOS
Finder → *Get Info* (from the plist).

**If CI fails** — the failing step name says which check broke. Reproduce locally with the
same command: `go build ./...`, `go test ./...`, `gofmt -l .`,
`cd frontend && npx tsc --noEmit`, or `wails build`.

## Adoption: how many people downloaded the app

GitHub tracks a **`download_count` per release asset** for free — no backend, no
in-app telemetry, nothing that leaves a user's machine. To total those counts across
all releases:

```bash
go run ./cmd/downloads          # per-release breakdown + grand total
go run ./cmd/downloads -json    # raw per-asset counts as JSON
```

Set `GITHUB_TOKEN` if you hit the unauthenticated rate limit (60 req/hr); a bare token
with no scopes is enough for this public repo. Notes on what the number means:

- It counts **downloads, not people** — GitHub does not dedupe by user, so treat it as a
  rough adoption signal.
- It counts only the **GitHub-hosted** asset. Mirrors, Homebrew casks, or any other
  distribution channel are not included.

This covers "how many downloads." Counting **sessions run** is a different problem: every
session lives only in the user's local SQLite (`internal/store`), so there is no way to
know without adding a telemetry backend — deliberately out of scope for now.

## Later: fully silent updates

This was planned as a two-step upgrade to be done if an Apple Developer account ever
arrived. **Both steps have since shipped, in this order:**

1. ~~Add **signing + notarization** to `release.yml`.~~ **Done** — Apple certs in GitHub
   Secrets, `codesign` + `notarytool` + `stapler` steps. This alone removed the Gatekeeper
   step for users.
2. ~~Have the app **install its own updates**.~~ **Done** — `InstallUpdate` downloads,
   verifies, swaps, and relaunches on a click (see "How self-install works" above). This was
   originally scoped as "embed Sparkle"; a home-grown version turned out to need no nested
   framework, no cgo bridge, and no hosted appcast feed — just `ditto`/`codesign`/`spctl`
   shelled out from Go, which is also why `release.yml`'s "no nested code" signing
   assumption never had to change.

What's left is narrower than the original plan: not "add self-installing," but **"make the
check and apply happen without a click"** — a periodic background check while the app is
running (today it only checks at launch), and applying without the user pressing **Update &
Restart**. That's genuinely what a framework like Sparkle buys at this point, since the
signature-verification and swap-and-relaunch mechanics it would otherwise provide already
exist here. It remains optional: a background timer and a truly silent apply are a real
scope increase for a personal project, against a click that already takes a few seconds.
