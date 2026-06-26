// Small, pure formatting helpers for the session history view.

// formatSessionDate renders an ISO timestamp as e.g. "Oct 24, 2023".
// Returns "" for an unparseable value.
export function formatSessionDate(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "";
  return d.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

// formatDuration renders the gap between start and end as e.g. "45m 12s" (or
// "1h 02m 05s"). Returns "—" when the session never ended or the times are bad.
export function formatDuration(startIso: string, endIso?: string | null): string {
  if (!endIso) return "—";
  const start = new Date(startIso).getTime();
  const end = new Date(endIso).getTime();
  if (isNaN(start) || isNaN(end) || end < start) return "—";

  const totalSec = Math.round((end - start) / 1000);
  const h = Math.floor(totalSec / 3600);
  const m = Math.floor((totalSec % 3600) / 60);
  const s = totalSec % 60;

  const pad = (n: number) => String(n).padStart(2, "0");
  if (h > 0) return `${h}h ${pad(m)}m ${pad(s)}s`;
  if (m > 0) return `${m}m ${pad(s)}s`;
  return `${s}s`;
}

// prettyModel turns an OpenRouter id like "anthropic/claude-sonnet-4" into a
// readable label like "Claude Sonnet 4". Falls back to the raw id. This is a
// lightweight prettifier; the exact catalog names from ListAvailableModels could
// be used instead if precise display names ever matter.
export function prettyModel(modelId: string): string {
  if (!modelId) return "";
  const tail = modelId.split("/").pop() || modelId;
  return tail
    .split(/[-_]/)
    .filter(Boolean)
    .map((w) => (/^\d/.test(w) ? w : w.charAt(0).toUpperCase() + w.slice(1)))
    .join(" ");
}
