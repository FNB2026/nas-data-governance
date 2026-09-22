// OnboardingOverlay: lightweight first-run introduction (UI-P7-C).
//
// Product rule: the goal is NOT to teach NDG, it is to make the user
// comfortable starting a first read-only scan. So this is a single
// dismissible overlay — not a multi-page wizard — and it deliberately
// avoids implementation vocabulary (SHA-256, inode, Journal, stale
// check, hardlink). Those belong to progressive disclosure elsewhere.
//
// It answers exactly three questions:
//   1. NDG 是什么
//   2. 扫描不会修改源文件
//   3. 第一次为什么建议选较小的目录

import { useCallback, useEffect, useRef, useState } from "react";

export interface OnboardingOverlayProps {
  busy: boolean;
  /** Pick a directory via the native dialog. Resolves to "" when cancelled. */
  onPickDirectory: () => Promise<string>;
  /** Create a project from the picked directory, then continue to scanning. */
  onCreateProject: (scanRoot: string) => void;
  /** Secondary action: leave onboarding and use an existing project. */
  onUseExisting: () => void;
  /** Dismiss without creating anything. */
  onDismiss: () => void;
  /** Optional error surfaced by the create flow. */
  error?: string | null;
}

const SAFETY_FACTS = [
  { title: "扫描只读", body: "扫描过程只读取文件并记录指纹，不改动、不移动、不删除任何源文件。" },
  { title: "隔离而非删除", body: "治理动作先进入隔离区，随时可以恢复，不会直接抹掉数据。" },
  { title: "恢复锁", body: "存在未完成的执行计划时会锁定写操作，先处理完再继续。" },
  { title: "本地优先", body: "不使用外部 AI、不上传云端、不发送遥测，界面中的路径可一键脱敏。" },
  { title: "由你决定", body: "系统只给建议和证据，是否保留、隔离或清理始终由你确认。" },
];

const FOCUSABLE_SELECTOR = [
  "a[href]",
  "button:not([disabled])",
  "input:not([disabled])",
  "select:not([disabled])",
  "textarea:not([disabled])",
  '[tabindex]:not([tabindex="-1"])',
].join(",");

/** Visible-at-runtime focusable descendants, in DOM order. */
function focusableWithin(root: HTMLElement | null): HTMLElement[] {
  if (!root) return [];
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)).filter(
    (el) => !el.hidden && el.getAttribute("aria-hidden") !== "true",
  );
}

export default function OnboardingOverlay({
  busy,
  onPickDirectory,
  onCreateProject,
  onUseExisting,
  onDismiss,
  error,
}: OnboardingOverlayProps) {
  const [showSafety, setShowSafety] = useState(false);
  const [picking, setPicking] = useState(false);
  const panelRef = useRef<HTMLDivElement>(null);

  // Keep the latest dismiss handler in a ref so the focus trap effect can
  // stay mounted exactly once (re-running it would steal focus back).
  const onDismissRef = useRef(onDismiss);
  onDismissRef.current = onDismiss;

  // ---- UI-P8-B: modal focus contract ----
  // 1. focus moves into the dialog on open (the container, so the dialog
  //    name/description are announced before its controls);
  // 2. Tab / Shift+Tab cycle inside the dialog and cannot reach the page
  //    behind it;
  // 3. Escape closes;
  // 4. focus returns to whatever was focused before the dialog opened.
  useEffect(() => {
    const previouslyFocused = document.activeElement as HTMLElement | null;
    const panel = panelRef.current;
    (panel ?? previouslyFocused)?.focus();

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onDismissRef.current();
        return;
      }
      if (event.key !== "Tab") return;

      const items = focusableWithin(panel);
      if (items.length === 0) {
        event.preventDefault();
        panel?.focus();
        return;
      }

      const active = document.activeElement as HTMLElement | null;
      const index = active ? items.indexOf(active) : -1;
      const next = event.shiftKey
        ? index <= 0
          ? items.length - 1
          : index - 1
        : index === -1 || index === items.length - 1
          ? 0
          : index + 1;

      event.preventDefault();
      items[next].focus();
    };

    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      previouslyFocused?.focus?.();
    };
  }, []);

  const handlePrimary = useCallback(async () => {
    setPicking(true);
    try {
      const picked = await onPickDirectory();
      if (picked) {
        onCreateProject(picked);
      } else {
        // Cancelled or no native dialog available: hand control back to
        // the start card so the user can still type a path manually.
        onDismiss();
      }
    } finally {
      setPicking(false);
    }
  }, [onCreateProject, onDismiss, onPickDirectory]);

  return (
    <div
      className="onboarding-backdrop"
      role="dialog"
      aria-modal="true"
      aria-labelledby="onboarding-title"
      aria-describedby="onboarding-lead"
    >
      {/* eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex */}
      <div className="onboarding-panel" ref={panelRef} tabIndex={-1}>
        <div className="onboarding-header">
          <div className="onboarding-brand">
            <span className="app-brand-mark" aria-hidden="true">N</span>
            <span id="onboarding-title" className="onboarding-title">开始使用 NDG</span>
          </div>
          <button
            type="button"
            className="onboarding-close"
            aria-label="关闭引导"
            onClick={onDismiss}
          >
            ×
          </button>
        </div>

        <p className="onboarding-lead muted" id="onboarding-lead">
          本地优先的 NAS 数据治理工作台。找出重复文件，先给你证据，再由你决定。
        </p>

        <ol className="onboarding-points">
          <li>
            <span className="onboarding-point-title">NDG 是什么</span>
            <span className="onboarding-point-body muted">
              帮你在 NAS 或本地目录中找出重复文件，并解释每组文件所处的目录语境。
            </span>
          </li>
          <li>
            <span className="onboarding-point-title">扫描不会修改源文件</span>
            <span className="onboarding-point-body muted">
              第一次扫描是纯只读的：只有你先看清楚，后面才谈得上处理。
            </span>
          </li>
          <li>
            <span className="onboarding-point-title">建议第一次选择较小的目录</span>
            <span className="onboarding-point-body muted">
              先在一个小目录里走完一遍，确认结果符合预期，再扫描整个资料库。
            </span>
          </li>
        </ol>

        <div className="onboarding-actions">
          <button
            type="button"
            className="start-primary"
            disabled={busy || picking}
            onClick={() => void handlePrimary()}
          >
            {picking ? "选择中…" : "选择数据目录"}
          </button>
          <button
            type="button"
            className="secondary"
            disabled={busy || picking}
            onClick={onUseExisting}
          >
            打开已有项目
          </button>
          <button
            type="button"
            className="link-button"
            aria-expanded={showSafety}
            onClick={() => setShowSafety((v) => !v)}
          >
            {showSafety ? "▾" : "▸"} 了解 NDG 的安全机制
          </button>
        </div>

        {showSafety && (
          <div className="onboarding-safety">
            <ul className="onboarding-safety-list">
              {SAFETY_FACTS.map((fact) => (
                <li key={fact.title}>
                  <span className="onboarding-point-title">{fact.title}</span>
                  <span className="onboarding-point-body muted">{fact.body}</span>
                </li>
              ))}
            </ul>
          </div>
        )}

        {error && <p className="onboarding-error" role="alert">{error}</p>}

        <p className="onboarding-footnote muted">
          引导只解释到这里，其余细节会在你真正用到时再出现。
        </p>
      </div>
    </div>
  );
}
