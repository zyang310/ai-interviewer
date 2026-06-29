// useUpdateCheck checks GitHub for a newer app release once on mount and exposes
// the result so the app shell can show an "update available" banner. It fails
// silent: any error — offline, a dev build, or the Wails runtime being absent in
// browser preview — leaves the result null and surfaces nothing, because a
// failed update check must never disrupt the app. Dismissal is session-scoped.
import { useEffect, useState } from "react";
import { CheckForUpdate, models } from "./wailsBridge";

export function useUpdateCheck() {
  const [info, setInfo] = useState<models.UpdateInfo | null>(null);
  const [dismissed, setDismissed] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const result = await CheckForUpdate();
        if (!cancelled && result?.available) setInfo(result);
      } catch {
        // Offline, dev build, or no Wails runtime (browser preview) — stay silent.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // Show the banner only when an update is available and not dismissed this session.
  return {
    update: info && !dismissed ? info : null,
    dismiss: () => setDismissed(true),
  };
}
