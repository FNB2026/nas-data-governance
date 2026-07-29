import { useEffect, useCallback } from "react";

export type ToastType = "success" | "error" | "warning" | "info";

export interface ToastItem {
  id: number;
  type: ToastType;
  title: string;
  message?: string;
}

export interface ToastContainerProps {
  toasts: ToastItem[];
  onDismiss: (id: number) => void;
}

const TOAST_AUTO_DISMISS_MS = 6000;

const TOAST_ICONS: Record<ToastType, string> = {
  success: "✓",
  error: "✕",
  warning: "⚠",
  info: "ℹ",
};

function ToastRow({
  toast,
  onDismiss,
}: {
  toast: ToastItem;
  onDismiss: (id: number) => void;
}) {
  const handleDismiss = useCallback(() => onDismiss(toast.id), [toast.id, onDismiss]);

  useEffect(() => {
    const timer = setTimeout(handleDismiss, TOAST_AUTO_DISMISS_MS);
    return () => clearTimeout(timer);
  }, [handleDismiss]);

  return (
    <div
      className={`toast toast--${toast.type}`}
      role="alert"
      onClick={handleDismiss}
    >
      <span className="toast-icon">{TOAST_ICONS[toast.type]}</span>
      <div className="toast-content">
        <span className="toast-title">{toast.title}</span>
        {toast.message && <span className="toast-message">{toast.message}</span>}
      </div>
      <button className="toast-close" aria-label="关闭" onClick={handleDismiss}>
        ×
      </button>
    </div>
  );
}

export default function ToastContainer({ toasts, onDismiss }: ToastContainerProps) {
  if (toasts.length === 0) return null;

  return (
    <div className="toast-container" aria-live="polite">
      {toasts.map((toast) => (
        <ToastRow key={toast.id} toast={toast} onDismiss={onDismiss} />
      ))}
    </div>
  );
}
