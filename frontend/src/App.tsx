import { useState, useEffect } from "react";
import Chat from "./components/Chat";
import ProblemPanel from "./components/ProblemPanel";
import Settings from "./components/Settings";
import {
  GetAuthStatus,
  ListProblems,
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

function App() {
  const [authStatus, setAuthStatus] = useState<models.AuthStatus>(
    new models.AuthStatus({ openRouterConfigured: false, elevenLabsConfigured: false })
  );
  const [problem, setProblem] = useState<models.Problem | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [loading, setLoading] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [error, setError] = useState("");

  // On mount: load auth status and the default problem.
  useEffect(() => {
    GetAuthStatus()
      .then(setAuthStatus)
      .catch(() => {});

    ListProblems()
      .then((problems) => {
        if (problems.length > 0) setProblem(problems[0]);
      })
      .catch(() => {});
  }, []);

  // Auto-open settings if no API key is configured.
  useEffect(() => {
    if (!authStatus.openRouterConfigured) {
      setSettingsOpen(true);
    }
  }, [authStatus.openRouterConfigured]);

  async function handleStart() {
    if (!problem) return;
    setError("");
    try {
      const session = await StartSession(problem.id, "");
      setSessionId(session.id);
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

  const isActive = sessionId !== null;

  return (
    <div className="app">
      {/* Header */}
      <header className="app-header">
        <h1 className="app-title">AI Interviewer</h1>

        <div className="header-controls">
          {!isActive ? (
            <button
              className="btn btn-primary"
              onClick={handleStart}
              disabled={!authStatus.openRouterConfigured || !problem}
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
        <div className="panel-problem">
          <ProblemPanel problem={problem} />
        </div>
        <div className="panel-divider" />
        <div className="panel-chat">
          <Chat
            messages={messages}
            onSend={handleSend}
            loading={loading}
            disabled={!isActive}
          />
        </div>
      </main>

      {/* Settings modal */}
      {settingsOpen && (
        <Settings
          authStatus={authStatus}
          onUpdate={setAuthStatus}
          onClose={() => setSettingsOpen(false)}
        />
      )}
    </div>
  );
}

export default App;
