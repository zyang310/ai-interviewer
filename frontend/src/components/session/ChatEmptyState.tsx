import "./ChatEmptyState.css";

interface StarterChip {
  icon: string;
  label: string;
  message: string;
  accent?: boolean; // true only for the waving_hand chip (secondary-tinted icon)
}

// Fixed quick-start prompts shown before the first message. Clicking one sends
// `message` immediately through the same onSend path as the textarea — it does
// not just prefill the draft. Deliberately named "starter chip", never
// "opener", to avoid confusion with Company Practice's AI opener
// (internal/ai/prompts.go's CompanyOpening/MockOpening, CompanySessionStart.opening).
const STARTER_CHIPS: StarterChip[] = [
  { icon: "waving_hand", label: "I'm ready to start", message: "I'm ready to start.", accent: true },
  { icon: "description", label: "Walk me through the problem", message: "Can you walk me through the problem?" },
  { icon: "lightbulb", label: "Give me a hint", message: "Can you give me a hint?" },
];

interface Props {
  onSend: (text: string) => void;
  disabled: boolean;
  showMicHint: boolean;
}

// ChatEmptyState is the "ready to begin" placeholder shown in Chat before the
// first message — a breathing icon avatar, headline, body copy, and three
// quick-start chips that send a canned first message via onSend.
export default function ChatEmptyState({ onSend, disabled, showMicHint }: Props) {
  return (
    <div className="chat-empty">
      <div className="chat-empty-avatar">
        <span className="material-symbols-outlined">forum</span>
      </div>

      <h2 className="chat-empty-headline">Ready when you are</h2>

      <p className="chat-empty-body">
        Send a message to open the conversation. Your interviewer can see the
        problem on your screen and will respond in real time — just like the
        real thing.
      </p>

      <div className="chat-starter-chips">
        {STARTER_CHIPS.map((chip) => (
          <button
            key={chip.label}
            className="chat-starter-chip"
            onClick={() => onSend(chip.message)}
            disabled={disabled}
          >
            <span
              className={`material-symbols-outlined${chip.accent ? " chat-starter-chip-icon--accent" : ""}`}
            >
              {chip.icon}
            </span>
            {chip.label}
          </button>
        ))}
      </div>

      {showMicHint && (
        <p className="chat-empty-mic-hint">
          <span className="material-symbols-outlined">mic</span>
          or press the mic to speak your answer
        </p>
      )}
    </div>
  );
}
