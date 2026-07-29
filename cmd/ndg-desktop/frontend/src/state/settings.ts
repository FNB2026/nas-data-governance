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
    const parsed = JSON.parse(raw);
    return { ...DEFAULT_SETTINGS, ...parsed };
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
 * Replaces the home directory and intermediate path segments with asterisks,
 * keeping the first segment and last two segments visible.
 *
 * Example: /Users/john/Documents/Work/Projects/report.pdf
 *       → /Users/[hidden]/Projects/report.pdf
 */
export function maskPath(path: string): string {
  if (!path) return path;

  // Normalize separators
  const normalized = path.replace(/\\/g, "/");
  const segments = normalized.split("/").filter(Boolean);

  if (segments.length <= 2) return normalized;

  // Keep first segment (root or home) and last two segments
  const first = segments[0];
  const last1 = segments[segments.length - 1];
  const last2 = segments[segments.length - 2];
  const hiddenCount = segments.length - 3;

  const prefix = normalized.startsWith("/") ? "/" : "";
  return `${prefix}${first}/${"**".repeat(Math.min(hiddenCount, 1))}/${last2}/${last1}`;
}
