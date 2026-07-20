import { useState } from "react";
import { CheckForUpdate, InstallUpdate, OpenReleasePage, OpenURL, models } from "../../lib/wailsBridge";
import MogiLogo from "../common/MogiLogo";
import "./AboutSection.css";

// About → Project links. Opened in the user's real browser via OpenURL (not the
// webview). No LICENSE file in the repo, so the fourth card points at docs/.
const REPO_URL = "https://github.com/zyang310/mogi";
const ABOUT_LINKS: { icon: string; name: string; sub: string; href: string }[] = [
  { icon: "code", name: "Repository", sub: "Source on GitHub", href: REPO_URL },
  { icon: "deployed_code", name: "Releases", sub: "Download builds", href: `${REPO_URL}/releases` },
  { icon: "bug_report", name: "Report an issue", sub: "Bugs & requests", href: `${REPO_URL}/issues` },
  { icon: "description", name: "Documentation", sub: "Architecture & specs", href: `${REPO_URL}/tree/main/docs` },
];

interface Props {
  appVersion: string;
  // GOOS reported by the backend's hotkey status ("darwin" | "windows" | "linux"),
  // undefined until it loads — drives the identity block's build line.
  goos?: string;
  // Update-check errors surface through the shell's shared error/success lines.
  setError: (msg: string) => void;
  setSuccess: (msg: string) => void;
}

// AboutSection is the Settings → About pane: app identity, an on-demand update
// check (mirrors the launch-time banner), and project links.
export default function AboutSection({ appVersion, goos, setError, setSuccess }: Props) {
  const [checking, setChecking] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [updateInfo, setUpdateInfo] = useState<models.UpdateInfo | null>(null);
  const [checkedOnce, setCheckedOnce] = useState(false);

  // On-demand update check for the About section. Mirrors the launch-time banner
  // but driven by the button; errors surface inline via the shared error line.
  async function checkForUpdate() {
    setChecking(true);
    setError("");
    setSuccess("");
    try {
      setUpdateInfo(await CheckForUpdate());
      setCheckedOnce(true);
    } catch (e: any) {
      setError(e?.message || String(e));
    } finally {
      setChecking(false);
    }
  }

  // Installs the checked update in place: downloads, verifies the signature,
  // and quits the app so a detached helper can swap the bundle and relaunch it.
  // Falls back to the release page when there's no packaged asset to install.
  async function installUpdate() {
    if (!updateInfo?.downloadUrl) {
      OpenReleasePage(updateInfo?.releaseUrl || "").catch(() => {});
      return;
    }
    setInstalling(true);
    setError("");
    try {
      await InstallUpdate(updateInfo.downloadUrl);
      // Resolving means the app is already quitting to install — nothing left to do.
    } catch (e: any) {
      setInstalling(false);
      setError(e?.message || String(e));
    }
  }

  // About identity build line, e.g. "macOS · local build" (GOOS + version).
  const osLabel =
    goos === "darwin"
      ? "macOS"
      : goos === "windows"
        ? "Windows"
        : goos === "linux"
          ? "Linux"
          : "";
  const buildLabel = `${osLabel ? osLabel + " · " : ""}${
    appVersion && appVersion !== "dev" ? "release build" : "local build"
  }`;

  return (
    <>
      <header className="settings-head">
        <h1>About</h1>
        <p>Version, updates, and project links.</p>
      </header>

      {/* Identity block. */}
      <div className="about-identity">
        <span className="about-logo">
          <MogiLogo size={46} variant="cream" />
        </span>
        <div className="about-identity-meta">
          <div className="about-name">Mogi</div>
          <div className="about-tagline">
            Realtime mock-interview copilot for your desktop.
          </div>
        </div>
        <div className="about-identity-side">
          <span className="about-version-pill">
            <span className="about-version-dot" />
            Version {appVersion || "dev"}
          </span>
          <span className="about-build">{buildLabel}</span>
        </div>
      </div>

      {/* Software updates. */}
      <div className="settings-card">
        <div className="settings-card-head">
          <span className="material-symbols-outlined">system_update_alt</span>
          <h3 className="settings-card-title">Software updates</h3>
        </div>
        <p className="settings-hint">
          Releases are published on GitHub. Updating downloads the new build,
          verifies it, and restarts the app on the new version.
        </p>
        <div className="about-update-row">
          <button
            className="btn btn-primary btn-icon"
            onClick={checkForUpdate}
            disabled={checking || installing}
          >
            <span className="material-symbols-outlined">refresh</span>
            {checking ? "Checking…" : "Check for updates"}
          </button>
          {updateInfo?.available && (
            <button
              className="btn btn-ghost btn-icon"
              onClick={installUpdate}
              disabled={installing}
            >
              <span className="material-symbols-outlined">download</span>
              {installing ? "Installing…" : `Update to ${updateInfo.latestVersion}`}
            </button>
          )}
          {checkedOnce && !checking && (
            updateInfo?.available ? (
              <span className="about-update-status about-update-status--new">
                <span className="about-update-dot" />
                {updateInfo.latestVersion} available — you have{" "}
                {updateInfo.currentVersion}
              </span>
            ) : updateInfo?.latestVersion ? (
              <span className="about-update-status about-update-status--ok">
                <span className="about-update-dot" />
                Up to date — you’re on the latest
              </span>
            ) : (
              <span className="about-update-status about-update-status--muted">
                No published releases to compare against yet
              </span>
            )
          )}
        </div>
      </div>

      {/* Project links. */}
      <div className="about-links-wrap">
        <div className="about-links-label">Project</div>
        <div className="about-links">
          {ABOUT_LINKS.map((l) => (
            <button
              type="button"
              className="about-link"
              key={l.name}
              onClick={() => OpenURL(l.href)}
            >
              <div className="about-link-top">
                <span className="about-link-icon">
                  <span className="material-symbols-outlined">{l.icon}</span>
                </span>
                <span className="material-symbols-outlined about-link-arrow">
                  north_east
                </span>
              </div>
              <div>
                <div className="about-link-name">{l.name}</div>
                <div className="about-link-sub">{l.sub}</div>
              </div>
            </button>
          ))}
        </div>
      </div>
    </>
  );
}
