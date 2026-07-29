// Global keyboard shortcuts hook.
// Listens for Cmd/Ctrl+1-7 to switch pages, Cmd/Ctrl+O to focus project path,
// and Cmd/Ctrl+R to refresh data. Avoids intercepting when modifier-free
// typing in input fields is detected (except for the combo shortcuts).

import { useEffect } from "react";
import { NAV_ITEMS, type AppRoute } from "../app/routes";

export interface KeyboardShortcutHandlers {
  onRouteChange: (route: AppRoute) => void;
  onFocusProjectPath?: () => void;
  onRefresh?: () => void;
}

export function useKeyboardShortcuts(handlers: KeyboardShortcutHandlers) {
  const { onRouteChange, onFocusProjectPath, onRefresh } = handlers;

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Only respond to Cmd (macOS) or Ctrl (other platforms) combos
      const mod = e.metaKey || e.ctrlKey;
      if (!mod) return;

      // Cmd/Ctrl + 1-7: switch navigation pages
      if (e.key >= "1" && e.key <= "7") {
        const idx = parseInt(e.key, 10) - 1;
        if (idx < NAV_ITEMS.length) {
          e.preventDefault();
          onRouteChange(NAV_ITEMS[idx].id);
        }
        return;
      }

      // Cmd/Ctrl + O: focus project path input (user needs to be on sources page)
      if (e.key === "o" || e.key === "O") {
        if (onFocusProjectPath) {
          e.preventDefault();
          onFocusProjectPath();
        }
        return;
      }

      // Cmd/Ctrl + R: refresh current data (don't intercept browser refresh
      // in dev — only act if a handler is provided)
      if (e.key === "r" || e.key === "R") {
        if (onRefresh) {
          e.preventDefault();
          onRefresh();
        }
        return;
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onRouteChange, onFocusProjectPath, onRefresh]);
}
