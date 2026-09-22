// Explicit external-link opener.
//
// The About section is the only place in the app that can leave the
// sandbox, and only on a direct user click. This helper prefers the
// native Wails browser opener and falls back to window.open outside the
// Wails runtime (e.g. `vite dev` in a plain browser, unit tests).

import { BrowserOpenURL } from "../wailsjs/runtime/runtime";
import { hasWailsRuntime } from "./utils";

/**
 * Normalizes a URL for the OS browser. Non-ASCII path segments (the
 * Chinese user-guide filename) and anchors are percent-encoded so the
 * native opener receives an ASCII-safe URL.
 */
export function normalizeExternalUrl(url: string): string {
  try {
    return encodeURI(url);
  } catch {
    return url;
  }
}

/**
 * Opens a URL outside the application. Returns false when the request
 * could not be dispatched, so callers can surface a fallback hint.
 */
export function openExternal(url: string): boolean {
  if (!url) return false;
  const target = normalizeExternalUrl(url);

  if (hasWailsRuntime()) {
    try {
      BrowserOpenURL(target);
      return true;
    } catch {
      return false;
    }
  }

  if (typeof window !== "undefined" && typeof window.open === "function") {
    try {
      window.open(target, "_blank", "noopener,noreferrer");
      return true;
    } catch {
      return false;
    }
  }
  return false;
}
