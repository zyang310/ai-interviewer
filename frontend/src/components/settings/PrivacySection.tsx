import { useState } from "react";
import { ClearAllLocalData, RevealDatabaseFile } from "../../lib/wailsBridge";
import "./PrivacySection.css";

// The Privacy "where your data lives" map. `tone` drives the badge color:
// "ok" (matcha) for data that never leaves the device, "warn" (gold) for data
// streamed to a provider only during a live session.
const PRIVACY_ROWS: {
  icon: string;
  name: string;
  desc: string;
  badge: string;
  tone: "ok" | "warn";
}[] = [
  {
    icon: "settings",
    name: "Settings & preferences",
    desc: "Themes, timings, hotkeys and model choices.",
    badge: "On device",
    tone: "ok",
  },
  {
    icon: "key",
    name: "API keys",
    desc: "Stored in the local database; sent only inside authenticated requests to your providers.",
    badge: "On device",
    tone: "ok",
  },
  {
    icon: "history",
    name: "Interview history & transcripts",
    desc: "Every session and its notes stay in the local SQLite file.",
    badge: "On device",
    tone: "ok",
  },
  {
    icon: "screenshot_monitor",
    name: "Screen captures",
    desc: "Sent to your model provider during a live session to answer — never written to disk.",
    badge: "In-session only",
    tone: "warn",
  },
  {
    icon: "graphic_eq",
    name: "Voice audio",
    desc: "Streamed to your speech provider only while a spoken interview is running.",
    badge: "In-session only",
    tone: "warn",
  },
];

interface Props {
  // Errors/success surface through the shell's shared status lines.
  setError: (msg: string) => void;
  setSuccess: (msg: string) => void;
  // Runs after ClearAllLocalData succeeds: the shell reloads auth + prefs from
  // the now-empty store so the whole UI reflects the reset without a restart.
  onDataCleared: () => Promise<void>;
}

// PrivacySection is the Settings → Privacy pane: a data-locality map plus the
// reveal-database and destructive "clear all local data" actions (the latter
// gated on typing CONFIRM).
export default function PrivacySection({ setError, setSuccess, onDataCleared }: Props) {
  // Reveal-in-finder + the destructive "clear all" flow (open the confirm
  // popup, then require an exact "CONFIRM" before the irreversible wipe).
  const [revealing, setRevealing] = useState(false);
  const [clearOpen, setClearOpen] = useState(false);
  const [clearConfirm, setClearConfirm] = useState("");
  const [clearing, setClearing] = useState(false);

  // Open the OS file manager with the SQLite database selected.
  async function revealDatabase() {
    setRevealing(true);
    setError("");
    setSuccess("");
    try {
      await RevealDatabaseFile();
    } catch (e: any) {
      setError(e?.message || String(e));
    } finally {
      setRevealing(false);
    }
  }

  // Wipe every local record. Guarded by an exact "CONFIRM" match; on success the
  // shell reloads auth + prefs via onDataCleared. Theme lives in localStorage,
  // so it's left intact.
  async function clearAllData() {
    if (clearConfirm !== "CONFIRM") return;
    setClearing(true);
    setError("");
    setSuccess("");
    try {
      await ClearAllLocalData();
      await onDataCleared();
      setClearOpen(false);
      setClearConfirm("");
      setSuccess("All local data cleared. The app is back to a first-run state.");
    } catch (e: any) {
      setError(e?.message || String(e));
    } finally {
      setClearing(false);
    }
  }

  return (
    <>
      <header className="settings-head">
        <h1>Privacy</h1>
        <p>Where your data lives, and what leaves this device.</p>
      </header>

      {/* Reassurance banner. */}
      <div className="privacy-banner">
        <span className="privacy-banner-icon">
          <span className="material-symbols-outlined">verified_user</span>
        </span>
        <div>
          <div className="privacy-banner-title">Everything stays on this device</div>
          <div className="privacy-banner-desc">
            Mogi has no account and no cloud of its own. Your data sits in a local
            SQLite database and only leaves for the provider requests you trigger.
          </div>
        </div>
      </div>

      {/* Data map: one row per kind of data, badged by where it lives. */}
      <div className="settings-card datamap">
        <div className="datamap-head">Where your data lives</div>
        {PRIVACY_ROWS.map((r, i) => (
          <div
            className={`datamap-row${i === PRIVACY_ROWS.length - 1 ? "" : " has-border"}`}
            key={r.name}
          >
            <span className="datamap-icon">
              <span className="material-symbols-outlined">{r.icon}</span>
            </span>
            <div className="datamap-meta">
              <div className="datamap-name">{r.name}</div>
              <div className="datamap-desc">{r.desc}</div>
            </div>
            <span className={`datamap-badge datamap-badge--${r.tone}`}>
              <span className="datamap-badge-dot" />
              {r.badge}
            </span>
          </div>
        ))}
      </div>

      {/* Actions: reveal the DB file, or wipe everything (gated below). */}
      <div className="privacy-actions">
        <button
          className="btn btn-ghost btn-icon"
          onClick={revealDatabase}
          disabled={revealing}
        >
          <span className="material-symbols-outlined">folder_open</span>
          {revealing ? "Revealing…" : "Reveal database file"}
        </button>
        <button
          type="button"
          className="privacy-danger-btn"
          onClick={() => {
            setClearOpen(true);
            setClearConfirm("");
            setError("");
            setSuccess("");
          }}
          disabled={clearing}
        >
          <span className="material-symbols-outlined">delete_forever</span>
          Clear all local data
        </button>
      </div>

      {/* Destructive confirm — a centered popup, still gated on typing CONFIRM.
          Scrim click dismisses (unless mid-wipe); the card stops propagation. */}
      {clearOpen && (
        <div
          className="privacy-modal-overlay"
          onClick={() => {
            if (clearing) return;
            setClearOpen(false);
            setClearConfirm("");
          }}
        >
          <div className="privacy-modal" onClick={(e) => e.stopPropagation()}>
            <div className="privacy-modal-icon">
              <span className="material-symbols-outlined">delete_forever</span>
            </div>
            <h2 className="privacy-modal-title">Delete everything?</h2>
            <p className="privacy-modal-desc">
              Your settings, API keys, and interview history will be permanently
              removed from this device. This cannot be undone.
            </p>
            <input
              className={`settings-input privacy-modal-input${
                clearConfirm === "CONFIRM" ? " privacy-modal-input--valid" : ""
              }`}
              value={clearConfirm}
              onChange={(e) => setClearConfirm(e.target.value)}
              placeholder="Type CONFIRM"
              autoFocus
              spellCheck={false}
              autoCapitalize="characters"
              disabled={clearing}
              onKeyDown={(e) => e.key === "Enter" && clearAllData()}
            />
            <button
              type="button"
              className="privacy-modal-go"
              onClick={clearAllData}
              disabled={clearConfirm !== "CONFIRM" || clearing}
            >
              <span className="material-symbols-outlined">delete_forever</span>
              {clearing ? "Clearing…" : "Clear everything"}
            </button>
            <button
              type="button"
              className="privacy-modal-cancel"
              onClick={() => {
                setClearOpen(false);
                setClearConfirm("");
              }}
              disabled={clearing}
            >
              Keep my data
            </button>
          </div>
        </div>
      )}
    </>
  );
}
