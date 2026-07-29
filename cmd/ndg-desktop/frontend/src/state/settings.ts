// Local settings store: persists user preferences in localStorage.
// Currently manages path privacy mode (masking file paths in the UI).

const STORAGE_KEY = "ndg-settings";

export interface AppSettings {
  /** When true, file paths are masked in the UI for privacy (e.g. screenshots). */
  pathPrivacyMode: boolean;
}

const DEFAULT_SETTINGS: AppSettings = {
  pathPrivacyMode: false,
};

export function loadSettings(): AppSettings {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return DEFAULT_SETTINGS;
    const parsed: unknown = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object") return DEFAULT_SETTINGS;
    return {
      pathPrivacyMode:
        "pathPrivacyMode" in parsed && parsed.pathPrivacyMode === true,
    };
  } catch {
    return DEFAULT_SETTINGS;
  }
}

export function saveSettings(settings: AppSettings): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
  } catch {
    // localStorage may be unavailable in some contexts; fail silently.
  }
}

/**
 * Masks a file path for privacy mode.
 * Hides mount, share, user, and intermediate segments. The final segment is
 * partially masked while preserving its extension so screenshots remain useful.
 *
 * Example: /data/archive/Documents/Work/Projects/report.pdf
 *       → …/[hidden]/r***t.pdf
 */
export function maskPath(path: string): string {
  if (!path) return path;

  // Normalize separators
  const normalized = path.replace(/\\/g, "/");
  const segments = normalized.split("/").filter(Boolean);

  if (segments.length === 0) return normalized;

  const finalSegment = segments[segments.length - 1];
  const dotIndex = finalSegment.lastIndexOf(".");
  const hasExtension = dotIndex > 0;
  const stem = hasExtension ? finalSegment.slice(0, dotIndex) : finalSegment;
  const extension = hasExtension ? finalSegment.slice(dotIndex) : "";
  const maskedStem = stem.length <= 1
    ? "***"
    : `${stem.slice(0, 1)}***${stem.length > 2 ? stem.slice(-1) : ""}`;
  const maskedFinalSegment = `${maskedStem}${extension}`;

  if (segments.length === 1) return maskedFinalSegment;
  return `…/[hidden]/${maskedFinalSegment}`;
}
