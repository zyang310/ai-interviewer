import { models } from "../lib/wailsBridge";
import "./ProblemPanel.css";

interface Props {
  problem: models.Problem | null;
}

export default function ProblemPanel({ problem }: Props) {
  if (!problem) {
    return (
      <div className="problem-panel">
        <p className="problem-empty">No problem loaded.</p>
      </div>
    );
  }

  return (
    <div className="problem-panel">
      <div className="problem-header">
        <h2 className="problem-title">{problem.title}</h2>
        <span className={`problem-difficulty ${problem.difficulty}`}>
          {problem.difficulty}
        </span>
      </div>

      <section className="problem-section">
        <h3>Description</h3>
        <p>{problem.description}</p>
      </section>

      <section className="problem-section">
        <h3>Examples</h3>
        <pre className="problem-examples">{problem.examples}</pre>
      </section>

      <section className="problem-section">
        <h3>Constraints</h3>
        <pre className="problem-constraints">{problem.constraints}</pre>
      </section>
    </div>
  );
}
