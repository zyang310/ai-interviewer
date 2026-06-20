import { useState } from "react";
import "./Overlay.css";

interface Message {
  role: "user" | "assistant";
  content: string;
}

interface Props {
  messages: Message[];
  latestAiText: string;
  onEnd: () => void;
  onExpand: () => void;
  onHistoryToggle: (open: boolean) => void;
}

/**
 * Compact always-on-top overlay bar shown during an interview while the user
 * works in their own IDE. Voice (mic) is a placeholder until ElevenLabs STT is
 * wired — the transcript currently mirrors the latest interviewer message.
 */
export default function Overlay({
  messages,
  latestAiText,
  onEnd,
  onExpand,
  onHistoryToggle,
}: Props) {
  const [historyOpen, setHistoryOpen] = useState(false);
  const [muted, setMuted] = useState(false);

  function toggleHistory() {
    const next = !historyOpen;
    setHistoryOpen(next);
    onHistoryToggle(next);
  }

  return (
    <div className="overlay-root">
      <div className="overlay-bar">
        {/* Grab handle (drags the window) */}
        <div className="overlay-grab" title="Drag to move">
          <span className="material-symbols-outlined">drag_indicator</span>
        </div>

        {/* Live indicator */}
        <div className="overlay-live">
          <span className="overlay-live-dot" />
          <span className="overlay-live-label">Live</span>
        </div>

        {/* Real-time transcript (latest interviewer line) */}
        <div className="overlay-transcript">
          <span className="overlay-transcript-speaker">AI:</span>
          <span className="overlay-transcript-text">{latestAiText}</span>
        </div>

        {/* Controls */}
        <div className="overlay-controls">
          <button
            className={`overlay-icon-btn${historyOpen ? " is-active" : ""}`}
            onClick={toggleHistory}
            title="Conversation history"
          >
            <span className="material-symbols-outlined">history</span>
          </button>
          <button
            className={`overlay-icon-btn${muted ? " is-muted" : ""}`}
            onClick={() => setMuted((m) => !m)}
            title="Microphone (voice coming soon)"
          >
            <span className="material-symbols-outlined">{muted ? "mic_off" : "mic"}</span>
          </button>
          <button
            className="overlay-icon-btn"
            onClick={onExpand}
            title="Expand to full window"
          >
            <span className="material-symbols-outlined">open_in_full</span>
          </button>
          <button className="overlay-end-btn" onClick={onEnd}>
            End Session
          </button>
        </div>
      </div>

      {/* Under-glow for AI presence */}
      <div className="overlay-glow" />

      {/* Conversation history dropdown */}
      {historyOpen && (
        <div className="overlay-history">
          <div className="overlay-history-header">
            <span>Conversation History</span>
            <span className="overlay-live-dot" />
          </div>
          <div className="overlay-history-body">
            {messages.length === 0 ? (
              <p className="overlay-history-empty">No messages yet.</p>
            ) : (
              messages.map((m, i) => (
                <div key={i} className={`overlay-history-row ${m.role}`}>
                  <span className="overlay-history-role">
                    {m.role === "assistant" ? "AI" : "You"}
                  </span>
                  <p className="overlay-history-text">{m.content}</p>
                </div>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
}
