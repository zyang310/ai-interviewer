import "./MessageBubble.css";

interface Props {
  role: "user" | "assistant";
  content: string;
}

export default function MessageBubble({ role, content }: Props) {
  return (
    <div className={`bubble-row ${role}`}>
      <div className={`bubble ${role}`}>
        <span className="bubble-label">{role === "user" ? "You" : "Interviewer"}</span>
        <p className="bubble-text">{content}</p>
      </div>
    </div>
  );
}
