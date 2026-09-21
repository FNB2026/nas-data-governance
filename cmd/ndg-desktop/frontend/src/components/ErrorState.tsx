// ErrorState: standard fetch/load error display, optionally retryable.

import type { ReactNode } from "react";

export interface ErrorStateProps {
  message: string;
  onRetry?: () => void;
  detail?: ReactNode;
}

export default function ErrorState({ message, onRetry, detail }: ErrorStateProps) {
  return (
    <div className="state-error" role="alert">
      <p className="error">{message}</p>
      {detail && (
        <details className="state-error-detail">
          <summary>技术详情</summary>
          <pre>{detail}</pre>
        </details>
      )}
      {onRetry && (
        <button type="button" className="btn-sm" onClick={onRetry}>
          重试
        </button>
      )}
    </div>
  );
}