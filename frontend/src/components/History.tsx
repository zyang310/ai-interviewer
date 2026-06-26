import { useEffect, useState } from "react";
import {
  ListSessions,
  GetSessionTranscript,
  DeleteSession,
  models,
} from "../lib/wailsBridge";
import SessionHistoryCard from "./SessionHistoryCard";
import "./History.css";

// History is the Session History page: a reverse-chronological list of past
// sessions, each expandable to its full transcript and deletable. It owns its own
// data fetch and per-card transcript cache (transcripts load lazily on expand).
export default function History() {
  const [sessions, setSessions] = useState<models.SessionSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [transcripts, setTranscripts] = useState<Record<string, models.Message[]>>({});
  const [transcriptLoading, setTranscriptLoading] = useState<string | null>(null);
  const [transcriptErrors, setTranscriptErrors] = useState<Record<string, string>>({});

  // Load the session list on mount.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      setError("");
      try {
        const list = await ListSessions();
        if (!cancelled) setSessions(list ?? []);
      } catch (e: any) {
        if (!cancelled) setError(e?.message || String(e));
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // Toggle a card open/closed, lazy-loading its transcript on first open.
  async function toggle(id: string) {
    if (expandedId === id) {
      setExpandedId(null);
      return;
    }
    setExpandedId(id);
    if (transcripts[id]) return; // already cached

    setTranscriptLoading(id);
    setTranscriptErrors((prev) => ({ ...prev, [id]: "" }));
    try {
      const msgs = await GetSessionTranscript(id);
      setTranscripts((prev) => ({ ...prev, [id]: msgs ?? [] }));
    } catch (e: any) {
      setTranscriptErrors((prev) => ({ ...prev, [id]: e?.message || String(e) }));
    } finally {
      setTranscriptLoading((cur) => (cur === id ? null : cur));
    }
  }

  // Delete a session and drop it from the list (no full refetch).
  async function remove(id: string) {
    try {
      await DeleteSession(id);
      setSessions((prev) => prev.filter((s) => s.id !== id));
      if (expandedId === id) setExpandedId(null);
    } catch (e: any) {
      setError(e?.message || String(e));
    }
  }

  return (
    <div className="history-page">
      <div className="history-inner">
        <header className="history-head">
          <h1>Session History</h1>
          <p>Review past technical interviews and their transcripts.</p>
        </header>

        {loading ? (
          <p className="history-status">Loading sessions…</p>
        ) : error ? (
          <div className="history-status error">{error}</div>
        ) : sessions.length === 0 ? (
          <div className="history-empty">
            <span className="material-symbols-outlined">history</span>
            <p className="history-empty-title">No sessions yet</p>
            <p className="history-empty-sub">
              Finished interviews will appear here with their full transcript.
            </p>
          </div>
        ) : (
          <div className="history-list">
            {sessions.map((s) => (
              <SessionHistoryCard
                key={s.id}
                summary={s}
                expanded={expandedId === s.id}
                transcript={transcripts[s.id]}
                loadingTranscript={transcriptLoading === s.id}
                transcriptError={transcriptErrors[s.id]}
                onToggle={() => toggle(s.id)}
                onDelete={() => remove(s.id)}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
