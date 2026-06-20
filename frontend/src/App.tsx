import { useState, useEffect } from "react";
import Chat from "./components/Chat";
import CapturePanel from "./components/CapturePanel";
import RegionSelector from "./components/RegionSelector";
import Settings from "./components/Settings";
import SetupPage from "./components/SetupPage";
import {
  GetAuthStatus,
  GetPreferences,
  StartSession,
  EndSession,
  SendMessage,
  models,
} from "./lib/wailsBridge";
import "./App.css";

interface ChatMessage {
  role: "user" | "assistant";
  content: string;
}

function formatTime(sec: number): string {
  const m = Math.floor(sec / 60).toString().padStart(2, "0");
  const s = (sec % 60).toString().padStart(2, "0");
  return `${m}:${s}`;
}

function App() {
  const [authStatus, setAuthStatus] = useState<models.AuthStatus>(
    new models.AuthStatus({
      openRouterConfigured: false,
      elevenLabsConfigured: false,
    })
  );
  const [authLoaded, setAuthLoaded] = useState(false);
  const [setupDone, setSetupDone] = useState(false);
  const [prefs, setPrefs] = useState<models.Preferences | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [sessionStartedAt, setSessionStartedAt] = useState<Date | null>(null);
  const [elapsedSec, setElapsedSec] = useState(0);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [loading, setLoading] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [regionOpen, setRegionOpen] = useState(false);
  const [error, setError] = useState("");

  async function loadPrefs() {
    try {
      const p = await GetPreferences();
      setPrefs(p);
    } catch {
      // Wails runtime not present in browser preview
    }
  }

  // On mount: load auth status and preferences.
  useEffect(() => {
    (async () => {
      try {
        const s = await GetAuthStatus();
        setAuthStatus(s);
      } catch {
        // Wails runtime not present (browser preview) or key not set — show setup page.
      } finally {
        setAuthLoaded(true);
      }
    })();
    loadPrefs();
  }, []);

  // Tick the session timer every second while a session is active.
  useEffect(() => {
    if (!sessionStartedAt) {
      setElapsedSec(0);
      return;
    }
    const tick = () =>
      setElapsedSec(Math.floor((Date.now() - sessionStartedAt.getTime()) / 1000));
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [sessionStartedAt]);

  // Auto-open settings if no API key is configured.
  useEffect(() => {
    if (!authStatus.openRouterConfigured) {
      setSettingsOpen(true);
    }
  }, [authStatus.openRouterConfigured]);

  async function handleStart() {
    setError("");
    try {
      const session = await StartSession("");
      setSessionId(session.id);
      setSessionStartedAt(new Date(session.startedAt));
      setMessages([]);
    } catch (e: any) {
      setError(e?.message || String(e));
    }
  }

  async function handleEnd() {
    if (!sessionId) return;
    setError("");
    try {
      await EndSession(sessionId);
    } catch (e: any) {
      setError(e?.message || String(e));
    } finally {
      setSessionId(null);
      setSessionStartedAt(null);
    }
  }

  async function handleSend(text: string) {
    setError("");
    setMessages((prev) => [...prev, { role: "user", content: text }]);
    setLoading(true);
    try {
      const response = await SendMessage(text);
      setMessages((prev) => [...prev, { role: "assistant", content: response }]);
    } catch (e: any) {
      setError(e?.message || String(e));
      // Remove the user message on failure so they can re-send.
      setMessages((prev) => prev.slice(0, -1));
    } finally {
      setLoading(false);
    }
  }

  // Always show the welcome/setup page first; "Continue to Hub" dismisses it.
  if (!authLoaded) return null;
  if (!setupDone) {
    return (
      <SetupPage
        authStatus={authStatus}
        onAuthChange={setAuthStatus}
        onContinue={() => setSetupDone(true)}
      />
    );
  }

  const isActive = sessionId !== null;
  const limitSec = (prefs?.sessionLimitMinutes ?? 30) * 60;
  const warnSec = (prefs?.softWarningMinutes ?? 25) * 60;
  const timedOut = isActive && limitSec > 0 && elapsedSec >= limitSec;
  const nearLimit = isActive && limitSec > 0 && warnSec > 0 && elapsedSec >= warnSec && !timedOut;

  return (
    <div className="app">
      {/* Header */}
      <header className="app-header">
        <h1 className="app-title">AI Interviewer</h1>

        <div className="header-controls">
          {isActive && limitSec > 0 && (
            <span
              className={`session-timer${nearLimit ? " timer-warning" : ""}${timedOut ? " timer-expired" : ""}`}
            >
              {formatTime(elapsedSec)} / {formatTime(limitSec)}
            </span>
          )}
          {!isActive ? (
            <button
              className="btn btn-primary"
              onClick={handleStart}
              disabled={!authStatus.openRouterConfigured}
            >
              Start Interview
            </button>
          ) : (
            <button className="btn btn-danger" onClick={handleEnd}>
              End Interview
            </button>
          )}
          <button
            className="btn btn-ghost"
            onClick={() => setSettingsOpen(true)}
            title="Settings"
          >
            Settings
          </button>
        </div>
      </header>

      {/* Warning banner (approaching time limit) */}
      {nearLimit && (
        <div className="app-warning">
          {Math.ceil((limitSec - elapsedSec) / 60)} minute(s) remaining in this session.
        </div>
      )}

      {/* Timeout banner */}
      {timedOut && (
        <div className="app-error">
          <span>Session time limit reached — review your work or end the interview.</span>
        </div>
      )}

      {/* Error banner */}
      {error && (
        <div className="app-error">
          <span>{error}</span>
          <button className="error-dismiss" onClick={() => setError("")}>
            &times;
          </button>
        </div>
      )}

      {/* Main content */}
      <main className="app-body">
        <div className="panel-capture">
          <CapturePanel
            isActive={isActive}
            prefs={prefs}
            onSetRegion={() => setRegionOpen(true)}
          />
        </div>
        <div className="panel-divider" />
        <div className="panel-chat">
          <Chat
            messages={messages}
            onSend={handleSend}
            loading={loading}
            disabled={!isActive || timedOut}
          />
        </div>
      </main>

      {/* Region selector modal */}
      {regionOpen && (
        <RegionSelector
          initialDisplay={prefs?.captureDisplay ?? 0}
          onClose={() => setRegionOpen(false)}
          onSaved={loadPrefs}
        />
      )}

      {/* Settings modal */}
      {settingsOpen && (
        <Settings
          authStatus={authStatus}
          onUpdate={setAuthStatus}
          onClose={() => {
            setSettingsOpen(false);
            loadPrefs();
          }}
        />
      )}
    </div>
  );
}

export default App;
