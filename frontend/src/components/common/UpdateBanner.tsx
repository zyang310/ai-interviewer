import { useState } from "react";
import { InstallUpdate, OpenReleasePage, models } from "../../lib/wailsBridge";
import "./UpdateBanner.css";

interface UpdateBannerProps {
  update: models.UpdateInfo;
  onDismiss: () => void;
  // Surfaces an install failure into the app's shared error banner. Optional —
  // without it a failure just re-enables the button silently.
  onError?: (msg: string) => void;
}

// UpdateBanner notifies the user that a newer app release is available and
// installs it in place: InstallUpdate downloads the build, verifies it is
// genuinely signed and notarized, then quits the app so a detached helper can
// swap the bundle and relaunch it — the window closes and reopens on the new
// version with no manual drag-to-Applications step. The backend only reports an
// update as available once its .zip asset exists, so downloadUrl is always set
// here; the release-page fallback is pure defense. Rendered only on non-overlay
// idle screens so it never intrudes on the floating interview overlay or an
// in-progress session.
export default function UpdateBanner({ update, onDismiss, onError }: UpdateBannerProps) {
  const [installing, setInstalling] = useState(false);

  async function handleUpdate() {
    if (!update.downloadUrl) {
      OpenReleasePage(update.releaseUrl).catch(() => {});
      return;
    }
    setInstalling(true);
    try {
      await InstallUpdate(update.downloadUrl);
      // Resolving means the app is already quitting to install — nothing left to do.
    } catch (e: any) {
      setInstalling(false);
      onError?.(e?.message || String(e));
    }
  }

  return (
    <div className="update-banner">
      <span className="update-banner-dot" />
      <span className="update-banner-text">
        A new version is available — <strong>{update.latestVersion}</strong>
      </span>
      <div className="update-banner-actions">
        <button
          className="btn btn-primary btn-icon"
          onClick={handleUpdate}
          disabled={installing}
        >
          <span className="material-symbols-outlined">download</span>
          {installing ? "Installing…" : "Update & Restart"}
        </button>
        <button
          className="update-banner-dismiss"
          onClick={onDismiss}
          aria-label="Dismiss update notice"
          title="Dismiss"
          disabled={installing}
        >
          &times;
        </button>
      </div>
    </div>
  );
}
