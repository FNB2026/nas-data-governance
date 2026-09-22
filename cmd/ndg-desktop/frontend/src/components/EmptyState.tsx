// EmptyState: standard empty-data placeholder with optional hint and action.

import type { ReactNode } from "react";

export interface EmptyStateProps {
  title: string;
  hint?: ReactNode;
  actionLabel?: string;
  onAction?: () => void;
}

export default function EmptyState({ title, hint, actionLabel, onAction }: EmptyStateProps) {
  return (
    <div className="empty-state">
      <p className="state-empty-title">{title}</p>
      {hint && <p className="muted">{hint}</p>}
      {actionLabel && onAction && (
        <button type="button" className="btn-sm" onClick={onAction}>
          {actionLabel}
        </button>
      )}
    </div>
  );
}